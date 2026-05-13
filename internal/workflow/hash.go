package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func HashChangeDir(dir string) (string, []string, error) {
	paths, err := artifactPaths(dir)
	if err != nil {
		return "", nil, err
	}
	if len(paths) == 0 {
		return "", nil, fmt.Errorf("%s: no OpenSpec artifacts found", dir)
	}
	hasher := sha256.New()
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", nil, err
		}
		normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
		_, _ = hasher.Write([]byte(rel))
		_, _ = hasher.Write([]byte{'\n'})
		_, _ = hasher.Write([]byte(normalized))
		_, _ = hasher.Write([]byte("\n---tmact-artifact---\n"))
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), paths, nil
}

func artifactPaths(dir string) ([]string, error) {
	candidates := []string{"proposal.md", "design.md", "tasks.md"}
	var paths []string
	for _, rel := range candidates {
		info, err := os.Stat(filepath.Join(dir, rel))
		if err == nil && !info.IsDir() {
			paths = append(paths, rel)
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	specsDir := filepath.Join(dir, "specs")
	err := filepath.WalkDir(specsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Base(path) != "spec.md" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func EnsureProposal(dir string, create bool) error {
	path := filepath.Join(dir, "proposal.md")
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if !create {
		return fmt.Errorf("%s does not exist; set create_missing_proposal: true to create a template", path)
	}
	template := "# Proposal\n\n## Intent\n\nTBD\n\n## Scope\n\nTBD\n\n## Success Criteria\n\nTBD\n"
	return os.WriteFile(path, []byte(template), 0o644)
}
