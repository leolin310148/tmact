package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

type Options struct {
	DryRun bool
	Once   bool
}

type Runner struct {
	cfg     Config
	options Options
	now     func() time.Time
}

type ruleState struct {
	config       RuleConfig
	lastAccepted map[string]time.Time
	runs         int
}

type event struct {
	Timestamp string      `json:"ts"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Rule      string      `json:"rule,omitempty"`
	DryRun    bool        `json:"dry_run,omitempty"`
	Status    string      `json:"status,omitempty"`
	Reason    string      `json:"reason,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

func NewRunner(cfg Config, options Options) *Runner {
	return &Runner{
		cfg:     cfg,
		options: options,
		now:     time.Now,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	start := r.now()
	rules := make([]ruleState, 0, len(r.cfg.Rules))
	for _, cfg := range r.cfg.Rules {
		rules = append(rules, ruleState{
			config:       cfg,
			lastAccepted: map[string]time.Time{},
		})
	}

	ticker := time.NewTicker(r.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		now := r.now()
		if r.cfg.MaxRuntime.Duration > 0 && now.Sub(start) >= r.cfg.MaxRuntime.Duration {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_runtime"})
		}

		if err := r.runOnce(now, rules); err != nil {
			return err
		}
		if r.options.Once {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runOnce(now time.Time, rules []ruleState) error {
	raw, err := tmux.CapturePane(r.cfg.Target, r.cfg.CaptureLines)
	if err != nil {
		_ = r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "error", Target: r.cfg.Target, Status: "failed", Reason: err.Error()})
		return err
	}
	detected := prompt.DetectDirectoryAccess(raw)

	if detected == nil {
		return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "state", Target: r.cfg.Target, Status: "no_prompt"})
	}

	for i := range rules {
		if err := r.maybeRunRule(now, detected, &rules[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) maybeRunRule(now time.Time, detected *prompt.DirectoryAccess, rule *ruleState) error {
	if rule.config.MaxRuns > 0 && rule.runs >= rule.config.MaxRuns {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.cfg.Target,
			Rule:      rule.config.Name,
			Status:    "max_runs",
			Details:   promptDetails(detected),
		})
	}

	decision := evaluateDirectoryAccess(rule.config, detected)
	if !decision.Accept {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.cfg.Target,
			Rule:      rule.config.Name,
			Status:    "blocked",
			Reason:    decision.Reason,
			Details:   decision.Details,
		})
	}

	if last, ok := rule.lastAccepted[decision.Signature]; ok && now.Sub(last) < rule.config.Cooldown.Duration {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.cfg.Target,
			Rule:      rule.config.Name,
			Status:    "cooldown",
			Reason:    "recently_accepted",
			Details:   decision.Details,
		})
	}

	err := r.acceptSelected()
	status := "ok"
	reason := decision.Reason
	if err != nil {
		status = "failed"
		reason = err.Error()
	}
	if emitErr := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "prompt_accept",
		Target:    r.cfg.Target,
		Rule:      rule.config.Name,
		DryRun:    r.options.DryRun,
		Status:    status,
		Reason:    reason,
		Details:   decision.Details,
	}); emitErr != nil && err == nil {
		err = emitErr
	}
	if err != nil {
		return err
	}

	rule.lastAccepted[decision.Signature] = now
	rule.runs++
	return nil
}

func (r *Runner) acceptSelected() error {
	if r.options.DryRun {
		return nil
	}
	return tmux.SendKeys(r.cfg.Target, []string{"Enter"})
}

func (r *Runner) emit(e event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	if r.cfg.LogPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.cfg.LogPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(r.cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(append(data, '\n'))
	return err
}
