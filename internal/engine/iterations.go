package engine

import (
	"fmt"

	"gmeter/internal/config"
)

// ValidateIterations checks the stopping contract without trusting imported totals.
func (r Result) ValidateIterations(p config.Plan) error {
	if r.Iterations != p.Iterations || (r.SchemaVersion == 7) != (p.Iterations > 0) {
		return fmt.Errorf("iteration settings or schema disagree")
	}
	if p.Iterations == 0 {
		if r.StopReason != "" {
			return fmt.Errorf("unexpected iteration stop reason")
		}
		return nil
	}
	if r.ConcurrencyLimit != p.VUs {
		return fmt.Errorf("user count disagrees with iteration settings")
	}
	completed := r.Completed
	if p.IsJourney() {
		completed = r.Journeys.Completed
	}
	target := int64(p.VUs) * int64(p.Iterations)
	if completed < 0 || completed > target {
		return fmt.Errorf("completed iterations exceed the per-user limit")
	}
	switch r.StopReason {
	case "iterations":
		if r.Interrupted || completed != target {
			return fmt.Errorf("iteration completion requires every user to finish")
		}
	case "duration":
		if r.Interrupted || p.LoadDuration == 0 || r.LoadSeconds != p.LoadDuration.Seconds() {
			return fmt.Errorf("duration stop requires the configured time limit")
		}
	case "interrupted":
		if !r.Interrupted {
			return fmt.Errorf("interrupted stop requires an interrupted result")
		}
	default:
		return fmt.Errorf("invalid iteration stop reason")
	}
	return nil
}
