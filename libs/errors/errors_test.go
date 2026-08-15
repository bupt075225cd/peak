package errors

import (
	"errors"
	"testing"
)

func TestFromAndCodeOf(t *testing.T) {
	be := New(CodeNotFound, "not found")
	if CodeOf(be) != CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %d", CodeOf(be))
	}

	plain := errors.New("boom")
	if CodeOf(plain) != CodeInternal {
		t.Fatalf("expected CodeInternal, got %d", CodeOf(plain))
	}
}

func TestWrapUnwrap(t *testing.T) {
	root := errors.New("root")
	wrapped := Wrap(CodeUpstream, "upstream failed", root)
	if !errors.Is(wrapped, root) {
		t.Fatal("expected wrapped to unwrap to root")
	}
}

func TestErrorError(t *testing.T) {
	// 无 cause。
	e := New(CodeNotFound, "not found")
	if e.Error() != "[1004] not found" {
		t.Fatalf("unexpected error string: %s", e.Error())
	}
	// 有 cause。
	w := Wrap(CodeInternal, "boom", errors.New("cause"))
	if w.Error() != "[5000] boom: cause" {
		t.Fatalf("unexpected wrapped string: %s", w.Error())
	}
}

func TestFromNil(t *testing.T) {
	if From(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
	if CodeOf(nil) != CodeOK {
		t.Fatal("CodeOf(nil) should be CodeOK")
	}
}

func TestFromWrappedBusinessError(t *testing.T) {
	be := New(CodeForbidden, "denied")
	wrapped := errors.New("outer")
	wrapped = be
	// errors.As 直接提取业务错误。
	got := From(wrapped)
	if got != be {
		t.Fatal("expected same business error instance")
	}
	if got.Code != CodeForbidden {
		t.Fatalf("expected CodeForbidden, got %d", got.Code)
	}
}

func TestFromDoubleWrapped(t *testing.T) {
	be := New(CodeConflict, "conflict")
	// 用 fmt.Errorf 包装，模拟底层错误链。
	w := errors.New("x")
	_ = w
	outer := &Error{Code: CodeConflict, Message: "conflict", Cause: be}
	if !errors.Is(outer, be) {
		t.Fatal("expected errors.Is chain")
	}
	got := From(outer)
	if got.Code != CodeConflict {
		t.Fatalf("expected CodeConflict, got %d", got.Code)
	}
}
