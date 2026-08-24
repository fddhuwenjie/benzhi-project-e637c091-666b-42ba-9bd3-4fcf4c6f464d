package workflow

import (
	"errors"
	"testing"
	"time"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/store"
)

func testService(t *testing.T) (*Service, time.Time) {
	t.Helper()
	storage, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	showcase := domain.Showcase{ID: "case-1", Name: "青铜器柜", GalleryZone: "二层东厅", CollectionLevel: domain.CollectionRare, SensorIDs: []string{"TH-01"}, TargetTemperatureMin: 18, TargetTemperatureMax: 22, TargetHumidityMin: 45, TargetHumidityMax: 55, Active: true}
	if err := storage.SeedShowcases([]domain.Showcase{showcase}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	return NewWithClock(storage, func() time.Time { return now }), now
}

func createForTest(t *testing.T, service *Service, now time.Time, key string) domain.IncidentDetail {
	t.Helper()
	detail, _, err := service.CreateIncident(CreateIncidentCommand{ShowcaseID: "case-1", DetectedAt: now.Add(-time.Minute), Source: domain.SourceSensor, Metric: domain.MetricHumidity, ObservedValue: 70, Reporter: "值班员", Description: "湿度持续升高"}, key)
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func TestFullWorkflowAndArchive(t *testing.T) {
	service, now := testService(t)
	detail := createForTest(t, service, now, "create-key-0001")
	if detail.Incident.State != domain.StateAssessed || detail.Incident.Severity != domain.SeverityCritical {
		t.Fatalf("分级错误: %#v", detail.Incident)
	}
	if detail.Assessment == nil || detail.Assessment.Scope != domain.ImpactEmergency || detail.Assessment.Deviation != 15 {
		t.Fatalf("影响评估错误: %#v", detail.Assessment)
	}
	plan, err := service.SubmitPlan(SubmitPlanCommand{IncidentID: detail.Incident.ID, ExpectedRevision: detail.Incident.Revision, Owner: "保护专员", DueAt: now.Add(2 * time.Hour), Steps: []domain.PlanStep{{Order: 1, Instruction: "检查密封条"}}, Actor: "值班主管"}, "plan-key-000001")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := service.ApprovePlan(ApprovePlanCommand{IncidentID: detail.Incident.ID, ExpectedRevision: plan.Incident.Revision, Approver: "王主管", Approved: true}, "approve-key-001")
	if err != nil {
		t.Fatal(err)
	}
	actioned, err := service.RecordAction(RecordActionCommand{IncidentID: detail.Incident.ID, ExpectedRevision: approved.Incident.Revision, PerformedAt: now, Operator: "保护专员", Description: "更换密封条", EvidenceRef: "photo://seal-1"}, "action-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.RecordVerification(RecordVerificationCommand{IncidentID: detail.Incident.ID, ExpectedRevision: actioned.Incident.Revision, MeasuredAt: now.Add(time.Hour), Temperature: 20, Humidity: 50, InstrumentID: "METER-1", Operator: "复测员"}, "verify-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	if first.Incident.State != domain.StateVerifying {
		t.Fatalf("首次达标应继续观察: %s", first.Incident.State)
	}
	second, err := service.RecordVerification(RecordVerificationCommand{IncidentID: detail.Incident.ID, ExpectedRevision: first.Incident.Revision, MeasuredAt: now.Add(2 * time.Hour), Temperature: 20.2, Humidity: 51, InstrumentID: "METER-1", Operator: "复测员"}, "verify-key-0002")
	if err != nil {
		t.Fatal(err)
	}
	if second.Incident.State != domain.StateClosed || second.Archive == nil {
		t.Fatalf("事件未关闭归档: %#v", second)
	}
	if len(service.Archives("密封条")) != 1 {
		t.Fatal("档案检索失败")
	}
}

func TestDeduplicationIdempotencyAndRevisionConflict(t *testing.T) {
	service, now := testService(t)
	first := createForTest(t, service, now, "create-key-0002")
	duplicate, created, err := service.CreateIncident(CreateIncidentCommand{ShowcaseID: "case-1", DetectedAt: now.Add(-time.Minute), Source: domain.SourceSensor, Metric: domain.MetricHumidity, ObservedValue: 70, Reporter: "值班员", Description: "湿度持续升高"}, "create-key-0003")
	if err != nil || created || duplicate.Incident.ID != first.Incident.ID {
		t.Fatalf("指纹去重失败: %v %v", created, err)
	}
	_, err = service.SubmitPlan(SubmitPlanCommand{IncidentID: first.Incident.ID, ExpectedRevision: first.Incident.Revision - 1, Owner: "甲", DueAt: now.Add(time.Hour), Steps: []domain.PlanStep{{Order: 1, Instruction: "检查"}}, Actor: "主管"}, "plan-key-000002")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("应返回修订冲突: %v", err)
	}
}
