package backup

import (
	"fmt"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/config"
)

// BackupMode 定义备份模式（增量策略 + 归档备份）
type BackupMode string

const (
	BackupModeFull         BackupMode = "full"         // 全量备份（所有数据库支持; 达梦: BACKUP DATABASE FULL; Oracle: BACKUP DATABASE）
	BackupModeIncremental  BackupMode = "incremental"  // 差异增量备份（达梦: BACKUP DATABASE INCREMENT; Oracle: BACKUP INCREMENTAL LEVEL 1）
	BackupModeDifferential BackupMode = "differential" // 累积增量备份（达梦: BACKUP DATABASE INCREMENT CUMULATIVE; Oracle: BACKUP INCREMENTAL LEVEL 1 CUMULATIVE）
	BackupModeLevel0       BackupMode = "level0"       // Level 0 增量基础备份（仅 Oracle: BACKUP INCREMENTAL LEVEL 0，作为 Level 1 增量策略的基础）
	BackupModeArchive      BackupMode = "archive"      // 独立归档日志备份（达梦: BACKUP ARCHIVELOG; Oracle: BACKUP ARCHIVELOG ALL）
)

// BackupType 定义备份类型（技术方式）
type BackupType string

const (
	BackupTypeLogical  BackupType = "logical"  // 逻辑备份（导出 SQL 文件; MySQL/PostgreSQL/达梦支持）
	BackupTypePhysical BackupType = "physical" // 物理备份（复制数据文件; MySQL/PostgreSQL/Oracle/达梦支持）
)

// RestoreMode 定义还原模式（物理还原的细分策略，Oracle/达梦支持）
type RestoreMode string

const (
	RestoreModeFull        RestoreMode = "full"        // 全量还原（默认，Oracle RMAN 和达梦 dmrman 自动处理增量链）
	RestoreModeArchive     RestoreMode = "archive"     // 归档还原（Oracle: RESTORE ARCHIVELOG; 达梦: RESTORE ARCHIVE LOG）
	RestoreModeControlFile RestoreMode = "controlfile" // 控制文件还原（仅 Oracle: RESTORE CONTROLFILE FROM AUTOBACKUP）
)

// BackupOptions 备份操作的可选参数
type BackupOptions struct {
	Mode            BackupMode        // 备份模式: full/incremental/differential/level0/archive
	Type            BackupType        // 备份类型: logical/physical
	Encryption      bool              // 是否启用加密（物理备份，Oracle/达梦支持）
	EncryptionKey   string            // 加密密钥（需配合 Encryption 使用; Oracle: SET ENCRYPTION; 达梦: IDENTIFIED BY）
	TargetPath      string            // 备份目标路径
	ArchiveLogDest  string            // 归档日志目标路径
	ArchiveFromLSN  string            // 归档备份起始 LSN（仅达梦: BACKUP ARCHIVELOG FROM LSN，配合 archive 模式）
	ArchiveUntilLSN string            // 归档备份结束 LSN（仅达梦: BACKUP ARCHIVELOG TO LSN，配合 archive 模式）
	ExtraParams     map[string]string // 额外参数（MSSQL 使用 database 键指定数据库名）
	Timeout         time.Duration     // 操作超时时间
}

// BackupResult 备份操作返回结果
type BackupResult struct {
	BackupFile string            // 备份文件/目录路径
	BackupSize int64             // 备份大小（字节）
	Duration   time.Duration     // 备份耗时
	StartTime  time.Time         // 备份开始时间
	EndTime    time.Time         // 备份结束时间
	Metadata   map[string]string // 额外元数据（如 backup_set_key）
}

// BackupInfo 备份元信息
type BackupInfo struct {
	BackupID       string    // 备份 ID
	CompletionTime time.Time // 完成时间
	BackupType     string    // 备份类型: logical 或 physical（对应 BackupType 常量）
	BackupMode     string    // 备份模式: full/incremental/differential/level0/archive（对应 BackupMode 常量）
	Status         string    // 备份状态
	Size           int64     // 备份大小
	Tag            string    // 备份标签
	DeviceType     string    // 设备类型
	BackupPath     string    // 备份路径
}

// ProgressCallback 进度回调函数
type ProgressCallback func(percent float64, message string)

