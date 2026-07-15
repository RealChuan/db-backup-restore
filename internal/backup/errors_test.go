package backup

import (
	"errors"
	"fmt"
	"strings"
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
		if !strings.Contains(msg, substr) {
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
	if !strings.Contains(msg, "stdout:") {
		t.Errorf("有 Stdout 时应包含 stdout, got: %s", msg)
	}
	if !strings.Contains(msg, "RMAN-03002") {
		t.Errorf("应包含 stdout 内容, got: %s", msg)
	}
	if strings.Contains(msg, "stderr:") {
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
	if strings.Contains(msg, "stderr:") {
		t.Errorf("没有 Stderr 时不应包含 stderr, got: %s", msg)
	}
	if strings.Contains(msg, "stdout:") {
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
	// 直接验证 Unwrap 返回 Cause
	if !errors.Is(ce, cause) {
		t.Errorf("errors.Is(ce, cause) = false, want true")
	}
	// 无 Cause 时应返回 nil
	ceNoCause := &CommandError{Tool: "mysql", Message: "no cause"}
	if got := ceNoCause.Unwrap(); got != nil {
		t.Errorf("Unwrap() 无 Cause 时应返回 nil, got %v", got)
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
	// 嵌套包装后仍应能匹配
	wrapped := fmt.Errorf("wrapped: %w", ce)
	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is 在包装后仍应能匹配 Cause")
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
