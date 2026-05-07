package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func CapturePane(target string, lines int) (string, error) {
	if lines <= 0 {
		lines = 120
	}

	cmd := exec.Command(
		"tmux",
		"capture-pane",
		"-t",
		target,
		"-p",
		"-J",
		"-S",
		"-"+strconv.Itoa(lines),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("tmux capture-pane failed: %s", stderr.String())
		}
		return "", fmt.Errorf("tmux capture-pane failed: %w", err)
	}
	return string(output), nil
}

func PasteText(target string, text string, enter bool) error {
	trimmed := strings.TrimRight(text, "\n")
	if trimmed != "" && !strings.Contains(trimmed, "\n") {
		if err := SendLiteral(target, trimmed); err != nil {
			return err
		}
		if enter {
			time.Sleep(100 * time.Millisecond)
			return SendKeys(target, []string{"Enter"})
		}
		return nil
	}

	bufferName := "tmact-paste"
	load := exec.Command("tmux", "load-buffer", "-b", bufferName, "-")
	load.Stdin = bytes.NewBufferString(text)

	var loadStderr bytes.Buffer
	load.Stderr = &loadStderr
	if err := load.Run(); err != nil {
		if loadStderr.Len() > 0 {
			return fmt.Errorf("tmux load-buffer failed: %s", loadStderr.String())
		}
		return fmt.Errorf("tmux load-buffer failed: %w", err)
	}

	if err := runTmux("paste-buffer", "-t", target, "-b", bufferName); err != nil {
		return err
	}
	if enter {
		time.Sleep(100 * time.Millisecond)
		return SendKeys(target, []string{"Enter"})
	}
	return nil
}

func SendLiteral(target string, text string) error {
	return runTmux("send-keys", "-t", target, "-l", text)
}

func SendKeys(target string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	args := append([]string{"send-keys", "-t", target}, keys...)
	return runTmux(args...)
}

func runTmux(args ...string) error {
	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("tmux %s failed: %s", args[0], stderr.String())
		}
		return fmt.Errorf("tmux %s failed: %w", args[0], err)
	}
	return nil
}
