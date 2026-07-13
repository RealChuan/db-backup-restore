package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
	"github.com/RealChuan/db-backup-restore/pkg/svcmgmt"
)

// ValidateBackup 验证备份有效性
func (d *DamengBackup) ValidateBackup(ctx context.Context, backupID string, _ ...BackupOptions) error {
	if backupID == "" {
		return errors.New("必须指定备份集路径")
	}
	if err := sanitizeDamengBackupPath(backupID); err != nil {
		return fmt.Errorf("无效的备份路径: %w", err)
	}
	safePath := escapeDamengRMANString(backupID)
	script := fmt.Sprintf(`VALIDATE BACKUPSET '%s'`, safePath)
	output, err := d.execDmrman(ctx, script)
	if err != nil {
		return fmt.Errorf("验证失败: %w, 输出: %s", err, output)
	}
	return nil
}

// VerifyBackupStatus 检查备份状态
func (d *DamengBackup) VerifyBackupStatus(ctx context.Context) error {
	script := "CHECK BACKUPSET;"
	output, err := d.execDmrman(ctx, script)
	if err != nil {
		return fmt.Errorf("检查备份状态失败: %w, 输出: %s", err, output)
	}
	return nil
}

// archModeRegex 预编译正则表达式，匹配 disql 查询 V$DATABASE.ARCH_MODE 的输出格式：
// 行号     ARCH_MODE
// ---------- ---------
// 1          Y
var archModeRegex = regexp.MustCompile(`(?m)^[ \t]*[0-9]+[ \t]+([YN])[ \t]*\n`)

// isArchiveLogMode 检查达梦数据库是否处于归档模式
// 通过 disql 查询 V$DATABASE.ARCH_MODE 字段，Y 表示已开启归档，N 表示未开启
func (d *DamengBackup) isArchiveLogMode(ctx context.Context) (bool, error) {
	output, err := d.execSQL(ctx, "SELECT ARCH_MODE FROM V$DATABASE;")
	if err != nil {
		return false, fmt.Errorf("查询归档模式失败: %w", err)
	}
	matches := archModeRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		// 正则匹配失败时返回错误，避免基于不可靠的字符串包含检查进行猜测
		return false, fmt.Errorf("无法解析归档模式查询结果，disql 输出格式异常: %s", output)
	}
	return matches[1] == "Y", nil
}

// EnableArchiveLogMode 将达梦数据库切换到 ARCHIVELOG 模式
// 需要将数据库置于 MOUNT 状态，开启归档后重新 OPEN
// archDir: 归档日志目录路径，用于 ALTER DATABASE ADD ARCHIVELOG 配置
func (d *DamengBackup) EnableArchiveLogMode(ctx context.Context, archDir string) error {
	archived, err := d.isArchiveLogMode(ctx)
	if err != nil {
		return err
	}
	if archived {
		return nil
	}

	// 创建归档日志目录（如果不存在且已指定）
	if archDir != "" {
		if err := fileutil.EnsureDir(archDir); err != nil {
			return fmt.Errorf("创建归档日志目录失败: %w", err)
		}
	}

	// 构建开启归档的 SQL 脚本
	// 达梦开启归档需在 MOUNT 状态下操作，与 Oracle 类似
	sqlScript := `ALTER DATABASE MOUNT;
ALTER DATABASE NORMAL;
ALTER DATABASE ARCHIVELOG;`
	if archDir != "" {
		safeArchDir := strings.ReplaceAll(archDir, `'`, `''`)
		sqlScript += fmt.Sprintf(
			"\nALTER DATABASE ADD ARCHIVELOG 'DEST=%s, TYPE=LOCAL, FILE_SIZE=2048, SPACE_LIMIT=204800';",
			safeArchDir,
		)
	}
	sqlScript += "\nALTER DATABASE OPEN;"

	logging.InfoCtx(ctx, "正在启用达梦归档模式", "arch_dir", archDir)
	output, err := d.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("启用归档模式失败: %w, 输出: %s", err, output)
	}

	// 验证归档模式已开启
	archived, err = d.isArchiveLogMode(ctx)
	if err != nil {
		return fmt.Errorf("验证归档模式失败: %w", err)
	}
	if !archived {
		return errors.New("启用归档模式后验证失败，数据库仍处于非归档模式")
	}

	logging.InfoCtx(ctx, "达梦归档模式已成功启用")
	return nil
}

