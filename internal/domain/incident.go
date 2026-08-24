package domain

import "time"

type EnvironmentIncident struct {
	ID             string         `json:"id"`
	ShowcaseID     string         `json:"showcase_id"`
	DetectedAt     time.Time      `json:"detected_at"`
	Source         IncidentSource `json:"source"`
	Metric         Metric         `json:"metric"`
	ObservedValue  float64        `json:"observed_value"`
	Unit           string         `json:"unit"`
	Severity       Severity       `json:"severity"`
	State          IncidentState  `json:"state"`
	Fingerprint    string         `json:"fingerprint"`
	Revision       int64          `json:"revision"`
	Reporter       string         `json:"reporter"`
	Description    string         `json:"description"`
	ReportCount    int            `json:"report_count"`
	LastReportedAt *time.Time     `json:"last_reported_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	ClosedAt       *time.Time     `json:"closed_at,omitempty"`
}

type IncidentReport struct {
	ID            string         `json:"id"`
	IncidentID    string         `json:"incident_id"`
	Reporter      string         `json:"reporter"`
	DetectedAt    time.Time      `json:"detected_at"`
	Source        IncidentSource `json:"source"`
	Metric        Metric         `json:"metric"`
	ObservedValue float64        `json:"observed_value"`
	Description   string         `json:"description"`
	CreatedAt     time.Time      `json:"created_at"`
}

type ImpactScope string

const (
	ImpactShowcaseOnly ImpactScope = "showcase_only"
	ImpactZoneWatch    ImpactScope = "zone_watch"
	ImpactEmergency    ImpactScope = "collection_emergency"
)

type ImpactAssessment struct {
	IncidentID      string          `json:"incident_id"`
	CollectionLevel CollectionLevel `json:"collection_level"`
	Metric          Metric          `json:"metric"`
	TargetMin       float64         `json:"target_min"`
	TargetMax       float64         `json:"target_max"`
	Deviation       float64         `json:"deviation"`
	Severity        Severity        `json:"severity"`
	Scope           ImpactScope     `json:"scope"`
	ResponseDueAt   time.Time       `json:"response_due_at"`
	Rationale       string          `json:"rationale"`
	AssessedAt      time.Time       `json:"assessed_at"`
}
