package agentusage

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// homeDir returns the current user's home directory, or "" when it cannot be
// resolved (callers treat that as "no credentials").
func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// claudeCredentialsJSON returns the raw contents of Claude Code's stored OAuth
// credentials. On Linux/WSL these live in ~/.claude/.credentials.json; on macOS
// Claude Code stores them in the login keychain instead, so we fall back to the
// `security` CLI. Returns ("", nil) when no credentials exist anywhere.
func claudeCredentialsJSON() (string, error) {
	if home := homeDir(); home != "" {
		path := filepath.Join(home, ".claude", ".credentials.json")
		if data, err := os.ReadFile(path); err == nil {
			return string(data), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if runtime.GOOS == "darwin" {
		return claudeKeychainJSON()
	}
	return "", nil
}

// claudeKeychainJSON reads the "Claude Code-credentials" generic password from
// the macOS login keychain. A non-zero exit (item not found) yields ("", nil).
func claudeKeychainJSON() (string, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w")
	out, err := cmd.Output()
	if err != nil {
		// Item-not-found and access-denied both surface as exit errors; treat as
		// "no credentials" so the provider reports a clean not-logged-in state.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// codexHome resolves the Codex config directory, honoring $CODEX_HOME.
func codexHome() string {
	if v := strings.TrimSpace(os.Getenv("CODEX_HOME")); v != "" {
		return v
	}
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".codex")
	}
	return ""
}