// DisableArchiveLogMode 将达梦数据库从 ARCHIVELOG 模式切换为 NOARCHIVELOG 模式
// 需要将数据库置于 MOUNT 状态，关闭归档后重新 OPEN
func (d *DamengBackup) DisableArchiveLogMode(ctx context.Context) error {
	archived, err := d.isArchiveLogMode(ctx)
	if err != nil {
		return err
	}
	if !archived {
		return nil
	}

	sqlScript := `ALTER DATABASE MOUNT;
ALTER DATABASE NORMAL;
ALTER DATABASE NOARCHIVELOG;
ALTER DATABASE OPEN;`

	logging.InfoCtx(ctx, "正在关闭达梦归档模式")
	output, err := d.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("关闭归档模式失败: %w, 输出: %s", err, output)
	}

	// 验证归档模式已关闭
	archived, err = d.isArchiveLogMode(ctx)
	if err != nil {
		return fmt.Errorf("验证归档模式失败: %w", err)
	}
	if archived {
		return errors.New("关闭归档模式后验证失败，数据库仍处于归档模式")
	}

	logging.InfoCtx(ctx, "达梦归档模式已成功关闭")
	return nil
}

// archivePathRegex 预编译正则表达式，匹配 disql 查询 V$ARCHIVE_FILE.ARCH_PATH 的输出格式：
// 行号     ARCH_PATH
// ---------- ------------------------------
// 1          /dmdata/arch/arch_1.log
var archivePathRegex = regexp.MustCompile(`(?m)^[ \t]*[0-9]+[ \t]+(\S+)[ \t]*\n`)

// getValidArchivePaths 查询当前实例归档链中所有合法归档文件的路径
// 通过 disql 查询 V$ARCHIVE_FILE.ARCH_PATH 视图
func (d *DamengBackup) getValidArchivePaths(ctx context.Context) (map[string]struct{}, error) {
	output, err := d.execSQL(ctx, "SELECT ARCH_PATH FROM V$ARCHIVE_FILE;")
	if err != nil {
		return nil, fmt.Errorf("查询合法归档列表失败: %w", err)
	}

	validPaths := make(map[string]struct{})
	matches := archivePathRegex.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		if len(m) >= 2 && m[1] != "" {
			// filepath.Clean 归一化路径分隔符，避免数据库返回的路径与 OS 原生分隔符不一致导致比较失败
			validPaths[filepath.Clean(m[1])] = struct{}{}
		}
	}
	return validPaths, nil
}

// findStaleArchiveFiles 找出归档目录中不在合法归档列表里的幽灵文件
// archDir: 归档日志目录路径
// validPaths: 合法归档文件路径集合（来自 V$ARCHIVE_FILE 查询结果）
func findStaleArchiveFiles(archDir string, validPaths map[string]struct{}) []string {
	entries, err := os.ReadDir(archDir)
	if err != nil {
		return nil
	}

	var staleFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(archDir, entry.Name())
		if _, valid := validPaths[fullPath]; !valid {
			staleFiles = append(staleFiles, fullPath)
		}
	}
	return staleFiles
}

// purgeStaleArchives 清理归档目录中不属于当前实例归档链的幽灵归档文件
// 流程：
//  1. 查询 V$ARCHIVE_FILE 获取当前实例归档链的合法文件列表
//  2. 遍历归档目录中的文件
//  3. 删除不在合法列表中的文件（即 DB_MAGIC/LSN 与当前实例不匹配的幽灵归档）
//
// 清理失败仅记录警告，不阻塞后续备份/还原流程
func (d *DamengBackup) purgeStaleArchives(ctx context.Context, archDir string) {
	if archDir == "" {
		return
	}
	if _, err := os.Stat(archDir); errors.Is(err, fs.ErrNotExist) {
		return
	}

	logging.InfoCtx(ctx, "开始执行达梦归档幽灵清理", "arch_dir", archDir)

	validPaths, err := d.getValidArchivePaths(ctx)
	if err != nil {
		logging.WarnCtx(ctx, "查询合法归档列表失败，跳过幽灵清理", "error", err)
		return
	}

	// 安全防护：合法归档列表为空时跳过清理，避免误删所有归档文件
	// 空列表可能是非归档模式，也可能是查询解析异常，删除所有文件风险过高
	if len(validPaths) == 0 {
		logging.WarnCtx(ctx, "合法归档列表为空，跳过幽灵清理以防止误删", "arch_dir", archDir)
		return
	}

	staleFiles := findStaleArchiveFiles(archDir, validPaths)
	if len(staleFiles) == 0 {
		logging.InfoCtx(ctx, "归档目录无幽灵文件", "arch_dir", archDir)
		return
	}

	logging.InfoCtx(ctx, "发现幽灵归档文件", "count", len(staleFiles), "arch_dir", archDir)

	for _, file := range staleFiles {
		if err := os.Remove(file); err != nil {
			logging.WarnCtx(ctx, "删除幽灵归档文件失败", "file", file, "error", err)
			continue
		}
		logging.InfoCtx(ctx, "已删除幽灵归档文件", "file", file)
	}
}

