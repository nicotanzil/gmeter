package engine

import (
	"fmt"

	"gmeter/internal/config"
)

type FailurePolicy struct {
	Action       string `json:"action"`
	UsersStopped int64  `json:"users_stopped"`
}

func (r *runner) failurePolicy() *FailurePolicy {
	if !r.plan.HasFailurePolicy() {
		return nil
	}
	return &FailurePolicy{Action: r.plan.ErrorAction(), UsersStopped: r.usersStopped.Load()}
}

func (f FailurePolicy) Validate(c config.Config, failed int64) error {
	if f.Action != c.ErrorAction() || f.UsersStopped < 0 || f.UsersStopped > int64(c.VUs) || f.UsersStopped > failed || (f.Action != config.StopUser && f.UsersStopped != 0) {
		return fmt.Errorf("failure action or stopped-user counts disagree")
	}
	return nil
}

func (r Result) validateFailurePolicy(p config.Plan) error {
	if !p.HasFailurePolicy() {
		if r.FailurePolicy != nil || r.SchemaVersion == 9 {
			return fmt.Errorf("unexpected failure policy")
		}
		return nil
	}
	if r.SchemaVersion != 9 || r.FailurePolicy == nil {
		return fmt.Errorf("failure actions require schema 9")
	}
	failed := r.Failed
	if p.IsJourney() {
		failed = r.Journeys.Failed
	}
	if err := r.FailurePolicy.Validate(p.Config, failed); err != nil {
		return err
	}
	if r.FailurePolicy.UsersStopped > 0 && r.StopReason == "iterations" {
		return fmt.Errorf("users stopped on error before normal iteration completion")
	}
	if r.FailurePolicy.UsersStopped > 0 && r.Passed {
		return fmt.Errorf("users stopped on error cannot pass the run")
	}
	return nil
}
