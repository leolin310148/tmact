package modelroute

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSectionExtractionAndFenceBoundaries(t *testing.T) {
	text := "# Doc\nintro\n## 章節標題\nwanted\n### Child\nchild\n```md\n## Fake boundary\n```\n~~~\n# Also fake\n~~~~\n## Other\nunwanted\n"
	got, start, end, status := sectionContent(text, "章節標題")
	if status != "attached" || start != 3 || end != 12 || !strings.Contains(got, "child") || !strings.Contains(got, "## Fake boundary") || strings.Contains(got, "unwanted") {
		t.Fatalf("status=%s lines=%d-%d content=%q", status, start, end, got)
	}
	_, _, _, status = sectionContent(text, "Fake boundary")
	if status != "section_not_found" {
		t.Fatal(status)
	}
	crlf := strings.ReplaceAll(text, "\n", "\r\n")
	got, _, _, status = sectionContent(crlf, "章節標題")
	if status != "attached" || !strings.Contains(got, "\r\n") {
		t.Fatal("lost CRLF section")
	}
}

func TestSectionSelectorsRemainDistinctAndMissingIsNotWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	text := "# Doc\n## Alpha Heading\nfirst\n### Child\nnested\n## Beta\nsecond\n## 重複\nx\n## 重複\ny\n"
	os.WriteFile(path, []byte(text), 0600)
	files := CollectPromptFiles("`doc.md#Alpha Heading` doc.md#Beta doc.md#missing doc.md#重複", dir)
	if len(files) != 4 {
		t.Fatalf("sections ate each other: %+v", fileMetadata(files))
	}
	if !strings.Contains(files[0].Content, "nested") || strings.Contains(files[0].Content, "second") || !strings.Contains(files[1].Content, "second") {
		t.Fatal(fileMetadata(files))
	}
	if files[2].Status != "section_not_found" || files[3].Status != "section_ambiguous" || files[2].Content != "" || files[3].Content != "" {
		t.Fatal(fileMetadata(files))
	}
	for _, f := range files {
		if f.SourceSHA256 == "" || f.SourceBytes != int64(len(text)) {
			t.Fatal("missing source evidence")
		}
	}
	slug := CollectPromptFiles("doc.md#alpha-heading doc.md#"+url.PathEscape("Alpha Heading"), dir)
	if len(slug) != 2 || slug[0].Content != files[0].Content || slug[1].Content != files[0].Content {
		t.Fatal(fileMetadata(slug))
	}
	f := CollectPromptFiles("file://"+path+"#Beta", dir)
	if len(f) != 1 || f[0].Section != "Beta" || !strings.Contains(f[0].Content, "second") {
		t.Fatal(f)
	}
}

func TestSectionNearEndOfLargeFileIsReadBeforeExcerptLimit(t *testing.T) {
	dir := t.TempDir()
	s := strings.Repeat("prefix\n", 20000) + "\n## Tail\nimportant\n## End\nno\n"
	os.WriteFile(filepath.Join(dir, "long.md"), []byte(s), 0600)
	f := CollectPromptFiles("long.md#Tail", dir)
	if len(f) != 1 || f[0].Status != "attached" || f[0].Truncated || !strings.Contains(f[0].Content, "important") || f[0].OriginalBytes != len(f[0].Content) || f[0].IncludedBytes != len(f[0].Content) {
		t.Fatal(fileMetadata(f))
	}
}

func TestMultipleSectionsDoNotConsumePhysicalFileSlots(t *testing.T) {
	dir := t.TempDir()
	var body, prompt strings.Builder
	for i := 0; i < 10; i++ {
		heading := string(rune('A' + i))
		body.WriteString("## " + heading + "\nbody\n")
		prompt.WriteString("doc.md#" + heading + " ")
	}
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte(body.String()), 0600)
	os.WriteFile(filepath.Join(dir, "other.md"), []byte("other"), 0600)
	prompt.WriteString("other.md")
	files := CollectPromptFiles(prompt.String(), dir)
	if len(files) != 11 {
		t.Fatalf("sections consumed file slots: %+v", fileMetadata(files))
	}
	for _, f := range files {
		if f.Status != "attached" {
			t.Fatal(fileMetadata(files))
		}
	}
}
