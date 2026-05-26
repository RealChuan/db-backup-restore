package backup

import (
	"context"
	"errors"
	"fmt"
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

// HandleError 统一错误处理
func HandleError(err error) {
	if err == nil {
		return
	}
	var be *BackupError
	if errors.As(err, &be) {
		logging.Error("备份错误", "type", string(be.Type), "op", be.Op, "message", be.Message)
		if be.Cause != nil {
			logging.Debug("错误原因", "error", be.Cause)
		}
	} else {
		logging.Error("未知错误", "error", err)
	}
}
