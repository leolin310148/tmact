package workflow

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Phase2Marker = "TMAct-OpenSpec-Phase2:"

type ImplementationState struct {
	Change             string                         `json:"change"`
	Status             string                         `json:"status"`
	Phase              string                         `json:"phase"`
	Outcome            string                         `json:"outcome,omitempty"`
	Reason             string                         `json:"reason,omitempty"`
	Turn               int                            `json:"turn"`
	PendingStage       string                         `json:"pending_stage,omitempty"`
	PendingRole        string                         `json:"pending_role,omitempty"`
	AcceptedChangeHash string                         `json:"accepted_change_hash,omitempty"`
	CurrentChangeHash  string                         `json:"current_change_hash,omitempty"`
	Artifacts          []string                       `json:"artifacts,omitempty"`
	LastValidation     *ValidationResult              `json:"last_validation,omitempty"`
	Gate               ImplementationGateResult       `json:"gate"`
	Stages             map[string]ImplementationStage `json:"stages,omitempty"`
	UpdatedAt          time.Time                      `json:"updated_at"`
}

type ImplementationStage struct {
	Complete   bool   `json:"complete"`
	Passed     bool   `json:"passed,omitempty"`
	Failed     bool   `json:"failed,omitempty"`
	Kind       string `json:"kind,omitempty"`
	ChangeHash string `json:"change_hash,omitempty"`
	CommentID  string `json:"comment_id,omitempty"`
	Body       string `json:"body,omitempty"`
}

