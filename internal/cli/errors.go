package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// commandError gives scripts stable exit codes while preserving wrapped causes.
type commandError struct {
	code   string
	status int
	err    error
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

func usageError(err error) error {
	return &commandError{code: "invalid_arguments", status: 2, err: err}
}

func commandFailure(code string, status int, format string, args ...any) error {
	return &commandError{code: code, status: status, err: fmt.Errorf(format, args...)}
}

// executeCommand keeps errors on stderr, including when stdout contains an array
// or a watch stream. Successful output contracts are therefore unchanged.
func executeCommand(root *cobra.Command) int {
	cmd, err := root.ExecuteC()
	if err == nil {
		return 0
	}
	code, status := "operation_failed", 1
	var typed *commandError
	switch {
	case errors.As(err, &typed):
		code, status = typed.code, typed.status
	case errors.Is(err, context.DeadlineExceeded):
		code, status = "timeout", 3
	case errors.Is(err, context.Canceled):
		code, status = "canceled", 130
	case strings.HasPrefix(err.Error(), "unknown command"), strings.HasPrefix(err.Error(), "accepts "):
		code, status = "invalid_arguments", 2
	}
	jsonOutput, _ := root.PersistentFlags().GetBool("json")
	if cmd == nil {
		cmd = root
	}
	if jsonOutput {
		_ = writeJSON(cmd.ErrOrStderr(), map[string]any{"error": map[string]any{"code": code, "message": err.Error(), "exit_status": status}})
	} else {
		p := newPalette(cmd.ErrOrStderr())
		fmt.Fprintf(cmd.ErrOrStderr(), "%s %v\n", p.paint(p.warn, "Error:"), err)
	}
	return status
}