// backupPhysical 执行达梦联机物理备份（通过 disql 执行 BACKUP DATABASE 语句）
//
// 设计说明：达梦的物理备份分联机和脱机两种方式：
//   - 联机备份：数据库处于运行状态，通过 disql 执行 BACKUP DATABASE 语句
//   - 脱机备份：数据库已停止，通过 dmrman 工具执行（需指定 dm.ini 路径）
//
// 本工具采用联机备份方式，无需停止数据库即可备份。
// 还原操作则使用 dmrman 脱机还原（需先停止数据库）。
func (d *DamengBackup) backupPhysical(ctx context.Context, backupDir string, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if !fileutil.IsAdmin() {
		return nil, errors.New("物理备份需要管理员权限，请以管理员身份运行程序")
	}

	// 联机物理备份必须处于 ARCHIVELOG 模式，否则报错 [-7015]
	archived, err := d.isArchiveLogMode(ctx)
	if err != nil {
		return nil, fmt.Errorf("检查归档模式失败: %w", err)
	}
	if !archived {
		if callback != nil {
			callback(0, "数据库未开启归档模式，正在尝试启用...")
		}
		// 归档目录从 opts.ArchiveLogDest 获取（应用层从 BaseBackupDir 自动推导）
		if err := d.EnableArchiveLogMode(ctx, opts.ArchiveLogDest); err != nil {
			return nil, fmt.Errorf("启用归档模式失败: %w", err)
		}
		if callback != nil {
			callback(0, "归档模式已启用")
		}
	}

	// 幽灵归档清理：在备份前清理归档目录中不属于当前实例的残留归档文件
	if d.config.GetExtraTyped().AutoGhostCleanup() && opts.ArchiveLogDest != "" {
		d.purgeStaleArchives(ctx, opts.ArchiveLogDest)
	}

	// 设置超时保护：防止备份进程挂起导致永不结束和内存持续上涨
	backupCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		backupCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := fileutil.EnsureDir(backupDir); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}

	if callback != nil {
		callback(0, "开始达梦物理备份...")
	}

	timestamp := time.Now().Format("20060102_150405")

	backupSetPath, err := d.executeBackupByMode(backupCtx, backupDir, timestamp, opts, callback)
	if err != nil {
		return nil, err
	}

	if callback != nil {
		callback(100, "物理备份完成")
	}

	result.BackupFile = backupSetPath
	result.BackupSize = fileutil.GetDirSize(backupSetPath)
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Metadata["backup_mode"] = string(opts.Mode)

	return result, nil
}

// executeBackupByMode 根据备份模式执行对应的备份操作，返回备份集路径
func (d *DamengBackup) executeBackupByMode(ctx context.Context, backupDir, timestamp string, opts BackupOptions, callback ProgressCallback) (string, error) {
	switch opts.Mode {
	case BackupModeFull:
		return d.executeFullBackup(ctx, backupDir, timestamp, opts, callback)
	case BackupModeIncremental, BackupModeDifferential:
		return d.executeIncrementalBackup(ctx, backupDir, timestamp, opts, callback)
	case BackupModeArchive:
		return d.executeArchiveOnlyBackup(ctx, backupDir, timestamp, opts, callback)
	default:
		return "", fmt.Errorf("不支持的备份模式: %s", opts.Mode)
	}
}

// executeFullBackup 执行全量备份，并在配置了归档目录时自动备份归档日志
// 通过 disql 联机执行 BACKUP DATABASE FULL 语句
func (d *DamengBackup) executeFullBackup(ctx context.Context, backupDir, timestamp string, opts BackupOptions, callback ProgressCallback) (string, error) {
	backupSetName := fmt.Sprintf("DM_FULL_%s", timestamp)
	backupSetPath := filepath.Join(backupDir, fmt.Sprintf("dm_full_%s", timestamp))

	script := d.buildFullBackupScript(backupSetName, backupSetPath, opts)
	if callback != nil {
		callback(20, "执行全量备份...")
	}
	output, err := d.execSQLStreaming(ctx, script, d.makeLogLineCallback(ctx))
	if err != nil {
		return "", fmt.Errorf("全量备份失败: %w, 输出: %s", err, output)
	}

	// 全量备份后执行归档日志备份（归档目录已配置时）
	if opts.ArchiveLogDest != "" {
		d.backupArchiveAfterFull(ctx, backupDir, timestamp, opts, callback)
	}

	return backupSetPath, nil
}

