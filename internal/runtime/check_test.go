package runtime_test

import (
	"errors"
	"testing"

	"github.com/srliao/tt/internal/runtime"
)

func TestCheck_ValidCodePasses(t *testing.T) {
	code := "const n = ctx.state.get(\"n\") || 0;\nctx.state.set(\"n\", n + 1);\n"
	if err := runtime.Check(code); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheck_DoesNotRunCode(t *testing.T) {
	// ctx is undefined outside a run; executing would throw.
	if err := runtime.Check(`ctx.queueTask({title: "x"}); throw new Error("ran");`); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheck_SyntaxErrorReportsLineAndColumn(t *testing.T) {
	err := runtime.Check("const a = 1;\nconst b = ;\n")
	var ce *runtime.CheckError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *runtime.CheckError", err, err)
	}
	if ce.Line != 2 || ce.Column != 11 {
		t.Errorf("position = %d:%d, want 2:11", ce.Line, ce.Column)
	}
	if ce.Message == "" {
		t.Error("message is empty")
	}
}

func TestCheck_CompileErrorReportsLine(t *testing.T) {
	// Parses fine, but redeclaring a let binding is a compile-time error.
	err := runtime.Check("let a = 1;\nlet a = 2;\n")
	var ce *runtime.CheckError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *runtime.CheckError", err, err)
	}
	if ce.Line != 2 {
		t.Errorf("line = %d, want 2", ce.Line)
	}
}
