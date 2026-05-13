package workflow

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type ReviewReport struct {
	Role          string
	Kind          string
	ChangeHash    string
	OpenSpecValid bool
	Blocking      bool
	ReplyTo       string
	Body          string
	Timestamp     time.Time
}

type ImplementationReport struct {
	Role       string
	Stage      string
	Kind       string
	ChangeHash string
	Blocking   bool
	ReplyTo    string
	Body       string
	Timestamp  time.Time
}

func WriteReviewReport(cfg Config, report ReviewReport) (Comment, error) {
	if err := validateReviewReport(cfg, report); err != nil {
		return Comment{}, err
	}
	changeDir, err := ChangeDir(cfg.Change)
	if err != nil {
		return Comment{}, err
	}
	if info, err := os.Stat(changeDir); err != nil {
		return Comment{}, err
	} else if !info.IsDir() {
		return Comment{}, fmt.Errorf("%s is not a directory", changeDir)
	}
	comment := Comment{
		Timestamp:     reportTimestamp(report.Timestamp),
		Role:          strings.TrimSpace(report.Role),
		Kind:          strings.TrimSpace(report.Kind),
		ChangeHash:    strings.TrimSpace(report.ChangeHash),
		OpenSpecValid: report.OpenSpecValid,
		Blocking:      report.Blocking,
		ReplyTo:       strings.TrimSpace(report.ReplyTo),
		Body:          strings.TrimSpace(report.Body),
	}
	if isBlockingKind(comment.Kind) {
		comment.Blocking = true
	}
	comment.ID = commentFingerprint(comment)
	comments, err := LoadComments(CommentsPath(changeDir))
	if err != nil {
		return Comment{}, err
	}
	if _, err := AppendNewComments(CommentsPath(changeDir), comments, []Comment{comment}); err != nil {
		return Comment{}, err
	}
	return comment, nil
}

func WriteImplementationReport(cfg Config, report ImplementationReport) (ImplementationComment, error) {
	if err := validateImplementationReport(cfg, report); err != nil {
		return ImplementationComment{}, err
	}
	changeDir, err := ChangeDir(cfg.Change)
	if err != nil {
		return ImplementationComment{}, err
	}
	comment := ImplementationComment{
		Timestamp:  reportTimestamp(report.Timestamp),
		Role:       strings.TrimSpace(report.Role),
		Stage:      markerStageName(canonicalImplementationStage(strings.TrimSpace(report.Stage))),
		Kind:       strings.TrimSpace(report.Kind),
		ChangeHash: strings.TrimSpace(report.ChangeHash),
		Blocking:   report.Blocking,
		ReplyTo:    strings.TrimSpace(report.ReplyTo),
		Body:       strings.TrimSpace(report.Body),
	}
	if isImplementationBlockingKind(comment.Kind) {
		comment.Blocking = true
	}
	comment.ID = implementationCommentFingerprint(comment)
	if info, err := os.Stat(changeDir); err == nil && info.IsDir() {
		comments, err := LoadImplementationComments(Phase2CommentsPath(changeDir))
		if err != nil {
			return ImplementationComment{}, err
		}
		if _, err := AppendNewImplementationComments(Phase2CommentsPath(changeDir), comments, []ImplementationComment{comment}); err != nil {
			return ImplementationComment{}, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return ImplementationComment{}, err
	} else if !(comment.Stage == "archive" && comment.Kind == "complete") {
		return ImplementationComment{}, fmt.Errorf("%s does not exist", changeDir)
	}
	sidecarPath, err := Phase2SidecarCommentsPath(cfg.Change)
	if err != nil {
		return ImplementationComment{}, err
	}
	sidecarComments, err := LoadImplementationComments(sidecarPath)
	if err != nil {
		return ImplementationComment{}, err
	}
	if _, err := AppendNewImplementationComments(sidecarPath, sidecarComments, []ImplementationComment{comment}); err != nil {
		return ImplementationComment{}, err
	}
	return comment, nil
}

func validateReviewReport(cfg Config, report ReviewReport) error {
	role := strings.TrimSpace(report.Role)
	if role == "" {
		return errors.New("role is required")
	}
	if strings.TrimSpace(cfg.Roles[role]) == "" {
		return fmt.Errorf("unknown review role %q", role)
	}
	if !containsString(cfg.Discussion.RoleOrder, role) {
		return fmt.Errorf("role %q is not in discussion.role_order", role)
	}
	kind := strings.TrimSpace(report.Kind)
	switch kind {
	case "accept", "request_changes", "reject", "withdraw_accept", "decision":
	default:
		return fmt.Errorf("unsupported review kind %q", kind)
	}
	if err := validateReportHash(report.ChangeHash); err != nil {
		return err
	}
	if kind == "decision" && strings.TrimSpace(report.ReplyTo) == "" {
		return errors.New("reply_to is required for decision reports")
	}
	return nil
}

func validateImplementationReport(cfg Config, report ImplementationReport) error {
	role := strings.TrimSpace(report.Role)
	if role == "" {
		return errors.New("role is required")
	}
	if strings.TrimSpace(cfg.Roles[role]) == "" {
		return fmt.Errorf("unknown implementation role %q", role)
	}
	stage := canonicalImplementationStage(strings.TrimSpace(report.Stage))
	expectedRole, ok := implementationStageRole(stage)
	if !ok {
		return fmt.Errorf("unsupported implementation stage %q", report.Stage)
	}
	if role != expectedRole {
		return fmt.Errorf("stage %q must be reported by role %q", report.Stage, expectedRole)
	}
	kind := strings.TrimSpace(report.Kind)
	switch kind {
	case "complete", "pass", "fail", "request_changes", "blocked", "decision", "withdraw":
	default:
		return fmt.Errorf("unsupported implementation kind %q", kind)
	}
	if err := validateReportHash(report.ChangeHash); err != nil {
		return err
	}
	if kind == "decision" && strings.TrimSpace(report.ReplyTo) == "" {
		return errors.New("reply_to is required for decision reports")
	}
	return nil
}

func validateReportHash(changeHash string) error {
	if strings.TrimSpace(changeHash) == "" {
		return errors.New("change_hash is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(changeHash), "sha256:") {
		return errors.New("change_hash must start with sha256:")
	}
	return nil
}

func reportTimestamp(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now()
	}
	return ts
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
