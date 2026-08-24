package workflow

import (
	"time"

	"showcaseguard/internal/domain"
)

type CreateIncidentCommand struct {
	ShowcaseID    string                `json:"showcase_id"`
	DetectedAt    time.Time             `json:"detected_at"`
	Source        domain.IncidentSource `json:"source"`
	Metric        domain.Metric         `json:"metric"`
	ObservedValue float64               `json:"observed_value"`
	Reporter      string                `json:"reporter"`
	Description   string                `json:"description"`
}

type SubmitPlanCommand struct {
	IncidentID       string            `json:"-"`
	ExpectedRevision int64             `json:"expected_revision"`
	Owner            string            `json:"owner"`
	DueAt            time.Time         `json:"due_at"`
	Steps            []domain.PlanStep `json:"steps"`
	RiskNotes        string            `json:"risk_notes"`
	Actor            string            `json:"actor"`
}

type ApprovePlanCommand struct {
	IncidentID       string `json:"-"`
	ExpectedRevision int64  `json:"expected_revision"`
	Approver         string `json:"approver"`
	Approved         bool   `json:"approved"`
	Comment          string `json:"comment"`
}

type RecordActionCommand struct {
	IncidentID       string    `json:"-"`
	ExpectedRevision int64     `json:"expected_revision"`
	PerformedAt      time.Time `json:"performed_at"`
	Operator         string    `json:"operator"`
	Description      string    `json:"description"`
	EvidenceRef      string    `json:"evidence_ref"`
	StepOrder        int       `json:"step_order"`
	Completed        bool      `json:"completed"`
}

type RecordVerificationCommand struct {
	IncidentID       string    `json:"-"`
	ExpectedRevision int64     `json:"expected_revision"`
	MeasuredAt       time.Time `json:"measured_at"`
	Temperature      float64   `json:"temperature"`
	Humidity         float64   `json:"humidity"`
	InstrumentID     string    `json:"instrument_id"`
	Comment          string    `json:"comment"`
	Operator         string    `json:"operator"`
}

type IncidentFilter struct {
	ShowcaseID string
	State      domain.IncidentState
}

type QueueItem struct {
	Incident domain.EnvironmentIncident `json:"incident"`
	Showcase domain.Showcase            `json:"showcase"`
}
