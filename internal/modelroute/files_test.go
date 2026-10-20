package modelroute

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPromptPaths(t *testing.T) {
	for _, tt := range []struct {
		name, prompt string
		want         []string
	}{
		{"absolute and relative", "Read /tmp/task.md and docs/plan.txt", []string{"/tmp/task.md", "docs/plan.txt"}},
		{"quoted spaces", "Read `docs/my plan.md` and \"/tmp/my task.TXT\"", []string{"docs/my plan.md", "/tmp/my task.TXT"}},
		{"Markdown and line references", "Read [plan](./plan.md:12) plus /tmp/task.txt#section", []string{"./plan.md", "/tmp/task.txt"}},
		{"URL is not a local path", "https://example.com/task.md ftp://host/task.txt", []string{}},
		{"file URL", "file:///tmp/my%20task.md", []string{"/tmp/my task.md"}},
		{"Chinese punctuation", "請讀 /tmp/任務.md，並照做。", []string{"/tmp/任務.md"}},
		{"deduplicate", "task.md task.md `task.md`", []string{"task.md"}},
		{"no code files", "main.go data.json docs/readme.md.bak", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := PromptPaths(tt.prompt); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestCollectPromptFilesSnapshotsAndNoRecursion(t *testing.T) {
	dir := t.TempDir()
	content := "Actual complex task. Do not automatically read secret.txt."
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("not referenced in original prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "task.md"), filepath.Join(dir, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	files := CollectPromptFiles("Read task.md, alias.txt and missing.md", dir)
	if len(files) != 2 || files[0].Status != "attached" || files[0].Content != content || files[0].SHA256 == "" || files[1].Status != "unreadable" {
		t.Fatalf("files=%+v", files)
	}
	expected, _ := filepath.EvalSymlinks(filepath.Join(dir, "task.md"))
	if files[0].Path != expected {
		t.Fatal(files[0].Path)
	}
	if fileMetadata(files)[0].Content != "" || files[0].Content != content {
		t.Fatal("metadata must redact without mutating snapshot")
	}
}

func TestCollectPromptFilesBoundsAndTextValidation(t *testing.T) {
	dir := t.TempDir()
	large := strings.Repeat("中", MaxTaskFileBytes)
	if err := os.WriteFile(filepath.Join(dir, "large.md"), []byte(large), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "binary.txt"), []byte{0xff, 0xfe, 0x80}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.md"), 0700); err != nil {
		t.Fatal(err)
	}
	files := CollectPromptFiles("large.md binary.txt folder.md", dir)
	if len(files) != 3 || !files[0].Truncated || len(files[0].Content) > MaxTaskFileBytes || !utf8.ValidString(files[0].Content) {
		t.Fatalf("large snapshot invalid: %+v", fileMetadata(files))
	}
	if files[1].Status != "invalid_utf8" || files[2].Status != "not_regular" {
		t.Fatal(fileMetadata(files))
	}
}

func TestCollectPromptFilesHomeAndBudget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("home task"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := CollectPromptFiles("~/task.md", t.TempDir()); len(got) != 1 || got[0].Content != "home task" {
		t.Fatal(got)
	}
	var refs []string
	for i := range MaxTaskFiles + 2 {
		refs = append(refs, filepath.Join(dir, string(rune('a'+i))+".md"))
	}
	files := CollectPromptFiles(strings.Join(refs, " "), dir)
	if len(files) != MaxTaskFiles+1 || files[len(files)-1].Status != "file_count_limit" {
		t.Fatal(fileMetadata(files))
	}
}
