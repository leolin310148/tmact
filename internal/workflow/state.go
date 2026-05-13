package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Change         string                   `json:"change"`
	Status         string                   `json:"status"`
	Phase          string                   `json:"phase"`
	Outcome        string                   `json:"outcome,omitempty"`
	Reason         string                   `json:"reason,omitempty"`
	Turn           int                      `json:"turn"`
	PendingRole    string                   `json:"pending_role,omitempty"`
	ChangeHash     string                   `json:"change_hash,omitempty"`
	Artifacts      []string                 `json:"artifacts,omitempty"`
	LastValidation *ValidationResult        `json:"last_validation,omitempty"`
	Gate           GateResult               `json:"gate"`
	Agreements     map[string]RoleAgreement `json:"agreements,omitempty"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

func StatePath(changeDir string) string {
	return filepath.Join(changeDir, "phase1-state.json")
}

func LoadState(path string) (State, error) {
	var state State
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func WriteState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