// OutputFormat 输出格式
type OutputFormat string

const (
	OutputFormatText OutputFormat = "text" // 文本格式输出
	OutputFormatJSON OutputFormat = "json" // JSON 格式输出
)

// DBConfig 数据库连接配置
type DBConfig = config.DBConfig

// RestoreOptions 还原操作可选参数
type RestoreOptions struct {
	TargetDatabaseName     string            // 还原的目标数据库名（MySQL/PostgreSQL/MSSQL 逻辑还原时指定）
	RemapSchema            string            // 模式映射（仅达梦 dimp: REMAP_SCHEMA=source:target，将源模式数据导入目标模式）
	RecoveryPointInTime    time.Time         // 时间点还原（Oracle/达梦支持，可与 BackupIdentifier 组合）
	RecoverySCN            string            // 按 SCN 还原（仅 Oracle 支持，可与 BackupIdentifier 组合）
	RecoveryLSN            string            // 按 LSN 还原（仅达梦支持，配合 archive 模式使用）
	BackupIdentifier       string            // 备份标识符（Oracle/达梦: TAG 或备份集路径; MySQL/PostgreSQL/MSSQL: 备份文件路径）
	BackupType             BackupType        // 备份类型: logical/physical
	RestoreMode            RestoreMode       // 还原模式: full/archive/controlfile（Oracle/达梦支持）
	NoRedo                 bool              // 还原时是否跳过归档日志，即 NOREDO（仅 Oracle 支持）
	ArchiveFromSeq         string            // 归档还原起始序列号（仅 Oracle 支持，配合 archive 模式使用）
	ArchiveUntilSeq        string            // 归档还原结束序列号（仅 Oracle 支持，配合 archive 模式使用）
	ArchiveLogDest         string            // 归档日志目录路径（Oracle/达梦: 从 BaseBackupDir 自动推导，用于 RECOVER WITH ARCHIVEDIR）
	ExtraParams            map[string]string // 额外参数
	CatalogPath            string            // 备份文件注册路径（仅 Oracle 支持，异机还原时使用 CATALOG START WITH 注册备份）
	AutoRestoreControlFile bool              // 自动恢复控制文件（仅 Oracle 支持，在数据库还原流程中先恢复控制文件）
	Timeout                time.Duration     // 操作超时时间
}

// RestoreResult 还原操作返回结果
type RestoreResult struct {
	Duration       time.Duration // 还原耗时
	RestoredToSCN  string        // 还原到的 SCN（Oracle 使用）
	RestoredFiles  []string      // 已还原的文件列表
	RestoredSize   int64         // 已还原的数据大小
	TargetDatabase string        // 目标数据库名
}

// ParseBackupMode 将字符串解析为 BackupMode
func ParseBackupMode(s string) (BackupMode, error) {
	switch BackupMode(s) {
	case BackupModeFull:
		return BackupModeFull, nil
	case BackupModeIncremental:
		return BackupModeIncremental, nil
	case BackupModeDifferential:
		return BackupModeDifferential, nil
	case BackupModeLevel0:
		return BackupModeLevel0, nil
	case BackupModeArchive:
		return BackupModeArchive, nil
	default:
		return "", fmt.Errorf("invalid backup mode: %s", s)
	}
}

// ParseBackupType 将字符串解析为 BackupType。
// 空字符串默认为 logical，与 ParseRestoreMode/ParseOutputFormat 的"空值即默认"语义一致。
func ParseBackupType(s string) (BackupType, error) {
	switch s {
	case "", "logical":
		return BackupTypeLogical, nil
	case "physical":
		return BackupTypePhysical, nil
	default:
		return "", fmt.Errorf("invalid backup type: %s", s)
	}
}

// ParseRestoreMode 将字符串解析为 RestoreMode
func ParseRestoreMode(s string) (RestoreMode, error) {
	switch s {
	case string(RestoreModeFull), "":
		return RestoreModeFull, nil
	case string(RestoreModeArchive):
		return RestoreModeArchive, nil
	case string(RestoreModeControlFile):
		return RestoreModeControlFile, nil
	default:
		return "", fmt.Errorf("invalid restore mode: %s", s)
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
