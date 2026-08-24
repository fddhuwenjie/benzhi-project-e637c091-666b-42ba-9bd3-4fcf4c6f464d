package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

func (s Showcase) Validate() error {
	fields := map[string]string{}
	if strings.TrimSpace(s.ID) == "" {
		fields["id"] = "不能为空"
	}
	if strings.TrimSpace(s.Name) == "" {
		fields["name"] = "不能为空"
	}
	if s.TargetTemperatureMin >= s.TargetTemperatureMax {
		fields["target_temperature"] = "最低值必须小于最高值"
	}
	if s.TargetHumidityMin < 0 || s.TargetHumidityMax > 100 || s.TargetHumidityMin >= s.TargetHumidityMax {
		fields["target_humidity"] = "湿度范围必须位于 0-100 且最低值小于最高值"
	}
	switch s.CollectionLevel {
	case CollectionGeneral, CollectionKey, CollectionRare:
	default:
		fields["collection_level"] = "必须为 general、key 或 rare"
	}
	if len(fields) > 0 {
		return Validation("展柜保护目标无效", fields)
	}
	return nil
}

func ValidateIncidentInput(showcase Showcase, source IncidentSource, metric Metric, value float64, reporter string, detectedAt time.Time) error {
	fields := map[string]string{}
	if !showcase.Active {
		fields["showcase_id"] = "展柜已停用"
	}
	switch source {
	case SourceSensor, SourceManual:
	default:
		fields["source"] = "必须为 sensor 或 manual"
	}
	switch metric {
	case MetricTemperature, MetricHumidity:
	default:
		fields["metric"] = "必须为 temperature 或 humidity"
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		fields["observed_value"] = "必须为有限数值"
	}
	if metric == MetricHumidity && (value < 0 || value > 100) {
		fields["observed_value"] = "相对湿度必须位于 0-100"
	}
	if strings.TrimSpace(reporter) == "" {
		fields["reporter"] = "不能为空"
	}
	if detectedAt.IsZero() {
		fields["detected_at"] = "不能为空"
	}
	if len(fields) > 0 {
		return Validation("异常登记信息无效", fields)
	}
	return nil
}

// IncidentFingerprint 将时间归入五分钟窗口，同一展柜、来源和指标的重复上报可稳定命中。
func IncidentFingerprint(showcaseID string, source IncidentSource, metric Metric, value float64, detectedAt time.Time) string {
	window := detectedAt.UTC().Truncate(5 * time.Minute)
	canonical := fmt.Sprintf("%s|%s|%s|%.2f|%s", strings.TrimSpace(showcaseID), source, metric, value, window.Format(time.RFC3339))
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:16])
}

func AssessSeverity(showcase Showcase, metric Metric, value float64) Severity {
	_, _, _, severity := assessmentInputs(showcase, metric, value)
	return severity
}

func assessmentInputs(showcase Showcase, metric Metric, value float64) (min, max, deviation float64, severity Severity) {
	var criticalStep, highStep float64
	if metric == MetricTemperature {
		min, max, highStep, criticalStep = showcase.TargetTemperatureMin, showcase.TargetTemperatureMax, 2, 5
	} else {
		min, max, highStep, criticalStep = showcase.TargetHumidityMin, showcase.TargetHumidityMax, 8, 15
	}
	if value < min {
		deviation = min - value
	} else if value > max {
		deviation = value - max
	}
	base := SeverityLow
	if deviation > 0 {
		base = SeverityMedium
	}
	if deviation >= highStep {
		base = SeverityHigh
	}
	if deviation >= criticalStep {
		base = SeverityCritical
	}
	return min, max, deviation, elevateForCollection(base, showcase.CollectionLevel)
}

func AssessImpact(showcase Showcase, metric Metric, value float64, incidentID string, assessedAt time.Time) ImpactAssessment {
	min, max, deviation, severity := assessmentInputs(showcase, metric, value)
	scope := ImpactShowcaseOnly
	responseWindow := 24 * time.Hour
	switch severity {
	case SeverityMedium:
		responseWindow = 8 * time.Hour
	case SeverityHigh:
		scope, responseWindow = ImpactZoneWatch, 2*time.Hour
	case SeverityCritical:
		scope, responseWindow = ImpactEmergency, 30*time.Minute
	}
	metricName, unit := "温度", "°C"
	if metric == MetricHumidity {
		metricName, unit = "相对湿度", "%RH"
	}
	rationale := fmt.Sprintf("%s读数偏离目标区间 %.1f–%.1f %s 共 %.1f %s，结合藏品级别 %s 评为 %s", metricName, min, max, unit, deviation, unit, showcase.CollectionLevel, severity)
	return ImpactAssessment{
		IncidentID: incidentID, CollectionLevel: showcase.CollectionLevel, Metric: metric,
		TargetMin: min, TargetMax: max, Deviation: deviation, Severity: severity, Scope: scope,
		ResponseDueAt: assessedAt.UTC().Add(responseWindow), Rationale: rationale, AssessedAt: assessedAt.UTC(),
	}
}

func elevateForCollection(severity Severity, level CollectionLevel) Severity {
	order := []Severity{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	index := 0
	for i, candidate := range order {
		if candidate == severity {
			index = i
		}
	}
	if level == CollectionKey {
		index++
	}
	if level == CollectionRare {
		index += 2
	}
	if index >= len(order) {
		index = len(order) - 1
	}
	return order[index]
}

func TargetContains(showcase Showcase, temperature, humidity float64) bool {
	return temperature >= showcase.TargetTemperatureMin && temperature <= showcase.TargetTemperatureMax &&
		humidity >= showcase.TargetHumidityMin && humidity <= showcase.TargetHumidityMax
}