type ImplementationComment struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"ts"`
	Role       string    `json:"role"`
	Stage      string    `json:"stage"`
	Kind       string    `json:"kind"`
	ChangeHash string    `json:"change_hash"`
	Blocking   bool      `json:"blocking"`
	ReplyTo    string    `json:"reply_to,omitempty"`
	Body       string    `json:"body,omitempty"`
	Raw        string    `json:"raw,omitempty"`
}

type ImplementationGateResult struct {
	Passed       bool                           `json:"passed"`
	Reasons      []string                       `json:"reasons,omitempty"`
	PendingStage string                         `json:"pending_stage,omitempty"`
	PendingRole  string                         `json:"pending_role,omitempty"`
	Stages       map[string]ImplementationStage `json:"stages,omitempty"`
}

func Phase2StatePath(changeDir string) string {
	return filepath.Join(changeDir, "phase2-state.json")
}

func Phase2CommentsPath(changeDir string) string {
	return filepath.Join(changeDir, "phase2-comments.jsonl")
}

func Phase2SidecarStatePath(change string) (string, error) {
	dir, err := workflowSidecarDir(change)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "phase2-state.json"), nil
}

func Phase2SidecarCommentsPath(change string) (string, error) {
	dir, err := workflowSidecarDir(change)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "phase2-comments.jsonl"), nil
}

func workflowSidecarDir(change string) (string, error) {
	if _, err := ChangeDir(change); err != nil {
		return "", err
	}
	return filepath.Join(".tmact", "workflow", filepath.Clean(change)), nil
}

func LoadImplementationState(path string) (ImplementationState, error) {
	var state ImplementationState
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func WriteImplementationState(path string, state ImplementationState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ParseImplementationCommentsFromText(text string, now time.Time) ([]ImplementationComment, error) {
	var comments []ImplementationComment
	for _, line := range markerLogicalLines(text, Phase2Marker) {
		comment, ok, err := ParseImplementationCommentLine(line, now)
		if err != nil {
			return nil, err
		}
		if ok {
			comments = append(comments, comment)
		}
	}
	return comments, nil
}

func ParseImplementationCommentLine(line string, now time.Time) (ImplementationComment, bool, error) {
	index := strings.Index(line, Phase2Marker)
	if index < 0 {
		return ImplementationComment{}, false, nil
	}
	raw := strings.TrimSpace(line[index:])
	fields, err := parseMarkerFields(strings.TrimSpace(strings.TrimPrefix(raw, Phase2Marker)))
	if err != nil {
		return ImplementationComment{}, false, err
	}
	comment := ImplementationComment{
		Timestamp:  now,
		Role:       fields["role"],
		Stage:      fields["stage"],
		Kind:       fields["kind"],
		ChangeHash: fields["change_hash"],
		ReplyTo:    fields["reply_to"],
		Body:       fields["body"],
		Raw:        raw,
	}
	if comment.Role == "" || comment.Stage == "" || comment.Kind == "" || comment.ChangeHash == "" {
		return ImplementationComment{}, false, fmt.Errorf("invalid phase2 marker: role, stage, kind, and change_hash are required")
	}
	if value := fields["blocking"]; value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return ImplementationComment{}, false, fmt.Errorf("invalid blocking value %q", value)
		}
		comment.Blocking = parsed
	}
	if isImplementationBlockingKind(comment.Kind) {
		comment.Blocking = true
	}
	comment.ID = implementationCommentFingerprint(comment)
	return comment, true, nil
}

func LoadImplementationComments(path string) ([]ImplementationComment, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var comments []ImplementationComment
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var comment ImplementationComment
		if err := json.Unmarshal([]byte(line), &comment); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, scanner.Err()
}

func LoadImplementationCommentsForChange(change string) ([]ImplementationComment, error) {
	changeDir, err := ChangeDir(change)
	if err != nil {
		return nil, err
	}
	comments, err := LoadImplementationComments(Phase2CommentsPath(changeDir))
	if err != nil {
		return nil, err
	}
	sidecarPath, err := Phase2SidecarCommentsPath(change)
	if err != nil {
		return nil, err
	}
	sidecarComments, err := LoadImplementationComments(sidecarPath)
	if err != nil {
		return nil, err
	}
	return mergeImplementationComments(comments, sidecarComments), nil
}

func AppendNewImplementationComments(path string, existing []ImplementationComment, observed []ImplementationComment) ([]ImplementationComment, error) {
	seen := map[string]bool{}
	for _, comment := range existing {
		seen[implementationCommentFingerprint(comment)] = true
	}
	var fresh []ImplementationComment
	for _, comment := range observed {
		fingerprint := implementationCommentFingerprint(comment)
		if seen[fingerprint] {
			continue
		}
		comment.ID = fingerprint
		fresh = append(fresh, comment)
		seen[fingerprint] = true
	}
	if len(fresh) == 0 {
		return existing, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	for _, comment := range fresh {
		data, err := json.Marshal(comment)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return nil, err
		}
		existing = append(existing, comment)
	}
	return existing, nil
}

func mergeImplementationComments(primary []ImplementationComment, sidecar []ImplementationComment) []ImplementationComment {
	merged := append([]ImplementationComment(nil), primary...)
	seen := map[string]bool{}
	for _, comment := range merged {
		seen[implementationCommentFingerprint(comment)] = true
	}
	for _, comment := range sidecar {
		fingerprint := implementationCommentFingerprint(comment)
		if seen[fingerprint] {
			continue
		}
		merged = append(merged, comment)
		seen[fingerprint] = true
	}
	return merged
}

func EvaluateImplementationGate(stageOrder []string, acceptedHash string, validation *ValidationResult, comments []ImplementationComment) ImplementationGateResult {
	result := ImplementationGateResult{Stages: ImplementationStagesFor(acceptedHash, comments)}
	if acceptedHash == "" {
		result.Reasons = appendReason(result.Reasons, "missing_accepted_hash")
		return result
	}
	for _, stage := range stageOrder {
		stageState := result.Stages[stage]
		switch stage {
		case "swe_apply":
			if stageState.Failed {
				result.Reasons = appendReason(result.Reasons, "apply_blocked")
				return result
			}
			if !stageState.Complete {
				result.PendingStage = stage
				result.PendingRole = "swe"
				result.Reasons = appendReason(result.Reasons, "missing_apply")
				return result
			}
		case "qa_verify":
			if stageState.Failed {
				result.Reasons = appendReason(result.Reasons, "qa_failed")
				return result
			}
			if !stageState.Passed {
				result.PendingStage = stage
				result.PendingRole = "qa"
				result.Reasons = appendReason(result.Reasons, "missing_verify")
				return result
			}
		case "pm_archive":
			if validation == nil || !validation.Passed || validation.Stale || validation.ChangeHash != acceptedHash {
				result.Reasons = appendReason(result.Reasons, "validation_not_passed")
				return result
			}
			if stageState.Failed {
				result.Reasons = appendReason(result.Reasons, "archive_blocked")
				return result
			}
			if !stageState.Complete {
				result.PendingStage = stage
				result.PendingRole = "pm"
				result.Reasons = appendReason(result.Reasons, "missing_archive")
				return result
			}
		}
	}
	result.Passed = len(result.Reasons) == 0
	return result
}

func ImplementationStagesFor(acceptedHash string, comments []ImplementationComment) map[string]ImplementationStage {
	stages := map[string]ImplementationStage{}
	for _, comment := range comments {
		if comment.ChangeHash != acceptedHash {
			continue
		}
		key := canonicalImplementationStage(comment.Stage)
		stage := stages[key]
		stage.Kind = comment.Kind
		stage.ChangeHash = comment.ChangeHash
		stage.CommentID = comment.ID
		stage.Body = comment.Body
		stage.Failed = false
		switch comment.Kind {
		case "complete":
			stage.Complete = true
			if comment.Stage == "archive" || comment.Stage == "pm_archive" {
				stage.Passed = true
			}
		case "pass":
			stage.Passed = true
			stage.Complete = true
		case "withdraw":
			stage = ImplementationStage{Kind: comment.Kind, ChangeHash: comment.ChangeHash, CommentID: comment.ID, Body: comment.Body}
		case "fail", "request_changes", "blocked":
			stage.Failed = true
			stage.Complete = false
			stage.Passed = false
		}
		stages[key] = stage
	}
	return stages
}

func RenderCommand(command CommandConfig, change string) string {
	parts := []string{command.Command}
	parts = append(parts, command.Args...)
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(part, "{{change}}", change)
	}
	return strings.Join(parts, " ")
}

func implementationCommentFingerprint(comment ImplementationComment) string {
	input := strings.Join([]string{
		comment.Role,
		comment.Stage,
		comment.Kind,
		comment.ChangeHash,
		strconv.FormatBool(comment.Blocking),
		comment.ReplyTo,
		comment.Body,
		comment.Raw,
	}, "\x00")
	sum := sha256.Sum256([]byte(input))
	return "p2-" + hex.EncodeToString(sum[:])[:12]
}

func canonicalImplementationStage(stage string) string {
	switch stage {
	case "apply":
		return "swe_apply"
	case "verify":
		return "qa_verify"
	case "archive":
		return "pm_archive"
	default:
		return stage
	}
}

func markerStageName(stage string) string {
	switch stage {
	case "swe_apply":
		return "apply"
	case "qa_verify":
		return "verify"
	case "pm_archive":
		return "archive"
	default:
		return stage
	}
}

func isImplementationBlockingKind(kind string) bool {
	switch kind {
	case "fail", "request_changes", "blocked":
		return true
	default:
		return false
	}
}
