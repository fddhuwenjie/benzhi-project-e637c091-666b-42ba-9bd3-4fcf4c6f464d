package domain

import (
	"math"
	"sort"
	"strings"
	"time"
)

func ValidatePlan(owner string, dueAt time.Time, steps []PlanStep, now time.Time) error {
	fields := map[string]string{}
	if strings.TrimSpace(owner) == "" {
		fields["owner"] = "不能为空"
	}
	if dueAt.IsZero() || !dueAt.After(now) {
		fields["due_at"] = "必须晚于当前时间"
	}
	if len(steps) == 0 {
		fields["steps"] = "至少需要一个处置步骤"
	}
	seen := map[int]bool{}
	for _, step := range steps {
		if step.Order <= 0 || strings.TrimSpace(step.Instruction) == "" || seen[step.Order] {
			fields["steps"] = "步骤序号必须唯一且内容不能为空"
		}
		seen[step.Order] = true
	}
	if len(fields) > 0 {
		return Validation("处置方案无效", fields)
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })
	return nil
}

func ValidatePlanRevision(previous MitigationPlan, owner string, dueAt time.Time, steps []PlanStep, riskNotes string, now time.Time) error {
	if err := ValidatePlan(owner, dueAt, steps, now); err != nil {
		return err
	}
	changed := strings.TrimSpace(owner) != strings.TrimSpace(previous.Owner) || !dueAt.UTC().Equal(previous.DueAt.UTC()) || strings.TrimSpace(riskNotes) != strings.TrimSpace(previous.RiskNotes)
	if len(steps) != len(previous.Steps) {
		changed = true
	} else {
		for i := range steps {
			if steps[i].Order != previous.Steps[i].Order || strings.TrimSpace(steps[i].Instruction) != strings.TrimSpace(previous.Steps[i].Instruction) {
				changed = true
				break
			}
		}
	}
	if !changed {
		return Validation("处置方案未发生实质变更", map[string]string{"plan": "至少修改责任人、时限、步骤或风险提示"})
	}
	return nil
}

func ValidateAction(operator, description string, performedAt time.Time) error {
	fields := map[string]string{}
	if strings.TrimSpace(operator) == "" {
		fields["operator"] = "不能为空"
	}
	if strings.TrimSpace(description) == "" {
		fields["description"] = "不能为空"
	}
	if performedAt.IsZero() {
		fields["performed_at"] = "不能为空"
	}
	if len(fields) > 0 {
		return Validation("现场动作无效", fields)
	}
	return nil
}

func ValidateVerification(operator, instrument string, measuredAt time.Time, temperature, humidity float64) error {
	fields := map[string]string{}
	if strings.TrimSpace(operator) == "" {
		fields["operator"] = "不能为空"
	}
	if strings.TrimSpace(instrument) == "" {
		fields["instrument_id"] = "不能为空"
	}
	if measuredAt.IsZero() {
		fields["measured_at"] = "不能为空"
	}
	if math.IsNaN(temperature) || math.IsInf(temperature, 0) {
		fields["temperature"] = "必须为有限数值"
	}
	if math.IsNaN(humidity) || math.IsInf(humidity, 0) || humidity < 0 || humidity > 100 {
		fields["humidity"] = "必须位于 0-100"
	}
	if len(fields) > 0 {
		return Validation("复测记录无效", fields)
	}
	return nil
}
