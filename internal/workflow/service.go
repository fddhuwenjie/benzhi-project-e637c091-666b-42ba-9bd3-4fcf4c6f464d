package workflow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/store"
)

type Clock func() time.Time

type Service struct {
	store *store.Store
	now   Clock
}

func New(storage *store.Store) *Service {
	return &Service{store: storage, now: time.Now}
}

func NewWithClock(storage *store.Store, clock Clock) *Service {
	return &Service{store: storage, now: clock}
}

func (s *Service) Showcases() []domain.Showcase {
	return s.store.Showcases(true)
}

func (s *Service) Queue(filter IncidentFilter) []QueueItem {
	incidents := s.store.Incidents(filter.ShowcaseID, filter.State)
	result := make([]QueueItem, 0, len(incidents))
	for _, incident := range incidents {
		showcase, err := s.store.Showcase(incident.ShowcaseID)
		if err == nil {
			result = append(result, QueueItem{Incident: incident, Showcase: showcase})
		}
	}
	return result
}

func (s *Service) Detail(id string) (domain.IncidentDetail, error) {
	return s.store.Detail(id)
}

func (s *Service) Archives(query string) []domain.ArchiveRecord {
	return s.store.Archives(query)
}

func (s *Service) SearchArchives(q store.ArchiveQuery) store.ArchiveResult {
	return s.store.SearchArchives(q)
}

func requestHash(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("编码请求摘要: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func newID(prefix string) string {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(random)
}

func validateKey(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return domain.Validation("幂等键无效", map[string]string{"idempotency_key": "长度必须为 8-128 个字符"})
	}
	return nil
}

func checkRevision(incident domain.EnvironmentIncident, expected int64) error {
	if expected <= 0 {
		return domain.Validation("缺少期望修订号", map[string]string{"expected_revision": "必须大于 0"})
	}
	if incident.Revision != expected {
		return domain.Conflict(fmt.Sprintf("修订号冲突：当前为 %d，请刷新后重试", incident.Revision))
	}
	return nil
}

func checkIdempotency(data *store.Data, key, operation, hash string) (string, bool, error) {
	record, exists := data.Idempotency[key]
	if !exists {
		return "", false, nil
	}
	if record.Operation != operation || record.RequestHash != hash {
		return "", false, &domain.Problem{Kind: domain.ErrIdempotencyKey, Message: "该幂等键已用于不同请求"}
	}
	return record.ResourceID, true, nil
}

func saveIdempotency(data *store.Data, key, operation, hash, resourceID string, now time.Time) {
	data.Idempotency[key] = store.IdempotencyRecord{Key: key, Operation: operation, RequestHash: hash, ResourceID: resourceID, CreatedAt: now}
}

func event(incident domain.EnvironmentIncident, kind, actor, summary string, from domain.IncidentState, data map[string]any, at time.Time) domain.AuditEvent {
	return domain.AuditEvent{
		ID: newID("evt"), IncidentID: incident.ID, Type: kind, Actor: actor, At: at,
		FromState: from, ToState: incident.State, Revision: incident.Revision, Summary: summary, Data: data,
	}
}

