package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
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
	if val, ok := config.Extra["INSTANCE"]; ok && val != "" {
		instanceName = val
	}
	if instanceName == "" {
		instanceName = "localhost"
	}
	return &MSSQLBackup{
		BaseBackup:   BaseBackup{config: config},
		instanceName: instanceName,
	}, nil
}

// buildConnectionString 构建 sqlcmd 连接参数
func (m *MSSQLBackup) buildConnectionString() string {
	var args []string
	// 构建服务器地址
	server := m.config.Host
	if m.instanceName != "" && m.instanceName != "localhost" {
		server = fmt.Sprintf("%s\\%s", m.config.Host, m.instanceName)
	} else if m.config.Port != 0 {
		server = fmt.Sprintf("%s,%d", m.config.Host, m.config.Port)
	}
	args = append(args, "-S", server)
	args = append(args, "-U", m.config.User)
	args = append(args, "-P", m.config.Password)
	if m.config.Database != "" {
		args = append(args, "-d", m.config.Database)
	}
	return strings.Join(args, " ")
}

// execSQL 执行 SQL 命令（通过 sqlcmd）
func (m *MSSQLBackup) execSQL(ctx context.Context, sqlText string) (string, error) {
	utils.Infof("\n========== SQL 命令开始 ==========\n%s\n========== SQL 命令结束 ==========", sqlText)

	var cmd *exec.Cmd
	if m.isWindowsAuth() {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-E", "-C", "-Q", sqlText)
	} else {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-U", m.config.User, "-P", m.config.Password, "-C", "-Q", sqlText)
	}
	if m.config.Database != "" {
		cmd.Args = append(cmd.Args, "-d", m.config.Database)
	}
	cmd.Env = append(os.Environ(), m.env...)

	output, err := utils.ExecCommand(ctx, cmd)
	utils.Infof("\n========== SQL 执行输出开始 ==========\n%s\n========== SQL 执行输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("sqlcmd 执行失败: %w", err)
	}
	return output, nil
}

// isWindowsAuth 判断是否使用 Windows 身份验证
func (m *MSSQLBackup) isWindowsAuth() bool {
	if m.config.User == "" && m.config.Password == "" {
		return true
	}
	if val, ok := m.config.Extra["AUTH_TYPE"]; ok && val == "windows" {
		return true
	}
	return false
}

// buildServerArg 构建服务器参数
func (m *MSSQLBackup) buildServerArg() string {
	if m.instanceName != "" && m.instanceName != "localhost" {
		return fmt.Sprintf("%s\\%s", m.config.Host, m.instanceName)
	}
	if m.config.Port != 0 {
		return fmt.Sprintf("%s,%d", m.config.Host, m.config.Port)
	}
	return m.config.Host
}

// Backup 执行 SQL Server 备份
func (m *MSSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		backupDir = "./backups"
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		result.Error = err
		return result, err
	}

	databaseName := m.config.Database
	if opts.ExtraParams != nil && opts.ExtraParams["database"] != "" {
		databaseName = opts.ExtraParams["database"]
	}

	databases := m.parseDatabaseNames(databaseName)

	if len(databases) == 0 {
		return m.backupAllDatabases(ctx, opts, backupDir, callback)
	}

	if len(databases) == 1 {
		return m.backupSingleDatabase(ctx, opts, backupDir, databases[0], callback)
	}

	return m.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
}

