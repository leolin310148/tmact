package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Update struct {
	State       string
	Owner       string
	Stage       string
	Cycle       *int
	Blockers    []string
	SetBlockers bool
	UpdatedAt   time.Time
}

type Event struct {
	Timestamp string                 `json:"ts"`
	Kind      string                 `json:"kind"`
	Path      string                 `json:"path"`
	State     string                 `json:"state,omitempty"`
	From      string                 `json:"from,omitempty"`
	To        string                 `json:"to,omitempty"`
	Owner     string                 `json:"owner,omitempty"`
	Stage     string                 `json:"stage,omitempty"`
	Cycle     *int                   `json:"cycle,omitempty"`
	Agent     string                 `json:"agent,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type StatusFile struct {
	path string
	doc  yaml.Node
}

func Load(path string) (*StatusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file := &StatusFile{path: path}
	if err := yaml.Unmarshal(data, &file.doc); err != nil {
		return nil, err
	}
	if err := file.ensureMapping(); err != nil {
		return nil, err
	}
	if file.State() == "" {
		return nil, errors.New("status state is required")
	}
	return file, nil
}

func LoadOrNew(path string) (*StatusFile, error) {
	file, err := Load(path)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return New(path), nil
}

func New(path string) *StatusFile {
	return &StatusFile{
		path: path,
		doc: yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			},
		},
	}
}

func (f *StatusFile) Path() string {
	return f.path
}

func (f *StatusFile) State() string {
	return f.scalar("state")
}

func (f *StatusFile) Apply(update Update) error {
	if update.State == "" {
		return errors.New("state is required")
	}
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = time.Now()
	}
	if err := f.ensureMapping(); err != nil {
		return err
	}

	f.setScalar("state", update.State)
	if update.Owner != "" {
		f.setScalar("owner", update.Owner)
	}
	if update.Stage != "" {
		f.setScalar("stage", update.Stage)
	}
	if update.Cycle != nil {
		f.setInt("cycle", *update.Cycle)
	}
	if update.SetBlockers {
		f.setStringSequence("blockers", update.Blockers)
	}
	f.setScalar("updated_at", update.UpdatedAt.UTC().Format(time.RFC3339))
	return nil
}

func (f *StatusFile) Data() (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := f.mapping().Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	return out, nil
}

func (f *StatusFile) Write() error {
	if f.State() == "" {
		return errors.New("status state is required")
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(&f.doc)
	if err != nil {
		return err
	}

	dir := filepath.Dir(f.path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(f.path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, f.path)
}

func Set(path string, update Update) (map[string]interface{}, Event, error) {
	file, err := LoadOrNew(path)
	if err != nil {
		return nil, Event{}, err
	}
	if err := file.Apply(update); err != nil {
		return nil, Event{}, err
	}
	if err := file.Write(); err != nil {
		return nil, Event{}, err
	}

	event := eventFromUpdate("set", path, update)
	event.State = update.State
	if err := AppendEvent(path, event); err != nil {
		return nil, Event{}, err
	}
	data, err := file.Data()
	return data, event, err
}

func Transition(path string, from string, update Update) (map[string]interface{}, Event, error) {
	if from == "" {
		return nil, Event{}, errors.New("from state is required")
	}
	if update.State == "" {
		return nil, Event{}, errors.New("to state is required")
	}

	file, err := Load(path)
	if err != nil {
		return nil, Event{}, err
	}
	current := file.State()
	if current != from {
		return nil, Event{}, fmt.Errorf("current state %q does not match --from %q", current, from)
	}
	if err := file.Apply(update); err != nil {
		return nil, Event{}, err
	}
	if err := file.Write(); err != nil {
		return nil, Event{}, err
	}

	event := eventFromUpdate("transition", path, update)
	event.From = from
	event.To = update.State
	event.State = update.State
	if err := AppendEvent(path, event); err != nil {
		return nil, Event{}, err
	}
	data, err := file.Data()
	return data, event, err
}

func AppendEvent(statusPath string, event Event) error {
	if event.Kind == "" {
		return errors.New("event kind is required")
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Path == "" {
		event.Path = statusPath
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventPath := filepath.Join(filepath.Dir(statusPath), "events.jsonl")
	file, err := os.OpenFile(eventPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(data, '\n'))
	return err
}

func eventFromUpdate(kind string, path string, update Update) Event {
	ts := update.UpdatedAt
	if ts.IsZero() {
		ts = time.Now()
	}
	return Event{
		Timestamp: ts.UTC().Format(time.RFC3339),
		Kind:      kind,
		Path:      path,
		Owner:     update.Owner,
		Stage:     update.Stage,
		Cycle:     update.Cycle,
	}
}

func (f *StatusFile) ensureMapping() error {
	if f.doc.Kind == 0 {
		f.doc = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode},
			},
		}
	}
	if f.doc.Kind != yaml.DocumentNode || len(f.doc.Content) != 1 {
		return errors.New("status must be a YAML mapping")
	}
	if f.doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("status must be a YAML mapping")
	}
	return nil
}

func (f *StatusFile) mapping() *yaml.Node {
	return f.doc.Content[0]
}

func (f *StatusFile) scalar(key string) string {
	node := f.valueNode(key)
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func (f *StatusFile) valueNode(key string) *yaml.Node {
	mapping := f.mapping()
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func (f *StatusFile) setScalar(key string, value string) {
	node := f.valueNode(key)
	if node == nil {
		f.mapping().Content = append(f.mapping().Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
		)
		return
	}
	node.Kind = yaml.ScalarNode
	node.Tag = "!!str"
	node.Value = value
	node.Content = nil
}

func (f *StatusFile) setInt(key string, value int) {
	node := f.valueNode(key)
	if node == nil {
		f.mapping().Content = append(f.mapping().Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(value)},
		)
		return
	}
	node.Kind = yaml.ScalarNode
	node.Tag = "!!int"
	node.Value = strconv.Itoa(value)
	node.Content = nil
}

func (f *StatusFile) setStringSequence(key string, values []string) {
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}

	node := f.valueNode(key)
	if node == nil {
		f.mapping().Content = append(f.mapping().Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			sequence,
		)
		return
	}
	*node = *sequence
}
