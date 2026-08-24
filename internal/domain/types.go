package domain

type CollectionLevel string

const (
	CollectionGeneral CollectionLevel = "general"
	CollectionKey     CollectionLevel = "key"
	CollectionRare    CollectionLevel = "rare"
)

type IncidentSource string

const (
	SourceSensor IncidentSource = "sensor"
	SourceManual IncidentSource = "manual"
)

type Metric string

const (
	MetricTemperature Metric = "temperature"
	MetricHumidity    Metric = "humidity"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type IncidentState string

const (
	StateReported    IncidentState = "reported"
	StateAssessed    IncidentState = "assessed"
	StatePlanPending IncidentState = "plan_pending"
	StateApproved    IncidentState = "approved"
	StateExecuting   IncidentState = "executing"
	StateVerifying   IncidentState = "verifying"
	StateClosed      IncidentState = "closed"
)

var StateLabels = map[IncidentState]string{
	StateReported:    "已登记",
	StateAssessed:    "已分级",
	StatePlanPending: "待审批",
	StateApproved:    "待执行",
	StateExecuting:   "处置中",
	StateVerifying:   "复测中",
	StateClosed:      "已归档",
}

type PlanStatus string

const (
	PlanPending  PlanStatus = "pending"
	PlanApproved PlanStatus = "approved"
	PlanRejected PlanStatus = "rejected"
)