func (s *Service) CreateIncident(command CreateIncidentCommand, key string) (domain.IncidentDetail, bool, error) {
	if err := validateKey(key); err != nil {
		return domain.IncidentDetail{}, false, err
	}
	showcase, err := s.store.Showcase(command.ShowcaseID)
	if err != nil {
		return domain.IncidentDetail{}, false, err
	}
	if err := domain.ValidateIncidentInput(showcase, command.Source, command.Metric, command.ObservedValue, command.Reporter, command.DetectedAt); err != nil {
		return domain.IncidentDetail{}, false, err
	}
	hash, err := requestHash(command)
	if err != nil {
		return domain.IncidentDetail{}, false, err
	}
	now := s.now().UTC()
	fingerprint := domain.IncidentFingerprint(command.ShowcaseID, command.Source, command.Metric, command.ObservedValue, command.DetectedAt)
	var resultID string
	created := false
	err = s.store.Mutate(func(data *store.Data) ([]domain.AuditEvent, error) {
		if id, found, idemErr := checkIdempotency(data, key, "create_incident", hash); idemErr != nil {
			return nil, idemErr
		} else if found {
			resultID = id
			return nil, nil
		}
		for _, existing := range data.Incidents {
			if existing.Fingerprint == fingerprint && existing.State != domain.StateClosed {
				resultID = existing.ID
				report := domain.IncidentReport{ID: newID("report"), IncidentID: existing.ID, Reporter: strings.TrimSpace(command.Reporter), DetectedAt: command.DetectedAt.UTC(), Source: command.Source, Metric: command.Metric, ObservedValue: command.ObservedValue, Description: strings.TrimSpace(command.Description), CreatedAt: now}
				data.Reports[existing.ID] = append(data.Reports[existing.ID], report)
				existing.UpdatedAt = now
				existing.ReportCount = len(data.Reports[existing.ID])
				if existing.LastReportedAt == nil || report.DetectedAt.After(*existing.LastReportedAt) { latest := report.DetectedAt; existing.LastReportedAt = &latest }
				existing.Revision++
				data.Incidents[existing.ID] = existing
				saveIdempotency(data, key, "create_incident", hash, resultID, now)
				return []domain.AuditEvent{event(existing, "incident.reported_again", command.Reporter, "异常收到补报并聚合", existing.State, map[string]any{"report_id": report.ID, "source": command.Source}, now)}, nil
			}
		}
		unit := "°C"
		if command.Metric == domain.MetricHumidity {
			unit = "%RH"
		}
		incident := domain.EnvironmentIncident{
			ID: newID("inc"), ShowcaseID: command.ShowcaseID, DetectedAt: command.DetectedAt.UTC(),
			Source: command.Source, Metric: command.Metric, ObservedValue: command.ObservedValue, Unit: unit,
			Severity: domain.AssessSeverity(showcase, command.Metric, command.ObservedValue),
			State:    domain.StateReported, Fingerprint: fingerprint, Revision: 1, Reporter: strings.TrimSpace(command.Reporter),
			Description: strings.TrimSpace(command.Description), CreatedAt: now, UpdatedAt: now,
		}
		assessment := domain.AssessImpact(showcase, command.Metric, command.ObservedValue, incident.ID, now)
		first := event(incident, "incident.reported", command.Reporter, "异常已登记", "", map[string]any{"metric": command.Metric, "value": command.ObservedValue}, now)
		from := incident.State
		if err := domain.Transition(&incident, domain.StateAssessed, now); err != nil {
			return nil, err
		}
		second := event(incident, "incident.assessed", "system", "已按保护级别和偏差幅度完成分级", from, map[string]any{"severity": incident.Severity}, now)
		data.Incidents[incident.ID] = incident
		data.Assessments[incident.ID] = assessment
		data.Reports[incident.ID] = []domain.IncidentReport{{ID: newID("report"), IncidentID: incident.ID, Reporter: incident.Reporter, DetectedAt: incident.DetectedAt, Source: incident.Source, Metric: incident.Metric, ObservedValue: incident.ObservedValue, Description: incident.Description, CreatedAt: now}}
		incident.ReportCount = 1
		incident.LastReportedAt = &incident.DetectedAt
		data.Incidents[incident.ID] = incident
		resultID, created = incident.ID, true
		saveIdempotency(data, key, "create_incident", hash, resultID, now)
		return []domain.AuditEvent{first, second}, nil
	})
	if err != nil {
		return domain.IncidentDetail{}, false, err
	}
	detail, err := s.store.Detail(resultID)
	return detail, created, err
}

