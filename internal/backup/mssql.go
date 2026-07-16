package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/fileutil"
)

// MSSQLBackup 实现 DatabaseBackup 接口，针对 SQL Server 数据库
type MSSQLBackup struct {
	BaseBackup
	instanceName string   // SQL Server 实例名
	env          []string // 环境变量
}

// NewMSSQLBackup 创建 SQL Server 备份实例
func NewMSSQLBackup(config *DBConfig) (*MSSQLBackup, error) {
	if config.Type != "mssql" {
		return nil, errors.New("config.Type 必须是 mssql")
	}
	instanceName := os.Getenv("MSSQL_INSTANCE")
	if val := config.GetExtraTyped().Instance(); val != "" {
		instanceName = val
	}
	if instanceName == "" {
		instanceName = defaultHost
	}
	return &MSSQLBackup{
		BaseBackup:   BaseBackup{config: config},
		instanceName: instanceName,
	}, nil
}

// Backup 执行 SQL Server 备份
func (m *MSSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		return nil, NewNotSupportedError("backup", "mssql")
	}

	if opts.Type == BackupTypeLogical {
		return nil, errors.New("MSSQL 不支持逻辑备份，请指定 --backup-type physical")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := fileutil.EnsureDir(backupDir); err != nil {
		return nil, err
	}

	databaseName := m.config.Database
	if opts.ExtraParams != nil && opts.ExtraParams["database"] != "" {
		databaseName = opts.ExtraParams["database"]
	}

	databases := m.parseDatabaseNames(databaseName)

	if len(databases) == 0 {
		return m.backupLogicalAll(ctx, backupDir, callback)
	}

	if len(databases) == 1 {
		return m.backupLogicalSingle(ctx, backupDir, databases[0], callback)
	}

	return m.backupLogicalMultiple(ctx, backupDir, databases, callback)
}

// Restore 执行 SQL Server 还原
func (m *MSSQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行还原...")
	}

	var backupFile string
	if opts.BackupIdentifier != "" {
		backupFile = opts.BackupIdentifier
	}

	if backupFile == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份文件路径")
	}

	databaseName := opts.TargetDatabaseName
	if databaseName == "" {
		var err error
		databaseName, err = m.getDatabaseNameFromBackup(ctx, backupFile)
		if err != nil {
			return nil, fmt.Errorf("获取备份文件中的数据库名失败: %w", err)
		}
	}

	sqlScript, err := m.buildRestoreScript(opts, databaseName, backupFile)
	if err != nil {
		return nil, fmt.Errorf("构建还原脚本失败: %w", err)
	}

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return nil, fmt.Errorf("还原失败: %w, 输出: %s", err, output)
	}

	if callback != nil {
		callback(100, "还原完成")
	}

	result.Duration = time.Since(startTime)
	result.TargetDatabase = databaseName

	return result, nil
}

// ListDatabases 获取所有用户数据库（排除系统数据库）。
func (m *MSSQLBackup) ListDatabases(ctx context.Context) ([]string, error) {
	sqlScript := `
SELECT name 
FROM sys.databases 
WHERE name NOT IN ('master', 'tempdb', 'model', 'msdb') 
  AND state = 0 
ORDER BY name;
`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return nil, err
	}

	var databases []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "name" {
			continue
		}
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			continue
		}
		if strings.Trim(line, "-") == "" {
			continue
		}
		databases = append(databases, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取输出失败: %w", err)
	}

	return databases, nil
}

// ListBackups 列出所有备份
func (m *MSSQLBackup) ListBackups(ctx context.Context, _ ...BackupOptions) ([]BackupInfo, error) {
	sqlScript := `
SELECT 
    bs.backup_set_id AS BackupID,
    CASE bs.type 
        WHEN 'D' THEN 'FULL'
        WHEN 'I' THEN 'INCREMENTAL'
        WHEN 'L' THEN 'LOG'
        ELSE 'UNKNOWN'
    END AS BackupType,
    bs.backup_start_date AS StartTime,
    bs.backup_finish_date AS CompletionTime,
    bs.backup_size AS Size,
    bs.name AS Tag,
    bmf.physical_device_name AS BackupPath,
    'DISK' AS DeviceType,
    'AVAILABLE' AS Status
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
ORDER BY bs.backup_finish_date DESC
`
	output, err := m.execSQLWithCSV(ctx, sqlScript)
	if err != nil {
		return nil, fmt.Errorf("列出备份失败: %w", err)
	}
	return m.parseBackupList(output)
}

