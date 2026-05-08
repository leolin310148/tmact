package watch

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tmact/internal/prompt"
)

type decision struct {
	Accept    bool
	Reason    string
	Signature string
	Details   map[string]interface{}
}

func evaluateDirectoryAccess(rule RuleConfig, detected *prompt.DirectoryAccess) decision {
	if detected == nil {
		return decision{Accept: false, Reason: "not_found"}
	}
	if detected.SelectedOption == nil {
		return decision{
			Accept:    false,
			Reason:    "no_selected_option",
			Signature: promptSignature(detected),
			Details:   promptDetails(detected),
		}
	}
	if !isAcceptOption(detected.SelectedOption.Label) {
		return decision{
			Accept:    false,
			Reason:    "selected_option_not_accepting",
			Signature: promptSignature(detected),
			Details:   promptDetails(detected),
		}
	}
	if len(detected.Paths) == 0 {
		return decision{
			Accept:    false,
			Reason:    "no_paths",
			Signature: promptSignature(detected),
			Details:   promptDetails(detected),
		}
	}

	for _, requested := range detected.Paths {
		if !pathAllowed(requested, rule.AllowPaths, rule.AllowPathPatterns) {
			details := promptDetails(detected)
			details["blocked_path"] = requested
			return decision{
				Accept:    false,
				Reason:    "path_not_allowed",
				Signature: promptSignature(detected),
				Details:   details,
			}
		}
	}

	return decision{
		Accept:    true,
		Reason:    "allowed",
		Signature: promptSignature(detected),
		Details:   promptDetails(detected),
	}
}

func isAcceptOption(label string) bool {
	label = strings.TrimSpace(strings.ToLower(label))
	return label == "yes" || strings.HasPrefix(label, "yes,")
}

func pathAllowed(requested string, allowedPaths []string, allowedPathPatterns []string) bool {
	requestedPath, err := normalizePath(requested)
	if err != nil {
		return false
	}
	for _, allowed := range allowedPaths {
		allowedPath, err := normalizePath(allowed)
		if err != nil {
			continue
		}
		if requestedPath == allowedPath {
			return true
		}
		if strings.HasPrefix(requestedPath, allowedPath+string(filepath.Separator)) {
			return true
		}
	}
	for _, pattern := range allowedPathPatterns {
		if pathPatternAllowed(requestedPath, pattern) {
			return true
		}
	}
	return false
}

func pathPatternAllowed(requestedPath string, pattern string) bool {
	patternPath, err := normalizePath(pattern)
	if err != nil {
		return false
	}
	matched, err := filepath.Match(patternPath, requestedPath)
	return err == nil && matched
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = abs
	}
	return filepath.Clean(path), nil
}

func promptSignature(detected *prompt.DirectoryAccess) string {
	paths := append([]string(nil), detected.Paths...)
	sort.Strings(paths)

	parts := []string{
		detected.Title,
		detected.Question,
		strings.Join(paths, "\x00"),
	}
	if detected.SelectedOption != nil {
		parts = append(parts, fmt.Sprintf("%d:%s", detected.SelectedOption.Number, detected.SelectedOption.Label))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func promptDetails(detected *prompt.DirectoryAccess) map[string]interface{} {
	details := map[string]interface{}{
		"title":    detected.Title,
		"question": detected.Question,
		"paths":    detected.Paths,
	}
	if detected.SelectedOption != nil {
		details["selected_option"] = map[string]interface{}{
			"number": detected.SelectedOption.Number,
			"label":  detected.SelectedOption.Label,
		}
	}
	return details
}
