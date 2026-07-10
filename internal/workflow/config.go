// Package workflow implements the generic, revision-aware workflow v2 engine.
package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

const Version = 2

var idPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", node.Value, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

type Config struct {
	Version      int                       `yaml:"version" json:"version"`
	Workspace    WorkspaceConfig           `yaml:"workspace" json:"workspace"`
	AgentsConfig string                    `yaml:"agents_config,omitempty" json:"agents_config,omitempty"`
	Variables    map[string]VariableConfig `yaml:"variables,omitempty" json:"variables,omitempty"`
	Actors       map[string]ActorConfig    `yaml:"actors,omitempty" json:"actors,omitempty"`
	Revisions    map[string]RevisionConfig `yaml:"revisions,omitempty" json:"revisions,omitempty"`
	Defaults     DefaultsConfig            `yaml:"defaults,omitempty" json:"defaults"`
	Stages       []StageConfig             `yaml:"stages" json:"stages"`
	ConfigPath   string                    `yaml:"-" json:"-"`
}

type WorkspaceConfig struct {
	Root string `yaml:"root" json:"root"`
}

type VariableConfig struct {
	Type     string `yaml:"type" json:"type"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
	Default  any    `yaml:"default,omitempty" json:"default,omitempty"`
	Enum     []any  `yaml:"enum,omitempty" json:"enum,omitempty"`
}

type ActorConfig struct {
	Agent  string        `yaml:"agent,omitempty" json:"agent,omitempty"`
	Launch *LaunchConfig `yaml:"launch,omitempty" json:"launch,omitempty"`
}

type LaunchConfig struct {
	Runtime     string `yaml:"runtime" json:"runtime"`
	Session     string `yaml:"session" json:"session"`
	Dir         string `yaml:"dir,omitempty" json:"dir,omitempty"`
	TrustFolder bool   `yaml:"trust_folder,omitempty" json:"trust_folder,omitempty"`
	Reuse       *bool  `yaml:"reuse,omitempty" json:"reuse,omitempty"`
	OnFinish    string `yaml:"on_finish,omitempty" json:"on_finish,omitempty"`
}

type RevisionConfig struct {
	Files *FilesRevisionConfig `yaml:"files,omitempty" json:"files,omitempty"`
	Git   *GitRevisionConfig   `yaml:"git,omitempty" json:"git,omitempty"`
}

type FilesRevisionConfig struct {
	Paths []string `yaml:"paths" json:"paths"`
}
type GitRevisionConfig struct {
	Dir string `yaml:"dir,omitempty" json:"dir,omitempty"`
}

type DefaultsConfig struct {
	MaxParallel  int         `yaml:"max_parallel,omitempty" json:"max_parallel"`
	Timeout      Duration    `yaml:"timeout,omitempty" json:"timeout"`
	Retry        RetryConfig `yaml:"retry,omitempty" json:"retry"`
	PollInterval Duration    `yaml:"poll_interval,omitempty" json:"poll_interval"`
	IdleAfter    Duration    `yaml:"idle_after,omitempty" json:"idle_after"`
}

type RetryConfig struct {
	MaxAttempts int      `yaml:"max_attempts,omitempty" json:"max_attempts"`
	Backoff     Duration `yaml:"backoff,omitempty" json:"backoff"`
}

type StageConfig struct {
	ID                string      `yaml:"id" json:"id"`
	Type              string      `yaml:"type" json:"type"`
	Needs             []string    `yaml:"needs,omitempty" json:"needs,omitempty"`
	When              *Condition  `yaml:"when,omitempty" json:"when,omitempty"`
	BindRevisions     []string    `yaml:"bind_revisions,omitempty" json:"bind_revisions,omitempty"`
	ProducesRevisions []string    `yaml:"produces_revisions,omitempty" json:"produces_revisions,omitempty"`
	Retry             RetryConfig `yaml:"retry,omitempty" json:"retry"`
	Timeout           Duration    `yaml:"timeout,omitempty" json:"timeout"`

	Actor    string            `yaml:"actor,omitempty" json:"actor,omitempty"`
	Prompt   string            `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Outcomes map[string]string `yaml:"outcomes,omitempty" json:"outcomes,omitempty"`

	Argv             []string          `yaml:"argv,omitempty" json:"argv,omitempty"`
	Cwd              string            `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Env              map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	InheritEnv       []string          `yaml:"inherit_env,omitempty" json:"inherit_env,omitempty"`
	SuccessExitCodes []int             `yaml:"success_exit_codes,omitempty" json:"success_exit_codes,omitempty"`

	Condition *Condition       `yaml:"condition,omitempty" json:"condition,omitempty"`
	Input     map[string]Input `yaml:"input,omitempty" json:"input,omitempty"`
}

type Input struct {
	Type     string `yaml:"type" json:"type"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

type Condition struct {
	All      []Condition        `yaml:"all,omitempty" json:"all,omitempty"`
	Any      []Condition        `yaml:"any,omitempty" json:"any,omitempty"`
	Not      *Condition         `yaml:"not,omitempty" json:"not,omitempty"`
	Stage    *StageCondition    `yaml:"stage,omitempty" json:"stage,omitempty"`
	Revision *RevisionCondition `yaml:"revision,omitempty" json:"revision,omitempty"`
	Evidence *EvidenceCondition `yaml:"evidence,omitempty" json:"evidence,omitempty"`
	Variable *VariableCondition `yaml:"variable,omitempty" json:"variable,omitempty"`
}

type StageCondition struct {
	ID      string `yaml:"id" json:"id"`
	Status  string `yaml:"status,omitempty" json:"status,omitempty"`
	Outcome string `yaml:"outcome,omitempty" json:"outcome,omitempty"`
}
type RevisionCondition struct {
	Name   string `yaml:"name" json:"name"`
	Equals string `yaml:"equals,omitempty" json:"equals,omitempty"`
	Stage  string `yaml:"stage,omitempty" json:"stage,omitempty"`
}
type EvidenceCondition struct {
	Stage  string `yaml:"stage" json:"stage"`
	Result string `yaml:"result" json:"result"`
}
type VariableCondition struct {
	Name  string `yaml:"name" json:"name"`
	Op    string `yaml:"op,omitempty" json:"op,omitempty"`
	Value any    `yaml:"value" json:"value"`
}

type Loaded struct {
	Config    Config
	Variables map[string]any
	Raw       []byte
	Hash      string
}

func Load(path string, supplied map[string]string) (Loaded, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return Loaded{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Loaded{}, fmt.Errorf("decode workflow config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Loaded{}, errors.New("workflow config must contain exactly one YAML document")
		}
		return Loaded{}, fmt.Errorf("decode trailing YAML: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Loaded{}, err
	}
	cfg.ConfigPath = abs
	applyDefaults(&cfg)
	vars, err := resolveVariables(cfg.Variables, supplied)
	if err != nil {
		return Loaded{}, err
	}
	if err := canonicalizeWorkspace(&cfg); err != nil {
		return Loaded{}, err
	}
	if err := Validate(cfg, vars); err != nil {
		return Loaded{}, err
	}
	hashBytes, _ := json.Marshal(struct {
		Config    Config         `json:"config"`
		Variables map[string]any `json:"variables"`
	}{cfg, vars})
	sum := sha256Bytes(hashBytes)
	return Loaded{Config: cfg, Variables: vars, Raw: raw, Hash: sum}, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Workspace.Root == "" {
		cfg.Workspace.Root = "."
	}
	if cfg.Defaults.MaxParallel == 0 {
		cfg.Defaults.MaxParallel = 1
	}
	if cfg.Defaults.Timeout.Duration == 0 {
		cfg.Defaults.Timeout.Duration = 30 * time.Minute
	}
	if cfg.Defaults.Retry.MaxAttempts == 0 {
		cfg.Defaults.Retry.MaxAttempts = 1
	}
	if cfg.Defaults.PollInterval.Duration == 0 {
		cfg.Defaults.PollInterval.Duration = time.Second
	}
	if cfg.Defaults.IdleAfter.Duration == 0 {
		cfg.Defaults.IdleAfter.Duration = 2 * time.Second
	}
	for i := range cfg.Stages {
		if cfg.Stages[i].Retry.MaxAttempts == 0 {
			cfg.Stages[i].Retry = cfg.Defaults.Retry
		}
		if cfg.Stages[i].Timeout.Duration == 0 {
			cfg.Stages[i].Timeout = cfg.Defaults.Timeout
		}
		if cfg.Stages[i].Type == "command" && len(cfg.Stages[i].SuccessExitCodes) == 0 {
			cfg.Stages[i].SuccessExitCodes = []int{0}
		}
	}
	for name, actor := range cfg.Actors {
		if actor.Launch != nil {
			if actor.Launch.Reuse == nil {
				b := true
				actor.Launch.Reuse = &b
			}
			if actor.Launch.OnFinish == "" {
				actor.Launch.OnFinish = "keep"
			}
			cfg.Actors[name] = actor
		}
	}
}

func canonicalizeWorkspace(cfg *Config) error {
	root := cfg.Workspace.Root
	if !filepath.IsAbs(root) {
		root = filepath.Join(filepath.Dir(cfg.ConfigPath), root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("workspace.root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace.root %s is not a directory", resolved)
	}
	cfg.Workspace.Root = filepath.Clean(resolved)
	return nil
}

func resolveVariables(defs map[string]VariableConfig, supplied map[string]string) (map[string]any, error) {
	for key := range supplied {
		if _, ok := defs[key]; !ok {
			return nil, fmt.Errorf("unknown variable %q", key)
		}
	}
	out := map[string]any{}
	keys := make([]string, 0, len(defs))
	for k := range defs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def := defs[key]
		if def.Type == "" {
			def.Type = "string"
		}
		raw, provided := supplied[key]
		var value any
		var err error
		if provided {
			value, err = parseScalar(def.Type, raw)
		} else if def.Default != nil {
			value, err = coerceScalar(def.Type, def.Default)
		} else if def.Required {
			return nil, fmt.Errorf("variable %q is required (use --var %s=value)", key, key)
		} else {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", key, err)
		}
		if len(def.Enum) > 0 {
			found := false
			for _, candidate := range def.Enum {
				cv, e := coerceScalar(def.Type, candidate)
				if e == nil && fmt.Sprint(cv) == fmt.Sprint(value) {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("variable %q value %v is not in enum", key, value)
			}
		}
		out[key] = value
	}
	return out, nil
}

func parseScalar(kind, raw string) (any, error) {
	switch kind {
	case "string":
		return raw, nil
	case "bool":
		return strconv.ParseBool(raw)
	case "int":
		return strconv.ParseInt(raw, 10, 64)
	case "float":
		return strconv.ParseFloat(raw, 64)
	default:
		return nil, fmt.Errorf("unsupported scalar type %q", kind)
	}
}
func coerceScalar(kind string, v any) (any, error) {
	switch x := v.(type) {
	case string:
		return parseScalar(kind, x)
	case int:
		return parseScalar(kind, strconv.Itoa(x))
	case int64:
		return parseScalar(kind, strconv.FormatInt(x, 10))
	case float64:
		return parseScalar(kind, strconv.FormatFloat(x, 'g', -1, 64))
	case bool:
		return parseScalar(kind, strconv.FormatBool(x))
	default:
		return nil, fmt.Errorf("value must be a scalar")
	}
}

func ParseAssignments(values []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range values {
		key, value, ok := strings.Cut(v, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid assignment %q; want key=value", v)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate assignment %q", key)
		}
		out[key] = value
	}
	return out, nil
}

func rejectDuplicateKeys(raw []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(raw, &node); err != nil {
		return err
	}
	var walk func(*yaml.Node) error
	walk = func(n *yaml.Node) error {
		if n.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				k := n.Content[i].Value
				if seen[k] {
					return fmt.Errorf("duplicate YAML key %q at line %d", k, n.Content[i].Line)
				}
				seen[k] = true
			}
		}
		for _, c := range n.Content {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(&node)
}

func Validate(cfg Config, vars map[string]any) error {
	if cfg.Version != Version {
		return fmt.Errorf("version must be %d", Version)
	}
	if cfg.Defaults.MaxParallel < 1 {
		return errors.New("defaults.max_parallel must be positive")
	}
	if cfg.Defaults.Timeout.Duration <= 0 {
		return errors.New("defaults.timeout must be positive")
	}
	if cfg.Defaults.PollInterval.Duration <= 0 {
		return errors.New("defaults.poll_interval must be positive")
	}
	if cfg.Defaults.IdleAfter.Duration < 0 {
		return errors.New("defaults.idle_after cannot be negative")
	}
	if cfg.Defaults.Retry.MaxAttempts < 1 {
		return errors.New("defaults.retry.max_attempts must be positive")
	}
	if cfg.Defaults.Retry.Backoff.Duration < 0 {
		return errors.New("defaults.retry.backoff cannot be negative")
	}
	if len(cfg.Stages) == 0 {
		return errors.New("stages must not be empty")
	}
	for name, def := range cfg.Variables {
		if !idPattern.MatchString(name) {
			return fmt.Errorf("invalid variable id %q", name)
		}
		kind := def.Type
		if kind == "" {
			kind = "string"
		}
		if !contains([]string{"string", "bool", "int", "float"}, kind) {
			return fmt.Errorf("variable %q has unsupported type %q", name, kind)
		}
	}
	for name, actor := range cfg.Actors {
		if !idPattern.MatchString(name) {
			return fmt.Errorf("invalid actor id %q", name)
		}
		choices := 0
		if actor.Agent != "" {
			choices++
		}
		if actor.Launch != nil {
			choices++
		}
		if choices != 1 {
			return fmt.Errorf("actor %q must set exactly one of agent or launch", name)
		}
		if actor.Launch != nil {
			if actor.Launch.Runtime == "" || actor.Launch.Session == "" {
				return fmt.Errorf("actor %q launch.runtime and launch.session are required", name)
			}
			if !contains([]string{"claude", "codex", "gemini", "copilot"}, actor.Launch.Runtime) {
				return fmt.Errorf("actor %q has unsupported runtime %q", name, actor.Launch.Runtime)
			}
			if actor.Launch.TrustFolder && !contains([]string{"claude", "codex"}, actor.Launch.Runtime) {
				return fmt.Errorf("actor %q trust_folder only supports claude or codex", name)
			}
			if !contains([]string{"keep", "stop"}, actor.Launch.OnFinish) {
				return fmt.Errorf("actor %q launch.on_finish must be keep or stop", name)
			}
			launchDir, err := safeWorkspacePath(cfg.Workspace.Root, actor.Launch.Dir)
			if err != nil {
				return fmt.Errorf("actor %q launch.dir: %w", name, err)
			}
			if filepath.Clean(launchDir) != filepath.Clean(cfg.Workspace.Root) {
				return fmt.Errorf("actor %q launch.dir must equal workspace.root", name)
			}
		}
	}
	for name, rev := range cfg.Revisions {
		if !idPattern.MatchString(name) {
			return fmt.Errorf("invalid revision id %q", name)
		}
		choices := 0
		if rev.Files != nil {
			choices++
			if len(rev.Files.Paths) == 0 {
				return fmt.Errorf("revision %q files.paths is required", name)
			}
			for i, path := range rev.Files.Paths {
				if _, err := template.New(fmt.Sprintf("revision-%s-%d", name, i)).Funcs(safeTemplateFuncs).Parse(path); err != nil {
					return fmt.Errorf("revision %q template: %w", name, err)
				}
			}
		}
		if rev.Git != nil {
			choices++
		}
		if choices != 1 {
			return fmt.Errorf("revision %q must set exactly one of files or git", name)
		}
		if rev.Git != nil {
			if _, err := template.New("revision-" + name).Funcs(safeTemplateFuncs).Parse(rev.Git.Dir); err != nil {
				return fmt.Errorf("revision %q template: %w", name, err)
			}
		}
	}
	ids := map[string]bool{}
	stageByID := map[string]StageConfig{}
	for i, s := range cfg.Stages {
		if !idPattern.MatchString(s.ID) {
			return fmt.Errorf("stage %d has invalid id %q", i+1, s.ID)
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate stage id %q", s.ID)
		}
		ids[s.ID] = true
		stageByID[s.ID] = s
	}
	for _, s := range cfg.Stages {
		for _, need := range s.Needs {
			if !ids[need] {
				return fmt.Errorf("stage %q needs unknown stage %q", s.ID, need)
			}
		}
		for _, name := range append(append([]string{}, s.BindRevisions...), s.ProducesRevisions...) {
			if _, ok := cfg.Revisions[name]; !ok {
				return fmt.Errorf("stage %q references unknown revision %q", s.ID, name)
			}
		}
		if s.Retry.MaxAttempts < 1 {
			return fmt.Errorf("stage %q retry.max_attempts must be positive", s.ID)
		}
		if s.Retry.Backoff.Duration < 0 {
			return fmt.Errorf("stage %q retry.backoff cannot be negative", s.ID)
		}
		if s.Timeout.Duration <= 0 {
			return fmt.Errorf("stage %q timeout must be positive", s.ID)
		}
		if err := validateStage(cfg, s); err != nil {
			return err
		}
		if err := validateStageTemplates(s); err != nil {
			return err
		}
		if err := validateCondition(s.When, cfg, ids); err != nil {
			return fmt.Errorf("stage %q when: %w", s.ID, err)
		}
		if err := validateCondition(s.Condition, cfg, ids); err != nil {
			return fmt.Errorf("stage %q condition: %w", s.ID, err)
		}
	}
	if err := detectCycle(cfg.Stages); err != nil {
		return err
	}
	_ = vars
	_ = stageByID
	return nil
}

func validateStage(cfg Config, s StageConfig) error {
	switch s.Type {
	case "agent":
		if _, ok := cfg.Actors[s.Actor]; !ok {
			return fmt.Errorf("stage %q references unknown actor %q", s.ID, s.Actor)
		}
		if strings.TrimSpace(s.Prompt) == "" {
			return fmt.Errorf("stage %q prompt is required", s.ID)
		}
		if len(s.Outcomes) == 0 {
			return fmt.Errorf("stage %q outcomes are required", s.ID)
		}
		if len(s.Argv) > 0 || s.Condition != nil || len(s.Input) > 0 {
			return fmt.Errorf("stage %q contains fields for a different stage type", s.ID)
		}
	case "command":
		if len(s.Argv) == 0 || strings.TrimSpace(s.Argv[0]) == "" {
			return fmt.Errorf("stage %q argv is required", s.ID)
		}
		if strings.ContainsAny(s.Argv[0], "|;&><\n") || contains([]string{"sh", "bash", "zsh", "fish", "dash", "ksh", "cmd", "powershell", "pwsh"}, filepath.Base(s.Argv[0])) {
			return fmt.Errorf("stage %q argv must not use a shell", s.ID)
		}
		if s.Actor != "" || s.Prompt != "" || s.Condition != nil || len(s.Input) > 0 {
			return fmt.Errorf("stage %q contains fields for a different stage type", s.ID)
		}
		if _, err := safeWorkspacePath(cfg.Workspace.Root, s.Cwd); err != nil {
			return fmt.Errorf("stage %q cwd: %w", s.ID, err)
		}
	case "gate":
		if s.Condition == nil {
			return fmt.Errorf("stage %q condition is required", s.ID)
		}
		if s.Actor != "" || s.Prompt != "" || len(s.Argv) > 0 || len(s.Input) > 0 || len(s.Outcomes) > 0 {
			return fmt.Errorf("stage %q contains fields for a different stage type", s.ID)
		}
	case "human":
		if len(s.Outcomes) == 0 {
			return fmt.Errorf("stage %q outcomes are required", s.ID)
		}
		if s.Actor != "" || s.Prompt != "" || len(s.Argv) > 0 || s.Condition != nil {
			return fmt.Errorf("stage %q contains fields for a different stage type", s.ID)
		}
		for name, input := range s.Input {
			kind := input.Type
			if kind == "" {
				kind = "string"
			}
			if !contains([]string{"string", "bool", "int", "float"}, kind) {
				return fmt.Errorf("stage %q input %q has unsupported type %q", s.ID, name, kind)
			}
		}
	default:
		return fmt.Errorf("stage %q has unsupported type %q", s.ID, s.Type)
	}
	for outcome, disp := range s.Outcomes {
		if outcome == "" || !contains([]string{"success", "retry", "blocked", "failed"}, disp) {
			return fmt.Errorf("stage %q outcome %q has invalid disposition %q", s.ID, outcome, disp)
		}
	}
	return nil
}

func validateStageTemplates(s StageConfig) error {
	values := append([]string{}, s.Argv...)
	values = append(values, s.Cwd, s.Prompt)
	for _, value := range s.Env {
		values = append(values, value)
	}
	for i, value := range values {
		if value == "" {
			continue
		}
		if _, err := template.New(fmt.Sprintf("stage-%s-%d", s.ID, i)).Funcs(safeTemplateFuncs).Parse(value); err != nil {
			return fmt.Errorf("stage %q template: %w", s.ID, err)
		}
	}
	return nil
}

func validateCondition(c *Condition, cfg Config, stages map[string]bool) error {
	if c == nil {
		return nil
	}
	n := 0
	if len(c.All) > 0 {
		n++
	}
	if len(c.Any) > 0 {
		n++
	}
	if c.Not != nil {
		n++
	}
	if c.Stage != nil {
		n++
	}
	if c.Revision != nil {
		n++
	}
	if c.Evidence != nil {
		n++
	}
	if c.Variable != nil {
		n++
	}
	if n != 1 {
		return errors.New("condition must set exactly one of all, any, not, stage, revision, evidence, variable")
	}
	if len(c.All) > 0 {
		for i := range c.All {
			if err := validateCondition(&c.All[i], cfg, stages); err != nil {
				return err
			}
		}
	}
	if len(c.Any) > 0 {
		for i := range c.Any {
			if err := validateCondition(&c.Any[i], cfg, stages); err != nil {
				return err
			}
		}
	}
	if c.Not != nil {
		return validateCondition(c.Not, cfg, stages)
	}
	if c.Stage != nil {
		if !stages[c.Stage.ID] {
			return fmt.Errorf("unknown stage %q", c.Stage.ID)
		}
		if c.Stage.Status == "" && c.Stage.Outcome == "" {
			return errors.New("stage condition needs status or outcome")
		}
		if c.Stage.Status != "" && !contains([]string{StagePending, StageRunnable, StageRunning, StageWaitingReport, StageWaitingHuman, StageSucceeded, StageFailed, StageBlocked, StageStale, StageSkipped}, c.Stage.Status) {
			return fmt.Errorf("invalid stage status %q", c.Stage.Status)
		}
	}
	if c.Revision != nil {
		if _, ok := cfg.Revisions[c.Revision.Name]; !ok {
			return fmt.Errorf("unknown revision %q", c.Revision.Name)
		}
		if c.Revision.Equals == "" && c.Revision.Stage == "" {
			return errors.New("revision condition needs equals or stage")
		}
		if c.Revision.Stage != "" && !stages[c.Revision.Stage] {
			return fmt.Errorf("unknown stage %q", c.Revision.Stage)
		}
	}
	if c.Evidence != nil && !stages[c.Evidence.Stage] {
		return fmt.Errorf("unknown evidence stage %q", c.Evidence.Stage)
	}
	if c.Variable != nil {
		if _, ok := cfg.Variables[c.Variable.Name]; !ok {
			return fmt.Errorf("unknown variable %q", c.Variable.Name)
		}
		if c.Variable.Op == "" {
			c.Variable.Op = "eq"
		}
		if !contains([]string{"eq", "ne", "lt", "lte", "gt", "gte"}, c.Variable.Op) {
			return fmt.Errorf("unsupported variable op %q", c.Variable.Op)
		}
	}
	return nil
}

func detectCycle(stages []StageConfig) error {
	deps := map[string][]string{}
	for _, s := range stages {
		deps[s.ID] = s.Needs
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("workflow DAG contains a cycle at stage %q", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, d := range deps[id] {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for _, s := range stages {
		if err := visit(s.ID); err != nil {
			return err
		}
	}
	return nil
}

func safeWorkspacePath(root, relative string) (string, error) {
	if relative == "" {
		return root, nil
	}
	if filepath.IsAbs(relative) {
		return "", errors.New("must be relative to workspace")
	}
	path := filepath.Clean(filepath.Join(root, relative))
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("escapes workspace")
	}
	return path, nil
}
func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
