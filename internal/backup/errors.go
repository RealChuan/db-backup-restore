package backup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

type ErrorType string

const (
	ErrorTypeNotSupported ErrorType = "NOT_SUPPORTED"
)

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

func HandleError(err error) {
	if err == nil {
		return
	}
	var be *BackupError
	if errors.As(err, &be) {
		logging.Error(fmt.Sprintf("[%s] %s: %s", be.Type, be.Op, be.Message))
		if be.Cause != nil {
			logging.Debug(fmt.Sprintf("错误原因: %v", be.Cause))
		}
	} else {
		logging.Error(fmt.Sprintf("未知错误: %v", err))
	}
}
