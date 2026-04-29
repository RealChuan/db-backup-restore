package backup

import (
	"fmt"
	"time"

	"db-backup-restore/pkg/utils"
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

func NewNotSupportedError(op, dbType string) error {
	return &BackupError{
		Type:      ErrorTypeNotSupported,
		Op:        op,
		DBType:    dbType,
		Message:   fmt.Sprintf("%s 操作在 %s 数据库上不支持", op, dbType),
		Timestamp: time.Now(),
		TraceID:   utils.GetTraceID(),
	}
}

func HandleError(err error) {
	if err == nil {
		return
	}
	if be, ok := err.(*BackupError); ok {
		utils.Errorf("[%s] %s: %s", be.Type, be.Op, be.Message)
		if be.Cause != nil {
			utils.Debugf("错误原因: %v", be.Cause)
		}
	} else {
		utils.Errorf("未知错误: %v", err)
	}
}