func (s *Service) SubmitPlan(command SubmitPlanCommand, key string) (domain.IncidentDetail, error) {
	if err := validateKey(key); err != nil {
		return domain.IncidentDetail{}, err
	}
	if strings.TrimSpace(command.Actor) == "" {
		return domain.IncidentDetail{}, domain.Validation("提交人不能为空", map[string]string{"actor": "不能为空"})
	}
	now := s.now().UTC()
	if err := domain.ValidatePlan(command.Owner, command.DueAt, command.Steps, now); err != nil {
		return domain.IncidentDetail{}, err
	}
	hash, err := requestHash(command)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	err = s.store.Mutate(func(data *store.Data) ([]domain.AuditEvent, error) {
		if _, found, idemErr := checkIdempotency(data, key, "submit_plan", hash); idemErr != nil {
			return nil, idemErr
		} else if found {
			return nil, nil
		}
		incident, exists := data.Incidents[command.IncidentID]
		if !exists {
			return nil, domain.ErrNotFound
		}
		if err := checkRevision(incident, command.ExpectedRevision); err != nil {
			return nil, err
		}
		if incident.State != domain.StateAssessed {
			return nil, domain.InvalidState("只有已分级事件可以提交处置方案")
		}
		if previous, exists := data.Plans[incident.ID]; exists && previous.Status == domain.PlanRejected {
			if err := domain.ValidatePlanRevision(previous, command.Owner, command.DueAt, command.Steps, command.RiskNotes, now); err != nil {
				return nil, err
			}
		}
		version := 1
		if previous, exists := data.Plans[incident.ID]; exists {
			version = int(previous.Revision + 1)
		}
		plan := domain.MitigationPlan{
			ID: newID("plan"), IncidentID: incident.ID, Owner: strings.TrimSpace(command.Owner), DueAt: command.DueAt.UTC(),
			Steps: append([]domain.PlanStep(nil), command.Steps...), RiskNotes: strings.TrimSpace(command.RiskNotes),
			Status: domain.PlanPending, Revision: int64(version), CreatedAt: now,
		}
		from := incident.State
		if err := domain.Transition(&incident, domain.StatePlanPending, now); err != nil {
			return nil, err
		}
		data.Incidents[incident.ID], data.Plans[incident.ID] = incident, plan
		saveIdempotency(data, key, "submit_plan", hash, incident.ID, now)
		return []domain.AuditEvent{event(incident, "plan.submitted", command.Actor, "处置方案已提交审批", from, map[string]any{"owner": plan.Owner, "due_at": plan.DueAt}, now)}, nil
	})
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	return s.store.Detail(command.IncidentID)
}

func (s *Service) ApprovePlan(command ApprovePlanCommand, key string) (domain.IncidentDetail, error) {
	if err := validateKey(key); err != nil {
		return domain.IncidentDetail{}, err
	}
	if strings.TrimSpace(command.Approver) == "" {
		return domain.IncidentDetail{}, domain.Validation("审批人不能为空", map[string]string{"approver": "不能为空"})
	}
	hash, err := requestHash(command)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	now := s.now().UTC()
	err = s.store.Mutate(func(data *store.Data) ([]domain.AuditEvent, error) {
		if _, found, idemErr := checkIdempotency(data, key, "approve_plan", hash); idemErr != nil {
			return nil, idemErr
		} else if found {
			return nil, nil
		}
		incident, exists := data.Incidents[command.IncidentID]
		if !exists {
			return nil, domain.ErrNotFound
		}
		if err := checkRevision(incident, command.ExpectedRevision); err != nil {
			return nil, err
		}
		if incident.State != domain.StatePlanPending {
			return nil, domain.InvalidState("只有待审批方案可以审批")
		}
		plan, exists := data.Plans[incident.ID]
		if !exists {
			return nil, domain.ErrNotFound
		}
		from := incident.State
		if !command.Approved && strings.TrimSpace(command.Comment) == "" {
			return nil, domain.Validation("退回意见不能为空", map[string]string{"comment": "退回时必须填写审批意见"})
		}
		target, kind, summary := domain.StateApproved, "plan.approved", "处置方案已批准"
		priorPlan := plan
		plan.Approver, plan.Revision = strings.TrimSpace(command.Approver), plan.Revision+1
		if command.Approved {
			plan.Status = domain.PlanApproved
			approved := now
			plan.ApprovedAt = &approved
		} else {
			rejected := now
			priorPlan.Approver = strings.TrimSpace(command.Approver)
			data.PlanVersions[incident.ID] = append(data.PlanVersions[incident.ID], domain.MitigationPlanVersion{Version: int(priorPlan.Revision), Plan: priorPlan, Approver: strings.TrimSpace(command.Approver), Comment: strings.TrimSpace(command.Comment), RejectedAt: &rejected})
			plan.Revision = priorPlan.Revision
			plan.Status, target, kind, summary = domain.PlanRejected, domain.StateAssessed, "plan.rejected", "处置方案已退回修订"
		}
		if err := domain.Transition(&incident, target, now); err != nil {
			return nil, err
		}
		data.Incidents[incident.ID], data.Plans[incident.ID] = incident, plan
		saveIdempotency(data, key, "approve_plan", hash, incident.ID, now)
		return []domain.AuditEvent{event(incident, kind, command.Approver, summary, from, map[string]any{"comment": command.Comment}, now)}, nil
	})
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	return s.store.Detail(command.IncidentID)
}

