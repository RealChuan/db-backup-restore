package backup

import (
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/config"
)

// BackupMode 定义备份模式（增量策略）
type BackupMode string

const (
	BackupModeFull         BackupMode = "full"
	BackupModeIncremental  BackupMode = "incremental"
	BackupModeDifferential BackupMode = "differential"
)

// BackupType 定义备份类型（技术方式）
type BackupType string

const (
	BackupTypeLogical  BackupType = "logical"
	BackupTypePhysical BackupType = "physical"
)

// BackupOptions 备份操作的可选参数
type BackupOptions struct {
	Mode              BackupMode
	Type              BackupType
	ParallelWorkers   int
	EnableCompression bool
	CompressionLevel  int
	Encryption        bool
	EncryptionKey     string
	TargetPath        string
	ArchiveLogDest    string
	ExtraParams       map[string]string
	Timeout           time.Duration
}

// BackupResult 备份操作返回结果
type BackupResult struct {
	BackupFile string
	BackupSize int64
	Duration   time.Duration
	StartTime  time.Time
	EndTime    time.Time
	Metadata   map[string]string
}

// BackupInfo 备份元信息
type BackupInfo struct {
	BackupID       string
	CompletionTime time.Time
	BackupType     string
	Status         string
	Size           int64
	Tag            string
	DeviceType     string
	BackupPath     string
}

// ProgressCallback 进度回调函数
type ProgressCallback func(percent float64, message string)

// OutputFormat 输出格式
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

// DBConfig 数据库连接配置
type DBConfig = config.DBConfig

// RestoreOptions 还原操作可选参数
type RestoreOptions struct {
	TargetDatabaseName  string
	Overwrite           bool
	RecoveryPointInTime time.Time
	BackupID            string
	BackupIdentifier    string
	BackupType          BackupType
	ExtraParams         map[string]string
	Timeout             time.Duration
}

// RestoreResult 还原操作返回结果
type RestoreResult struct {
	Duration       time.Duration
	RestoredToSCN  string
	RestoredFiles  []string
	RestoredSize   int64
	TargetDatabase string
}

// ParseBackupMode 将字符串解析为 BackupMode
func ParseBackupMode(s string) (BackupMode, error) {
	switch s {
	case "full":
		return BackupModeFull, nil
	case "incremental":
		return BackupModeIncremental, nil
	case "differential":
		return BackupModeDifferential, nil
	default:
		return "", fmt.Errorf("invalid backup mode: %s", s)
	}
}

// ParseBackupType 将字符串解析为 BackupType
func ParseBackupType(s string) (BackupType, error) {
	switch s {
	case "logical":
		return BackupTypeLogical, nil
	case "physical":
		return BackupTypePhysical, nil
	default:
		return "", fmt.Errorf("invalid backup type: %s", s)
	}
}

// ParseOutputFormat 解析输出格式
func ParseOutputFormat(s string) (OutputFormat, error) {
	switch s {
	case "text", "":
		return OutputFormatText, nil
	case "json":
		return OutputFormatJSON, nil
	default:
		return "", fmt.Errorf("invalid output format: %s", s)
	}
}
