package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/peerpane"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

type Options struct {
	DryRun    bool
	Once      bool
	Control   func() (string, error)
	Heartbeat func(string) error
}

var (
	ErrStopRequested = errors.New("loop stop requested")
	errMaxActions    = errors.New("loop max actions reached")
)

const controlPollInterval = 250 * time.Millisecond

type Runner struct {
	cfg                Config
	options            Options
	now                func() time.Time
	capturePane        func(string, int) (string, error)
	sendText           func(string, string, bool) error
	sendKeys           func(string, []string) error
	fetchUsage         func(context.Context, ...string) agentusage.Snapshot
	idleIgnorePatterns []*regexp.Regexp

	// Cached quota snapshot. The provider endpoints are rate-limited, so
	// fetchUsage runs at most once per Quota.RefreshInterval and the last
	// snapshot is reused in between.
	quotaSnap     agentusage.Snapshot
	quotaHave     bool
	quotaFetched  time.Time
	lastPhase     string
	lastHeartbeat time.Time
}

type actionState struct {
	config  ActionConfig
	nextRun time.Time
	runs    int
}

type flowState struct {
	config  FlowConfig
	nextRun time.Time
	runs    int
}

type paneState struct {
	Hash              string                  `json:"hash"`
	Idle              bool                    `json:"idle"`
	IdleFor           string                  `json:"idle_for"`
	InteractivePrompt *prompt.Prompt          `json:"interactive_prompt,omitempty"`
	PermissionPrompt  *prompt.DirectoryAccess `json:"permission_prompt,omitempty"`
}

type event struct {
	Timestamp string      `json:"ts"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Action    string      `json:"action,omitempty"`
	DryRun    bool        `json:"dry_run,omitempty"`
	Status    string      `json:"status,omitempty"`
	Reason    string      `json:"reason,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

func NewRunner(cfg Config, options Options) *Runner {
	compiled := make([]*regexp.Regexp, 0, len(cfg.IdleIgnorePatterns))
	for _, pattern := range cfg.IdleIgnorePatterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return &Runner{
		cfg:                cfg,
		options:            options,
		now:                time.Now,
		capturePane:        tmux.CapturePane,
		sendText:           tmux.PasteText,
		sendKeys:           tmux.SendKeys,
		fetchUsage:         agentusage.Fetch,
		idleIgnorePatterns: compiled,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.configurePeerTarget(ctx); err != nil {
		return err
	}
	if err := r.heartbeat("starting"); err != nil {
		return err
	}
	start := r.now()
	lastChangedAt := start
	if r.cfg.AssumeIdleOnStart {
		lastChangedAt = start.Add(-r.cfg.IdleAfter.Duration)
	}

	actions := make([]actionState, 0, len(r.cfg.Actions))
	for _, cfg := range r.cfg.Actions {
		actions = append(actions, actionState{
			config:  cfg,
			nextRun: start.Add(cfg.InitialDelay.Duration),
		})
	}
	flows := make([]flowState, 0, len(r.cfg.Flows))
	for _, cfg := range r.cfg.Flows {
		flows = append(flows, flowState{
			config:  cfg,
			nextRun: start.Add(cfg.InitialDelay.Duration),
		})
	}

	var lastHash string
	actionCount := 0
	ticker := time.NewTicker(r.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		if err := r.awaitRunnable(ctx); err != nil {
			return err
		}
		now := r.now()
		if r.cfg.MaxRuntime.Duration > 0 && now.Sub(start) >= r.cfg.MaxRuntime.Duration {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_runtime"})
		}

		if err := r.heartbeat("observing"); err != nil {
			return err
		}
		state, changed, err := r.observe(now, lastHash, lastChangedAt)
		if err != nil {
			_ = r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "error", Target: r.cfg.Target, Status: "failed", Reason: err.Error()})
			return err
		}
		if lastHash == "" {
			lastHash = state.Hash
		} else if changed {
			lastHash = state.Hash
			lastChangedAt = now
			state.Idle = false
			state.IdleFor = "0s"
		}

		if state.InteractivePrompt != nil && r.cfg.StopOnPermissionPrompt {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "permission_prompt", Details: state.InteractivePrompt})
		}

		quotaSkip, quotaReason, err := r.evaluateQuota(ctx, now)
		if err != nil {
			return err
		}

		executedThisCycle := 0
		for i := range actions {
			if r.cfg.MaxActions > 0 && actionCount >= r.cfg.MaxActions {
				return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_actions"})
			}

			executed, err := r.maybeRunAction(ctx, now, state, &actions[i], quotaSkip, quotaReason)
			if err != nil {
				return err
			}
			if executed {
				actionCount++
				executedThisCycle++
			}
		}
		for i := range flows {
			if r.cfg.MaxActions > 0 && actionCount >= r.cfg.MaxActions {
				return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_actions"})
			}

			remaining := 0
			if r.cfg.MaxActions > 0 {
				remaining = r.cfg.MaxActions - actionCount
			}
			executed, err := r.maybeRunFlow(ctx, now, state, &flows[i], quotaSkip, quotaReason, remaining)
			if errors.Is(err, errMaxActions) {
				return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_actions"})
			}
			if err != nil {
				return err
			}
			actionCount += executed
			executedThisCycle += executed
		}

		if r.options.Once {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "state", Target: r.cfg.Target, Details: state})
		}
		if schedulesComplete(actions, flows) {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "actions_exhausted"})
		}

		phase := "sleeping"
		if quotaSkip {
			phase = "waiting_quota"
		} else if executedThisCycle == 0 && !state.Idle {
			phase = "waiting_idle"
		}
		if err := r.heartbeat(phase); err != nil {
			return err
		}

		if err := r.waitForTick(ctx, ticker.C); err != nil {
			return err
		}
	}
}

