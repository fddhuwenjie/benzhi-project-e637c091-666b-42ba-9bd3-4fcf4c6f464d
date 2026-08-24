package domain

import "time"

type VerificationRecord struct {
	ID           string    `json:"id"`
	IncidentID   string    `json:"incident_id"`
	MeasuredAt   time.Time `json:"measured_at"`
	Temperature  float64   `json:"temperature"`
	Humidity     float64   `json:"humidity"`
	InstrumentID string    `json:"instrument_id"`
	WithinTarget bool      `json:"within_target"`
	Comment      string    `json:"comment"`
	Operator     string    `json:"operator"`
}

type ArchiveRecord struct {
	ID                string    `json:"id"`
	IncidentID        string    `json:"incident_id"`
	ShowcaseID        string    `json:"showcase_id"`
	ShowcaseName      string    `json:"showcase_name"`
	GalleryZone       string    `json:"gallery_zone"`
	Severity          Severity  `json:"severity"`
	DetectedAt        time.Time `json:"detected_at"`
	ClosedAt          time.Time `json:"closed_at"`
	Owner             string    `json:"owner"`
	ActionCount       int       `json:"action_count"`
	VerificationCount int       `json:"verification_count"`
	ResolutionSummary string    `json:"resolution_summary"`
	SearchableText    string    `json:"searchable_text"`
	PlanDueAt         time.Time `json:"plan_due_at"`
	Overdue           bool      `json:"overdue"`
}

type AuditEvent struct {
	ID         string         `json:"id"`
	IncidentID string         `json:"incident_id"`
	Type       string         `json:"type"`
	Actor      string         `json:"actor"`
	At         time.Time      `json:"at"`
	FromState  IncidentState  `json:"from_state,omitempty"`
	ToState    IncidentState  `json:"to_state,omitempty"`
	Revision   int64          `json:"revision"`
	Summary    string         `json:"summary"`
	Data       map[string]any `json:"data,omitempty"`
}

type IncidentDetail struct {
	Incident      EnvironmentIncident     `json:"incident"`
	Showcase      Showcase                `json:"showcase"`
	Assessment    *ImpactAssessment       `json:"assessment,omitempty"`
	Plan          *MitigationPlan         `json:"plan,omitempty"`
	Actions       []FieldAction           `json:"actions"`
	Verifications []VerificationRecord    `json:"verifications"`
	Audit         []AuditEvent            `json:"audit"`
	Archive       *ArchiveRecord          `json:"archive,omitempty"`
	Reports       []IncidentReport        `json:"reports"`
	PlanHistory   []MitigationPlanVersion `json:"plan_history"`
	PlanVersions  []MitigationPlanVersion `json:"plan_versions"`
	PlanDiff      map[string]any          `json:"plan_diff,omitempty"`
	StepProgress  []StepProgress          `json:"step_progress"`
}
