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

	"db-backup-restore/pkg/utils"
)

// OracleBackup 实现 DatabaseBackup 接口，针对 Oracle 数据库
type OracleBackup struct {
	BaseBackup
	oracleHome string   // ORACLE_HOME 路径
	oracleSid  string   // ORACLE_SID
	binPath    string   // 工具目录（通常是 $ORACLE_HOME/bin）
	env        []string // 环境变量
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
	if val, ok := config.Extra["ORACLE_SID"]; ok && val != "" {
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
	}, nil
}

// execSQL 执行 SQL 命令（通过 sqlplus）
func (o *OracleBackup) execSQL(ctx context.Context, sqlText string) (string, error) {
	// 打印 SQL 命令
	utils.Infof("\n========== SQL 命令开始 ==========\n%s\n========== SQL 命令结束 ==========", sqlText)

	cmd := exec.CommandContext(ctx, "sqlplus", "-S", "/", "as", "sysdba")
	cmd.Env = append(os.Environ(), o.env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	go func() {
		defer stdin.Close()
		fmt.Fprintln(stdin, sqlText)
		fmt.Fprintln(stdin, "EXIT;")
	}()
	output, err := utils.ExecCommand(ctx, cmd)
	// 打印 SQL 输出，无论成功还是失败
	utils.Infof("\n========== SQL 执行输出开始 ==========\n%s\n========== SQL 执行输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("sqlplus 执行失败: %w", err)
	}
	return output, nil
}

// execRman 执行 RMAN 命令
func (o *OracleBackup) execRman(ctx context.Context, rmanScript string) (string, error) {
	// 打印 RMAN 命令
	utils.Infof("\n========== RMAN 命令开始 ==========\n%s\n========== RMAN 命令结束 ==========", rmanScript)

	cmd := exec.CommandContext(ctx, "rman", "target", "/")
	cmd.Env = append(os.Environ(), o.env...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	go func() {
		defer stdin.Close()
		fmt.Fprintln(stdin, rmanScript)
		fmt.Fprintln(stdin, "EXIT;")
	}()
	output, err := utils.ExecCommand(ctx, cmd)
	// 打印 RMAN 输出，无论成功还是失败
	utils.Infof("\n========== RMAN 执行输出开始 ==========\n%s\n========== RMAN 执行输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("rman 执行失败: %w", err)
	}
	return output, nil
}

// isArchiveLogMode 检查数据库是否处于归档模式
func (o *OracleBackup) isArchiveLogMode(ctx context.Context) (bool, error) {
	output, err := o.execSQL(ctx, "SELECT LOG_MODE FROM V$DATABASE;")
	if err != nil {
		return false, err
	}
	re := regexp.MustCompile(`(ARCHIVELOG|NOARCHIVELOG)`)
	match := re.FindString(output)
	return match == "ARCHIVELOG", nil
}

// EnableArchiveLogMode 将数据库切换到 ARCHIVELOG 模式（需要重启数据库）
func (o *OracleBackup) EnableArchiveLogMode(ctx context.Context, archiveDest string) error {
	archived, err := o.isArchiveLogMode(ctx)
	if err != nil {
		return err
	}
	if archived {
		return nil
	}

	// 创建归档日志目录（如果不存在）
	if archiveDest != "" {
		if err := os.MkdirAll(archiveDest, 0755); err != nil {
			return fmt.Errorf("创建归档日志目录失败: %w", err)
		}
	}

	sqlScript := fmt.Sprintf(`
SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE ARCHIVELOG;
%s
ALTER DATABASE OPEN;
`, o.setArchiveDestSQL(archiveDest))
	output, err := o.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("启用归档模式失败: %w, 输出: %s", err, output)
	}
	return nil
}

func (o *OracleBackup) setArchiveDestSQL(dest string) string {
	if dest == "" {
		return ""
	}
	return fmt.Sprintf("ALTER SYSTEM SET LOG_ARCHIVE_DEST_1='LOCATION=%s' SCOPE=BOTH;", dest)
}

// Backup 执行 Oracle 备份
func (o *OracleBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}
	archived, err := o.isArchiveLogMode(ctx)
	if err != nil {
		result.Error = err
		return result, err
	}
	if !archived {
		if callback != nil {
			callback(0, "数据库未开启归档模式，正在尝试启用...")
		}
		if err := o.EnableArchiveLogMode(ctx, opts.ArchiveLogDest); err != nil {
			result.Error = fmt.Errorf("启用归档模式失败: %w", err)
			return result, result.Error
		}
		if callback != nil {
			callback(0, "归档模式已启用")
		}
	}
	backupDir := opts.TargetPath
	if backupDir == "" {
		backupDir = filepath.Join(o.oracleHome, "database")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		result.Error = err
		return result, err
	}

	if err := o.configureAutobackupFormat(backupDir); err != nil {
		result.Error = fmt.Errorf("配置控制文件自动备份格式失败: %w", err)
		return result, result.Error
	}

	rmanScript := o.buildBackupScript(opts, backupDir)

	if callback != nil {
		callback(0, "开始执行 RMAN 备份...")
	}
	output, err := o.execRman(ctx, rmanScript)
	if err != nil {
		result.Error = fmt.Errorf("RMAN 备份失败: %w, 输出: %s", err, output)
		return result, result.Error
	}
	if callback != nil {
		callback(100, "RMAN 备份完成")
	}
	backupFiles, size, bsKey, err := o.parseBackupOutput(output, backupDir)
	if err != nil {
		result.Error = err
	} else {
		result.BackupFile = strings.Join(backupFiles, ",")
		result.BackupSize = size
		if bsKey != "" {
			result.Metadata["backup_set_key"] = bsKey
		}
	}
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = (err == nil)
	return result, nil
}

