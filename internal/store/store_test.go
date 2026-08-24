package store

import (
	"os"
	"testing"
	"time"

	"showcaseguard/internal/domain"
)

func TestSnapshotRoundTripAndRevisionMutation(t *testing.T) {
	directory := t.TempDir()
	s, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	showcase := domain.Showcase{ID: "case-1", Name: "一号柜", GalleryZone: "A", CollectionLevel: domain.CollectionKey, TargetTemperatureMin: 18, TargetTemperatureMax: 22, TargetHumidityMin: 45, TargetHumidityMax: 55, Active: true}
	if err := s.SeedShowcases([]domain.Showcase{showcase}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = s.Mutate(func(data *Data) ([]domain.AuditEvent, error) {
		data.Incidents["inc-1"] = domain.EnvironmentIncident{ID: "inc-1", ShowcaseID: showcase.ID, Fingerprint: "fp", State: domain.StateAssessed, Revision: 1, CreatedAt: now, UpdatedAt: now}
		return []domain.AuditEvent{{ID: "event-1", IncidentID: "inc-1", Type: "created", At: now, Revision: 1}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	incident, found := reopened.FindByFingerprint("fp")
	if !found || incident.ID != "inc-1" {
		t.Fatalf("恢复失败: %#v", incident)
	}
	detail, err := reopened.Detail("inc-1")
	if err != nil || len(detail.Audit) != 1 {
		t.Fatalf("审计恢复失败: %#v %v", detail.Audit, err)
	}
	content, err := os.ReadFile(directory + "/audit.jsonl")
	if err != nil || len(content) == 0 {
		t.Fatalf("审计日志未写入: %v", err)
	}
}