func (s *Service) RecordAction(command RecordActionCommand, key string) (domain.IncidentDetail, error) {
	if err := validateKey(key); err != nil {
		return domain.IncidentDetail{}, err
	}
	if err := domain.ValidateAction(command.Operator, command.Description, command.PerformedAt); err != nil {
		return domain.IncidentDetail{}, err
	}
	hash, err := requestHash(command)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	now := s.now().UTC()
	err = s.store.Mutate(func(data *store.Data) ([]domain.AuditEvent, error) {
		if _, found, idemErr := checkIdempotency(data, key, "record_action", hash); idemErr != nil {
			return nil, idemErr
		} else if found {
			return nil, nil
		}
		incident, exists := data.Incidents[command.IncidentID]
		if !exists {
			return nil, domain.ErrNotFound
		}
		if err := checkRevision(incident, command.ExpectedRevision); err != nil {
			return nil, err
		}
		if incident.State != domain.StateApproved && incident.State != domain.StateExecuting {
			return nil, domain.InvalidState("方案批准后才能记录现场动作")
		}
		plan, exists := data.Plans[incident.ID]
		if !exists || plan.Status != domain.PlanApproved {
			return nil, domain.InvalidState("缺少已批准的处置方案")
		}
		if plan.ApprovedAt != nil && command.PerformedAt.Before(*plan.ApprovedAt) {
			return nil, domain.Validation("现场动作无效", map[string]string{"performed_at": "不得早于方案批准时间"})
		}
		if command.PerformedAt.After(now) {
			return nil, domain.Validation("现场动作无效", map[string]string{"performed_at": "不得晚于当前时间"})
		}
		stepOrder := command.StepOrder
		if stepOrder == 0 && len(plan.Steps) == 1 {
			stepOrder = plan.Steps[0].Order
		}
		validStep := false
		for _, step := range plan.Steps {
			if step.Order == stepOrder {
				validStep = true
				break
			}
		}
		if !validStep {
			return nil, domain.Validation("现场动作无效", map[string]string{"step_order": "必须选择当前已批准方案中的步骤"})
		}
		completed := command.Completed
		if !completed && strings.TrimSpace(command.EvidenceRef) != "" {
			completed = true
		}
		action := domain.FieldAction{ID: newID("act"), IncidentID: incident.ID, PerformedAt: command.PerformedAt.UTC(), Operator: strings.TrimSpace(command.Operator), Description: strings.TrimSpace(command.Description), EvidenceRef: strings.TrimSpace(command.EvidenceRef), StepOrder: stepOrder, Completed: completed}
		from := incident.State
		if incident.State == domain.StateApproved {
			if err := domain.Transition(&incident, domain.StateExecuting, now); err != nil {
				return nil, err
			}
		} else {
			incident.Revision++
			incident.UpdatedAt = now
		}
		data.Incidents[incident.ID] = incident
		data.Actions[incident.ID] = append(data.Actions[incident.ID], action)
		saveIdempotency(data, key, "record_action", hash, incident.ID, now)
		return []domain.AuditEvent{event(incident, "action.recorded", command.Operator, "已记录现场处置动作", from, map[string]any{"description": action.Description, "evidence_ref": action.EvidenceRef, "step_order": action.StepOrder, "completed": action.Completed}, now)}, nil
	})
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	return s.store.Detail(command.IncidentID)
}

