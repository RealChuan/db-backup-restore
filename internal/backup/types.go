package backup

import (
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/config"
)

// BackupMode 定义备份模式（增量策略）
type BackupMode string

const (
	BackupModeFull         BackupMode = "full"         // 全量备份
	BackupModeIncremental  BackupMode = "incremental"  // 增量备份（仅 Oracle 支持）
	BackupModeDifferential BackupMode = "differential" // 差异备份（仅 Oracle 支持）
)

// BackupType 定义备份类型（技术方式）
type BackupType string

const (
	BackupTypeLogical  BackupType = "logical"  // 逻辑备份（导出SQL文件，MySQL/PostgreSQL支持）
	BackupTypePhysical BackupType = "physical" // 物理备份（复制数据文件，MySQL/PostgreSQL支持）
)

// BackupOptions 备份操作的可选参数
type BackupOptions struct {
	Mode              BackupMode        // 备份模式（增量策略）
	Type              BackupType        // 备份类型（技术方式）
	ParallelWorkers   int               // 并行度（Oracle/PostgreSQL 支持）
	EnableCompression bool              // 是否压缩（Oracle/PostgreSQL 支持）
	CompressionLevel  int               // 压缩级别 1-9
	Encryption        bool              // 是否加密（仅 Oracle 支持）
	EncryptionKey     string            // 加密密钥
	TargetPath        string            // 备份存储路径（若为空，使用默认）
	ArchiveLogDest    string            // 归档日志目标路径（仅 Oracle 支持）
	ExtraParams       map[string]string // 数据库特定参数
	Timeout           time.Duration     // 超时时间
}

// BackupResult 备份操作返回结果
type BackupResult struct {
	Success    bool              // 是否成功
	BackupFile string            // 备份文件/目录路径
	BackupSize int64             // 备份大小（字节）
	Duration   time.Duration     // 耗时
	StartTime  time.Time         // 开始时间
	EndTime    time.Time         // 结束时间
	Metadata   map[string]string // 其他元数据（如 LSN、SCN、备份集ID）
}

// RestoreOptions 还原操作可选参数
type RestoreOptions struct {
	TargetDatabaseName  string            // 目标数据库名（默认与原库名相同）
	Overwrite           bool              // 是否覆盖现有数据库
	RecoveryPointInTime time.Time         // 时间点恢复（指定要还原到的具体时间）
	BackupID            string            // 备份集ID（若指定，则还原该备份集，优先级高于 PointInTime）
	BackupIdentifier    string            // 备份标识符（Oracle: 标签名, MSSQL/MySQL/PostgreSQL: 备份文件路径）
	BackupType          BackupType        // 备份类型（logical/physical）
	ExtraParams         map[string]string // 数据库特定参数
	Timeout             time.Duration     // 超时时间
}

// RestoreResult 还原操作返回结果
type RestoreResult struct {
	Success       bool          // 是否成功
	Duration      time.Duration // 耗时
	RestoredToSCN string        // 还原到的SCN
}

// BackupInfo 备份元信息
type BackupInfo struct {
	BackupID       string    // 必选 - 备份集ID（BS Key）
	CompletionTime time.Time // 必选 - 备份完成时间

	BackupType string // 可选 - 备份类型：FULL, INCREMENTAL, ARCHIVELOG，Oracle/MSSQL使用
	Status     string // 可选 - 备份状态：AVAILABLE, EXPIRED, DELETED，Oracle使用真实值
	Size       int64  // 可选 - 备份大小（字节），Oracle不支持
	Tag        string // 可选 - 备份标签，Oracle/MSSQL使用
	DeviceType string // 可选 - 设备类型：DISK, SBT，Oracle使用真实值
	BackupPath string // 可选 - 备份文件路径，MSSQL/MySQL使用
}

// ProgressCallback 进度回调函数
type ProgressCallback func(percent float64, message string)

// DBConfig 数据库连接配置（类型别名，实际定义在 config 包）
type DBConfig = config.DBConfig

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

// OutputFormat 输出格式
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text"
	OutputFormatJSON OutputFormat = "json"
)

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
