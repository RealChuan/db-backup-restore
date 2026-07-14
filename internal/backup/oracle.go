package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// OracleBackup 实现 DatabaseBackup 接口，针对 Oracle 数据库
type OracleBackup struct {
	BaseBackup
	oracleHome string   // ORACLE_HOME 路径
	oracleSid  string   // ORACLE_SID
	binPath    string   // 工具目录（通常是 $ORACLE_HOME/bin）
	env        []string // 环境变量
	// 预编译正则表达式
	handleRegexes       []*regexp.Regexp
	backupSetKeyRegexes []*regexp.Regexp
	scnRegex            *regexp.Regexp
	backupListRegex     *regexp.Regexp
	archiveLogModeRegex *regexp.Regexp
}

// NewOracleBackup 创建 Oracle 备份实例，需提供连接配置
func NewOracleBackup(config *DBConfig) (*OracleBackup, error) {
	if config.Type != "oracle" {
		return nil, errors.New("config.Type 必须是 oracle")
	}
	oracleHome := os.Getenv("ORACLE_HOME")
	if val, ok := config.Extra["ORACLE_HOME"]; ok && val != "" {
		oracleHome = val
	}
	if oracleHome == "" {
		return nil, errors.New("未设置 ORACLE_HOME，请在 Extra 中提供")
	}
	oracleSid := os.Getenv("ORACLE_SID")
	if val := config.GetExtraTyped().OracleSID(); val != "" {
		oracleSid = val
	}
	if oracleSid == "" {
		oracleSid = config.Database
	}
	if oracleSid == "" {
		return nil, errors.New("未设置 ORACLE_SID，请在 Extra 或 Database 中提供")
	}
	binPath := filepath.Join(oracleHome, "bin")
	env := []string{
		fmt.Sprintf("ORACLE_HOME=%s", oracleHome),
		fmt.Sprintf("ORACLE_SID=%s", oracleSid),
		fmt.Sprintf("PATH=%s%c%s", binPath, os.PathListSeparator, os.Getenv("PATH")),
	}
	return &OracleBackup{
		BaseBackup: BaseBackup{config: config},
		oracleHome: oracleHome,
		oracleSid:  oracleSid,
		binPath:    binPath,
		env:        env,
		// 预编译正则表达式
		handleRegexes: []*regexp.Regexp{
			regexp.MustCompile(`片段句柄 = (\S+)`),         // 中文
			regexp.MustCompile(`piece handle = (\S+)`), // 英文
		},
		backupSetKeyRegexes: []*regexp.Regexp{
			regexp.MustCompile(`备份集键值 (\d+)`),          // 中文
			regexp.MustCompile(`backup set key (\d+)`), // 英文
		},
		scnRegex:            regexp.MustCompile(`恢复至 SCN (\d+)`),
		backupListRegex:     regexp.MustCompile(`^\s*(\d+)\s+([A-Z])\s+([A-Z])\s+([A-Z])\s+(\S+)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+\d+\s+\d+\s+(YES|NO)\s+TAG(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})`),
		archiveLogModeRegex: regexp.MustCompile(`(ARCHIVELOG|NOARCHIVELOG)`),
	}, nil
}

// execSQL 执行 SQL 命令（通过 sqlplus）
func (o *OracleBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "sqlplus", "script", MaskScript(sqlStatement))
	cmd := exec.CommandContext(ctx, "sqlplus", "-S", "/", "as", "sysdba")
	sqlInput := sqlStatement + "\nEXIT;\n"
	return runCapture(ctx, "sqlplus", cmd, withEnv(o.env), withStdin(strings.NewReader(sqlInput)), withGBKConversion())
}

// execRman 执行 RMAN 命令
func (o *OracleBackup) execRman(ctx context.Context, rmanScript string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "rman", "script", MaskScript(rmanScript))
	env := make([]string, len(o.env), len(o.env)+1)
	copy(env, o.env)
	env = append(env, "NLS_DATE_FORMAT=YYYY-MM-DD HH24:MI:SS")
	cmd := exec.CommandContext(ctx, "rman", "target", "/")
	rmanInput := rmanScript + "\nEXIT;\n"
	return runStreaming(ctx, "rman", cmd, withEnv(env), withStdin(strings.NewReader(rmanInput)), withGBKConversion())
}

