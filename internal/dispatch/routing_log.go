package dispatch

import "github.com/leolin310148/tmact/internal/modelroute"

func logDispatchOutcome(opts Options, report Report, err error) {
	if opts.ProbeAutoModel || opts.RoutingPreflight {
		return
	}
	trace := opts.RoutingTrace
	if trace.DispatchID == "" && report.Selection != nil {
		trace.DispatchID = report.Selection.DispatchID
	}
	if trace.DispatchID == "" {
		return
	}
	trace.Dir = report.Dir
	trace.Session = opts.Session
	trace.Execute = opts.Execute
	status := "planned"
	if opts.Execute {
		status = "dispatched"
	}
	if err != nil {
		status = "failed"
	}
	outcome := modelroute.Outcome{Status: status, Agent: report.Agent, Model: report.Model, Effort: report.Effort, Target: report.Target, Steps: map[string]string{}}
	for _, step := range report.Steps {
		outcome.Steps[step.Name] = step.Status
	}
	if report.Wait != nil && report.Wait.Outcome != nil {
		outcome.WaitReason = report.Wait.Outcome.Reason
	}
	modelroute.Log(modelroute.LogEntry{Event: "dispatch", Trace: trace, Outcome: &outcome, Selection: report.Selection})
}
