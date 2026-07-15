package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
)

// DamengBackup 实现 DatabaseBackup 接口，针对达梦数据库
type DamengBackup struct {
	BaseBackup
	dmHome     string   // DM_HOME 安装目录
	dmInstance string   // 达梦实例名
	binPath    string   // 工具目录 (DM_HOME/bin)
	env        []string // 环境变量
	dexpPath   string   // dexp 命令路径
	dimpPath   string   // dimp 命令路径
	dmrmanPath string   // dmrman 命令路径
	disqlPath  string   // disql 命令路径
	fsManager  *FileSystemBackupManager
}

// NewDamengBackup 创建达梦备份实例
func NewDamengBackup(config *DBConfig) (*DamengBackup, error) {
	if config.Type != DBTypeDameng {
		return nil, errors.New("config.Type 必须是 dameng")
	}

	dmHome := os.Getenv("DM_HOME")
	if val := config.GetExtraTyped().DamengHome(); val != "" {
		dmHome = val
	}
	if dmHome == "" {
		return nil, errors.New("未设置 DM_HOME，请在 Extra 中提供")
	}

	// 校验密码是否包含会导致 USERID 格式解析失败的特殊字符
	if err := validateDamengPassword(config.Password); err != nil {
		return nil, err
	}

	dmInstance := config.GetExtraTyped().DamengInstance()
	if dmInstance == "" {
		dmInstance = config.Database
	}

	binPath := filepath.Join(dmHome, "bin")
	env := []string{
		fmt.Sprintf("DM_HOME=%s", dmHome),
		fmt.Sprintf("PATH=%s%c%s", binPath, os.PathListSeparator, os.Getenv("PATH")),
	}

	dexpPath := fileutil.AddExeExt(filepath.Join(binPath, "dexp"))
	dimpPath := fileutil.AddExeExt(filepath.Join(binPath, "dimp"))
	dmrmanPath := fileutil.AddExeExt(filepath.Join(binPath, "dmrman"))
	disqlPath := fileutil.AddExeExt(filepath.Join(binPath, "disql"))

	return &DamengBackup{
		BaseBackup: BaseBackup{config: config},
		dmHome:     dmHome,
		dmInstance: dmInstance,
		binPath:    binPath,
		env:        env,
		dexpPath:   dexpPath,
		dimpPath:   dimpPath,
		dmrmanPath: dmrmanPath,
		disqlPath:  disqlPath,
		fsManager: NewFileSystemBackupManager("",
			WithLogicalGlob("*.dmp"),
			WithPhysicalGlobs("dm_full_*", "dm_incr_*", "dm_arch_*"),
			WithLogFileSuffixes(".log", ".restore.log")),
	}, nil
}

// Backup 执行达梦备份（根据类型调用不同实现）
func (d *DamengBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := fileutil.EnsureDir(backupDir); err != nil {
		return nil, err
	}

	switch opts.Type {
	case BackupTypePhysical:
		return d.backupPhysical(ctx, backupDir, opts, callback)
	case BackupTypeLogical:
		return d.backupLogical(ctx, backupDir, callback)
	default:
		return nil, errors.New("达梦仅支持 logical 和 physical 备份类型")
	}
}

// Restore 执行达梦还原（根据备份类型调用不同实现）
func (d *DamengBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	switch opts.BackupType {
	case BackupTypePhysical:
		return d.restorePhysical(ctx, opts, callback)
	case BackupTypeLogical:
		return d.restoreLogical(ctx, opts, callback)
	default:
		// 尝试根据文件扩展名自动判断
		if strings.HasSuffix(opts.BackupIdentifier, ".dmp") {
			return d.restoreLogical(ctx, opts, callback)
		}
		if info, err := os.Stat(opts.BackupIdentifier); err == nil && info.IsDir() {
			return d.restorePhysical(ctx, opts, callback)
		}
		return d.restoreLogical(ctx, opts, callback)
	}
}

// ListBackups 列出所有备份（委托给 FileSystemBackupManager）
func (d *DamengBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	return d.fsManager.ListBackups(ctx, d.getBackupDir(opts))
}

// DeleteBackup 删除指定备份（委托给 FileSystemBackupManager）
func (d *DamengBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	return d.fsManager.DeleteBackup(ctx, identifier, d.getBackupDir(opts))
}

