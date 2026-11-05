package cli

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/srliao/tt/internal/ttclient"
)

// Exit codes are part of the output contract; agents branch on them.
const (
	ExitOK         = 0
	ExitUsage      = 1
	ExitNotFound   = 2
	ExitValidation = 3
	ExitRemote     = 4
)

// CLI-side error codes. Server codes (ttclient.CodeValidation, ...) pass
// through unchanged from the response envelope, and the CLI reuses them for
// its own errors of the same kind.
const (
	CodeUsage           = "usage"
	CodeConfigMissing   = "config_missing"
	CodeConfigInvalid   = "config_invalid"
	CodeVersionMismatch = "version_mismatch"
)

// Error is the CLI's error shape: the server's {code, message, details}
// envelope plus the exit code it maps to.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Exit    int            `json:"-"`
}

func (e *Error) Error() string { return e.Message }

func usageErr(msg string, details map[string]any) *Error {
	return &Error{Code: CodeUsage, Message: msg, Details: details, Exit: ExitUsage}
}

func validationErr(msg string, details map[string]any) *Error {
	return &Error{Code: ttclient.CodeValidation, Message: msg, Details: details, Exit: ExitValidation}
}

// asError folds any error returned by a command into an *Error. Client
// errors carry their own code; anything else came from cobra's argument or
// flag parsing and is a usage error.
func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var ae *ttclient.APIError
	if errors.As(err, &ae) {
		return &Error{Code: ae.Code, Message: ae.Message, Details: ae.Details, Exit: exitFor(ae)}
	}
	return usageErr(err.Error(), nil)
}

func exitFor(ae *ttclient.APIError) int {
	switch ae.Code {
	case ttclient.CodeNotFound:
		return ExitNotFound
	case ttclient.CodeValidation, ttclient.CodeConflict:
		return ExitValidation
	}
	return ExitRemote
}

func writeError(w io.Writer, e *Error) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(e)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