// parseDatabaseNames 解析数据库名称（支持逗号分隔的多个数据库）
func (m *MSSQLBackup) parseDatabaseNames(databaseName string) []string {
	if databaseName == "" || databaseName == "all" {
		return nil
	}

	var names []string
	for _, name := range strings.Split(databaseName, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// backupSingleDatabase 备份单个数据库
func (m *MSSQLBackup) backupSingleDatabase(ctx context.Context, opts BackupOptions, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := m.checkDatabaseExists(ctx, databaseName); err != nil {
		result.Error = err
		return result, err
	}

	backupFileName := fmt.Sprintf("%s_%s.bak", databaseName, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, backupFileName)

	if callback != nil {
		callback(0, fmt.Sprintf("开始备份数据库 %s...", databaseName))
	}

	sqlScript := m.buildBackupScript(opts, backupDir, databaseName, backupPath)

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		result.Error = fmt.Errorf("备份失败: %w, 输出: %s", err, output)
		return result, result.Error
	}

	if callback != nil {
		callback(100, "备份完成")
	}

	if info, err := os.Stat(backupPath); err == nil {
		result.BackupFile = backupPath
		result.BackupSize = info.Size()
	} else {
		result.BackupFile = backupPath
	}

	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// backupMultipleDatabases 备份多个数据库
func (m *MSSQLBackup) backupMultipleDatabases(ctx context.Context, opts BackupOptions, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	var backupFiles []string
	var totalSize int64

	if callback != nil {
		callback(0, fmt.Sprintf("开始备份 %d 个数据库...", len(databases)))
	}

	for i, dbName := range databases {
		if callback != nil {
			percent := float64(i) / float64(len(databases)) * 100
			callback(percent, fmt.Sprintf("正在备份数据库 %s (%d/%d)", dbName, i+1, len(databases)))
		}

		// 复用 backupSingleDatabase
		singleResult, err := m.backupSingleDatabase(ctx, opts, backupDir, dbName, nil)
		if err != nil {
			utils.Warnf("备份数据库 %s 失败: %v, 继续备份其他数据库", dbName, err)
			continue
		}

		backupFiles = append(backupFiles, singleResult.BackupFile)
		totalSize += singleResult.BackupSize
	}

	if callback != nil {
		callback(100, "备份完成")
	}

	if len(backupFiles) == 0 {
		result.Error = errors.New("没有成功备份任何数据库")
		return result, result.Error
	}

	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = totalSize
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// backupAllDatabases 备份所有用户数据库
func (m *MSSQLBackup) backupAllDatabases(ctx context.Context, opts BackupOptions, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.getAllUserDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到用户数据库")
	}

	// 复用 backupMultipleDatabases
	return m.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
}

// getAllUserDatabases 获取所有用户数据库（排除系统数据库）
func (m *MSSQLBackup) getAllUserDatabases(ctx context.Context) ([]string, error) {
	sqlScript := `
SELECT name 
FROM sys.databases 
WHERE name NOT IN ('tempdb') 
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

	return databases, nil
}

// checkDatabaseExists 检查指定数据库是否存在
func (m *MSSQLBackup) checkDatabaseExists(ctx context.Context, databaseName string) error {
	sqlScript := fmt.Sprintf(`
IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = '%s')
BEGIN
    RAISERROR('数据库 %s 不存在', 16, 1)
END
`, databaseName, databaseName)
	_, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("数据库 %s 不存在或无法访问: %w", databaseName, err)
	}
	return nil
}

// buildBackupScript 根据选项生成备份脚本
func (m *MSSQLBackup) buildBackupScript(opts BackupOptions, backupDir, databaseName, backupPath string) string {
	var script strings.Builder
	script.WriteString(fmt.Sprintf("BACKUP DATABASE [%s] TO DISK = N'%s' WITH ", databaseName, backupPath))

	if opts.Compression {
		script.WriteString("COMPRESSION, ")
	}

	if opts.Description != "" {
		script.WriteString(fmt.Sprintf("DESCRIPTION = N'%s', ", opts.Description))
	}

	script.WriteString("STATS = 10")
	return script.String()
}

// Restore 执行 SQL Server 还原
func (m *MSSQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行还原...")
	}

	var backupFile string
	if opts.BackupTag != "" {
		backupFile = opts.BackupTag
	} else if opts.BackupID != "" {
		backupFile = opts.BackupID
	}

	if backupFile == "" {
		result.Error = errors.New("必须通过 -backup-tag 参数指定备份文件路径")
		return result, result.Error
	}

	databaseName := opts.TargetDB
	if databaseName == "" {
		var err error
		databaseName, err = m.getDatabaseNameFromBackup(ctx, backupFile)
		if err != nil {
			result.Error = fmt.Errorf("获取备份文件中的数据库名失败: %w", err)
			return result, result.Error
		}
	}

	sqlScript := m.buildRestoreScript(opts, databaseName, backupFile)

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		result.Error = fmt.Errorf("还原失败: %w, 输出: %s", err, output)
		return result, result.Error
	}

	if callback != nil {
		callback(100, "还原完成")
	}

	result.Duration = time.Since(startTime)
	result.Success = true

	return result, nil
}

// getDatabaseNameFromBackup 从备份文件中获取数据库名
func (m *MSSQLBackup) getDatabaseNameFromBackup(ctx context.Context, backupFile string) (string, error) {
	sqlScript := fmt.Sprintf(`RESTORE FILELISTONLY FROM DISK = N'%s';`, backupFile)
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 3 {
		return "", errors.New("无法从备份文件中获取数据库信息")
	}

	for i := 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			continue
		}
		if strings.Trim(line, "-") == "" {
			continue
		}

		fields := strings.SplitN(line, " ", 2)
		if len(fields) > 0 && fields[0] != "" {
			return fields[0], nil
		}
	}

	return "", errors.New("无法从备份文件中获取数据库名")
}

// buildRestoreScript 根据选项生成还原脚本
func (m *MSSQLBackup) buildRestoreScript(opts RestoreOptions, databaseName, backupFile string) string {
	var script strings.Builder
	script.WriteString(fmt.Sprintf("USE master; RESTORE DATABASE [%s] FROM DISK = N'%s' WITH ", databaseName, backupFile))

	if opts.Overwrite {
		script.WriteString("REPLACE, ")
	}

	if !opts.PointInTime.IsZero() {
		timeStr := opts.PointInTime.Format("2006-01-02 15:04:05")
		script.WriteString(fmt.Sprintf("STOPAT = '%s', ", timeStr))
	}

	script.WriteString("STATS = 10")
	return script.String()
}

// ListBackups 列出所有备份
func (m *MSSQLBackup) ListBackups(ctx context.Context) ([]BackupInfo, error) {
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
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return nil, fmt.Errorf("列出备份失败: %w", err)
	}
	return m.parseBackupList(output)
}

// parseBackupList 解析备份列表输出
func (m *MSSQLBackup) parseBackupList(output string) ([]BackupInfo, error) {
	var backups []BackupInfo
	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 5 {
		return backups, nil
	}

	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Contains(line, "行受影响") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}

		backupID := fields[0]
		backupType := fields[1]
		startTime, _ := time.Parse("2006-01-02 15:04:05", fields[2]+" "+strings.Split(fields[3], ".")[0])
		completionTime, _ := time.Parse("2006-01-02 15:04:05", fields[4]+" "+strings.Split(fields[5], ".")[0])
		size, _ := strconv.ParseInt(fields[6], 10, 64)

		tag := ""
		backupPath := ""
		if len(fields) > 7 {
			tag = fields[7]
			if tag == "NULL" {
				tag = ""
			}
		}
		if len(fields) > 8 {
			backupPath = strings.Join(fields[8:], " ")
		}

		info := BackupInfo{
			BackupID:       backupID,
			BackupType:     backupType,
			StartTime:      startTime,
			CompletionTime: completionTime,
			Size:           size,
			Tag:            tag,
			DeviceType:     "DISK",
			Status:         "AVAILABLE",
			BackupPath:     backupPath,
		}
		backups = append(backups, info)
	}

	return backups, nil
}

// DeleteBackup 删除指定备份
func (m *MSSQLBackup) DeleteBackup(ctx context.Context, identifier string) error {
	var sqlScript string

	if _, err := strconv.Atoi(identifier); err == nil {
		sqlScript = fmt.Sprintf(`
SET NOCOUNT ON;
DECLARE @bsid INT = %s;
DELETE rfg FROM msdb.dbo.restorefilegroup rfg JOIN msdb.dbo.restorehistory rh ON rfg.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = @bsid;
DELETE rf FROM msdb.dbo.restorefile rf JOIN msdb.dbo.restorehistory rh ON rf.restore_history_id = rh.restore_history_id WHERE rh.backup_set_id = @bsid;
DELETE FROM msdb.dbo.restorehistory WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupfilegroup WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupfile WHERE backup_set_id = @bsid;
DELETE FROM msdb.dbo.backupset WHERE backup_set_id = @bsid;`, identifier)
	} else {
		sqlScript = fmt.Sprintf("EXEC msdb.dbo.sp_delete_backuphistory @oldest_date = '%s';", identifier)
	}

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("删除备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// ValidateBackup 验证备份有效性
func (m *MSSQLBackup) ValidateBackup(ctx context.Context, backupID string) error {
	if backupID == "" {
		return errors.New("必须指定备份ID")
	}

	sqlScript := fmt.Sprintf(`
RESTORE VERIFYONLY FROM DISK = N'%s' WITH NOUNLOAD;
`, backupID)

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("验证失败: %w, 输出: %s", err, output)
	}

	if strings.Contains(output, "BACKUP DATABASE") && strings.Contains(output, "successfully") {
		return nil
	}

	if strings.Contains(output, "error") || strings.Contains(output, "failed") {
		return errors.New("备份验证失败")
	}

	return nil
}

// GetBackupInfo 获取指定备份的详细信息
func (m *MSSQLBackup) GetBackupInfo(ctx context.Context, backupID string) (map[string]string, error) {
	var sqlScript string
	if backupID != "" {
		sqlScript = fmt.Sprintf(`
SELECT * FROM msdb.dbo.backupset WHERE backup_set_id = %s;
`, backupID)
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
	sqlScript := fmt.Sprintf(`
EXEC msdb.dbo.sp_add_backup_filehistory 
    @backup_set_id = NULL,
    @file_name = N'%s';
`, backupPath)

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("注册备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (m *MSSQLBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	sqlScript := fmt.Sprintf("EXEC msdb.dbo.sp_delete_backuphistory @backup_set_id = %s;", backupID)
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("移除备份记录失败: %w, 输出: %s", err, output)
	}
	return nil
}

// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
func (m *MSSQLBackup) VerifyBackupStatus(ctx context.Context) error {
	sqlScript := `
DECLARE @backupSetId INT;
DECLARE backup_cursor CURSOR FOR
SELECT backup_set_id FROM msdb.dbo.backupset;

OPEN backup_cursor;
FETCH NEXT FROM backup_cursor INTO @backupSetId;

WHILE @@FETCH_STATUS = 0
BEGIN
    BEGIN TRY
        RESTORE VERIFYONLY FROM DISK = (SELECT physical_device_name FROM msdb.dbo.backupmediafamily WHERE media_set_id = (SELECT media_set_id FROM msdb.dbo.backupset WHERE backup_set_id = @backupSetId));
    END TRY
    BEGIN CATCH
        UPDATE msdb.dbo.backupset SET is_valid = 0 WHERE backup_set_id = @backupSetId;
    END CATCH
    FETCH NEXT FROM backup_cursor INTO @backupSetId;
END

CLOSE backup_cursor;
DEALLOCATE backup_cursor;
`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("检查备份状态失败: %w, 输出: %s", err, output)
	}
	return nil
}

// DeleteInvalidBackups 删除无效的备份记录
func (m *MSSQLBackup) DeleteInvalidBackups(ctx context.Context) error {
	sqlScript := `DELETE FROM msdb.dbo.backupset WHERE is_valid = 0;`
	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("删除无效备份失败: %w, 输出: %s", err, output)
	}
	return nil
}

// DeleteAllBackups 删除所有备份
func (m *MSSQLBackup) DeleteAllBackups(ctx context.Context) error {
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
	return nil
}

// Close 释放资源
func (m *MSSQLBackup) Close() error {
	return nil
}