// DeleteBackup 删除指定备份
func (m *MSSQLBackup) DeleteBackup(ctx context.Context, identifier string, _ ...BackupOptions) error {
	var sqlScript string
	var backupPaths []string

	if bsid, err := sanitizePositiveInt(identifier); err == nil {
		pathQuery := fmt.Sprintf(`
SET NOCOUNT ON;
SELECT bmf.physical_device_name 
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bs.backup_set_id = %d;`, bsid)
		pathOutput, err := m.execSQL(ctx, pathQuery)
		if err == nil {
			backupPaths = parseBackupPaths(pathOutput)
		} else {
			logging.WarnCtx(ctx, "查询备份文件路径失败", "error", err)
		}

		sqlScript = fmt.Sprintf(`
	SET NOCOUNT ON;
	DECLARE @bsid INT = %d;
	DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = @bsid;
	DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = @bsid;
	DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = @bsid;
	DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = @bsid;
	DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = @bsid;
	DELETE FROM msdb.dbo.backupset WHERE backup_set_id = @bsid;`, bsid)
	} else {
		cleanDate, err := sanitizeDateLiteral(identifier)
		if err != nil {
			return fmt.Errorf("无效的删除标识符: %w", err)
		}
		pathQuery := fmt.Sprintf(`
	SET NOCOUNT ON;
	SELECT bmf.physical_device_name 
	FROM msdb.dbo.backupset bs
	JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
	WHERE bs.backup_finish_date <= '%s';`, cleanDate)
		pathOutput, err := m.execSQL(ctx, pathQuery)
		if err == nil {
			backupPaths = parseBackupPaths(pathOutput)
		} else {
			logging.WarnCtx(ctx, "查询备份文件路径失败", "error", err)
		}

		sqlScript = fmt.Sprintf("EXEC msdb.dbo.sp_delete_backuphistory @oldest_date = '%s';", cleanDate)
	}

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("删除备份失败: %w, 输出: %s", err, output)
	}

	for _, path := range backupPaths {
		if err := os.Remove(path); err != nil {
			logging.WarnCtx(ctx, "删除备份文件失败", "path", path, "error", err)
		} else {
			logging.InfoCtx(ctx, "已删除备份文件", "path", path)
		}
	}

	return nil
}

// ValidateBackup 验证备份有效性
func (m *MSSQLBackup) ValidateBackup(ctx context.Context, backupID string, _ ...BackupOptions) error {
	if backupID == "" {
		return errors.New("必须指定备份ID")
	}

	var backupPath string

	if bsid, err := sanitizePositiveInt(backupID); err == nil {
		queryScript := fmt.Sprintf(`
SELECT bmf.physical_device_name 
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bs.backup_set_id = %d`, bsid)

		output, err := m.execSQL(ctx, queryScript)
		if err != nil {
			return fmt.Errorf("查询备份路径失败: %w", err)
		}

		scanner := bufio.NewScanner(strings.NewReader(output))
		found := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "c:\\") || strings.HasPrefix(line, "C:\\") {
				backupPath = line
				found = true
				break
			}
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("读取输出失败: %w", err)
		}

		if !found {
			return errors.New("未找到备份记录")
		}
	} else {
		cleanPath, err := sanitizeBackupPath(backupID, ".bak", ".trn")
		if err != nil {
			return fmt.Errorf("无效的备份文件路径: %w", err)
		}
		backupPath = cleanPath
	}

	sqlScript := fmt.Sprintf("RESTORE VERIFYONLY FROM DISK = N'%s' WITH NOUNLOAD;", backupPath)
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("验证失败: %w, 输出: %s", err, output)
	}

	if strings.Contains(strings.ToUpper(output), "ERROR") || strings.Contains(strings.ToUpper(output), "FAILED") {
		return errors.New("备份验证失败")
	}

	return nil
}

