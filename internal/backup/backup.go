package backup

import (
	"context"
	"errors"
	"time"
)

// BackupType 定义备份类型
// 各数据库备份类型支持情况：
// - Oracle: full, incremental, differential（需开启归档模式）
// - MSSQL: full（差异备份和事务日志备份需手动执行）
// - MySQL: full, logical（通过 mysqldump 实现）
// - PostgreSQL: full, logical, physical（physical 为目录格式备份）
type BackupType string

const (
	BackupFull         BackupType = "full"         // 全量备份（所有数据库支持）
	BackupIncremental  BackupType = "incremental"  // 增量备份（仅 Oracle 支持）
	BackupDifferential BackupType = "differential" // 差异备份（仅 Oracle 支持）
	BackupLogical      BackupType = "logical"      // 逻辑备份（MySQL/PostgreSQL 支持）
	BackupPhysical     BackupType = "physical"     // 物理备份（仅 PostgreSQL 支持）
)

// BackupOptions 备份操作的可选参数
type BackupOptions struct {
	Type             BackupType        // 备份类型
	Parallelism      int               // 并行度（Oracle/PostgreSQL 支持）
	Compression      bool              // 是否压缩（Oracle/PostgreSQL 支持）
	CompressionLevel int               // 压缩级别 1-9
	Encryption       bool              // 是否加密（仅 Oracle 支持）
	EncryptionKey    string            // 加密密钥
	TargetPath       string            // 备份存储路径（若为空，使用默认）
	ArchiveLogDest   string            // 归档日志目标路径（仅 Oracle 支持）
	ExtraParams      map[string]string // 数据库特定参数
	Timeout          time.Duration     // 超时时间
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
	BackupID       string    // 必选 - 备份集ID（BS Key）
	CompletionTime time.Time // 必选 - 备份完成时间

	BackupType string // 可选 - 备份类型：FULL, INCREMENTAL, ARCHIVELOG，Oracle/MSSQL使用
	Status     string // 可选 - 备份状态：AVAILABLE, EXPIRED, DELETED，Oracle使用真实值
	Size       int64  // 可选 - 备份大小（字节），Oracle不支持
	Tag        string // 可选 - 备份标签，Oracle/MSSQL使用
	DeviceType string // 可选 - 设备类型：DISK, SBT，Oracle使用真实值
	BackupPath string // 可选 - 备份文件路径，MSSQL/MySQL使用
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
	ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error)

	// DeleteBackup 删除指定备份（按备份ID或时间点）
	// identifier 可以是备份集ID、时间戳字符串（RFC3339）或备份标签
	DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error

	// ValidateBackup 验证备份文件完整性（Oracle/MSSQL 支持，MySQL/PostgreSQL 有限支持）
	ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error

	// GetBackupInfo 获取备份文件元信息（如备份时间、内容）
	GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error)

	// RegisterBackup 将指定路径的备份文件注册到备份目录库（仅 Oracle/MSSQL 支持）
	RegisterBackup(ctx context.Context, backupPath string) error

	// UnregisterBackup 从备份目录库中移除指定备份（仅 Oracle/MSSQL 支持）
	UnregisterBackup(ctx context.Context, backupID string) error

	// VerifyBackupStatus 检查备份文件的状态并更新备份目录库（仅 Oracle/MSSQL 支持）
	VerifyBackupStatus(ctx context.Context) error

	// DeleteInvalidBackups 删除无效的备份记录（仅 Oracle/MSSQL 支持）
	DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error

	// DeleteAllBackups 删除所有备份
	DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error

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
	case "mysql":
		return NewMySQLBackup(config)
	case "oracle":
		return NewOracleBackup(config)
	case "mssql":
		return NewMSSQLBackup(config)
	case "postgresql":
		return NewPostgreSQLBackup(config)
	default:
		return nil, errors.New("不支持的数据库类型: " + config.Type)
	}
}
