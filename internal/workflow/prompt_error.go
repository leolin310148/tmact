package workflow

import (
	"errors"
	"fmt"

	"tmact/internal/prompt"
)

type PermissionPromptError struct {
	Role   string
	Prompt prompt.Prompt
}

func (e PermissionPromptError) Error() string {
	if e.Prompt.Title == "" {
		return fmt.Sprintf("permission_prompt in %s: %s", e.Role, e.Prompt.Type)
	}
	return fmt.Sprintf("permission_prompt in %s: %s %s", e.Role, e.Prompt.Type, e.Prompt.Title)
}

func isPermissionPromptError(err error) bool {
	var promptErr PermissionPromptError
	return errors.As(err, &promptErr)
}