// GetBackupInfo 获取指定备份的详细信息
func (m *MSSQLBackup) GetBackupInfo(ctx context.Context, backupID string, _ ...BackupOptions) (map[string]string, error) {
	var sqlScript string
	if backupID != "" {
		bsid, err := sanitizePositiveInt(backupID)
		if err != nil {
			return nil, fmt.Errorf("无效的备份 ID: %w", err)
		}
		sqlScript = fmt.Sprintf(`
SELECT * FROM msdb.dbo.backupset WHERE backup_set_id = %d;
`, bsid)
	} else {
		sqlScript = "SELECT TOP 10 * FROM msdb.dbo.backupset ORDER BY backup_finish_date DESC;"
	}

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return nil, err
	}

	info := make(map[string]string)
	info["raw_output"] = output

	return info, nil
}

// RegisterBackup 将指定路径的备份文件注册到备份目录库
func (m *MSSQLBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	cleanPath, err := sanitizeBackupPath(backupPath)
	if err != nil {
		return fmt.Errorf("无效的备份路径: %w", err)
	}

	sqlScript := fmt.Sprintf(`
EXEC msdb.dbo.sp_add_backup_filehistory 
    @backup_set_id = NULL,
    @file_name = N'%s';
`, cleanPath)

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("注册备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (m *MSSQLBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	bsid, err := sanitizePositiveInt(backupID)
	if err != nil {
		return fmt.Errorf("无效的备份 ID: %w", err)
	}
	sqlScript := fmt.Sprintf("EXEC msdb.dbo.sp_delete_backuphistory @backup_set_id = %d;", bsid)
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("移除备份记录失败: %w, 输出: %s", err, output)
	}
	return nil
}

// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
func (m *MSSQLBackup) VerifyBackupStatus(ctx context.Context) error {
	sqlScript := `
SELECT 
    bs.backup_set_id, 
    bmf.physical_device_name 
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bmf.device_type = 2
`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("查询备份记录失败: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取输出失败: %w", err)
	}

	if len(lines) < 5 {
		logging.Info("没有需要检查的备份记录")
		return nil
	}

	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || isSQLCmdFooterLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		backupID := fields[0]
		backupPath := fields[1]

		cleanPath, err := sanitizeBackupPath(backupPath, ".bak", ".trn")
		if err != nil {
			logging.WarnCtx(ctx, "备份路径校验失败，跳过", "path", backupPath, "error", err)
			continue
		}

		verifyScript := fmt.Sprintf("RESTORE VERIFYONLY FROM DISK = N'%s' WITH NOUNLOAD;", cleanPath)
		if _, err := m.execSQL(ctx, verifyScript); err != nil {
			logging.WarnCtx(ctx, "备份验证失败", "backup_id", backupID, "error", err)

			deleteScript := fmt.Sprintf(`
SET NOCOUNT ON;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = %s;
DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = %s;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = %s;`,
				backupID, backupID, backupID, backupID, backupID, backupID)

			if _, err := m.execSQL(ctx, deleteScript); err != nil {
				logging.WarnCtx(ctx, "删除无效备份记录失败", "backup_id", backupID, "error", err)
			} else {
				logging.InfoCtx(ctx, "已删除无效备份记录", "backup_id", backupID)
				if err := os.Remove(backupPath); err != nil {
					logging.WarnCtx(ctx, "删除备份文件失败", "path", backupPath, "error", err)
				} else {
					logging.InfoCtx(ctx, "已删除备份文件", "path", backupPath)
				}
			}
		} else {
			logging.InfoCtx(ctx, "备份验证通过", "backup_id", backupID)
		}
	}

	logging.Info("备份状态检查完成")
	return nil
}

