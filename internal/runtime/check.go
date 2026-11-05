package runtime

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

// CheckError is a script that fails to compile. Line and Column are 1-based;
// both are 0 when goja does not report a position.
type CheckError struct {
	Message string `json:"message"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
}

func (e *CheckError) Error() string {
	if e.Line == 0 {
		return "script: " + e.Message
	}
	return fmt.Sprintf("script: line %d:%d: %s", e.Line, e.Column, e.Message)
}

// Check compiles code the way Runner.Run would, without running it. A
// failure is a *CheckError.
func Check(code string) error {
	prg, err := parser.ParseFile(nil, "", code, 0)
	if err != nil {
		var list parser.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			return &CheckError{Message: list[0].Message, Line: list[0].Position.Line, Column: list[0].Position.Column}
		}
		return &CheckError{Message: err.Error()}
	}
	if _, err := goja.CompileAST(prg, false); err != nil {
		var syn *goja.CompilerSyntaxError
		if errors.As(err, &syn) && syn.File != nil {
			pos := syn.File.Position(syn.Offset)
			return &CheckError{Message: syn.Message, Line: pos.Line, Column: pos.Column}
		}
		return &CheckError{Message: err.Error()}
	}
	return nil
}
