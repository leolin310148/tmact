package workflow

import "sort"

type GateResult struct {
	Passed       bool     `json:"passed"`
	Reasons      []string `json:"reasons,omitempty"`
	PendingRoles []string `json:"pending_roles,omitempty"`
}

type RoleAgreement struct {
	Accepted   bool   `json:"accepted"`
	ChangeHash string `json:"change_hash,omitempty"`
	CommentID  string `json:"comment_id,omitempty"`
}

func EvaluateGate(roleOrder []string, currentHash string, validation *ValidationResult, comments []Comment) GateResult {
	result := GateResult{}
	if currentHash == "" {
		result.Reasons = appendReason(result.Reasons, "missing_hash")
	}
	if validation == nil || !validation.Passed || validation.Stale || validation.ChangeHash != currentHash {
		result.Reasons = appendReason(result.Reasons, "validation_not_passed")
	}

	agreements := AgreementsFor(roleOrder, currentHash, comments)
	for _, role := range roleOrder {
		if !agreements[role].Accepted {
			result.PendingRoles = append(result.PendingRoles, role)
		}
	}
	if len(result.PendingRoles) > 0 {
		result.Reasons = appendReason(result.Reasons, "missing_agreement")
	}

	blocking := unresolvedBlockingComments(currentHash, comments)
	if len(blocking) > 0 {
		result.Reasons = appendReason(result.Reasons, "blocking_comments")
	}
	result.Passed = len(result.Reasons) == 0
	return result
}

func AgreementsFor(roleOrder []string, currentHash string, comments []Comment) map[string]RoleAgreement {
	knownRoles := map[string]bool{}
	agreements := map[string]RoleAgreement{}
	for _, role := range roleOrder {
		knownRoles[role] = true
		agreements[role] = RoleAgreement{}
	}
	for _, comment := range comments {
		if !knownRoles[comment.Role] || comment.ChangeHash != currentHash {
			continue
		}
		switch comment.Kind {
		case "accept":
			if comment.OpenSpecValid {
				agreements[comment.Role] = RoleAgreement{Accepted: true, ChangeHash: comment.ChangeHash, CommentID: comment.ID}
			}
		case "withdraw_accept", "reject", "request_changes":
			agreements[comment.Role] = RoleAgreement{Accepted: false, ChangeHash: comment.ChangeHash, CommentID: comment.ID}
		}
	}
	return agreements
}

func unresolvedBlockingComments(currentHash string, comments []Comment) []Comment {
	resolved := map[string]bool{}
	for _, comment := range comments {
		if comment.ChangeHash == currentHash && comment.Kind == "decision" && comment.ReplyTo != "" {
			resolved[comment.ReplyTo] = true
		}
	}
	var blocking []Comment
	for _, comment := range comments {
		if comment.ChangeHash != currentHash || resolved[comment.ID] {
			continue
		}
		if comment.Blocking || isBlockingKind(comment.Kind) {
			blocking = append(blocking, comment)
		}
	}
	sort.Slice(blocking, func(i, j int) bool {
		return blocking[i].ID < blocking[j].ID
	})
	return blocking
}

func appendReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
