package domain

import (
	"fmt"
	"time"
)

var allowedTransitions = map[IncidentState][]IncidentState{
	StateReported:    {StateAssessed},
	StateAssessed:    {StatePlanPending},
	StatePlanPending: {StateApproved, StateAssessed},
	StateApproved:    {StateExecuting},
	StateExecuting:   {StateVerifying},
	StateVerifying:   {StateExecuting, StateClosed},
}

func CanTransition(from, to IncidentState) bool {
	for _, candidate := range allowedTransitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func Transition(incident *EnvironmentIncident, to IncidentState, now time.Time) error {
	if !CanTransition(incident.State, to) {
		return InvalidState(fmt.Sprintf("不能从%s流转到%s", StateLabels[incident.State], StateLabels[to]))
	}
	incident.State = to
	incident.Revision++
	incident.UpdatedAt = now.UTC()
	if to == StateClosed {
		closed := now.UTC()
		incident.ClosedAt = &closed
	}
	return nil
}