// GetBackupInfo 获取指定备份的详细信息（委托给 FileSystemBackupManager）
func (d *DamengBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	return d.fsManager.GetBackupInfo(ctx, backupID, d.getBackupDir(opts))
}

// DeleteAllBackups 删除所有备份（委托给 FileSystemBackupManager）
func (d *DamengBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	return d.fsManager.DeleteAllBackups(ctx, d.getBackupDir(opts))
}

// buildConnectionString 构建 dexp/dimp 连接串（密码部分在日志中脱敏）
func (d *DamengBackup) buildConnectionString() string {
	return fmt.Sprintf("%s/%s@%s:%d", d.config.User, d.config.Password, d.config.Host, d.config.Port)
}

// buildConnectionStringMasked 构建脱敏连接串用于日志
func (d *DamengBackup) buildConnectionStringMasked() string {
	return fmt.Sprintf("%s/***@%s:%d", d.config.User, d.config.Host, d.config.Port)
}

// buildDisqlArgs 构建 disql 命令参数
func (d *DamengBackup) buildDisqlArgs() []string {
	return []string{d.buildConnectionString()}
}

// execDump 执行 dexp 逻辑导出命令
func (d *DamengBackup) execDump(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, d.dexpPath, args...)
	return runCapture(ctx, "dexp", cmd, withEnv(d.env), withGBKConversion())
}

// execRestore 执行 dimp 逻辑还原命令
func (d *DamengBackup) execRestore(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, d.dimpPath, args...)
	return runCapture(ctx, "dimp", cmd, withEnv(d.env), withGBKConversion())
}

// execSQL 通过 disql 执行 SQL 语句
func (d *DamengBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "disql", "script", MaskScript(sqlStatement))
	args := d.buildDisqlArgs()
	cmd := exec.CommandContext(ctx, d.disqlPath, args...)
	sqlInput := sqlStatement + "\nEXIT;\n"
	return runCapture(ctx, "disql", cmd, withEnv(d.env), withStdin(strings.NewReader(sqlInput)), withGBKConversion())
}

// execSQLStreaming 通过 disql 执行 SQL 语句，支持实时流式输出
func (d *DamengBackup) execSQLStreaming(ctx context.Context, sqlStatement string, onLine shellexec.LineCallback) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "disql", "script", MaskScript(sqlStatement))
	args := d.buildDisqlArgs()
	cmd := exec.CommandContext(ctx, d.disqlPath, args...)
	sqlInput := sqlStatement + "\nEXIT;\n"
	return runStreaming(ctx, "disql", cmd, withEnv(d.env), withStdin(strings.NewReader(sqlInput)), withGBKConversion(), withStreaming(onLine))
}

// execDmrman 执行 dmrman 脱机脚本命令（用于脱机还原/验证）
// 使用 CTLFILE 方式执行：先将脚本写入临时文件，再通过 dmrman CTLFILE=<file> 执行
// 这样避免交互模式 stdin 管道导致的挂起问题
func (d *DamengBackup) execDmrman(ctx context.Context, script string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "dmrman", "script", MaskScript(script))

	// 将脚本写入临时文件
	tmpFile, err := os.CreateTemp("", "dmrman_script_*.txt")
	if err != nil {
		return "", fmt.Errorf("创建 dmrman 临时脚本文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(script + "\nEXIT;\n"); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("写入 dmrman 临时脚本文件失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("关闭 dmrman 临时脚本文件失败: %w", err)
	}

	cmd := exec.CommandContext(ctx, d.dmrmanPath, fmt.Sprintf("CTLFILE=%s", tmpPath))
	return runStreaming(ctx, "dmrman", cmd, withEnv(d.env), withGBKConversion())
}

// registerDamengDriver 注册达梦驱动
func registerDamengDriver() error {
	return RegisterDriver(DriverMetadata{
		Name:                 DBTypeDameng,
		Version:              versionXML,
		Description:          "达梦数据库备份驱动，支持 dexp 逻辑备份、disql 联机物理备份和 dmrman 脱机还原（全量/增量/归档）",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, actionInfo, actionDeleteAll, actionValidate, actionVerifyStatus},
		SupportedBackupTypes: []BackupType{BackupTypeLogical, BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewDamengBackup(config)
	})
}
