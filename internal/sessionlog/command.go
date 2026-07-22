package sessionlog

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	safeCommandWord       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

var subcommandVerbs = map[string]bool{
	"az": true, "aws": true, "bun": true, "cargo": true, "docker": true,
	"docker-compose": true, "gh": true, "git": true, "gcloud": true,
	"go": true, "helm": true, "kubectl": true, "npm": true, "npx": true,
	"pnpm": true, "terraform": true, "tmact": true, "tofu": true, "yarn": true,
}

// SafeCommandSummary reduces a command to a privacy-safe executable verb and,
// for a small allowlist, its subcommand. It never returns arguments or
// environment assignments.
func SafeCommandSummary(command string) (string, string) {
	line := command
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	fields := strings.Fields(line)
	index := 0
	for index < len(fields) && environmentAssignment.MatchString(fields[index]) {
		index++
	}
	if index >= len(fields) {
		return "", ""
	}
	verb := cleanCommandWord(filepath.Base(fields[index]))
	index++
	if verb == "rtk" && index < len(fields) {
		if fields[index] == "proxy" {
			index++
		} else if fields[index] == "gain" {
			return "rtk", "gain"
		}
		if index < len(fields) {
			verb = cleanCommandWord(filepath.Base(fields[index]))
			index++
		}
	}
	if verb == "" || !subcommandVerbs[verb] || index >= len(fields) || strings.HasPrefix(fields[index], "-") {
		return verb, ""
	}
	return verb, cleanCommandWord(fields[index])
}

func cleanCommandWord(value string) string {
	value = strings.Trim(value, "'\"")
	if !safeCommandWord.MatchString(value) {
		return ""
	}
	return value
}
