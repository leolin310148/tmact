package workflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func ComputeRevisions(cfg Config, data TemplateData) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range sortedKeys(cfg.Revisions) {
		digest, err := ComputeRevision(cfg.Workspace.Root, cfg.Revisions[name], data)
		if err != nil {
			return nil, fmt.Errorf("revision %q: %w", name, err)
		}
		out[name] = digest
	}
	return out, nil
}

func ComputeRevision(root string, cfg RevisionConfig, data TemplateData) (string, error) {
	if cfg.Files != nil {
		return filesDigest(root, cfg.Files.Paths, data)
	}
	if cfg.Git != nil {
		return gitDigest(root, cfg.Git.Dir, data)
	}
	return "", fmt.Errorf("revision provider is missing")
}

func filesDigest(root string, patterns []string, data TemplateData) (string, error) {
	var files []string
	seen := map[string]bool{}
	for i, pattern := range patterns {
		rendered, err := Render(fmt.Sprintf("revision.files.paths[%d]", i), pattern, data)
		if err != nil {
			return "", err
		}
		path, err := safeWorkspacePath(root, rendered)
		if err != nil {
			return "", err
		}
		matches, err := filepath.Glob(path)
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			matches = []string{path}
		}
		for _, match := range matches {
			info, err := os.Lstat(match)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return "", err
			}
			if info.IsDir() {
				err = filepath.WalkDir(match, func(p string, d fs.DirEntry, e error) error {
					if e != nil {
						return e
					}
					if d.Type().IsRegular() {
						rel, err := filepath.Rel(root, p)
						if err != nil {
							return err
						}
						if !seen[rel] {
							files = append(files, rel)
							seen[rel] = true
						}
					}
					return nil
				})
				if err != nil {
					return "", err
				}
			} else if info.Mode().IsRegular() {
				rel, err := filepath.Rel(root, match)
				if err != nil {
					return "", err
				}
				if !seen[rel] {
					files = append(files, rel)
					seen[rel] = true
				}
			}
		}
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		path, err := safeWorkspacePath(root, rel)
		if err != nil {
			return "", err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(raw)
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func gitDigest(root, dirTemplate string, data TemplateData) (string, error) {
	dir := root
	if dirTemplate != "" {
		rendered, err := Render("revision.git.dir", dirTemplate, data)
		if err != nil {
			return "", err
		}
		dir, err = safeWorkspacePath(root, rendered)
		if err != nil {
			return "", err
		}
	}
	h := sha256.New()
	commands := [][]string{{"rev-parse", "HEAD"}, {"ls-files", "--stage", "-z"}, {"diff", "--cached", "--binary", "--no-ext-diff"}, {"diff", "--binary", "--no-ext-diff"}}
	for _, args := range commands {
		out, err := gitOutput(dir, args...)
		if err != nil {
			return "", err
		}
		h.Write([]byte(strings.Join(args, " ")))
		h.Write([]byte{0})
		h.Write(bytes.ReplaceAll(out, []byte("\r\n"), []byte("\n")))
		h.Write([]byte{0})
	}
	untracked, err := gitOutput(dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	names := strings.Split(strings.TrimSuffix(string(untracked), "\x00"), "\x00")
	sort.Strings(names)
	for _, name := range names {
		if name == "" {
			continue
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return out, nil
}

func sha256Bytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