// DeleteInvalidBackups 删除无效的备份记录（文件不存在的备份）
func (m *MSSQLBackup) DeleteInvalidBackups(ctx context.Context, _ ...BackupOptions) error {
	sqlScript := `
SELECT 
    bs.backup_set_id, 
    bmf.physical_device_name 
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id
WHERE bmf.device_type = 2
`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("查询备份记录失败: %w", err)
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取输出失败: %w", err)
	}

	var invalidBackupIDs []string
	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || isSQLCmdFooterLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			backupID := fields[0]
			backupPath := fields[1]
			if _, err := os.Stat(backupPath); os.IsNotExist(err) {
				invalidBackupIDs = append(invalidBackupIDs, backupID)
			}
		}
	}

	if len(invalidBackupIDs) == 0 {
		logging.Info("没有无效的备份记录")
		return nil
	}

	for _, backupID := range invalidBackupIDs {
		deleteScript := fmt.Sprintf(`
SET NOCOUNT ON;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = %s;
DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = %s;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = %s;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = %s;`,
			backupID, backupID, backupID, backupID, backupID, backupID)

		if _, err := m.execSQL(ctx, deleteScript); err != nil {
			logging.WarnCtx(ctx, "删除备份记录失败", "backup_id", backupID, "error", err)
		} else {
			logging.InfoCtx(ctx, "已删除无效备份记录", "backup_id", backupID)
		}
	}

	return nil
}

// DeleteAllBackups 删除所有备份
func (m *MSSQLBackup) DeleteAllBackups(ctx context.Context, _ ...BackupOptions) error {
	pathQuery := `
SET NOCOUNT ON;
SELECT bmf.physical_device_name 
FROM msdb.dbo.backupset bs
JOIN msdb.dbo.backupmediafamily bmf ON bs.media_set_id = bmf.media_set_id;`
	pathOutput, err := m.execSQL(ctx, pathQuery)
	var backupPaths []string
	if err == nil {
		backupPaths = parseBackupPaths(pathOutput)
	} else {
		logging.WarnCtx(ctx, "查询备份文件路径失败", "error", err)
	}

	sqlScript := `
SET NOCOUNT ON;
DELETE FROM msdb.dbo.restorefilegroup;
DELETE FROM msdb.dbo.restorefile;
DELETE FROM msdb.dbo.restorehistory;
DELETE FROM msdb.dbo.backupfilegroup;
DELETE FROM msdb.dbo.backupfile;
DELETE FROM msdb.dbo.backupset;
DELETE FROM msdb.dbo.backupmediafamily;
DELETE FROM msdb.dbo.backupmediaset;`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("删除所有备份失败: %w, 输出: %s", err, output)
	}

	for _, path := range backupPaths {
		if err := os.Remove(path); err != nil {
			logging.WarnCtx(ctx, "删除备份文件失败", "path", path, "error", err)
		} else {
			logging.InfoCtx(ctx, "已删除备份文件", "path", path)
		}
	}

	return nil
}

// registerMSSQLDriver 注册 MSSQL 驱动
func registerMSSQLDriver() error {
	return RegisterDriver(DriverMetadata{
		Name:                 DBTypeMSSQL,
		Version:              versionXML,
		Description:          "SQL Server 数据库备份驱动，支持 sqlcmd 命令备份",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, actionValidate, actionInfo, "register", "unregister", actionVerifyStatus, "delete-invalid", actionDeleteAll},
		SupportedBackupTypes: []BackupType{BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewMSSQLBackup(config)
	})
}

// isSQLCmdFooterLine 检测 sqlcmd 输出中的尾部统计行。
// sqlcmd 在输出末尾会附加类似 "N 行受影响" 或 "N rows affected" 的统计行，
// 不同语言/版本格式不同，需统一跳过。
func isSQLCmdFooterLine(line string) bool {
	lower := strings.ToLower(line)
	// 中文: "1 行受影响", "100 行受影响"
	if strings.Contains(lower, "行受影响") {
		return true
	}
	// 英文: "1 rows affected", "1 row affected"
	if strings.Contains(lower, "rows affected") || strings.Contains(lower, "row affected") {
		return true
	}
	return false
}