// Backup 执行 Oracle 备份
//
//nolint:gocyclo // 复杂度来自多步备份流程中的回调检查和错误分支，重构会降低可读性
func (o *OracleBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if opts.Type == BackupTypeLogical {
		return nil, errors.New("oracle RMAN 不支持逻辑备份，请使用 expdp 等工具或指定 --backup-type physical")
	}

	archived, err := o.isArchiveLogMode(ctx)
	if err != nil {
		return nil, err
	}
	if !archived {
		if callback != nil {
			callback(0, "数据库未开启归档模式，正在尝试启用...")
		}
		if err := o.EnableArchiveLogMode(ctx, opts.ArchiveLogDest); err != nil {
			return nil, fmt.Errorf("启用归档模式失败: %w", err)
		}
		if callback != nil {
			callback(0, "归档模式已启用")
		}
	}

	// 幽灵对象清理：在备份前自动执行 RMAN 交叉核对并清理过期/无效记录
	if o.config.GetExtraTyped().AutoGhostCleanup() {
		o.crosscheckAndCleanup(ctx)
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := fileutil.EnsureDir(backupDir); err != nil {
		return nil, err
	}

	if err := o.configureAutoBackupFormat(ctx, backupDir); err != nil {
		return nil, fmt.Errorf("配置控制文件自动备份格式失败: %w", err)
	}

	rmanScript, err := o.buildBackupScript(opts, backupDir)
	if err != nil {
		return nil, fmt.Errorf("构建备份脚本失败: %w", err)
	}

	if callback != nil {
		callback(0, "开始执行 RMAN 备份...")
	}
	output, err := o.execRman(ctx, rmanScript)
	if err != nil {
		return nil, fmt.Errorf("RMAN 备份失败: %w", err)
	}
	if callback != nil {
		callback(100, "RMAN 备份完成")
	}
	backupFiles, size, bsKey, err := o.parseBackupOutput(output, backupDir)
	if err != nil {
		return nil, err
	}
	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = size
	if bsKey != "" {
		result.Metadata["backup_set_key"] = bsKey
	}
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	return result, nil
}

// Restore 支持按备份ID、时间点、SCN 还原，并支持增量还原、归档还原和控制文件还原模式
func (o *OracleBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	restoreMode := opts.RestoreMode
	if restoreMode == "" {
		restoreMode = RestoreModeFull
	}

	// 幽灵对象清理：在还原前自动执行 RMAN 交叉核对并清理过期/无效记录
	o.crosscheckAndCleanup(ctx)

	if callback != nil {
		callback(0, "开始执行 RMAN 还原...")
	}

	var rmanScript string
	var scriptErr error
	switch restoreMode {
	case RestoreModeIncremental:
		return nil, errors.New("oracle 不支持 incremental 还原模式，RMAN 的 RESTORE DATABASE 自动处理增量链；如需跳过归档日志，请使用 --restore-mode full --no-redo")
	case RestoreModeFull:
		rmanScript, scriptErr = o.buildFullRestoreScript(opts)
	case RestoreModeArchive:
		rmanScript, scriptErr = o.buildArchiveRestoreScript(opts)
	case RestoreModeControlFile:
		rmanScript, scriptErr = o.buildControlFileRestoreScript(opts)
	default:
		rmanScript, scriptErr = o.buildFullRestoreScript(opts)
	}
	if scriptErr != nil {
		return nil, fmt.Errorf("构建还原脚本失败: %w", scriptErr)
	}

	output, err := o.execRman(ctx, rmanScript)
	if err != nil {
		return nil, fmt.Errorf("RMAN 还原失败: %w", err)
	}
	if callback != nil {
		callback(100, "还原完成")
	}

	// 尝试从输出中提取还原到的 SCN
	scnRegex := o.scnRegex
	if matches := scnRegex.FindStringSubmatch(output); len(matches) > 1 {
		result.RestoredToSCN = matches[1]
	}
	result.Duration = time.Since(startTime)
	return result, nil
}

// ListBackups 列出所有备份（按完成时间排序）
func (o *OracleBackup) ListBackups(ctx context.Context, _ ...BackupOptions) ([]BackupInfo, error) {
	script := "LIST BACKUP SUMMARY;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("列出备份失败: %w", err)
	}
	return o.parseBackupList(output)
}

// parseBackupList 解析 RMAN LIST BACKUP SUMMARY 输出
func (o *OracleBackup) parseBackupList(output string) ([]BackupInfo, error) {
	var backups []BackupInfo
	scanner := bufio.NewScanner(strings.NewReader(output))
	re := o.backupListRegex

	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 14 {
			completionTime, _ := time.Parse("2006-01-02 15:04:05", matches[6])

			// Oracle RMAN 备份类型码: F=Full, I=Incremental, A=ArchiveLog
			// Oracle 仅支持物理备份，备份类型统一为 physical
			var backupMode string
			switch matches[3] {
			case "F":
				backupMode = string(BackupModeFull)
			case "I":
				backupMode = string(BackupModeIncremental)
			case "A":
				backupMode = string(BackupModeArchive)
			default:
				backupMode = strings.ToLower(string(matches[3]))
			}

			status := ""
			switch matches[4] {
			case "A":
				status = "AVAILABLE"
			case "X":
				status = "EXPIRED"
			case "D":
				status = "DELETED"
			default:
				status = string(matches[4])
			}

			info := BackupInfo{
				BackupID:       matches[1],
				BackupType:     string(BackupTypePhysical),
				BackupMode:     backupMode,
				DeviceType:     matches[5],
				Status:         status,
				CompletionTime: completionTime,
				Tag:            fmt.Sprintf("TAG%s%s%sT%s%s%s", matches[8], matches[9], matches[10], matches[11], matches[12], matches[13]),
			}
			backups = append(backups, info)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取输出失败: %w", err)
	}
	return backups, nil
}

