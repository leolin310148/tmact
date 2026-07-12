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

// claudeCredentialCandidates returns every raw OAuth credential blob Claude Code
// might have stored. On Linux/WSL these live in ~/.claude/.credentials.json; on
// macOS Claude Code stores them in the login keychain. The two are not mutually
// exclusive: a macOS box can carry a stale ~/.claude/.credentials.json left over
// from an earlier setup while the keychain holds the token Claude Code actually
// keeps refreshed. Returning both lets the caller pick the freshest by
// expiresAt instead of blindly trusting whichever it reads first. Returns an
// empty slice when no credentials exist anywhere.
func claudeCredentialCandidates() ([]string, error) {
	var out []string
	if home := homeDir(); home != "" {
		path := filepath.Join(home, ".claude", ".credentials.json")
		if data, err := os.ReadFile(path); err == nil {
			out = append(out, string(data))
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	if runtime.GOOS == "darwin" {
		if kc, err := claudeKeychainJSON(); err == nil && kc != "" {
			out = append(out, kc)
		}
	}
	return out, nil
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
