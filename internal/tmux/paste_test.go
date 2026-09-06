package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasteTextLongSingleLinePreservesBytesInBracketedBuffer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMACT_TEST_PASTE_DIR", dir)
	script := `#!/bin/sh
[ "$1" = "-u" ] && shift
case "$1" in
load-buffer) cat > "$TMACT_TEST_PASTE_DIR/payload" ;;
paste-buffer) printf '%s\n' "$@" > "$TMACT_TEST_PASTE_DIR/args" ;;
delete-buffer) ;;
*) exit 42 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	prompt := "requirement (1) /tmp/screenshot.png " + strings.Repeat("完整內容 ", 4000) + " requirement (3) end"
	if err := PasteText("%7", prompt, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "payload"))
	if err != nil || string(got) != prompt {
		t.Fatalf("payload mismatch: bytes=%d want=%d err=%v", len(got), len(prompt), err)
	}
	args, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil || !strings.HasPrefix(string(args), "paste-buffer\n-p\n-t\n%7\n-b\n") {
		t.Fatalf("missing bracketed paste: %q err=%v", args, err)
	}
}
