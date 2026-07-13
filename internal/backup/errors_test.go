package backup

import (
	"errors"
	"testing"
)

func TestCommandError_Error(t *testing.T) {
	ce := &CommandError{
		Tool:    "mysqldump",
		Cmd:     "mysqldump -h 127.0.0.1 -u root -p*** db",
		Stderr:  "Access denied",
		Message: "mysqldump 执行失败",
		Cause:   errors.New("exit status 1"),
	}
	msg := ce.Error()
	if msg == "" {
		t.Error("CommandError.Error() 不应为空")
	}
	for _, substr := range []string{"mysqldump", "Access denied", "mysqldump 执行失败", "stderr:"} {
		if !containsSubstring(msg, substr) {
			t.Errorf("Error() 缺少 %q, got: %s", substr, msg)
		}
	}
}

func TestCommandError_ErrorWithStdout(t *testing.T) {
	ce := &CommandError{
		Tool:    "rman",
		Cmd:     "rman target /",
		Stdout:  "RMAN-03002: failure of backup command",
		Stderr:  "",
		Message: "rman 执行失败",
		Cause:   errors.New("exit status 1"),
	}
	msg := ce.Error()
	if !containsSubstring(msg, "stdout:") {
		t.Errorf("有 Stdout 时应包含 stdout, got: %s", msg)
	}
	if !containsSubstring(msg, "RMAN-03002") {
		t.Errorf("应包含 stdout 内容, got: %s", msg)
	}
	if containsSubstring(msg, "stderr:") {
		t.Errorf("没有 Stderr 时不应包含 stderr, got: %s", msg)
	}
}

func TestCommandError_ErrorNoStderr(t *testing.T) {
	ce := &CommandError{
		Tool:    "pg_dump",
		Message: "pg_dump 执行失败",
		Cause:   errors.New("exit status 1"),
	}
	msg := ce.Error()
	if containsSubstring(msg, "stderr:") {
		t.Errorf("没有 Stderr 时不应包含 stderr, got: %s", msg)
	}
	if containsSubstring(msg, "stdout:") {
		t.Errorf("没有 Stdout 时不应包含 stdout, got: %s", msg)
	}
}

func TestCommandError_Unwrap(t *testing.T) {
	cause := errors.New("exit status 1")
	ce := &CommandError{
		Tool:    "mysql",
		Message: "mysql 执行失败",
		Cause:   cause,
	}
	if !errors.Is(ce, cause) {
		t.Error("Unwrap 应返回 Cause")
	}
}

func TestCommandError_ErrorsIs(t *testing.T) {
	cause := errors.New("exit status 1")
	ce := &CommandError{
		Tool:    "mysql",
		Message: "mysql 执行失败",
		Cause:   cause,
	}
	if !errors.Is(ce, cause) {
		t.Error("errors.Is(ce, cause) 应为 true")
	}
}

func TestCommandError_ErrorsAs(t *testing.T) {
	ce := &CommandError{
		Tool:    "mysqldump",
		Stderr:  "Access denied",
		Message: "mysqldump 执行失败",
		Cause:   errors.New("exit status 1"),
	}
	var ce2 *CommandError
	if !errors.As(ce, &ce2) {
		t.Error("errors.As 应能提取 CommandError")
	}
	if ce2.Tool != "mysqldump" {
		t.Errorf("Tool = %q, want %q", ce2.Tool, "mysqldump")
	}
	if ce2.Stderr != "Access denied" {
		t.Errorf("Stderr = %q, want %q", ce2.Stderr, "Access denied")
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
