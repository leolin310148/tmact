package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteReviewReportAppendsComment(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Change: "demo",
		Roles:  map[string]string{"pm": "pm-agent", "swe": "swe-agent", "qa": "qa-agent", "reviewer": "reviewer-agent"},
		Discussion: DiscussionConfig{
			RoleOrder: []string{"pm", "swe", "qa", "reviewer"},
		},
	}
	comment, err := WriteReviewReport(cfg, ReviewReport{
		Role:          "qa",
		Kind:          "accept",
		ChangeHash:    "sha256:abc",
		OpenSpecValid: true,
		Body:          "accepted",
		Timestamp:     time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if comment.ID == "" {
		t.Fatalf("comment missing ID: %#v", comment)
	}
	comments, err := LoadComments(CommentsPath(changeDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Role != "qa" || comments[0].Kind != "accept" || !comments[0].OpenSpecValid {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestWriteReviewReportRejectsInvalidInputWithoutAppend(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Change: "demo",
		Roles:  map[string]string{"pm": "pm-agent", "swe": "swe-agent", "qa": "qa-agent", "reviewer": "reviewer-agent"},
		Discussion: DiscussionConfig{
			RoleOrder: []string{"pm", "swe", "qa", "reviewer"},
		},
	}
	_, err := WriteReviewReport(cfg, ReviewReport{Role: "unknown", Kind: "accept", ChangeHash: "sha256:abc"})
	if err == nil {
		t.Fatal("expected invalid role to fail")
	}
	if _, err := os.Stat(CommentsPath(changeDir)); !os.IsNotExist(err) {
		t.Fatalf("comments file should not be created, err=%v", err)
	}
}

func TestWriteImplementationReportAppendsPrimaryAndSidecarComments(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Change: "demo",
		Roles:  map[string]string{"swe": "swe-agent", "qa": "qa-agent", "pm": "pm-agent"},
		Implementation: ImplementationConfig{
			StageOrder: []string{"swe_apply", "qa_verify", "pm_archive"},
		},
	}
	comment, err := WriteImplementationReport(cfg, ImplementationReport{
		Role:       "qa",
		Stage:      "verify",
		Kind:       "pass",
		ChangeHash: "sha256:abc",
		Body:       "tests passed",
		Timestamp:  time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if comment.Stage != "verify" || comment.ID == "" {
		t.Fatalf("comment = %#v", comment)
	}
	primary, err := LoadImplementationComments(Phase2CommentsPath(changeDir))
	if err != nil {
		t.Fatal(err)
	}
	sidecarPath, err := Phase2SidecarCommentsPath("demo")
	if err != nil {
		t.Fatal(err)
	}
	sidecar, err := LoadImplementationComments(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(primary) != 1 || len(sidecar) != 1 || primary[0].ID != sidecar[0].ID {
		t.Fatalf("primary=%#v sidecar=%#v", primary, sidecar)
	}
}

func TestWriteImplementationReportRejectsMismatchedRole(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Change: "demo",
		Roles:  map[string]string{"swe": "swe-agent", "qa": "qa-agent", "pm": "pm-agent"},
		Implementation: ImplementationConfig{
			StageOrder: []string{"swe_apply", "qa_verify", "pm_archive"},
		},
	}
	_, err := WriteImplementationReport(cfg, ImplementationReport{Role: "qa", Stage: "apply", Kind: "complete", ChangeHash: "sha256:abc"})
	if err == nil {
		t.Fatal("expected mismatched role/stage to fail")
	}
	if _, err := os.Stat(Phase2CommentsPath(changeDir)); !os.IsNotExist(err) {
		t.Fatalf("phase2 comments file should not be created, err=%v", err)
	}
}
