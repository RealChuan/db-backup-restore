package backup

import (
	"context"
	"errors"
	"time"
)

// BackupType 定义备份类型
type BackupType string

const (
	BackupFull         BackupType = "full"         // 全量备份
	BackupIncremental  BackupType = "incremental"  // 增量备份
	BackupDifferential BackupType = "differential" // 差异备份
	BackupLogical      BackupType = "logical"      // 逻辑备份
	BackupPhysical     BackupType = "physical"     // 物理备份
)

// BackupOptions 备份操作的可选参数
type BackupOptions struct {
	Type             BackupType        // 备份类型
	Parallelism      int               // 并行度（若支持）
	Compression      bool              // 是否压缩
	CompressionLevel int               // 压缩级别 1-9
	Encryption       bool              // 是否加密
	EncryptionKey    string            // 加密密钥
	TargetPath       string            // 备份存储路径（若为空，使用默认）
	ArchiveLogDest   string            // 归档日志目标路径（Oracle专用）
	ExtraParams      map[string]string // 数据库特定参数
	Timeout          time.Duration     // 超时时间
	Description      string            // 备份描述/标签
}

// BackupResult 备份操作返回结果
type BackupResult struct {
	Success    bool              // 是否成功
	BackupFile string            // 备份文件/目录路径
	BackupSize int64             // 备份大小（字节）
	Duration   time.Duration     // 耗时
	StartTime  time.Time         // 开始时间
	EndTime    time.Time         // 结束时间
	Error      error             // 错误信息
	Metadata   map[string]string // 其他元数据（如 LSN、SCN、备份集ID）
}

// RestoreOptions 还原操作可选参数
type RestoreOptions struct {
	TargetDB    string            // 目标数据库名（默认与原库名相同）
	Overwrite   bool              // 是否覆盖现有数据库
	PointInTime time.Time         // 时间点恢复（指定要还原到的具体时间）
	BackupID    string            // 备份集ID（若指定，则还原该备份集，优先级高于 PointInTime）
	BackupTag   string            // 备份标签（可选）
	ExtraParams map[string]string // 数据库特定参数
	Timeout     time.Duration     // 超时时间
}

// RestoreResult 还原操作返回结果
type RestoreResult struct {
	Success       bool          // 是否成功
	Duration      time.Duration // 耗时
	RestoredToSCN string        // 还原到的SCN（若适用）
	Error         error
}

// BackupInfo 备份元信息
type BackupInfo struct {
	BackupID       string    // 备份集ID（BS Key）
	BackupType     string    // 备份类型：FULL, INCREMENTAL, ARCHIVELOG
	BackupTime     time.Time // 备份完成时间
	StartTime      time.Time // 备份开始时间
	Size           int64     // 备份大小（字节）
	Status         string    // AVAILABLE, EXPIRED, DELETED
	Tag            string    // 备份标签
	DeviceType     string    // DISK, SBT
	CompletionTime time.Time
	BackupPath     string // 备份文件路径
}

// ProgressCallback 进度回调函数（可选）
type ProgressCallback func(percent float64, message string)

// DatabaseBackup 抽象接口
type DatabaseBackup interface {
	// Backup 执行备份操作，支持上下文取消和进度回调
	Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error)

	// Restore 执行还原操作，支持按备份ID或时间点还原
	Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error)

	// ListBackups 列出所有可用的备份（按时间排序）
	ListBackups(ctx context.Context) ([]BackupInfo, error)

	// DeleteBackup 删除指定备份（按备份ID或时间点）
	// identifier 可以是备份集ID、时间戳字符串（RFC3339）或备份标签
	DeleteBackup(ctx context.Context, identifier string) error

	// ValidateBackup 验证备份文件完整性（可选）
	ValidateBackup(ctx context.Context, backupID string) error

	// GetBackupInfo 获取备份文件元信息（如备份时间、内容）
	GetBackupInfo(ctx context.Context, backupID string) (map[string]string, error)

	// RegisterBackup 将指定路径的备份文件注册到备份目录库
	RegisterBackup(ctx context.Context, backupPath string) error

	// UnregisterBackup 从备份目录库中移除指定备份
	UnregisterBackup(ctx context.Context, backupID string) error

	// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
	VerifyBackupStatus(ctx context.Context) error

	// DeleteInvalidBackups 删除无效的备份记录
	DeleteInvalidBackups(ctx context.Context) error

	// DeleteAllBackups 删除所有备份
	DeleteAllBackups(ctx context.Context) error

	// Close 释放资源（如数据库连接）
	Close() error
}

// BaseBackup 提供公共字段和方法（具体实现可嵌入）
type BaseBackup struct {
	config *DBConfig
}

type DBConfig struct {
	Type     string // 数据库类型：mysql, postgresql, oracle, mssql, dameng, kingbase, opengauss, gaussdb
	Host     string
	Port     int
	User     string
	Password string
	Database string // 默认数据库
	SSLMode  string
	Extra    map[string]string // 其他连接参数
}

// NewBackup 根据数据库类型创建备份实例
func NewBackup(config *DBConfig) (DatabaseBackup, error) {
	switch config.Type {
	case "oracle":
		return NewOracleBackup(config)
	case "mssql":
		return NewMSSQLBackup(config)
	default:
		return nil, errors.New("不支持的数据库类型: " + config.Type)
	}
}