func schedulesComplete(actions []actionState, flows []flowState) bool {
	for _, action := range actions {
		if action.config.MaxRuns == 0 || action.runs < action.config.MaxRuns {
			return false
		}
	}
	for _, flow := range flows {
		if flow.config.MaxRuns == 0 || flow.runs < flow.config.MaxRuns {
			return false
		}
	}
	return true
}

func (r *Runner) observe(now time.Time, previousHash string, lastChangedAt time.Time) (paneState, bool, error) {
	raw, err := r.capturePane(r.cfg.Target, r.cfg.CaptureLines)
	if err != nil {
		return paneState{}, false, err
	}

	hash := hashText(r.idleText(raw))
	idleFor := now.Sub(lastChangedAt)
	detected := prompt.Detect(raw)
	state := paneState{
		Hash:              hash,
		Idle:              idleFor >= r.cfg.IdleAfter.Duration,
		IdleFor:           idleFor.Truncate(time.Second).String(),
		InteractivePrompt: detected,
	}
	if detected != nil && detected.Type == prompt.TypeDirectoryAccess {
		state.PermissionPrompt = prompt.DirectoryAccessFromPrompt(detected)
	}
	return state, previousHash != "" && hash != previousHash, nil
}

func (r *Runner) idleText(raw string) string {
	if len(r.idleIgnorePatterns) == 0 {
		return raw
	}

	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		ignore := false
		for _, pattern := range r.idleIgnorePatterns {
			if pattern.MatchString(line) {
				ignore = true
				break
			}
		}
		if !ignore {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func (r *Runner) maybeRunAction(ctx context.Context, now time.Time, state paneState, action *actionState, quotaSkip bool, quotaReason string) (bool, error) {
	if now.Before(action.nextRun) {
		return false, nil
	}
	if action.config.MaxRuns > 0 && action.runs >= action.config.MaxRuns {
		return false, nil
	}
	if quotaSkip {
		if r.cfg.LogSkippedActions {
			return false, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    action.config.Name,
				Status:    quotaReason,
			})
		}
		return false, nil
	}
	if action.config.OnlyWhenIdle && !state.Idle {
		if r.cfg.LogSkippedActions {
			return false, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    action.config.Name,
				Status:    "not_idle",
				Details:   state,
			})
		}
		return false, nil
	}

	if err := r.awaitRunnable(ctx); err != nil {
		return false, err
	}
	if err := r.runAction(ctx, now, action.config.Name, action.config); err != nil {
		return false, err
	}

	action.runs++
	if action.config.Every.Duration > 0 {
		action.nextRun = now.Add(action.config.Every.Duration)
	} else {
		action.nextRun = time.Time{}
		action.config.MaxRuns = action.runs
	}
	return true, nil
}

func (r *Runner) maybeRunFlow(ctx context.Context, now time.Time, state paneState, flow *flowState, quotaSkip bool, quotaReason string, remaining int) (int, error) {
	if now.Before(flow.nextRun) {
		return 0, nil
	}
	if flow.config.MaxRuns > 0 && flow.runs >= flow.config.MaxRuns {
		return 0, nil
	}
	if quotaSkip {
		if r.cfg.LogSkippedActions {
			return 0, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    flow.config.Name,
				Status:    quotaReason,
			})
		}
		return 0, nil
	}
	if flow.config.OnlyWhenIdle && !state.Idle {
		if r.cfg.LogSkippedActions {
			return 0, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    flow.config.Name,
				Status:    "not_idle",
				Details:   state,
			})
		}
		return 0, nil
	}
	if remaining > 0 && len(flow.config.Steps) > remaining {
		return 0, errMaxActions
	}

	if err := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "flow",
		Target:    r.cfg.Target,
		Action:    flow.config.Name,
		DryRun:    r.options.DryRun,
		Status:    "start",
		Details: map[string]interface{}{
			"steps": len(flow.config.Steps),
		},
	}); err != nil {
		return 0, err
	}

	executed := 0
	for _, step := range flow.config.Steps {
		if err := r.awaitRunnable(ctx); err != nil {
			return executed, err
		}
		stepName := flow.config.Name + "." + step.Name
		if err := r.runAction(ctx, now, stepName, step); err != nil {
			return executed, err
		}
		executed++
	}

	if err := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "flow",
		Target:    r.cfg.Target,
		Action:    flow.config.Name,
		DryRun:    r.options.DryRun,
		Status:    "ok",
		Details: map[string]interface{}{
			"steps": executed,
		},
	}); err != nil {
		return executed, err
	}

	flow.runs++
	if flow.config.Every.Duration > 0 {
		flow.nextRun = now.Add(flow.config.Every.Duration)
	} else {
		flow.nextRun = time.Time{}
		flow.config.MaxRuns = flow.runs
	}
	return executed, nil
}

