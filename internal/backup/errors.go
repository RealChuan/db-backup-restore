package backup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// ErrorType 错误类型
type ErrorType string

const (
	ErrorTypeNotSupported ErrorType = "NOT_SUPPORTED"
)

// BackupError 备份/还原操作错误
type BackupError struct {
	Type      ErrorType
	Op        string
	DBType    string
	Message   string
	Cause     error
	Timestamp time.Time
	TraceID   string
}

func (e *BackupError) Error() string {
	var causeStr string
	if e.Cause != nil {
		causeStr = fmt.Sprintf(", 原因: %v", e.Cause)
	}
	return fmt.Sprintf("[%s] %s: %s%s", e.Type, e.Op, e.Message, causeStr)
}

func (e *BackupError) Unwrap() error {
	return e.Cause
}

// NewNotSupportedError 创建不支持操作错误
func NewNotSupportedError(ctx context.Context, op, dbType string) error {
	return &BackupError{
		Type:      ErrorTypeNotSupported,
		Op:        op,
		DBType:    dbType,
		Message:   fmt.Sprintf("%s 操作在 %s 数据库上不支持", op, dbType),
		Timestamp: time.Now(),
		TraceID:   logging.GetTraceID(ctx),
	}
}

// CommandError 命令执行失败错误，携带完整的诊断信息。
// 调用方可通过 errors.As 提取结构化字段，无需解析错误消息字符串。
type CommandError struct {
	Tool    string // 工具名: mysqldump / pg_dump / dexp / rman / sqlcmd
	Cmd     string // 完整命令（已脱敏）
	Stdout  string // 标准输出内容（RMAN 等工具将错误诊断输出到 stdout）
	Stderr  string // 标准错误输出
	Message string // 人类可读摘要
	Cause   error  // 原始错误
}

// Error 返回人类可读的错误描述，包含所有诊断信息
func (e *CommandError) Error() string {
	var parts []string
	if e.Stderr != "" {
		parts = append(parts, fmt.Sprintf("stderr: %s", e.Stderr))
	}
	if e.Stdout != "" {
		parts = append(parts, fmt.Sprintf("stdout: %s", e.Stdout))
	}
	if len(parts) > 0 {
		return fmt.Sprintf("%s: %s, %s", e.Tool, e.Message, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("%s: %s", e.Tool, e.Message)
}

// Unwrap 返回底层 Cause，支持 errors.Is/As 链式查找
func (e *CommandError) Unwrap() error {
	return e.Cause
}