func (o *OracleBackup) configureAutobackupFormat(backupDir string) error {
	script := fmt.Sprintf("CONFIGURE CONTROLFILE AUTOBACKUP FORMAT FOR DEVICE TYPE DISK TO '%s';",
		filepath.Join(backupDir, "cf_%F"))
	_, err := o.execRman(context.Background(), script)
	return err
}

func (o *OracleBackup) buildBackupScript(opts BackupOptions, backupDir string) string {
	// 逻辑备份不支持 RMAN，返回空脚本并记录错误
	if opts.Type == BackupLogical {
		utils.Errorf("RMAN 不支持逻辑备份 (BackupLogical)，请使用 expdp 等工具单独实现")
		return ""
	}

	var script strings.Builder
	script.WriteString("RUN {\n")

	// 定义 FORMAT 模板
	datafileFormat := filepath.Join(backupDir, "%U")
	cfFormat := filepath.Join(backupDir, "cf_%U")
	spfileFormat := filepath.Join(backupDir, "spfile_%U")

	// 分配通道并指定 FORMAT，确保所有备份片段都输出到指定目录
	if opts.Parallelism > 1 {
		for i := 1; i <= opts.Parallelism; i++ {
			script.WriteString(fmt.Sprintf("  ALLOCATE CHANNEL ch%d DEVICE TYPE DISK FORMAT '%s';\n", i, datafileFormat))
		}
	} else {
		script.WriteString(fmt.Sprintf("  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '%s';\n", datafileFormat))
	}

	// 配置压缩（可选）
	if opts.Compression {
		script.WriteString("  CONFIGURE COMPRESSION ALGORITHM 'MEDIUM';\n")
	}

	// 配置加密（可选）
	if opts.Encryption {
		script.WriteString("  CONFIGURE ENCRYPTION FOR DATABASE ON;\n")
		if opts.EncryptionKey != "" {
			script.WriteString(fmt.Sprintf("  SET ENCRYPTION IDENTIFIED BY '%s' ONLY;\n", opts.EncryptionKey))
		}
	}

	// 根据备份类型生成对应的 BACKUP 命令
	switch opts.Type {
	case BackupFull, BackupPhysical:
		script.WriteString(fmt.Sprintf("  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", datafileFormat))
	case BackupIncremental:
		script.WriteString(fmt.Sprintf("  BACKUP INCREMENTAL LEVEL 1 DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", datafileFormat))
	case BackupDifferential:
		script.WriteString(fmt.Sprintf("  BACKUP INCREMENTAL LEVEL 1 CUMULATIVE DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", datafileFormat))
	default:
		// 理论上不会走到这里，但兜底处理
		script.WriteString(fmt.Sprintf("  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", datafileFormat))
	}

	// 备份控制文件和 SPFILE（逻辑备份除外，但上面已拦截）
	script.WriteString(fmt.Sprintf("  BACKUP CURRENT CONTROLFILE FORMAT '%s';\n", cfFormat))
	script.WriteString(fmt.Sprintf("  BACKUP SPFILE FORMAT '%s';\n", spfileFormat))

	// 释放通道
	if opts.Parallelism > 1 {
		for i := 1; i <= opts.Parallelism; i++ {
			script.WriteString(fmt.Sprintf("  RELEASE CHANNEL ch%d;\n", i))
		}
	} else {
		script.WriteString("  RELEASE CHANNEL ch1;\n")
	}

	script.WriteString("}\n")
	// 删除过时的备份（根据保留策略）
	script.WriteString("DELETE NOPROMPT OBSOLETE;\n")
	return script.String()
}

