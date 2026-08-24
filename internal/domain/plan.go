package domain

import "time"

type PlanStep struct {
	Order       int    `json:"order"`
	Instruction string `json:"instruction"`
}

type MitigationPlan struct {
	ID         string     `json:"id"`
	IncidentID string     `json:"incident_id"`
	Owner      string     `json:"owner"`
	DueAt      time.Time  `json:"due_at"`
	Steps      []PlanStep `json:"steps"`
	RiskNotes  string     `json:"risk_notes"`
	Approver   string     `json:"approver,omitempty"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	Status     PlanStatus `json:"status"`
	Revision   int64      `json:"revision"`
	CreatedAt  time.Time  `json:"created_at"`
}

type MitigationPlanVersion struct {
	Version    int            `json:"version"`
	Plan       MitigationPlan `json:"plan"`
	Approver   string         `json:"approver,omitempty"`
	Comment    string         `json:"comment,omitempty"`
	RejectedAt *time.Time     `json:"rejected_at,omitempty"`
}

type FieldAction struct {
	ID          string    `json:"id"`
	IncidentID  string    `json:"incident_id"`
	PerformedAt time.Time `json:"performed_at"`
	Operator    string    `json:"operator"`
	Description string    `json:"description"`
	EvidenceRef string    `json:"evidence_ref,omitempty"`
	StepOrder   int       `json:"step_order"`
	Completed   bool      `json:"completed"`
}

type StepProgress struct {
	Order       int    `json:"order"`
	Instruction string `json:"instruction"`
	Completed   bool   `json:"completed"`
	ActionCount int    `json:"action_count"`
}