func (s *Service) RecordVerification(command RecordVerificationCommand, key string) (domain.IncidentDetail, error) {
	if err := validateKey(key); err != nil {
		return domain.IncidentDetail{}, err
	}
	if err := domain.ValidateVerification(command.Operator, command.InstrumentID, command.MeasuredAt, command.Temperature, command.Humidity); err != nil {
		return domain.IncidentDetail{}, err
	}
	hash, err := requestHash(command)
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	now := s.now().UTC()
	err = s.store.Mutate(func(data *store.Data) ([]domain.AuditEvent, error) {
		if _, found, idemErr := checkIdempotency(data, key, "record_verification", hash); idemErr != nil {
			return nil, idemErr
		} else if found {
			return nil, nil
		}
		incident, exists := data.Incidents[command.IncidentID]
		if !exists {
			return nil, domain.ErrNotFound
		}
		if err := checkRevision(incident, command.ExpectedRevision); err != nil {
			return nil, err
		}
		if incident.State != domain.StateExecuting && incident.State != domain.StateVerifying {
			return nil, domain.InvalidState("现场处置开始后才能录入复测")
		}
		plan, planExists := data.Plans[incident.ID]
		if !planExists || plan.Status != domain.PlanApproved {
			return nil, domain.InvalidState("缺少已批准的处置方案")
		}
		if len(data.Verifications[incident.ID]) == 0 {
			missing := make([]string, 0)
			for _, step := range plan.Steps {
				done := false
				for _, action := range data.Actions[incident.ID] {
					if action.StepOrder == step.Order && action.Completed && strings.TrimSpace(action.EvidenceRef) != "" {
						done = true
						break
					}
				}
				if !done {
					missing = append(missing, fmt.Sprint(step.Order))
				}
			}
			if len(missing) > 0 {
				return nil, domain.Conflict("复测前仍有未核销步骤：" + strings.Join(missing, ","))
			}
		}
		showcase := data.Showcases[incident.ShowcaseID]
		within := domain.TargetContains(showcase, command.Temperature, command.Humidity)
		record := domain.VerificationRecord{ID: newID("verify"), IncidentID: incident.ID, MeasuredAt: command.MeasuredAt.UTC(), Temperature: command.Temperature, Humidity: command.Humidity, InstrumentID: strings.TrimSpace(command.InstrumentID), WithinTarget: within, Comment: strings.TrimSpace(command.Comment), Operator: strings.TrimSpace(command.Operator)}
		previous := data.Verifications[incident.ID]
		from := incident.State
		target := domain.StateVerifying
		if !within {
			target = domain.StateExecuting
		}
		consecutive := within && len(previous) > 0 && previous[len(previous)-1].WithinTarget
		if consecutive {
			target = domain.StateClosed
		}
		if incident.State != target {
			if err := domain.Transition(&incident, target, now); err != nil {
				return nil, err
			}
		} else {
			incident.Revision++
			incident.UpdatedAt = now
		}
		data.Verifications[incident.ID] = append(previous, record)
		data.Incidents[incident.ID] = incident
		kind, summary := "verification.recorded", "复测读数未恢复，返回现场处置"
		if within {
			summary = "复测读数位于目标区间，继续观察"
		}
		if target == domain.StateClosed {
			kind, summary = "incident.closed", "连续复测达标，事件已关闭归档"
			actions := data.Actions[incident.ID]
			verifications := data.Verifications[incident.ID]
			overdue := now.After(plan.DueAt)
			archive := domain.ArchiveRecord{ID: newID("archive"), IncidentID: incident.ID, ShowcaseID: showcase.ID, ShowcaseName: showcase.Name, GalleryZone: showcase.GalleryZone, Severity: incident.Severity, DetectedAt: incident.DetectedAt, ClosedAt: now, Owner: plan.Owner, ActionCount: len(actions), VerificationCount: len(verifications), ResolutionSummary: fmt.Sprintf("完成 %d 项现场动作，连续两次复测达标", len(actions)), PlanDueAt: plan.DueAt, Overdue: overdue}
			parts := []string{archive.ID, incident.ID, showcase.ID, showcase.Name, showcase.GalleryZone, plan.Owner, incident.Reporter, incident.Description, archive.ResolutionSummary}
			for _, action := range actions {
				parts = append(parts, action.Description, action.EvidenceRef, action.Operator)
			}
			archive.SearchableText = strings.Join(parts, " ")
			data.Archives[incident.ID] = archive
		}
		saveIdempotency(data, key, "record_verification", hash, incident.ID, now)
		return []domain.AuditEvent{event(incident, kind, command.Operator, summary, from, map[string]any{"temperature": record.Temperature, "humidity": record.Humidity, "within_target": record.WithinTarget}, now)}, nil
	})
	if err != nil {
		return domain.IncidentDetail{}, err
	}
	return s.store.Detail(command.IncidentID)
}

func IsConflict(err error) bool {
	return errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvalidState) || errors.Is(err, domain.ErrIdempotencyKey)
}