// parseBackupOutput 扩展返回备份集ID
func (o *OracleBackup) parseBackupOutput(output, backupDir string) ([]string, int64, string, error) {
	var files []string
	var totalSize int64
	var bsKey string
	scanner := bufio.NewScanner(strings.NewReader(output))
	handleRe := regexp.MustCompile(`片段句柄 = (\S+)`)
	bsKeyRe := regexp.MustCompile(`备份集键值 (\d+)`)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := handleRe.FindStringSubmatch(line); len(matches) > 1 {
			handle := matches[1]
			if !filepath.IsAbs(handle) {
				handle = filepath.Join(backupDir, filepath.Base(handle))
			}
			files = append(files, handle)
			if info, err := os.Stat(handle); err == nil {
				totalSize += info.Size()
			}
		}
		if matches := bsKeyRe.FindStringSubmatch(line); len(matches) > 1 && bsKey == "" {
			bsKey = matches[1]
		}
	}
	return files, totalSize, bsKey, nil
}

// Restore 支持按备份ID或时间点还原
func (o *OracleBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}
	if callback != nil {
		callback(0, "开始执行 RMAN 还原...")
	}
	rmanScript := o.buildRestoreScript(opts)

	output, err := o.execRman(ctx, rmanScript)
	if err != nil {
		result.Error = fmt.Errorf("RMAN 还原失败: %w, 输出: %s", err, output)
		return result, result.Error
	}
	if callback != nil {
		callback(100, "还原完成")
	}
	// 尝试从输出中提取还原到的SCN
	scnRe := regexp.MustCompile(`恢复至 SCN (\d+)`)
	if matches := scnRe.FindStringSubmatch(output); len(matches) > 1 {
		result.RestoredToSCN = matches[1]
	}
	result.Duration = time.Since(startTime)
	result.Success = true
	return result, nil
}

