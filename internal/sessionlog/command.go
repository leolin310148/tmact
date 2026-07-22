package sessionlog

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	environmentName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	shellLiteralWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)
	safeCommandWord  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
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
	fields, ok := splitShellWords(line)
	if !ok {
		return "", ""
	}
	index := 0
	for index < len(fields) && fields[index].assignment {
		index++
	}
	if index >= len(fields) {
		return "", ""
	}
	for {
		verb := commandWord(fields[index])
		if verb == "" {
			return "", ""
		}
		index++
		switch verb {
		case "env":
			var valid bool
			index, valid = commandAfterEnv(fields, index)
			if !valid {
				return "", ""
			}
			if index >= len(fields) {
				return "env", ""
			}
			continue
		case "rtk":
			if index >= len(fields) {
				return "rtk", ""
			}
			if fields[index].dynamic {
				return "", ""
			}
			if fields[index].value == "gain" {
				return "rtk", "gain"
			}
			if fields[index].value == "proxy" {
				index++
			}
			if index >= len(fields) {
				return "rtk", ""
			}
			continue
		}

		if !subcommandVerbs[verb] || index >= len(fields) || fields[index].dynamic || strings.HasPrefix(fields[index].value, "-") {
			return verb, ""
		}
		return verb, cleanCommandWord(fields[index].value)
	}
}

type shellWord struct {
	value      string
	assignment bool
	dynamic    bool
}

// splitShellWords recognizes only the shell word structure needed to identify
// a static executable. It deliberately rejects operators, command
// substitutions, parameter braces, and malformed quoting rather than trying
// to emulate a shell.
func splitShellWords(line string) ([]shellWord, bool) {
	var words []shellWord
	for index := 0; index < len(line); {
		for index < len(line) && shellSpace(line[index]) {
			index++
		}
		if index >= len(line) || line[index] == '#' {
			break
		}

		var value strings.Builder
		word := shellWord{}
		nameIsPlain := true
		sawEquals := false
		for index < len(line) && !shellSpace(line[index]) {
			switch line[index] {
			case '\'', '"':
				quote := line[index]
				if !sawEquals {
					nameIsPlain = false
				}
				index++
				closed := false
				for index < len(line) {
					if line[index] == quote {
						index++
						closed = true
						break
					}
					if quote == '"' {
						if line[index] == '`' || (line[index] == '$' && index+1 < len(line) && (line[index+1] == '(' || line[index+1] == '{')) {
							return nil, false
						}
						if line[index] == '$' {
							word.dynamic = true
						}
						if line[index] == '\\' && index+1 < len(line) {
							next := line[index+1]
							if next == '$' || next == '`' || next == '"' || next == '\\' {
								value.WriteByte(next)
								index += 2
								continue
							}
						}
					}
					value.WriteByte(line[index])
					index++
				}
				if !closed {
					return nil, false
				}
			case '\\':
				if index+1 >= len(line) {
					return nil, false
				}
				if !sawEquals {
					nameIsPlain = false
				}
				value.WriteByte(line[index+1])
				index += 2
			case '`':
				return nil, false
			case '$':
				if index+1 < len(line) && (line[index+1] == '(' || line[index+1] == '{') {
					return nil, false
				}
				if !sawEquals {
					nameIsPlain = false
				}
				word.dynamic = true
				value.WriteByte(line[index])
				index++
			case ';', '&', '|', '<', '>', '(', ')':
				return nil, false
			case '=':
				if !sawEquals {
					word.assignment = nameIsPlain && environmentName.MatchString(value.String())
					sawEquals = true
				}
				value.WriteByte(line[index])
				index++
			default:
				value.WriteByte(line[index])
				index++
			}
		}
		word.value = value.String()
		words = append(words, word)
	}
	return words, true
}

func commandAfterEnv(words []shellWord, index int) (int, bool) {
	options := true
	for index < len(words) {
		word := words[index]
		assignment := word.assignment || environmentArgument(word.value)
		if options && !assignment {
			switch {
			case word.dynamic:
				return 0, false
			case word.value == "--":
				options = false
				index++
				continue
			case word.value == "-" || word.value == "-i" || word.value == "--ignore-environment" || word.value == "-v" || word.value == "--debug":
				index++
				continue
			case word.value == "-u" || word.value == "--unset" || word.value == "-C" || word.value == "--chdir" || word.value == "-P":
				if index+1 >= len(words) {
					return 0, false
				}
				index += 2
				continue
			case strings.HasPrefix(word.value, "--unset=") && len(word.value) > len("--unset="):
				index++
				continue
			case strings.HasPrefix(word.value, "--chdir=") && len(word.value) > len("--chdir="):
				index++
				continue
			case strings.HasPrefix(word.value, "-u") && len(word.value) > len("-u"):
				index++
				continue
			case strings.HasPrefix(word.value, "-C") && len(word.value) > len("-C"):
				index++
				continue
			case word.value == "-S" || word.value == "--split-string" || strings.HasPrefix(word.value, "--split-string="):
				return 0, false
			case strings.HasPrefix(word.value, "-"):
				return 0, false
			default:
				options = false
			}
		}
		if assignment {
			index++
			continue
		}
		return index, true
	}
	return index, true
}

func environmentArgument(value string) bool {
	separator := strings.IndexByte(value, '=')
	return separator > 0 && environmentName.MatchString(value[:separator])
}

func commandWord(word shellWord) string {
	if word.dynamic {
		return ""
	}
	return cleanCommandWord(filepath.Base(word.value))
}

func shellSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r'
}

func joinCommandWords(words []string) string {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		if shellLiteralWord.MatchString(word) {
			quoted = append(quoted, word)
			continue
		}
		quoted = append(quoted, "'"+strings.ReplaceAll(word, "'", "'\\''")+"'")
	}
	return strings.Join(quoted, " ")
}

func cleanCommandWord(value string) string {
	if !safeCommandWord.MatchString(value) {
		return ""
	}
	return value
}