// backupArchiveAfterFull 全量备份后执行归档日志备份（失败仅警告，不影响全量备份结果）
func (d *DamengBackup) backupArchiveAfterFull(ctx context.Context, backupDir, timestamp string, opts BackupOptions, callback ProgressCallback) {
	if callback != nil {
		callback(60, "执行归档日志备份...")
	}
	archSetName := fmt.Sprintf("DM_ARCH_%s", timestamp)
	archSetPath := filepath.Join(backupDir, fmt.Sprintf("dm_arch_%s", timestamp))
	archScript, archScriptErr := d.buildArchiveBackupScript(archSetName, archSetPath, opts)
	if archScriptErr != nil {
		logging.WarnCtx(ctx, "归档日志备份脚本构建失败，但全量备份已成功", "error", archScriptErr)
		return
	}
	archOutput, archErr := d.execSQLStreaming(ctx, archScript, d.makeLogLineCallback(ctx))
	if archErr != nil {
		logging.WarnCtx(ctx, "归档日志备份失败，但全量备份已成功", "error", archErr, "output", archOutput)
	}
}

// executeIncrementalBackup 执行增量备份（差异增量或累积增量）
// 通过 disql 联机执行 BACKUP DATABASE INCREMENT 语句
func (d *DamengBackup) executeIncrementalBackup(ctx context.Context, backupDir, timestamp string, opts BackupOptions, callback ProgressCallback) (string, error) {
	incrSetName := fmt.Sprintf("DM_INCR_%s", timestamp)
	incrSetPath := filepath.Join(backupDir, fmt.Sprintf("dm_incr_%s", timestamp))

	script, err := d.buildIncrementalBackupScript(incrSetName, incrSetPath, opts)
	if err != nil {
		return "", fmt.Errorf("构建增量备份脚本失败: %w", err)
	}
	if callback != nil {
		callback(20, "执行增量备份...")
	}
	output, err := d.execSQLStreaming(ctx, script, d.makeLogLineCallback(ctx))
	if err != nil {
		return "", fmt.Errorf("增量备份失败: %w, 输出: %s", err, output)
	}

	return incrSetPath, nil
}

// executeArchiveOnlyBackup 执行独立归档日志备份（不备份数据文件）
// 通过 disql 联机执行 BACKUP ARCHIVELOG 语句
func (d *DamengBackup) executeArchiveOnlyBackup(ctx context.Context, backupDir, timestamp string, opts BackupOptions, callback ProgressCallback) (string, error) {
	archSetName := fmt.Sprintf("DM_ARCH_%s", timestamp)
	backupSetPath := filepath.Join(backupDir, fmt.Sprintf("dm_arch_%s", timestamp))

	script, err := d.buildArchiveBackupScript(archSetName, backupSetPath, opts)
	if err != nil {
		return "", fmt.Errorf("构建归档日志备份脚本失败: %w", err)
	}
	if callback != nil {
		callback(20, "执行归档日志备份...")
	}
	output, err := d.execSQLStreaming(ctx, script, d.makeLogLineCallback(ctx))
	if err != nil {
		return "", fmt.Errorf("归档日志备份失败: %w, 输出: %s", err, output)
	}

	return backupSetPath, nil
}

// makeLogLineCallback 创建一个逐行日志回调，将 disql/dmrman 输出实时打印到日志
func (d *DamengBackup) makeLogLineCallback(ctx context.Context) shellexec.LineCallback {
	return func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		logging.InfoCtx(ctx, "备份输出", "line", line)
	}
}

