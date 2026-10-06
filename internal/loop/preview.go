package loop

import "time"

// Preview eligibility times, not promised execution times: idle/quota gates
// and previous flow steps still apply when the loop runs live.
func (r *Runner) previewSchedule(start time.Time) error {
	previewAction := func(action ActionConfig) map[string]interface{} {
		command := action.Command
		if action.Type == "clear" && command == "" {
			command = "/clear"
		}
		return map[string]interface{}{
			"name": action.Name, "type": action.Type, "text": action.Text,
			"command": command, "keys": action.Keys, "enter": actionEnter(action),
			"post_delay": action.PostDelay.Duration.String(),
		}
	}
	emit := func(name string, delay, every time.Duration, idle bool, maxRuns int, steps []map[string]interface{}) error {
		return r.emit(event{
			Timestamp: start.Format(time.RFC3339), Type: "schedule_preview",
			Target: r.cfg.Target, Action: name, DryRun: true, Status: "planned",
			Details: map[string]interface{}{
				"eligible_at": r.previewEligibleAt(start.Add(delay)),
				"every":       every.String(), "only_when_idle": idle, "max_runs": maxRuns,
				"steps": steps, "calendar": r.previewCalendar(),
			},
		})
	}
	for _, action := range r.cfg.Actions {
		if err := emit(action.Name, action.InitialDelay.Duration, action.Every.Duration, action.OnlyWhenIdle, action.MaxRuns, []map[string]interface{}{previewAction(action)}); err != nil {
			return err
		}
	}
	for _, flow := range r.cfg.Flows {
		steps := make([]map[string]interface{}, 0, len(flow.Steps))
		for _, step := range flow.Steps {
			steps = append(steps, previewAction(step))
		}
		if err := emit(flow.Name, flow.InitialDelay.Duration, flow.Every.Duration, flow.OnlyWhenIdle, flow.MaxRuns, steps); err != nil {
			return err
		}
	}
	return nil
}

// previewEligibleAt shifts a first eligible time into the next calendar
// window and renders it in the calendar's timezone when one is configured.
func (r *Runner) previewEligibleAt(at time.Time) string {
	if r.calendar == nil {
		return at.Format(time.RFC3339)
	}
	return r.calendar.NextOpen(at).In(r.calendar.Location()).Format(time.RFC3339)
}

func (r *Runner) previewCalendar() interface{} {
	if r.calendar == nil {
		return nil
	}
	return r.calendar.Describe()
}
