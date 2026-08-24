package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"showcaseguard/internal/domain"
)

type IdempotencyRecord struct {
	Key         string    `json:"key"`
	Operation   string    `json:"operation"`
	RequestHash string    `json:"request_hash"`
	ResourceID  string    `json:"resource_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type Data struct {
	Showcases     map[string]domain.Showcase                `json:"showcases"`
	Incidents     map[string]domain.EnvironmentIncident     `json:"incidents"`
	Assessments   map[string]domain.ImpactAssessment        `json:"assessments"`
	Plans         map[string]domain.MitigationPlan          `json:"plans"`
	PlanVersions  map[string][]domain.MitigationPlanVersion `json:"plan_versions"`
	Reports       map[string][]domain.IncidentReport        `json:"reports"`
	Actions       map[string][]domain.FieldAction           `json:"actions"`
	Verifications map[string][]domain.VerificationRecord    `json:"verifications"`
	Archives      map[string]domain.ArchiveRecord           `json:"archives"`
	Audit         map[string][]domain.AuditEvent            `json:"audit"`
	Idempotency   map[string]IdempotencyRecord              `json:"idempotency"`
}

func emptyData() Data {
	return Data{
		Showcases: map[string]domain.Showcase{}, Incidents: map[string]domain.EnvironmentIncident{},
		Assessments: map[string]domain.ImpactAssessment{},
		Plans:       map[string]domain.MitigationPlan{}, PlanVersions: map[string][]domain.MitigationPlanVersion{}, Reports: map[string][]domain.IncidentReport{}, Actions: map[string][]domain.FieldAction{},
		Verifications: map[string][]domain.VerificationRecord{}, Archives: map[string]domain.ArchiveRecord{},
		Audit: map[string][]domain.AuditEvent{}, Idempotency: map[string]IdempotencyRecord{},
	}
}

type Store struct {
	mu           sync.RWMutex
	data         Data
	fingerprints map[string]string
	snapshotPath string
	auditPath    string
}

func Open(directory string) (*Store, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("存储目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("创建存储目录: %w", err)
	}
	s := &Store{
		data: emptyData(), fingerprints: map[string]string{},
		snapshotPath: filepath.Join(directory, "snapshot.json"),
		auditPath:    filepath.Join(directory, "audit.jsonl"),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	if err := s.reconcileAuditLog(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	file, err := os.Open(s.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("打开快照: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 32<<20))
	decoder.DisallowUnknownFields()
	var loaded Data
	if err := decoder.Decode(&loaded); err != nil {
		return fmt.Errorf("解析快照: %w", err)
	}
	normalize(&loaded)
	s.data = loaded
	s.rebuildIndexes()
	return nil
}

func normalize(data *Data) {
	if data.Showcases == nil {
		data.Showcases = map[string]domain.Showcase{}
	}
	if data.Incidents == nil {
		data.Incidents = map[string]domain.EnvironmentIncident{}
	}
	if data.Assessments == nil {
		data.Assessments = map[string]domain.ImpactAssessment{}
	}
	if data.Plans == nil {
		data.Plans = map[string]domain.MitigationPlan{}
	}
	if data.PlanVersions == nil {
		data.PlanVersions = map[string][]domain.MitigationPlanVersion{}
	}
	if data.Reports == nil {
		data.Reports = map[string][]domain.IncidentReport{}
	}
	if data.Actions == nil {
		data.Actions = map[string][]domain.FieldAction{}
	}
	if data.Verifications == nil {
		data.Verifications = map[string][]domain.VerificationRecord{}
	}
	if data.Archives == nil {
		data.Archives = map[string]domain.ArchiveRecord{}
	}
	if data.Audit == nil {
		data.Audit = map[string][]domain.AuditEvent{}
	}
	if data.Idempotency == nil {
		data.Idempotency = map[string]IdempotencyRecord{}
	}
}

func (s *Store) rebuildIndexes() {
	s.fingerprints = map[string]string{}
	for id, incident := range s.data.Incidents {
		if incident.State != domain.StateClosed {
			s.fingerprints[incident.Fingerprint] = id
		}
	}
}

func cloneData(source Data) (Data, error) {
	encoded, err := json.Marshal(source)
	if err != nil {
		return Data{}, err
	}
	var cloned Data
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return Data{}, err
	}
	normalize(&cloned)
	return cloned, nil
}

// Mutate 在同一临界区完成校验、业务修改、快照提交和审计追加。
func (s *Store) Mutate(fn func(*Data) ([]domain.AuditEvent, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup, err := cloneData(s.data)
	if err != nil {
		return fmt.Errorf("备份事务数据: %w", err)
	}
	events, err := fn(&s.data)
	if err != nil {
		s.data = backup
		return err
	}
	for _, event := range events {
		s.data.Audit[event.IncidentID] = append(s.data.Audit[event.IncidentID], event)
	}
	if err := s.writeSnapshotLocked(); err != nil {
		s.data = backup
		return err
	}
	s.rebuildIndexes()
	if err := s.appendAuditLocked(events); err != nil {
		return err
	}
	return nil
}

func (s *Store) writeSnapshotLocked() error {
	temporary, err := os.CreateTemp(filepath.Dir(s.snapshotPath), ".snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时快照: %w", err)
	}
	temporaryName := temporary.Name()
	remove := true
	defer func() {
		temporary.Close()
		if remove {
			_ = os.Remove(temporaryName)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.data); err != nil {
		return fmt.Errorf("编码快照: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("同步快照: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭快照: %w", err)
	}
	if err := os.Chmod(temporaryName, 0o640); err != nil {
		return fmt.Errorf("设置快照权限: %w", err)
	}
	if err := os.Rename(temporaryName, s.snapshotPath); err != nil {
		return fmt.Errorf("替换快照: %w", err)
	}
	remove = false
	if directory, err := os.Open(filepath.Dir(s.snapshotPath)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (s *Store) appendAuditLocked(events []domain.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	file, err := os.OpenFile(s.auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("打开审计日志: %w", err)
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			file.Close()
			return fmt.Errorf("写入审计日志: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return fmt.Errorf("刷新审计日志: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("同步审计日志: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭审计日志: %w", err)
	}
	return nil
}

func (s *Store) SeedShowcases(showcases []domain.Showcase) error {
	return s.Mutate(func(data *Data) ([]domain.AuditEvent, error) {
		changed := false
		for _, showcase := range showcases {
			if err := showcase.Validate(); err != nil {
				return nil, err
			}
			if _, exists := data.Showcases[showcase.ID]; !exists {
				data.Showcases[showcase.ID] = showcase
				changed = true
			}
		}
		if !changed {
			return nil, nil
		}
		return nil, nil
	})
}

func (s *Store) FindByFingerprint(fingerprint string) (domain.EnvironmentIncident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, exists := s.fingerprints[fingerprint]
	if !exists {
		return domain.EnvironmentIncident{}, false
	}
	incident, exists := s.data.Incidents[id]
	return incident, exists
}

func (s *Store) Showcase(id string) (domain.Showcase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	showcase, ok := s.data.Showcases[id]
	if !ok {
		return domain.Showcase{}, domain.ErrNotFound
	}
	return showcase, nil
}

func (s *Store) Showcases(activeOnly bool) []domain.Showcase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Showcase, 0, len(s.data.Showcases))
	for _, showcase := range s.data.Showcases {
		if !activeOnly || showcase.Active {
			result = append(result, showcase)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (s *Store) Incidents(showcaseID string, state domain.IncidentState) []domain.EnvironmentIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := []domain.EnvironmentIncident{}
	for _, incident := range s.data.Incidents {
		if showcaseID != "" && incident.ShowcaseID != showcaseID {
			continue
		}
		if state != "" && incident.State != state {
			continue
		}
		result = append(result, incident)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result
}

func (s *Store) Incident(id string) (domain.EnvironmentIncident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	incident, ok := s.data.Incidents[id]
	if !ok {
		return domain.EnvironmentIncident{}, domain.ErrNotFound
	}
	return incident, nil
}

func (s *Store) Detail(id string) (domain.IncidentDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	incident, ok := s.data.Incidents[id]
	if !ok {
		return domain.IncidentDetail{}, domain.ErrNotFound
	}
	showcase := s.data.Showcases[incident.ShowcaseID]
	detail := domain.IncidentDetail{
		Incident: incident, Showcase: showcase,
		Actions:       append([]domain.FieldAction(nil), s.data.Actions[id]...),
		Verifications: append([]domain.VerificationRecord(nil), s.data.Verifications[id]...),
		Audit:         append([]domain.AuditEvent(nil), s.data.Audit[id]...),
		Reports:       append([]domain.IncidentReport(nil), s.data.Reports[id]...),
		PlanHistory:   append([]domain.MitigationPlanVersion(nil), s.data.PlanVersions[id]...),
		PlanVersions:  append([]domain.MitigationPlanVersion(nil), s.data.PlanVersions[id]...),
	}
	if assessment, exists := s.data.Assessments[id]; exists {
		copy := assessment
		detail.Assessment = &copy
	}
	if plan, exists := s.data.Plans[id]; exists {
		copy := plan
		detail.Plan = &copy
	}
	if detail.Plan != nil {
		for _, step := range detail.Plan.Steps {
			progress := domain.StepProgress{Order: step.Order, Instruction: step.Instruction}
			for _, action := range detail.Actions {
				if action.StepOrder == step.Order {
					progress.ActionCount++
					if action.Completed && strings.TrimSpace(action.EvidenceRef) != "" {
						progress.Completed = true
					}
				}
			}
			detail.StepProgress = append(detail.StepProgress, progress)
		}
		if len(detail.PlanHistory) > 0 {
			last := detail.PlanHistory[len(detail.PlanHistory)-1].Plan
			detail.PlanDiff = map[string]any{"owner_changed": detail.Plan.Owner != last.Owner, "due_at_changed": !detail.Plan.DueAt.Equal(last.DueAt), "steps_changed": fmt.Sprint(detail.Plan.Steps) != fmt.Sprint(last.Steps), "risk_notes_changed": detail.Plan.RiskNotes != last.RiskNotes}
		}
	}
	if archive, exists := s.data.Archives[id]; exists {
		copy := archive
		detail.Archive = &copy
	}
	return detail, nil
}

func (s *Store) Idempotency(key string) (IdempotencyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.data.Idempotency[key]
	return record, ok
}

func (s *Store) Archives(query string) []domain.ArchiveRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	result := []domain.ArchiveRecord{}
	for _, archive := range s.data.Archives {
		if query == "" || strings.Contains(strings.ToLower(archive.SearchableText), query) {
			result = append(result, archive)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ClosedAt.After(result[j].ClosedAt) })
	return result
}

type ArchiveQuery struct {
	Query, GalleryZone, Owner string
	Severity                  domain.Severity
	From, To                  *time.Time
	Overdue                   *bool
	Limit                     int
	Cursor                    string
}
type ArchiveResult struct {
	Items                                []domain.ArchiveRecord
	Total                                int
	NextCursor                           string
	AvgDuration, MaxDuration, AvgActions float64
	OverdueCount                         int
}

func (s *Store) SearchArchives(q ArchiveQuery) ArchiveResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]domain.ArchiveRecord, 0)
	needle := strings.ToLower(strings.TrimSpace(q.Query))
	for _, a := range s.data.Archives {
		if needle != "" && !strings.Contains(strings.ToLower(a.SearchableText), needle) {
			continue
		}
		if q.GalleryZone != "" && a.GalleryZone != q.GalleryZone {
			continue
		}
		if q.Owner != "" && a.Owner != q.Owner {
			continue
		}
		if q.Severity != "" && a.Severity != q.Severity {
			continue
		}
		if q.From != nil && a.ClosedAt.Before(*q.From) {
			continue
		}
		if q.To != nil && a.ClosedAt.After(*q.To) {
			continue
		}
		if q.Overdue != nil && a.Overdue != *q.Overdue {
			continue
		}
		all = append(all, a)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].ClosedAt.Equal(all[j].ClosedAt) {
			return all[i].IncidentID < all[j].IncidentID
		}
		return all[i].ClosedAt.After(all[j].ClosedAt)
	})
	total := len(all)
	start := 0
	if q.Cursor != "" {
		for i, a := range all {
			if a.IncidentID == q.Cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(all) {
		start = len(all)
	}
	end := len(all)
	if q.Limit > 0 && start+q.Limit < end {
		end = start + q.Limit
	}
	items := all[start:end]
	next := ""
	if end < len(all) {
		next = items[len(items)-1].IncidentID
	}
	var totalDuration float64
	var maxDuration float64
	overdue := 0
	actions := 0
	for _, a := range all {
		d := a.ClosedAt.Sub(a.DetectedAt).Hours()
		totalDuration += d
		if d > maxDuration {
			maxDuration = d
		}
		if a.Overdue {
			overdue++
		}
		actions += a.ActionCount
	}
	avg := 0.0
	avgAct := 0.0
	if total > 0 {
		avg = totalDuration / float64(total)
		avgAct = float64(actions) / float64(total)
	}
	return ArchiveResult{Items: items, Total: total, NextCursor: next, AvgDuration: avg, MaxDuration: maxDuration, OverdueCount: overdue, AvgActions: avgAct}
}