// buildFullBackupScript 构建全量备份 SQL 脚本（通过 disql 联机执行）
// 语法：BACKUP DATABASE FULL TO name BACKUPSET 'path' [COMPRESSED [LEVEL n]] [IDENTIFIED BY "password"] [PARALLEL n];
func (d *DamengBackup) buildFullBackupScript(backupSetName, backupSetPath string, opts BackupOptions) string {
	var script strings.Builder

	safeName := escapeDamengRMANString(backupSetName)
	safePath := escapeDamengRMANString(backupSetPath)

	// disql BACKUP 语法：TO 后为标识符（不加引号），BACKUPSET 后为字符串（单引号）
	fmt.Fprintf(&script, `BACKUP DATABASE FULL TO %s BACKUPSET '%s'`, safeName, safePath)

	if opts.EnableCompression {
		if opts.CompressionLevel > 0 {
			fmt.Fprintf(&script, " COMPRESSED LEVEL %d", opts.CompressionLevel)
		} else {
			script.WriteString(" COMPRESSED")
		}
	}

	if opts.Encryption && opts.EncryptionKey != "" {
		// 加密语法：IDENTIFIED BY "password"，不是 ENCRYPT WITH
		safeKey := escapeDamengRMANString(opts.EncryptionKey)
		fmt.Fprintf(&script, ` IDENTIFIED BY "%s"`, safeKey)
	}

	if opts.ParallelWorkers > 1 {
		fmt.Fprintf(&script, " PARALLEL %d", opts.ParallelWorkers)
	}

	script.WriteString(";")
	return script.String()
}

// buildArchiveBackupScript 构建归档日志备份 SQL 脚本（通过 disql 联机执行）
// 语法：BACKUP ARCHIVELOG [ALL|FROM LSN n TO LSN n] TO name BACKUPSET 'path' [COMPRESSED [LEVEL n]] [IDENTIFIED BY "password"] [PARALLEL n];
func (d *DamengBackup) buildArchiveBackupScript(archSetName, archSetPath string, opts BackupOptions) (string, error) {
	var script strings.Builder

	safeName := escapeDamengRMANString(archSetName)
	safePath := escapeDamengRMANString(archSetPath)

	// 根据 LSN 范围决定归档日志备份方式
	if opts.ArchiveFromLSN != "" && opts.ArchiveUntilLSN != "" {
		fromLSN, fromErr := sanitizeLSN(opts.ArchiveFromLSN)
		untilLSN, untilErr := sanitizeLSN(opts.ArchiveUntilLSN)
		if fromErr != nil || untilErr != nil {
			return "", fmt.Errorf("LSN 校验失败: from=%q until=%q", opts.ArchiveFromLSN, opts.ArchiveUntilLSN)
		}
		fmt.Fprintf(&script, `BACKUP ARCHIVELOG FROM LSN %d TO LSN %d TO %s BACKUPSET '%s'`, fromLSN, untilLSN, safeName, safePath)
	} else {
		fmt.Fprintf(&script, `BACKUP ARCHIVELOG ALL TO %s BACKUPSET '%s'`, safeName, safePath)
	}

	if opts.EnableCompression {
		if opts.CompressionLevel > 0 {
			fmt.Fprintf(&script, " COMPRESSED LEVEL %d", opts.CompressionLevel)
		} else {
			script.WriteString(" COMPRESSED")
		}
	}

	if opts.Encryption && opts.EncryptionKey != "" {
		safeKey := escapeDamengRMANString(opts.EncryptionKey)
		fmt.Fprintf(&script, ` IDENTIFIED BY "%s"`, safeKey)
	}

	if opts.ParallelWorkers > 1 {
		fmt.Fprintf(&script, " PARALLEL %d", opts.ParallelWorkers)
	}

	script.WriteString(";")
	return script.String(), nil
}

