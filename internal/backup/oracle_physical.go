package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// isArchiveLogMode 检查数据库是否处于归档模式
func (o *OracleBackup) isArchiveLogMode(ctx context.Context) (bool, error) {
	output, err := o.execSQL(ctx, "SELECT LOG_MODE FROM V$DATABASE;")
	if err != nil {
		return false, err
	}
	match := o.archiveLogModeRegex.FindString(output)
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
		if err := fileutil.EnsureDir(archiveDest); err != nil {
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

// DisableArchiveLogMode 将 Oracle 数据库从 ARCHIVELOG 模式切换为 NOARCHIVELOG 模式
// 需要关闭数据库、以 MOUNT 方式启动、关闭归档后重新 OPEN
func (o *OracleBackup) DisableArchiveLogMode(ctx context.Context) error {
	archived, err := o.isArchiveLogMode(ctx)
	if err != nil {
		return err
	}
	if !archived {
		return nil
	}

	sqlScript := `SHUTDOWN IMMEDIATE;
STARTUP MOUNT;
ALTER DATABASE NOARCHIVELOG;
ALTER DATABASE OPEN;`

	logging.DebugCtx(ctx, "正在关闭 Oracle 归档模式")
	output, err := o.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("关闭归档模式失败: %w, 输出: %s", err, output)
	}

	// 验证归档模式已关闭
	archived, err = o.isArchiveLogMode(ctx)
	if err != nil {
		return fmt.Errorf("验证归档模式失败: %w", err)
	}
	if archived {
		return errors.New("关闭归档模式后验证失败，数据库仍处于归档模式")
	}

	logging.DebugCtx(ctx, "Oracle 归档模式已成功关闭")
	return nil
}

func (o *OracleBackup) configureAutoBackupFormat(ctx context.Context, backupDir string) error {
	script := fmt.Sprintf("CONFIGURE CONTROLFILE AUTOBACKUP FORMAT FOR DEVICE TYPE DISK TO '%s';",
		filepath.Join(backupDir, "cf_%F"))
	_, err := o.execRman(ctx, script)
	return err
}

// buildBackupScript 构建 Oracle RMAN 备份脚本
// 安全设计：
//   - 对备份路径进行校验和清理，防止路径遍历攻击
//   - 使用 escapeOracleRMANString 对路径进行转义，防止 RMAN 脚本注入
//   - 使用 %U 格式生成唯一的备份文件名
//
// 支持模式：
//   - full: 全量备份 (BACKUP DATABASE)
//   - level0: Level 0 增量基础备份 (BACKUP INCREMENTAL LEVEL 0)，作为 Level 1 增量策略的基础
//   - incremental: 差异增量备份 (BACKUP INCREMENTAL LEVEL 1)
//   - differential: 累积增量备份 (BACKUP INCREMENTAL LEVEL 1 CUMULATIVE)
//   - archive: 独立归档日志备份 (BACKUP ARCHIVELOG ALL)
func (o *OracleBackup) buildBackupScript(opts BackupOptions, backupDir string) (string, error) {
	// 独立归档日志备份使用单独的脚本
	if opts.Mode == BackupModeArchive {
		return o.buildArchiveOnlyBackupScript(opts, backupDir)
	}

	var script strings.Builder
	script.WriteString("RUN {\n")

	// 安全校验：备份路径
	cleanDir, err := sanitizeBackupPath(backupDir)
	if err != nil {
		return "", fmt.Errorf("备份路径校验失败: %w", err)
	}

	// 构建备份文件格式
	datafileFormat := filepath.Join(cleanDir, "%U")      // 数据文件备份格式
	cfFormat := filepath.Join(cleanDir, "cf_%U")         // 控制文件备份格式
	spfileFormat := filepath.Join(cleanDir, "spfile_%U") // 参数文件备份格式

	// 安全转义：防止 RMAN 脚本注入
	safeDatafileFormat := escapeOracleRMANString(datafileFormat)
	safeCfFormat := escapeOracleRMANString(cfFormat)
	safeSpfileFormat := escapeOracleRMANString(spfileFormat)

	if opts.ParallelWorkers > 1 {
		for i := 1; i <= opts.ParallelWorkers; i++ {
			fmt.Fprintf(&script, "  ALLOCATE CHANNEL ch%d DEVICE TYPE DISK FORMAT '%s';\n", i, safeDatafileFormat)
		}
	} else {
		fmt.Fprintf(&script, "  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '%s';\n", safeDatafileFormat)
	}

	if opts.EnableCompression {
		level := mapCompressionLevelToOracle(opts.CompressionLevel)
		fmt.Fprintf(&script, "  CONFIGURE COMPRESSION ALGORITHM '%s';\n", level)
	}

	if opts.Encryption {
		script.WriteString("  CONFIGURE ENCRYPTION FOR DATABASE ON;\n")
		if opts.EncryptionKey != "" {
			safeKey := escapeOracleRMANString(opts.EncryptionKey)
			fmt.Fprintf(&script, "  SET ENCRYPTION IDENTIFIED BY '%s' ONLY;\n", safeKey)
		}
	}

	switch opts.Mode {
	case BackupModeFull:
		fmt.Fprintf(&script, "  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", safeDatafileFormat)
	case BackupModeLevel0:
		fmt.Fprintf(&script, "  BACKUP INCREMENTAL LEVEL 0 DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", safeDatafileFormat)
	case BackupModeIncremental:
		fmt.Fprintf(&script, "  BACKUP INCREMENTAL LEVEL 1 DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", safeDatafileFormat)
	case BackupModeDifferential:
		fmt.Fprintf(&script, "  BACKUP INCREMENTAL LEVEL 1 CUMULATIVE DATABASE PLUS ARCHIVELOG DELETE INPU FORMAT '%s';\n", safeDatafileFormat)
	default:
		fmt.Fprintf(&script, "  BACKUP DATABASE PLUS ARCHIVELOG DELETE INPUT FORMAT '%s';\n", safeDatafileFormat)
	}

	fmt.Fprintf(&script, "  BACKUP CURRENT CONTROLFILE FORMAT '%s';\n", safeCfFormat)
	fmt.Fprintf(&script, "  BACKUP SPFILE FORMAT '%s';\n", safeSpfileFormat)

	if opts.ParallelWorkers > 1 {
		for i := 1; i <= opts.ParallelWorkers; i++ {
			fmt.Fprintf(&script, "  RELEASE CHANNEL ch%d;\n", i)
		}
	} else {
		script.WriteString("  RELEASE CHANNEL ch1;\n")
	}

	script.WriteString("}\n")
	script.WriteString("DELETE NOPROMPT OBSOLETE;\n")
	return script.String(), nil
}

// buildArchiveOnlyBackupScript 构建独立归档日志备份脚本（不含数据文件备份）
// 语法：BACKUP ARCHIVELOG ALL [DELETE INPUT] FORMAT 'path';
func (o *OracleBackup) buildArchiveOnlyBackupScript(opts BackupOptions, backupDir string) (string, error) {
	var script strings.Builder
	script.WriteString("RUN {\n")

	cleanDir, err := sanitizeBackupPath(backupDir)
	if err != nil {
		return "", fmt.Errorf("备份路径校验失败: %w", err)
	}
	archFormat := filepath.Join(cleanDir, "arch_%U")
	safeArchFormat := escapeOracleRMANString(archFormat)

	if opts.ParallelWorkers > 1 {
		for i := 1; i <= opts.ParallelWorkers; i++ {
			fmt.Fprintf(&script, "  ALLOCATE CHANNEL ch%d DEVICE TYPE DISK FORMAT '%s';\n", i, safeArchFormat)
		}
	} else {
		fmt.Fprintf(&script, "  ALLOCATE CHANNEL ch1 DEVICE TYPE DISK FORMAT '%s';\n", safeArchFormat)
	}

	fmt.Fprintf(&script, "  BACKUP ARCHIVELOG ALL DELETE INPUT FORMAT '%s';\n", safeArchFormat)

	if opts.ParallelWorkers > 1 {
		for i := 1; i <= opts.ParallelWorkers; i++ {
			fmt.Fprintf(&script, "  RELEASE CHANNEL ch%d;\n", i)
		}
	} else {
		script.WriteString("  RELEASE CHANNEL ch1;\n")
	}

	script.WriteString("}\n")
	return script.String(), nil
}

// parseBackupOutput

// parseBackupOutput 扩展返回备份集ID
func (o *OracleBackup) parseBackupOutput(output, backupDir string) ([]string, int64, string, error) {
	var files []string
	var totalSize int64
	var backupSetKey string
	scanner := bufio.NewScanner(strings.NewReader(output))

	handleRegexes := o.handleRegexes
	backupSetKeyRegexes := o.backupSetKeyRegexes

	for scanner.Scan() {
		line := scanner.Text()

		for _, regex := range handleRegexes {
			if matches := regex.FindStringSubmatch(line); len(matches) > 1 {
				handle := matches[1]
				if !filepath.IsAbs(handle) {
					handle = filepath.Join(backupDir, filepath.Base(handle))
				}
				files = append(files, handle)
				if info, err := os.Stat(handle); err == nil {
					totalSize += info.Size()
				}
				break
			}
		}

		for _, regex := range backupSetKeyRegexes {
			if matches := regex.FindStringSubmatch(line); len(matches) > 1 && backupSetKey == "" {
				backupSetKey = matches[1]
				break
			}
		}
	}
	return files, totalSize, backupSetKey, nil
}

// buildFullRestoreScript 构建全量还原脚本
// 执行流程：SHUTDOWN → STARTUP MOUNT → [SET UNTIL] → RESTORE DATABASE [FROM TAG] → RECOVER DATABASE → OPEN
// 支持组合：TAG + PITR/SCN 可同时指定，TAG 选择备份集，PITR/SCN 指定恢复目标点
func (o *OracleBackup) buildFullRestoreScript(opts RestoreOptions) (string, error) {
	var script strings.Builder
	script.WriteString("RUN {\n")
	script.WriteString("  SHUTDOWN IMMEDIATE;\n")
	script.WriteString("  STARTUP MOUNT;\n")

	// SET UNTIL（PITR 或 SCN）必须先于 RESTORE
	if err := o.appendUntilClause(&script, opts); err != nil {
		return "", err
	}

	// RESTORE DATABASE（可选 FROM TAG）
	o.appendRestoreDatabase(&script, opts)

	script.WriteString("  RECOVER DATABASE;\n")

	if !opts.RecoveryPointInTime.IsZero() || opts.RecoverySCN != "" {
		script.WriteString("  ALTER DATABASE OPEN RESETLOGS;\n")
	} else {
		script.WriteString("  ALTER DATABASE OPEN;\n")
	}

	script.WriteString("}\n")
	return script.String(), nil
}

// buildIncrementalRestoreScript 构建增量还原脚本
// 与全量还原类似，但支持 NOREDO 选项跳过归档日志应用
// NOREDO 适用于：备库同步、归档日志不可用、仅应用增量备份的场景
// 支持组合：TAG + PITR/SCN 可同时指定
func (o *OracleBackup) buildIncrementalRestoreScript(opts RestoreOptions) (string, error) {
	var script strings.Builder
	script.WriteString("RUN {\n")
	script.WriteString("  SHUTDOWN IMMEDIATE;\n")
	script.WriteString("  STARTUP MOUNT;\n")

	// SET UNTIL（PITR 或 SCN）必须先于 RESTORE
	if err := o.appendUntilClause(&script, opts); err != nil {
		return "", err
	}

	// RESTORE DATABASE（可选 FROM TAG）
	o.appendRestoreDatabase(&script, opts)

	if opts.NoRedo {
		script.WriteString("  RECOVER DATABASE NOREDO;\n")
	} else {
		script.WriteString("  RECOVER DATABASE;\n")
	}

	script.WriteString("  ALTER DATABASE OPEN RESETLOGS;\n")

	script.WriteString("}\n")
	return script.String(), nil
}

// buildArchiveRestoreScript 构建归档还原脚本
// 执行流程：SHUTDOWN → STARTUP MOUNT → [SET UNTIL] → RESTORE DATABASE [FROM TAG] → RESTORE ARCHIVELOG → RECOVER DATABASE → OPEN RESETLOGS
// 支持按 SCN、按序列号范围还原归档日志
// 支持组合：TAG + PITR/SCN 可同时指定
func (o *OracleBackup) buildArchiveRestoreScript(opts RestoreOptions) (string, error) {
	var script strings.Builder
	script.WriteString("RUN {\n")
	script.WriteString("  SHUTDOWN IMMEDIATE;\n")
	script.WriteString("  STARTUP MOUNT;\n")

	// SET UNTIL（PITR 或 SCN）必须先于 RESTORE
	if err := o.appendUntilClause(&script, opts); err != nil {
		return "", err
	}

	// RESTORE DATABASE（可选 FROM TAG）
	o.appendRestoreDatabase(&script, opts)

	// RESTORE ARCHIVELOG
	switch {
	case opts.ArchiveFromSeq != "" && opts.ArchiveUntilSeq != "":
		fromSeq, fromErr := sanitizeSeq(opts.ArchiveFromSeq)
		untilSeq, untilErr := sanitizeSeq(opts.ArchiveUntilSeq)
		if fromErr != nil || untilErr != nil {
			return "", fmt.Errorf("归档序列号校验失败: from=%q until=%q", opts.ArchiveFromSeq, opts.ArchiveUntilSeq)
		}
		fmt.Fprintf(&script, "  RESTORE ARCHIVELOG FROM SEQUENCE %d UNTIL SEQUENCE %d;\n", fromSeq, untilSeq)
	case opts.ArchiveFromSeq != "":
		fromSeq, err := sanitizeSeq(opts.ArchiveFromSeq)
		if err != nil {
			return "", fmt.Errorf("归档序列号校验失败: %w", err)
		}
		fmt.Fprintf(&script, "  RESTORE ARCHIVELOG FROM SEQUENCE %d;\n", fromSeq)
	default:
		script.WriteString("  RESTORE ARCHIVELOG ALL;\n")
	}

	script.WriteString("  RECOVER DATABASE;\n")
	script.WriteString("  ALTER DATABASE OPEN RESETLOGS;\n")

	script.WriteString("}\n")
	return script.String(), nil
}

// buildControlFileRestoreScript 构建控制文件还原脚本
// 执行流程：STARTUP NOMOUNT → RESTORE CONTROLFILE FROM AUTOBACKUP → ALTER DATABASE MOUNT → RESTORE DATABASE → RECOVER DATABASE → OPEN RESETLOGS
// 适用于控制文件丢失的灾难恢复场景
func (o *OracleBackup) buildControlFileRestoreScript(opts RestoreOptions) (string, error) {
	var script strings.Builder
	script.WriteString("RUN {\n")
	script.WriteString("  STARTUP NOMOUNT;\n")

	if opts.BackupIdentifier != "" {
		// 从指定备份集还原控制文件
		safeTag := escapeOracleRMANString(opts.BackupIdentifier)
		fmt.Fprintf(&script, "  RESTORE CONTROLFILE FROM TAG='%s';\n", safeTag)
	} else {
		script.WriteString("  RESTORE CONTROLFILE FROM AUTOBACKUP;\n")
	}

	script.WriteString("  ALTER DATABASE MOUNT;\n")

	// SET UNTIL（PITR 或 SCN）
	if err := o.appendUntilClause(&script, opts); err != nil {
		return "", err
	}

	script.WriteString("  RESTORE DATABASE;\n")
	script.WriteString("  RECOVER DATABASE;\n")
	script.WriteString("  ALTER DATABASE OPEN RESETLOGS;\n")

	script.WriteString("}\n")
	return script.String(), nil
}

// appendUntilClause 向脚本追加 SET UNTIL 子句（PITR 或 SCN，二选一）
// 注意：SET UNTIL 必须在 RESTORE 之前
func (o *OracleBackup) appendUntilClause(script *strings.Builder, opts RestoreOptions) error {
	switch {
	case !opts.RecoveryPointInTime.IsZero():
		timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
		fmt.Fprintf(script, "  SET UNTIL TIME \"TO_DATE('%s', 'YYYY-MM-DD HH24:MI:SS')\";\n", timeStr)
	case opts.RecoverySCN != "":
		safeSCN, err := sanitizeSCN(opts.RecoverySCN)
		if err != nil {
			return fmt.Errorf("SCN 校验失败: %w", err)
		}
		fmt.Fprintf(script, "  SET UNTIL SCN %d;\n", safeSCN)
	}
	return nil
}

// appendRestoreDatabase 向脚本追加 RESTORE DATABASE 命令（可选 FROM TAG）
func (o *OracleBackup) appendRestoreDatabase(script *strings.Builder, opts RestoreOptions) {
	if opts.BackupIdentifier != "" {
		safeTag := escapeOracleRMANString(opts.BackupIdentifier)
		fmt.Fprintf(script, "  RESTORE DATABASE FROM TAG='%s';\n", safeTag)
	} else {
		script.WriteString("  RESTORE DATABASE;\n")
	}
}

// buildCrosscheckCleanupScript 构建 RMAN 幽灵对象清理脚本
// 依次执行：
//  1. CROSSCHECK BACKUP       — 标记物理文件已缺失的备份集为 EXPIRED
//  2. CROSSCHECK ARCHIVELOG   — 标记物理文件已缺失的归档日志为 EXPIRED
//  3. DELETE NOPROMPT EXPIRED BACKUP     — 删除 EXPIRED 状态的备份记录
//  4. DELETE NOPROMPT EXPIRED ARCHIVELOG — 删除 EXPIRED 状态的归档日志记录
//  5. DELETE NOPROMPT OBSOLETE           — 删除按保留策略已过期的备份
func (o *OracleBackup) buildCrosscheckCleanupScript() string {
	return `CROSSCHECK BACKUP;
CROSSCHECK ARCHIVELOG ALL;
DELETE NOPROMPT EXPIRED BACKUP;
DELETE NOPROMPT EXPIRED ARCHIVELOG ALL;
DELETE NOPROMPT OBSOLETE;`
}

// crosscheckAndCleanup 执行 RMAN 交叉核对并清理幽灵对象
// 清理失败仅记录警告，不返回错误，避免阻塞后续备份/还原流程
func (o *OracleBackup) crosscheckAndCleanup(ctx context.Context) {
	logging.InfoCtx(ctx, "开始执行 Oracle 幽灵对象清理")
	script := o.buildCrosscheckCleanupScript()
	output, err := o.execRman(ctx, script)
	if err != nil {
		logging.WarnCtx(ctx, "Oracle 幽灵对象清理失败，继续执行后续操作", "error", err)
		logging.DebugCtx(ctx, "Oracle 幽灵对象清理失败输出", "output", output)
		return
	}
	logging.InfoCtx(ctx, "Oracle 幽灵对象清理完成")
	logging.DebugCtx(ctx, "Oracle 幽灵对象清理输出", "output", output)
}

// mapCompressionLevelToOracle 将整数压缩级别映射到 Oracle RMAN 的压缩算法名称。
// 映射规则：1-3=LOW, 4-6=MEDIUM, 7-9=HIGH, 0(默认)=MEDIUM。
func mapCompressionLevelToOracle(level int) string {
	switch {
	case level >= 1 && level <= 3:
		return "LOW"
	case level >= 4 && level <= 6:
		return "MEDIUM"
	case level >= 7 && level <= 9:
		return "HIGH"
	default:
		return "MEDIUM"
	}
}