// buildRestoreScript 根据选项生成还原脚本，支持按时间点或标签还原
func (o *OracleBackup) buildRestoreScript(opts RestoreOptions) string {
	var script strings.Builder
	script.WriteString("RUN {\n")
	// 先关闭数据库并置于mount状态
	script.WriteString("  SHUTDOWN IMMEDIATE;\n")
	script.WriteString("  STARTUP MOUNT;\n")

	// 按标签还原
	if opts.BackupTag != "" {
		script.WriteString(fmt.Sprintf("  RESTORE DATABASE FROM TAG='%s';\n", opts.BackupTag))
		script.WriteString("  RECOVER DATABASE;\n")
	} else if !opts.PointInTime.IsZero() {
		// 按时间点还原
		timeStr := opts.PointInTime.Format("2006-01-02 15:04:05")
		script.WriteString(fmt.Sprintf("  SET UNTIL TIME \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr))
		script.WriteString("  RESTORE DATABASE;\n")
		script.WriteString("  RECOVER DATABASE;\n")
	} else {
		// 默认还原最新的完整备份（通用方式）
		script.WriteString("  RESTORE DATABASE;\n")
		script.WriteString("  RECOVER DATABASE;\n")
	}

	// 只有在按时间点还原时才使用RESETLOGS
	if !opts.PointInTime.IsZero() {
		script.WriteString("  ALTER DATABASE OPEN RESETLOGS;\n")
	} else {
		script.WriteString("  ALTER DATABASE OPEN;\n")
	}
	script.WriteString("}\n")
	return script.String()
}

// ListBackups 列出所有备份（按完成时间排序）
func (o *OracleBackup) ListBackups(ctx context.Context) ([]BackupInfo, error) {
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
	// 示例行: " 136     B  F  A DISK        07-4月 -26 1       1       NO         TAG20260407T165358"
	re := regexp.MustCompile(`^\s*(\d+)\s+([A-Z])\s+([A-Z])\s+([A-Z])\s+(\S+)\s+(\d{2}-\S+\s+-\d{2})\s+\d+\s+\d+\s+\S+\s+TAG(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})`)
	for scanner.Scan() {
		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 13 {
			// 解析日期时间
			year := matches[7]
			month := matches[8]
			day := matches[9]
			hour := matches[10]
			minute := matches[11]
			second := matches[12]
			timeStr := fmt.Sprintf("%s-%s-%s %s:%s:%s", year, month, day, hour, minute, second)
			t, _ := time.Parse("2006-01-02 15:04:05", timeStr)

			// 映射备份类型
			backupType := ""
			switch matches[3] {
			case "F":
				backupType = "FULL"
			case "I":
				backupType = "INCREMENTAL"
			case "A":
				backupType = "ARCHIVELOG"
			default:
				backupType = string(matches[3])
			}

			// 映射状态
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
				BackupType:     backupType,
				DeviceType:     matches[5],
				Status:         status,
				BackupTime:     t,
				CompletionTime: t,
				Size:           0, // 实际输出中没有大小信息
				Tag:            fmt.Sprintf("TAG%s%s%sT%s%s%s", year, month, day, hour, minute, second),
			}
			backups = append(backups, info)
		}
	}
	return backups, nil
}

// DeleteBackup 删除指定备份（按备份集ID或时间点）
func (o *OracleBackup) DeleteBackup(ctx context.Context, identifier string) error {
	var rmanCmd string
	// 判断 identifier 是否为时间格式
	var t time.Time
	var err error

	// 尝试解析RFC3339格式（带时区）
	if t, err = time.Parse(time.RFC3339, identifier); err == nil {
		// 按时间删除：删除完成时间早于指定时间的备份
		timeStr := t.Format("2006-01-02 15:04:05")
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUP COMPLETED BEFORE \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr)
	} else if t, err = time.Parse("2006-01-02T15:04:05", identifier); err == nil {
		// 尝试解析不带时区的格式
		timeStr := t.Format("2006-01-02 15:04:05")
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUP COMPLETED BEFORE \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr)
	} else {
		// 假设是备份集ID
		rmanCmd = fmt.Sprintf("DELETE NOPROMPT BACKUPSET %s;\n", identifier)
	}
	output, err := o.execRman(ctx, rmanCmd)
	if err != nil {
		return fmt.Errorf("删除备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// ValidateBackup 验证备份有效性
func (o *OracleBackup) ValidateBackup(ctx context.Context, backupID string) error {
	script := "RESTORE DATABASE VALIDATE CHECK LOGICAL;"
	if backupID != "" {
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
func (o *OracleBackup) GetBackupInfo(ctx context.Context, backupID string) (map[string]string, error) {
	var script string
	if backupID != "" {
		// 获取指定备份集的详细信息
		script = fmt.Sprintf("LIST BACKUPSET %s;", backupID)
	} else {
		// 获取所有备份的摘要信息
		script = "LIST BACKUP OF DATABASE SUMMARY;"
	}
	output, err := o.execRman(ctx, script)
	if err != nil {
		return nil, err
	}
	info := make(map[string]string)
	info["raw_output"] = output
	// 可以进一步解析备份集信息
	return info, nil
}

// Close 释放资源
func (o *OracleBackup) Close() error {
	return nil
}

// RegisterBackup 将指定路径的备份文件注册到备份目录库
func (o *OracleBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	script := fmt.Sprintf("CATALOG START WITH '%s';", backupPath)
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("注册备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (o *OracleBackup) UnregisterBackup(ctx context.Context, backupID string) error {
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
func (o *OracleBackup) DeleteInvalidBackups(ctx context.Context) error {
	script := "DELETE NOPROMPT EXPIRED BACKUP;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("删除无效备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// DeleteAllBackups 删除所有备份
func (o *OracleBackup) DeleteAllBackups(ctx context.Context) error {
	script := "DELETE NOPROMPT BACKUP;"
	output, err := o.execRman(ctx, script)
	if err != nil {
		return fmt.Errorf("删除所有备份失败: %w, 输出: %s", err, output)
	}
	return nil
}
