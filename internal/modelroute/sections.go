package modelroute

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var markdownHeading = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.+?)\s*$`)
var closingHashes = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
var markdownFence = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")

type heading struct {
	title, slug         string
	level, offset, line int
}

func headingSlug(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.ReplaceAll(title, "`", "")) {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte('-')
		case r == '-' || r == '_' || !unicode.IsPunct(r) && !unicode.IsSymbol(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sectionContent includes the heading and its children. Fenced examples never
// create headings. Duplicate literal titles/anchors are ambiguous, not first-win.
func sectionContent(text, selector string) (string, int, int, string) {
	lines := strings.SplitAfter(text, "\n")
	heads := []heading{}
	offset := 0
	fenceChar := byte(0)
	fenceLen := 0
	slugs := map[string]int{}
	for i, line := range lines {
		plain := strings.TrimRight(line, "\r\n")
		if fenceChar != 0 {
			t := strings.TrimLeft(plain, " ")
			indent := len(plain) - len(t)
			n := 0
			for n < len(t) && t[n] == fenceChar {
				n++
			}
			if indent <= 3 && n >= fenceLen && strings.TrimSpace(t[n:]) == "" {
				fenceChar = 0
			}
		} else if f := markdownFence.FindStringSubmatch(plain); f != nil {
			fenceChar = f[1][0]
			fenceLen = len(f[1])
		} else if h := markdownHeading.FindStringSubmatch(plain); h != nil {
			title := strings.TrimSpace(closingHashes.ReplaceAllString(h[2], ""))
			slug := headingSlug(title)
			base := slug
			if count := slugs[base]; count > 0 {
				slug = base + "-" + strconv.Itoa(count)
			}
			slugs[base]++
			heads = append(heads, heading{title: title, slug: slug, level: len(h[1]), offset: offset, line: i + 1})
		}
		offset += len(line)
	}
	matches := []int{}
	for i, h := range heads {
		if h.title == selector || h.slug == selector {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return "", 0, 0, "section_not_found"
	}
	if len(matches) > 1 {
		return "", 0, 0, "section_ambiguous"
	}
	at := matches[0]
	h := heads[at]
	end := len(text)
	endLine := len(lines)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		endLine--
	}
	for _, next := range heads[at+1:] {
		if next.level <= h.level {
			end = next.offset
			endLine = next.line - 1
			break
		}
	}
	return text[h.offset:end], h.line, endLine, "attached"
}