// evaluateQuota decides whether this cycle should be skipped because the target
// agent's provider usage is too high. It refreshes the cached snapshot at most
// once per Quota.RefreshInterval (the endpoints are rate-limited) and reuses the
// last reading in between. When quota can't be determined (missing/expired
// token, provider error, stale reading, or no windows) it fails open — runs
// anyway and logs once per refresh — unless FailClosed is set. Returns
// (skip, reason); reason is "quota_weekly", "quota_weekly_no_headroom",
// "quota_session_low", or (fail-closed only) "quota_unavailable".
func (r *Runner) evaluateQuota(ctx context.Context, now time.Time) (bool, string, error) {
	q := r.cfg.Quota
	if q == nil || !q.Enabled {
		return false, "", nil
	}

	interval := q.RefreshInterval.Duration
	if interval <= 0 {
		interval = defaultQuotaRefreshInterval
	}
	fetched := false
	if !r.quotaHave || now.Sub(r.quotaFetched) >= interval {
		r.quotaSnap = r.fetchUsage(ctx, q.Provider)
		r.quotaHave = true
		r.quotaFetched = now
		fetched = true
	}

	pu, ok := providerUsage(r.quotaSnap, q.Provider)
	if !ok || pu.Error != "" || pu.Stale || len(pu.Windows) == 0 {
		reason := "no usage data"
		if !ok {
			reason = "provider not found in snapshot"
		} else if pu.Error != "" {
			reason = pu.Error
		} else if pu.Stale {
			reason = "stale reading"
		}
		return r.quotaUnavailable(now, fetched, reason)
	}

	weeklyAt := q.WeeklySkipAtPercent
	if weeklyAt <= 0 {
		weeklyAt = defaultWeeklySkipAtPercent
	}
	minRemaining := q.SessionMinRemainingPercent
	if minRemaining <= 0 {
		minRemaining = defaultSessionMinRemainingPercent
	}

	var weekly, session *agentusage.RateWindow
	for i := range pu.Windows {
		switch pu.Windows[i].Name {
		case "weekly":
			weekly = &pu.Windows[i]
		case "session":
			session = &pu.Windows[i]
		}
	}

	if weekly == nil {
		return r.quotaUnavailable(now, fetched, "weekly window missing")
	}
	if session == nil {
		return r.quotaUnavailable(now, fetched, "session window missing")
	}

	// Weekly exhaustion is the more severe condition, so it wins the reason.
	if weekly.UsedPercent >= weeklyAt {
		return true, "quota_weekly", nil
	}
	if q.WeeklyRequireHeadroom {
		if weekly.Pace == nil {
			return r.quotaUnavailable(now, fetched, "weekly pace unavailable")
		}
		// Positive headroom means expected usage is ahead of actual usage: the
		// account has conserved more weekly quota than a linear burn schedule.
		headroom := weekly.Pace.ExpectedPercent - weekly.Pace.ActualPercent
		if headroom <= 0 {
			return true, "quota_weekly_no_headroom", nil
		}
	}
	if 100-session.UsedPercent <= minRemaining {
		return true, "quota_session_low", nil
	}
	return false, "", nil
}

func (r *Runner) quotaUnavailable(now time.Time, fetched bool, reason string) (bool, string, error) {
	q := r.cfg.Quota
	if fetched {
		// Log the fail-open/fail-closed decision once per refresh so missing
		// credentials or pace inputs are visible rather than silently changing
		// loop behavior.
		policy := "fail_open_run"
		if q.FailClosed {
			policy = "fail_closed_skip"
		}
		if err := r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "quota",
			Target:    r.cfg.Target,
			Status:    "unavailable",
			Reason:    reason,
			Details:   map[string]interface{}{"provider": q.Provider, "policy": policy},
		}); err != nil {
			return false, "", err
		}
	}
	if q.FailClosed {
		return true, "quota_unavailable", nil
	}
	return false, "", nil
}