// buildIncrementalBackupScript 构建增量备份 SQL 脚本（通过 disql 联机执行）
// 语法：BACKUP DATABASE INCREMENT [CUMULATIVE] WITH BACKUPDIR 'dir' TO name BACKUPSET 'path' [COMPRESSED [LEVEL n]] [IDENTIFIED BY "password"] [PARALLEL n];
func (d *DamengBackup) buildIncrementalBackupScript(backupSetName, backupSetPath string, opts BackupOptions) (string, error) {
	var script strings.Builder

	safeName := escapeDamengRMANString(backupSetName)
	safePath := escapeDamengRMANString(backupSetPath)

	baseDir := d.config.GetExtraTyped().DamengDataDir()
	if baseDir == "" {
		baseDir = d.config.GetExtraTyped().DamengHome()
	}
	if baseDir == "" {
		return "", errors.New("增量备份需要指定基础备份目录（DM_DATA_DIR 或 DM_HOME），但两者均未配置")
	}
	safeBaseDir := escapeDamengRMANString(baseDir)

	if opts.Mode == BackupModeDifferential {
		fmt.Fprintf(&script, `BACKUP DATABASE INCREMENT CUMULATIVE WITH BACKUPDIR '%s' TO %s BACKUPSET '%s'`, safeBaseDir, safeName, safePath)
	} else {
		fmt.Fprintf(&script, `BACKUP DATABASE INCREMENT WITH BACKUPDIR '%s' TO %s BACKUPSET '%s'`, safeBaseDir, safeName, safePath)
	}

	if opts.EnableCompression {
		if opts.CompressionLevel > 0 {
			fmt.Fprintf(&script, " COMPRESSED LEVEL %d", opts.CompressionLevel)
		} else {
			script.WriteString(" COMPRESSED")
		}
	}

	if opts.Encryption && opts.EncryptionKey != "" {
		safeKey := escapeDamengRMANString(opts.EncryptionKey)
		fmt.Fprintf(&script, ` IDENTIFIED BY "%s"`, safeKey)
	}

	if opts.ParallelWorkers > 1 {
		fmt.Fprintf(&script, " PARALLEL %d", opts.ParallelWorkers)
	}

	script.WriteString(";")
	return script.String(), nil
}

