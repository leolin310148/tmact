package modelroute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// cappedBuffer prevents an unexpected child response from growing without bound.
type cappedBuffer struct{ bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("decide output exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}

func runDecide(ctx context.Context, input Input, questions any) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	config, err := trustedConfig()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode routing input: %w", err)
	}
	f, err := os.CreateTemp("", "tmact-decide-questions-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(questions)
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	cmd := exec.CommandContext(ctx, "decide", "-config", config, "ask", "--input", "-", "--input-json", "--questions", f.Name(), "--format", "json", "--timeout", "40s", "--retries", "0")
	cmd.Stdin = bytes.NewReader(data)
	// Authentication stays in the inherited environment/config, never argv or
	// the decision input. Disable decide's full task/response usage log.
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "DECIDE_USAGE_LOG=") && !strings.HasPrefix(v, "DECIDE_CALLER=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "DECIDE_USAGE_LOG=off", "DECIDE_CALLER=tmact-auto-model")
	var stdout cappedBuffer
	cmd.Stdout = &stdout
	// Provider error text may contain upstream response bodies. Keep it out of
	// tmact reports, which can be persisted or forwarded to another machine.
	if err := cmd.Run(); err != nil {
		// A user decide config may enable its own confidence gate. Its
		// structured answer remains usable; tmact treats confidence as metadata.
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 3 {
			return stdout.Bytes(), nil
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("auto-model: decide timed out or was cancelled")
		}
		if _, ok := err.(*exec.Error); ok {
			return nil, fmt.Errorf("auto-model requires the decide CLI in PATH")
		}
		return nil, fmt.Errorf("auto-model: decide failed; check CLI configuration/authentication; no agent launched")
	}
	return stdout.Bytes(), nil
}

// Never load ./decide.yaml from the task repository. Explicit config must be
// absolute; otherwise use the user's global config, falling back to env-only.
func trustedConfig() (string, error) {
	if p := os.Getenv("DECIDE_CLI_CONFIG"); p != "" {
		if p == "none" || filepath.IsAbs(p) {
			return p, nil
		}
		return "", fmt.Errorf("auto-model: DECIDE_CLI_CONFIG must be absolute or none")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "none", nil
	}
	// decide follows XDG even on macOS, unlike os.UserConfigDir.
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		dir = p
	} else if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, ".config")
	}
	p := filepath.Join(dir, "decide-cli", "config.yaml")
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("auto-model: global decide config directory must be absolute")
	}
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return "none", nil
}
