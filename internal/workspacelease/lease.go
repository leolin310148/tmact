package workspacelease

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrNotRepository = errors.New("workspace is not a git repository")

type Lease struct {
	file *os.File
}

func Acquire(root, owner string) (*Lease, error) {
	path, err := lockPath(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file, true); err != nil {
		current := readOwner(file)
		_ = file.Close()
		if current == "" {
			current = "unknown workflow"
		}
		return nil, fmt.Errorf("workspace %s is leased by %s", root, current)
	}
	if err := file.Truncate(0); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	metadata := fmt.Sprintf("%s pid=%d acquired=%s\n", owner, os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if _, err := file.WriteString(metadata); err != nil {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, err
	}
	_ = file.Sync()
	return &Lease{file: file}, nil
}

func (lease *Lease) Release() {
	if lease == nil || lease.file == nil {
		return
	}
	_ = lease.file.Truncate(0)
	_ = unlockFile(lease.file)
	_ = lease.file.Close()
	lease.file = nil
}

func CheckAvailable(root, allowedOwner string) error {
	lease, err := Acquire(root, "availability-probe")
	if err == nil {
		lease.Release()
		return nil
	}
	if errors.Is(err, ErrNotRepository) {
		return nil
	}
	if allowedOwner != "" && strings.Contains(err.Error(), "leased by "+allowedOwner+" ") {
		return nil
	}
	return err
}

func lockPath(root string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", ErrNotRepository, strings.TrimSpace(stderr.String()))
	}
	canonical, err := filepath.EvalSymlinks(strings.TrimSpace(stdout.String()))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(canonical)))
	name := hex.EncodeToString(sum[:16]) + ".lock"
	return filepath.Join(os.TempDir(), "tmact-workspace-leases", name), nil
}

func readOwner(file *os.File) string {
	if _, err := file.Seek(0, 0); err != nil {
		return ""
	}
	raw := make([]byte, 512)
	n, _ := file.Read(raw)
	return strings.TrimSpace(string(raw[:n]))
}
