package workflow

import (
	"testing"
	"time"
)

func TestParseCommentMarker(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	line := `TMAct-OpenSpec-Comment: role=qa kind=request_changes change_hash=sha256:abc openspec_valid=true blocking=true reply_to=c-1 body="missing stale validation scenario"`
	comment, ok, err := ParseCommentLine(line, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("marker not found")
	}
	if comment.Role != "qa" || comment.Kind != "request_changes" || comment.ChangeHash != "sha256:abc" || !comment.OpenSpecValid || !comment.Blocking {
		t.Fatalf("comment = %#v", comment)
	}
	if comment.Body != "missing stale validation scenario" || comment.ReplyTo != "c-1" || comment.ID == "" {
		t.Fatalf("comment details = %#v", comment)
	}
}

func TestParseCommentsIgnoresAmbiguousProse(t *testing.T) {
	comments, err := ParseCommentsFromText("looks good to me\n", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestParseCommentsFromWrappedPaneText(t *testing.T) {
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	raw := `⏺ TMAct-OpenSpec-Comment: role=pm kind=accept change_hash=sha256:152a1e8b27c8507
  4e22bf8917981eac39af6f615aa8458ff70fa268b757eccf9 openspec_valid=true
  blocking=false body="accepted current
  artifacts"
`

	comments, err := ParseCommentsFromText(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("comments = %#v", comments)
	}
	comment := comments[0]
	if comment.ChangeHash != "sha256:152a1e8b27c85074e22bf8917981eac39af6f615aa8458ff70fa268b757eccf9" {
		t.Fatalf("change hash = %q", comment.ChangeHash)
	}
	if !comment.OpenSpecValid || comment.Blocking || comment.Body != "accepted current artifacts" {
		t.Fatalf("comment = %#v", comment)
	}
}