// restorePhysical 执行达梦物理还原（通过 dmrman 脱机还原）
// 还原前会停止达梦服务，还原完成后重新启动
func (d *DamengBackup) restorePhysical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	if !fileutil.IsAdmin() {
		return nil, errors.New("物理还原需要管理员权限，请以管理员身份运行程序")
	}

	backupSetPath, dmDataDir, err := d.validatePhysicalRestoreOpts(opts)
	if err != nil {
		return nil, err
	}

	// 幽灵归档清理：在还原前清理归档目录中不属于当前实例的残留归档文件
	// 必须在 stopService 之前执行，因为需要数据库在线才能查询 V$ARCHIVE_FILE
	if opts.ArchiveLogDest != "" {
		d.purgeStaleArchives(ctx, opts.ArchiveLogDest)
	}

	if callback != nil {
		callback(0, "开始执行物理还原...")
	}

	startTime := time.Now()
	result := &RestoreResult{}

	if err := d.executePhysicalRestore(ctx, backupSetPath, dmDataDir, opts, callback); err != nil {
		return nil, err
	}

	if callback != nil {
		callback(100, "物理还原完成")
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// validatePhysicalRestoreOpts 验证物理还原参数
func (d *DamengBackup) validatePhysicalRestoreOpts(opts RestoreOptions) (string, string, error) {
	backupSetPath := opts.BackupIdentifier
	if backupSetPath == "" {
		return "", "", errors.New("必须通过 --backup-identifier 参数指定备份集路径")
	}
	// 去除路径末尾的路径分隔符，避免在 dmrman 脚本中尾随反斜杠转义闭合引号
	backupSetPath = strings.TrimRight(backupSetPath, `/\`)

	if err := sanitizeDamengBackupPath(backupSetPath); err != nil {
		return "", "", fmt.Errorf("无效的备份集路径: %w", err)
	}

	dmDataDir := d.config.GetExtraTyped().DamengDataDir()
	if dmDataDir == "" {
		return "", "", errors.New("未配置达梦数据目录，请在配置文件中设置 Extra[\"DM_DATA_DIR\"]")
	}

	if err := validateDataDir(dmDataDir, DBTypeDameng); err != nil {
		return "", "", fmt.Errorf("DATA_DIR 校验失败: %w", err)
	}

	return backupSetPath, dmDataDir, nil
}

// executePhysicalRestore 执行物理还原的具体步骤
// 采用达梦官方推荐的方式：停止服务后，直接使用 dmrman 还原覆盖数据文件
// RESTORE DATABASE 会直接覆盖 dm.ini 指定路径下的数据文件，无需移动或重命名数据目录
func (d *DamengBackup) executePhysicalRestore(ctx context.Context, backupSetPath, dmDataDir string, opts RestoreOptions, callback ProgressCallback) error {
	// 构造 dm.ini 路径（dmrman RESTORE/RECOVER 命令需要 dm.ini 路径而非数据目录路径）
	dmIniPath := filepath.Join(dmDataDir, "dm.ini")

	// 停止达梦服务（等待服务完全停止，释放文件锁）
	if callback != nil {
		callback(5, "停止达梦服务...")
	}
	if err := d.stopService(ctx); err != nil {
		return fmt.Errorf("停止达梦服务失败: %w", err)
	}

	// 构建并执行还原脚本（使用 dm.ini 路径，dmrman 会直接覆盖数据文件）
	restoreScript := d.buildRestoreScriptByMode(backupSetPath, dmIniPath, opts)
	if callback != nil {
		callback(30, "执行还原...")
	}

	output, err := d.execDmrman(ctx, restoreScript)
	if err != nil {
		d.startService(ctx) //nolint:errcheck
		return fmt.Errorf("还原失败: %w, 输出: %s", err, output)
	}

	// 验证还原结果
	if callback != nil {
		callback(80, "验证还原结果...")
	}
	if err := validateDataDir(dmDataDir, DBTypeDameng); err != nil {
		logging.WarnCtx(ctx, "还原后数据目录验证失败，请手动检查", "error", err)
		d.startService(ctx) //nolint:errcheck
		return fmt.Errorf("还原后数据目录验证失败: %w", err)
	}

	// 启动达梦服务
	if callback != nil {
		callback(90, "启动达梦服务...")
	}
	if err := d.startService(ctx); err != nil {
		logging.WarnCtx(ctx, "启动达梦服务失败，请手动启动", "error", err)
	}

	return nil
}

// buildRestoreScriptByMode 根据还原模式构建对应的 dmrman 还原脚本
// dmIniPath: dm.ini 文件路径（dmrman 语法要求使用 ini 路径而非数据目录路径）
func (d *DamengBackup) buildRestoreScriptByMode(backupSetPath, dmIniPath string, opts RestoreOptions) string {
	switch opts.RestoreMode {
	case RestoreModeIncremental:
		return d.buildIncrementalRestoreScript(backupSetPath, dmIniPath, opts)
	case RestoreModeArchive:
		return d.buildArchiveRestoreScript(backupSetPath, dmIniPath, opts)
	default: // RestoreModeFull or empty
		return d.buildFullRestoreScript(backupSetPath, dmIniPath, opts)
	}
}

// buildFullRestoreScript 构建全量还原脚本（通过 dmrman 脱机执行）
// 流程：RESTORE DATABASE → RECOVER DATABASE [UNTIL TIME] → UPDATE DB_MAGIC
// dmIniPath: dm.ini 文件路径（dmrman 语法要求使用 ini 路径而非数据目录路径）
func (d *DamengBackup) buildFullRestoreScript(backupSetPath, dmIniPath string, opts RestoreOptions) string {
	var script strings.Builder

	safeBackupSet := escapeDamengRMANString(backupSetPath)
	safeDmIni := escapeDamengRMANString(dmIniPath)

	// RESTORE DATABASE
	fmt.Fprintf(&script, `RESTORE DATABASE '%s' FROM BACKUPSET '%s';`, safeDmIni, safeBackupSet)

	// RECOVER DATABASE
	archDir := opts.ArchiveLogDest
	if !opts.RecoveryPointInTime.IsZero() {
		timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
		if archDir != "" {
			safeArchDir := escapeDamengRMANString(archDir)
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s' UNTIL TIME '%s';", safeDmIni, safeArchDir, timeStr)
		} else {
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' FROM BACKUPSET '%s' UNTIL TIME '%s';", safeDmIni, safeBackupSet, timeStr)
		}
	} else {
		if archDir != "" {
			safeArchDir := escapeDamengRMANString(archDir)
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s';", safeDmIni, safeArchDir)
		} else {
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' FROM BACKUPSET '%s';", safeDmIni, safeBackupSet)
		}
	}

	// UPDATE DB_MAGIC
	fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' UPDATE DB_MAGIC;", safeDmIni)

	return script.String()
}

// buildIncrementalRestoreScript 构建增量还原脚本（通过 dmrman 脱机执行）
// 流程：RESTORE DATABASE → RECOVER DATABASE WITH BACKUPDIR → RECOVER DATABASE WITH ARCHIVEDIR → UPDATE DB_MAGIC
func (d *DamengBackup) buildIncrementalRestoreScript(backupSetPath, dmIniPath string, opts RestoreOptions) string {
	var script strings.Builder

	safeBackupSet := escapeDamengRMANString(backupSetPath)
	safeDmIni := escapeDamengRMANString(dmIniPath)

	// RESTORE DATABASE（从全量备份集还原数据文件）
	fmt.Fprintf(&script, `RESTORE DATABASE '%s' FROM BACKUPSET '%s';`, safeDmIni, safeBackupSet)

	// RECOVER DATABASE WITH BACKUPDIR（自动查找并应用增量备份集）
	backupDir := filepath.Dir(backupSetPath)
	safeBackupDir := escapeDamengRMANString(backupDir)
	fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH BACKUPDIR '%s';", safeDmIni, safeBackupDir)

	// RECOVER DATABASE WITH ARCHIVEDIR（应用归档日志）
	archDir := opts.ArchiveLogDest
	if !opts.RecoveryPointInTime.IsZero() {
		timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
		if archDir != "" {
			safeArchDir := escapeDamengRMANString(archDir)
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s' UNTIL TIME '%s';", safeDmIni, safeArchDir, timeStr)
		} else {
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' UNTIL TIME '%s';", safeDmIni, timeStr)
		}
	} else if archDir != "" {
		safeArchDir := escapeDamengRMANString(archDir)
		fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s';", safeDmIni, safeArchDir)
	}

	// UPDATE DB_MAGIC
	fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' UPDATE DB_MAGIC;", safeDmIni)

	return script.String()
}

// buildArchiveRestoreScript 构建归档还原脚本（通过 dmrman 脱机执行）
// 流程：RESTORE DATABASE → RESTORE ARCHIVE LOG → RECOVER DATABASE WITH ARCHIVEDIR [UNTIL TIME/LSN] → UPDATE DB_MAGIC
func (d *DamengBackup) buildArchiveRestoreScript(backupSetPath, dmIniPath string, opts RestoreOptions) string {
	var script strings.Builder

	safeBackupSet := escapeDamengRMANString(backupSetPath)
	safeDmIni := escapeDamengRMANString(dmIniPath)

	// RESTORE DATABASE（还原数据文件）
	fmt.Fprintf(&script, `RESTORE DATABASE '%s' FROM BACKUPSET '%s';`, safeDmIni, safeBackupSet)

	// RESTORE ARCHIVE LOG（从归档备份集还原归档日志到归档目录）
	archDir := opts.ArchiveLogDest
	if archDir != "" {
		safeArchDir := escapeDamengRMANString(archDir)
		fmt.Fprintf(&script, "\nRESTORE ARCHIVE LOG FROM BACKUPSET '%s' TO ARCHIVEDIR '%s';", safeBackupSet, safeArchDir)

		// RECOVER DATABASE WITH ARCHIVEDIR
		switch {
		case !opts.RecoveryPointInTime.IsZero():
			timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s' UNTIL TIME '%s';", safeDmIni, safeArchDir, timeStr)
		case opts.RecoveryLSN != "":
			lsn, err := sanitizeLSN(opts.RecoveryLSN)
			if err == nil {
				fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s' UNTIL LSN %d;", safeDmIni, safeArchDir, lsn)
			}
		default:
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' WITH ARCHIVEDIR '%s';", safeDmIni, safeArchDir)
		}
	} else {
		// 无归档目录，使用 FROM BACKUPSET 方式恢复
		if !opts.RecoveryPointInTime.IsZero() {
			timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' FROM BACKUPSET '%s' UNTIL TIME '%s';", safeDmIni, safeBackupSet, timeStr)
		} else {
			fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' FROM BACKUPSET '%s';", safeDmIni, safeBackupSet)
		}
	}

	// UPDATE DB_MAGIC
	fmt.Fprintf(&script, "\nRECOVER DATABASE '%s' UPDATE DB_MAGIC;", safeDmIni)

	return script.String()
}

// getServiceName 获取达梦服务名
func (d *DamengBackup) getServiceName() string {
	if d.dmInstance != "" {
		return "DmService" + d.dmInstance
	}
	return "DmServiceDMSERVER"
}

// getServiceConfig 构造达梦服务的 ServiceConfig
func (d *DamengBackup) getServiceConfig() svcmgmt.ServiceConfig {
	return svcmgmt.ServiceConfig{
		ServiceName: d.getServiceName(),
	}
}

// stopService 停止达梦数据库服务
func (d *DamengBackup) stopService(ctx context.Context) error {
	logging.InfoCtx(ctx, "正在停止达梦服务", "service", d.getServiceName())
	return svcmgmt.StopService(ctx, d.getServiceConfig())
}

// startService 启动达梦数据库服务
func (d *DamengBackup) startService(ctx context.Context) error {
	logging.InfoCtx(ctx, "正在启动达梦服务", "service", d.getServiceName())
	return svcmgmt.StartService(ctx, d.getServiceConfig())
}
