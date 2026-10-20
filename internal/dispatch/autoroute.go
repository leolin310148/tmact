package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/modelroute"
)

// normalizeAutoConstraints validates explicit overrides before any quota or
// decision request. An explicit model can also identify its launcher.
func normalizeAutoConstraints(opts Options) (Options, error) {
	if opts.RestartAgent && (opts.NoClear || opts.Queue) {
		return opts, fmt.Errorf("agent replacement cannot preserve an existing conversation")
	}
	if !opts.AutoModel && !opts.ProbeAutoModel {
		return opts, nil
	}
	if opts.ProbeAutoModel && opts.Execute {
		return opts, fmt.Errorf("auto-model probe must be a dry-run")
	}
	if opts.Agent != "" && opts.Agent != "claude" && opts.Agent != "codex" {
		return opts, fmt.Errorf("auto-model only supports claude or codex")
	}
	if opts.Model != "" && opts.Agent == "" {
		for _, a := range []string{"claude", "codex"} {
			if _, err := ValidateModel(a, opts.Model); err == nil {
				opts.Agent = a
				break
			}
		}
		if opts.Agent == "" {
			return opts, fmt.Errorf("auto-model: unsupported explicit model")
		}
	}
	if opts.Effort != "" {
		a := opts.Agent
		if a == "" {
			a = "codex"
		}
		if err := ValidateEffort(a, opts.Effort); err != nil {
			return opts, err
		}
	}
	return opts, nil
}

func prepareAutoLaunch(opts Options, deps Deps, report Report) (Options, Report, error) {
	if !opts.AutoModel && !opts.ProbeAutoModel {
		return opts, report, nil
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	fetch := deps.FetchUsage
	if fetch == nil {
		fetch = DefaultDeps().FetchUsage
	}
	task := opts.RoutingTask
	if task == "" {
		task = opts.Prompt
	}
	in, err := modelroute.Prepare(task, opts.Agent, opts.Model, opts.Effort, fetch(ctx), time.Now())
	in.Trace = opts.RoutingTrace
	in.Trace.Session = opts.Session
	dir := opts.RoutingDir
	if dir == "" {
		dir = opts.Dir
	}
	in.Trace.Dir = dir
	report.Selection = &modelroute.Selection{Status: "unavailable", Quotas: in.Quotas}
	if err != nil {
		if !opts.ProbeAutoModel && in.Trace.DispatchID != "" {
			modelroute.Log(modelroute.LogEntry{Event: "routing-unavailable", Trace: in.Trace, PolicyVersion: modelroute.PolicyVersion, Input: &in, Selection: report.Selection, Error: err.Error()})
		}
		return opts, report, err
	}
	modelroute.AttachPromptFiles(&in, dir)
	if opts.ProbeAutoModel {
		report.AutoModelSupported = true
		report.RoutingInput = &in
		return opts, report, nil
	}
	choose := deps.ChooseModel
	if choose == nil {
		choose = modelroute.Choose
	}
	selection, err := choose(ctx, in)
	if err != nil {
		return opts, report, err
	}
	if err := validateSelection(in, selection); err != nil {
		return opts, report, err
	}
	opts.Agent = selection.Agent
	opts.Model = selection.Model
	opts.Effort = selection.Effort
	report.Agent = opts.Agent
	report.Model = opts.Model
	report.Effort = opts.Effort
	report.Selection = selection
	return opts, report, nil
}

func validateSelection(in modelroute.Input, s *modelroute.Selection) error {
	if s == nil {
		return fmt.Errorf("auto-model: missing selection")
	}
	for _, c := range in.Candidates {
		if c.Agent == s.Agent && c.Model == s.Model && modelroute.SupportsEffort(c, s.Effort) {
			if _, err := ValidateModel(c.Agent, c.Model); err != nil {
				return err
			}
			return ValidateModelEffort(c.Agent, c.Model, s.Effort)
		}
	}
	return fmt.Errorf("auto-model: selected profile is outside eligible candidates")
}

func checkPlanFiles(opts Options, report *Report, dir string) error {
	if !opts.CapturePlan && opts.PlanFiles == nil {
		return nil
	}
	files, err := modelroute.FingerprintPromptFiles(opts.Prompt, dir)
	if err != nil {
		return err
	}
	report.PlanFiles = files
	if opts.PlanFiles != nil {
		a, _ := json.Marshal(files)
		b, _ := json.Marshal(*opts.PlanFiles)
		if string(a) != string(b) {
			return fmt.Errorf("plan is stale: referenced files changed; regenerate it")
		}
	}
	return nil
}