// DeleteBackup 删除指定备份（按备份集ID或时间点）
func (o *OracleBackup) DeleteBackup(ctx context.Context, identifier string, _ ...BackupOptions) error {
	var rmanCmd string
	var targetTime time.Time
	var err error

	if targetTime, err = time.Parse(time.RFC3339, identifier); err == nil {
		timeStr := targetTime.Format("2006-01-02 15:04:05")
		if _, perr := sanitizeDateLiteral(timeStr); perr != nil {
			return fmt.Errorf("invalid date format for delete: %w", perr)
		}
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUP COMPLETED BEFORE \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr)
	} else if targetTime, err = time.Parse("2006-01-02T15:04:05", identifier); err == nil {
		timeStr := targetTime.Format("2006-01-02 15:04:05")
		if _, perr := sanitizeDateLiteral(timeStr); perr != nil {
			return fmt.Errorf("invalid date format for delete: %w", perr)
		}
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUP COMPLETED BEFORE \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr)
	} else {
		bsid, perr := sanitizePositiveInt(identifier)
		if perr != nil {
			return fmt.Errorf("invalid backup identifier: must be a valid date or positive integer backup set ID: %w", perr)
		}
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUPSET %d;\n", bsid)
	}
	output, err := o.execRman(ctx, rmanCmd)
	if err != nil {
		return fmt.Errorf("删除备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// ValidateBackup 验证备份有效性
func (o *OracleBackup) ValidateBackup(ctx context.Context, backupID string, _ ...BackupOptions) error {
	script := "RESTORE DATABASE VALIDATE CHECK LOGICAL;"
	if backupID != "" {
		if err := sanitizeOracleBackupID(backupID); err != nil {
			return fmt.Errorf("invalid backup ID: %w", err)
		}
		script = fmt.Sprintf("VALIDATE BACKUPSET %s;", backupID)
	}
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("验证失败: %w, 输出: %s", err, output)
	}
	if strings.Contains(output, "RMAN-") && strings.Contains(output, "error") {
		return errors.New("备份验证发现错误")
	}
	return nil
}

// GetBackupInfo 获取指定备份的详细信息
func (o *OracleBackup) GetBackupInfo(ctx context.Context, backupID string, _ ...BackupOptions) (map[string]string, error) {
	var script string
	if backupID != "" {
		if err := sanitizeOracleBackupID(backupID); err != nil {
			return nil, fmt.Errorf("invalid backup ID: %w", err)
		}
		script = fmt.Sprintf("LIST BACKUPSET %s;", backupID)
	} else {
		script = "LIST BACKUP OF DATABASE SUMMARY;"
	}
	output, err := o.execRman(ctx, script)
	if err != nil {
		return nil, err
	}
	info := make(map[string]string)
	info["raw_output"] = output
	return info, nil
}

// RegisterBackup 将指定路径的备份文件注册到备份目录库
func (o *OracleBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	if err := sanitizeOracleBackupPath(backupPath); err != nil {
		return fmt.Errorf("invalid backup path: %w", err)
	}
	safePath := escapeOracleRMANString(backupPath)
	script := fmt.Sprintf("CATALOG START WITH '%s';", safePath)
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("注册备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (o *OracleBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	if err := sanitizeOracleBackupID(backupID); err != nil {
		return fmt.Errorf("invalid backup ID: %w", err)
	}
	script := fmt.Sprintf("CHANGE BACKUPSET %s UNCATALOG;", backupID)
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("移除备份记录失败: %w, 输出: %s", err, output)
	}
	return nil
}

// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
func (o *OracleBackup) VerifyBackupStatus(ctx context.Context) error {
	script := "CROSSCHECK BACKUP;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("检查备份状态失败: %w, 输出: %s", err, output)
	}
	return nil
}

// DeleteInvalidBackups 删除无效的备份记录
func (o *OracleBackup) DeleteInvalidBackups(ctx context.Context, _ ...BackupOptions) error {
	script := "DELETE NOPROMPT EXPIRED BACKUP;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("删除无效备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// DeleteAllBackups 删除所有备份
func (o *OracleBackup) DeleteAllBackups(ctx context.Context, _ ...BackupOptions) error {
	script := "DELETE NOPROMPT BACKUP;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("删除所有备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// registerOracleDriver 注册 Oracle 驱动
func registerOracleDriver() error {
	return RegisterDriver(DriverMetadata{
		Name:                 DBTypeOracle,
		Version:              versionXML,
		Description:          "Oracle 数据库备份驱动，支持 RMAN 物理备份/还原（全量、Level 0、差异增量、累积增量、归档、控制文件）",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, actionValidate, actionInfo, "register", "unregister", actionVerifyStatus, "delete-invalid", actionDeleteAll},
		SupportedBackupTypes: []BackupType{BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewOracleBackup(config)
	})
}