// providerUsage returns the ProviderUsage for name from a snapshot.
func providerUsage(snap agentusage.Snapshot, name string) (agentusage.ProviderUsage, bool) {
	for _, p := range snap.Providers {
		if p.Provider == name {
			return p, true
		}
	}
	return agentusage.ProviderUsage{}, false
}

func (r *Runner) runAction(ctx context.Context, now time.Time, name string, action ActionConfig) error {
	if err := r.heartbeat("action:" + name); err != nil {
		return err
	}
	err := r.executeAction(action)
	status := "ok"
	reason := ""
	if err != nil {
		status = "failed"
		reason = err.Error()
	}

	if emitErr := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "action",
		Target:    r.cfg.Target,
		Action:    name,
		DryRun:    r.options.DryRun,
		Status:    status,
		Reason:    reason,
		Details: map[string]interface{}{
			"type":       action.Type,
			"post_delay": action.PostDelay.Duration.String(),
		},
	}); emitErr != nil && err == nil {
		err = emitErr
	}
	if err != nil {
		return err
	}

	if action.PostDelay.Duration > 0 && !r.options.DryRun {
		if err := r.waitDuration(ctx, action.PostDelay.Duration); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) heartbeat(phase string) error {
	if r.options.Heartbeat == nil {
		return nil
	}
	now := r.now()
	if phase == r.lastPhase && now.Sub(r.lastHeartbeat) < 2*time.Second {
		return nil
	}
	if err := r.options.Heartbeat(phase); err != nil {
		return err
	}
	r.lastPhase = phase
	r.lastHeartbeat = now
	return nil
}

func (r *Runner) desiredState() (string, error) {
	if r.options.Control == nil {
		return "running", nil
	}
	state, err := r.options.Control()
	if err != nil {
		return "", err
	}
	if state == "" {
		state = "running"
	}
	return state, nil
}

func (r *Runner) awaitRunnable(ctx context.Context) error {
	for {
		state, err := r.desiredState()
		if err != nil {
			return err
		}
		switch state {
		case "running":
			if r.lastPhase == "paused" {
				if err := r.heartbeat("resuming"); err != nil {
					return err
				}
			}
			return nil
		case "stopped":
			if err := r.emit(event{
				Timestamp: r.now().Format(time.RFC3339),
				Type:      "stop",
				Target:    r.cfg.Target,
				Reason:    "requested",
			}); err != nil {
				return err
			}
			return ErrStopRequested
		case "paused":
			if err := r.heartbeat("paused"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported loop desired state %q", state)
		}
		timer := time.NewTimer(controlPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *Runner) waitDuration(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	return r.waitControlled(ctx, timer.C)
}

func (r *Runner) waitForTick(ctx context.Context, tick <-chan time.Time) error {
	return r.waitControlled(ctx, tick)
}

func (r *Runner) waitControlled(ctx context.Context, done <-chan time.Time) error {
	controlTicker := time.NewTicker(controlPollInterval)
	defer controlTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		case <-controlTicker.C:
			if err := r.awaitRunnable(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *Runner) executeAction(action ActionConfig) error {
	if r.options.DryRun {
		return nil
	}

	switch action.Type {
	case "send_text":
		return r.sendText(r.cfg.Target, action.Text, actionEnter(action))
	case "send_keys":
		return r.sendKeys(r.cfg.Target, action.Keys)
	case "clear":
		command := action.Command
		if command == "" {
			command = "/clear"
		}
		return r.sendText(r.cfg.Target, command, actionEnter(action))
	default:
		return fmt.Errorf("unsupported action type %q", action.Type)
	}
}

func (r *Runner) configurePeerTarget(ctx context.Context) error {
	peerName, remoteTarget := statusd.SplitPeerTarget(r.cfg.Target)
	if peerName == "" {
		return nil
	}
	if remoteTarget == "" || !strings.HasPrefix(remoteTarget, "%") {
		return fmt.Errorf("peer target must be a tmux pane id like peer@%%12, got %q", r.cfg.Target)
	}
	configPath := r.cfg.StatusdConfig
	if configPath == "" {
		configPath = statusd.DefaultFileConfigPath()
	}
	peer, err := peerpane.LoadConfigPeer(configPath, peerName)
	if err != nil {
		return err
	}
	client := peerpane.Client{Peer: peer, HTTPClient: &http.Client{}}
	r.capturePane = func(_ string, lines int) (string, error) {
		return client.Capture(ctx, remoteTarget, lines)
	}
	r.sendText = func(_ string, text string, enter bool) error {
		return client.SendText(ctx, remoteTarget, text, enter)
	}
	r.sendKeys = func(_ string, keys []string) error {
		return client.SendKeys(ctx, remoteTarget, keys)
	}
	return nil
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

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
