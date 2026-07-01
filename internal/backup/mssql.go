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

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
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

// execSQL 执行 SQL 命令（通过 sqlcmd）
func (m *MSSQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "SQL 命令", "sql", sqlStatement)

	var cmd *exec.Cmd
	if m.isWindowsAuth() {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-E", "-C", "-Q", sqlStatement)
	} else {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-U", m.config.User, "-P", m.config.Password, "-C", "-Q", sqlStatement)
	}
	if m.config.Database != "" {
		cmd.Args = append(cmd.Args, "-d", m.config.Database)
	}
	cmd.Env = append(os.Environ(), m.env...)

	output, err := shellexec.ExecCommand(cmd)
	logging.InfoCtx(ctx, "SQL 执行输出", "output", output)
	if err != nil {
		return output, fmt.Errorf("sqlcmd 执行失败: %w", err)
	}
	return output, nil
}

// execSQLWithCSV 执行 SQL 命令（通过 sqlcmd）并以 CSV 格式输出
func (m *MSSQLBackup) execSQLWithCSV(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "SQL 命令(CSV)", "sql", sqlStatement)

	var cmd *exec.Cmd
	if m.isWindowsAuth() {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-E", "-C", "-Q", sqlStatement, "-h-1", "-s,", "-w", "65535")
	} else {
		cmd = exec.CommandContext(ctx, "sqlcmd", "-S", m.buildServerArg(), "-U", m.config.User, "-P", m.config.Password, "-C", "-Q", sqlStatement, "-h-1", "-s,", "-w", "65535")
	}
	if m.config.Database != "" {
		cmd.Args = append(cmd.Args, "-d", m.config.Database)
	}
	cmd.Env = append(os.Environ(), m.env...)

	output, err := shellexec.ExecCommand(cmd)
	logging.InfoCtx(ctx, "SQL 执行输出(CSV)", "output", output)
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
	if m.config.GetExtraTyped().IsWindowsAuth() {
		return true
	}
	return false
}

// buildServerArg 构建服务器参数
func (m *MSSQLBackup) buildServerArg() string {
	if m.instanceName != "" && m.instanceName != defaultHost {
		return fmt.Sprintf("%s\\%s", m.config.Host, m.instanceName)
	}
	if m.config.Port != 0 {
		return fmt.Sprintf("%s,%d", m.config.Host, m.config.Port)
	}
	return m.config.Host
}

// Backup 执行 SQL Server 备份
func (m *MSSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	if opts.Mode == BackupModeIncremental || opts.Mode == BackupModeDifferential {
		return nil, NewNotSupportedError(ctx, "backup", "mssql")
	}

	if opts.Type == BackupTypeLogical {
		return nil, errors.New("MSSQL 不支持逻辑备份，请指定 --backup-type physical")
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		return nil, errors.New("必须通过 -target-path 参数指定备份路径")
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, err
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

// backupSingleDatabase 备份单个数据库
func (m *MSSQLBackup) backupSingleDatabase(ctx context.Context, opts BackupOptions, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := m.checkDatabaseExists(ctx, databaseName); err != nil {
		return nil, err
	}

	backupFileName := fmt.Sprintf("%s_%s.bak", databaseName, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, backupFileName)

	if callback != nil {
		callback(0, fmt.Sprintf("开始备份数据库 %s...", databaseName))
	}

	sqlScript, err := m.buildBackupScript(opts, backupDir, databaseName, backupPath)
	if err != nil {
		return nil, fmt.Errorf("build backup script failed: %w", err)
	}

	output, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return nil, fmt.Errorf("备份失败: %w, 输出: %s", err, output)
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
			logging.WarnCtx(ctx, "备份数据库失败，继续备份其他数据库", "db", dbName, "error", err)
			continue
		}

		backupFiles = append(backupFiles, singleResult.BackupFile)
		totalSize += singleResult.BackupSize
	}

	if callback != nil {
		callback(100, "备份完成")
	}

	if len(backupFiles) == 0 {
		return nil, errors.New("没有成功备份任何数据库")
	}

	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = totalSize
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()

	return result, nil
}

// backupAllDatabases 备份所有用户数据库
func (m *MSSQLBackup) backupAllDatabases(ctx context.Context, opts BackupOptions, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到用户数据库")
	}

	// 复用 backupMultipleDatabases
	return m.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
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

	return databases, nil
}

// checkDatabaseExists 检查指定数据库是否存在
func (m *MSSQLBackup) checkDatabaseExists(ctx context.Context, databaseName string) error {
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	sqlScript := fmt.Sprintf(`
IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = N'%s')
BEGIN
    RAISERROR('数据库 %s 不存在', 16, 1)
END
`, databaseName, databaseName)
	_, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("database %q does not exist or cannot be accessed: %w", databaseName, err)
	}
	return nil
}

// buildBackupScript 根据选项生成备份脚本
func (m *MSSQLBackup) buildBackupScript(opts BackupOptions, _ string, databaseName, backupPath string) (string, error) {
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return "", fmt.Errorf("invalid database name: %w", err)
	}
	cleanPath, err := sanitizeBackupPath(backupPath, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("invalid backup path: %w", err)
	}

	var script strings.Builder
	fmt.Fprintf(&script, "BACKUP DATABASE [%s] TO DISK = N'%s' WITH ", databaseName, cleanPath)

	if opts.EnableCompression {
		script.WriteString("COMPRESSION, ")
	}

	script.WriteString("STATS = 10")
	return script.String(), nil
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
		return nil, fmt.Errorf("build restore script failed: %w", err)
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

// getDatabaseNameFromBackup 从备份文件中获取数据库名
func (m *MSSQLBackup) getDatabaseNameFromBackup(ctx context.Context, backupFile string) (string, error) {
	cleanPath, err := sanitizeBackupPath(backupFile, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("invalid backup file path: %w", err)
	}

	sqlScript := fmt.Sprintf(`RESTORE FILELISTONLY FROM DISK = N'%s';`, cleanPath)
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

// buildRestoreScript 根据选项生成 MSSQL 还原脚本
// 安全设计：
//   - 对数据库名进行校验，防止 SQL 注入
//   - 对备份文件路径进行校验，防止路径遍历攻击
//   - 使用 Unicode 字符串格式 (N'...') 支持中文等 Unicode 字符
func (m *MSSQLBackup) buildRestoreScript(opts RestoreOptions, databaseName, backupFile string) (string, error) {
	// 安全校验：数据库名
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return "", fmt.Errorf("invalid database name: %w", err)
	}
	// 安全校验：备份文件路径
	cleanPath, err := sanitizeBackupPath(backupFile, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("invalid backup file path: %w", err)
	}

	var script strings.Builder
	fmt.Fprintf(&script, "USE master; RESTORE DATABASE [%s] FROM DISK = N'%s' WITH ", databaseName, cleanPath)

	if opts.Overwrite {
		script.WriteString("REPLACE, ")
	}

	if !opts.RecoveryPointInTime.IsZero() {
		timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
		if _, err := sanitizeDateLiteral(timeStr); err != nil {
			return "", fmt.Errorf("invalid recovery point in time: %w", err)
		}
		fmt.Fprintf(&script, "STOPAT = '%s', ", timeStr)
	}

	script.WriteString("STATS = 10")
	return script.String(), nil
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

// parseBackupList 解析备份列表输出
func (m *MSSQLBackup) parseBackupList(output string) ([]BackupInfo, error) {
	var backups []BackupInfo
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.Contains(line, "行受影响") || strings.Contains(line, "rows affected") {
			continue
		}

		info, ok := m.parseBackupListLine(line)
		if !ok {
			continue
		}
		backups = append(backups, info)
	}

	return backups, nil
}

// parseBackupListLine 解析单行备份列表
func (m *MSSQLBackup) parseBackupListLine(line string) (BackupInfo, bool) {
	fields := strings.Split(line, ",")
	if len(fields) < 9 {
		return BackupInfo{}, false
	}

	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
		fields[i] = strings.Trim(fields[i], "\"")
	}

	backupID := fields[0]
	if _, err := strconv.Atoi(backupID); err != nil {
		return BackupInfo{}, false
	}

	backupType := fields[1]

	var completionTime time.Time
	if len(fields) > 3 && fields[3] != "" {
		dateTimeStr := strings.TrimSpace(fields[3])
		dateTimeStr = strings.Split(dateTimeStr, ".")[0]
		completionTime, _ = time.Parse("2006-01-02 15:04:05", dateTimeStr)
	}

	size, _ := strconv.ParseInt(fields[4], 10, 64)

	tag := ""
	if len(fields) > 5 {
		tag = strings.TrimSpace(fields[5])
		if tag == "NULL" {
			tag = ""
		}
	}

	backupPath := m.parseBackupPath(fields)

	return BackupInfo{
		BackupID:       backupID,
		BackupType:     backupType,
		CompletionTime: completionTime,
		Size:           size,
		Tag:            tag,
		BackupPath:     backupPath,
	}, true
}

// parseBackupPath 从字段列表中解析备份路径（路径可能包含逗号）
func (m *MSSQLBackup) parseBackupPath(fields []string) string {
	if len(fields) <= 6 {
		return ""
	}
	backupPath := strings.TrimSpace(fields[6])
	for i := 7; i < len(fields); i++ {
		testPath := backupPath + "," + strings.TrimSpace(fields[i])
		if strings.Contains(testPath, ".") && (strings.HasSuffix(testPath, ".bak") || strings.HasSuffix(testPath, ".trn")) {
			backupPath = testPath
			break
		}
	}
	return backupPath
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
			return fmt.Errorf("invalid delete identifier: %w", err)
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

		if !found {
			return errors.New("未找到备份记录")
		}
	} else {
		cleanPath, err := sanitizeBackupPath(backupID, ".bak", ".trn")
		if err != nil {
			return fmt.Errorf("invalid backup file path: %w", err)
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
			return nil, fmt.Errorf("invalid backup id: %w", err)
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
		return fmt.Errorf("invalid backup path: %w", err)
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
		return fmt.Errorf("invalid backup id for unregister: %w", err)
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

	if len(lines) < 5 {
		logging.Info("没有需要检查的备份记录")
		return nil
	}

	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Contains(line, "行受影响") {
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

	var invalidBackupIDs []string
	for i := 3; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Contains(line, "行受影响") {
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

// parseBackupPaths 解析 SQL 查询返回的备份文件路径
func parseBackupPaths(output string) []string {
	var paths []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "physical_device_name" {
			continue
		}
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			continue
		}
		if strings.Contains(line, "----------") {
			continue
		}
		if strings.Contains(line, "行受影响") || strings.Contains(line, "rows affected") {
			continue
		}
		if len(line) > 0 {
			paths = append(paths, line)
		}
	}
	return paths
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

// Close 释放资源
func (m *MSSQLBackup) Close() error {
	return nil
}

// registerMSSQLDriver 注册 MSSQL 驱动
func registerMSSQLDriver() {
	RegisterDriver(DriverMetadata{
		Name:                 DBTypeMSSQL,
		Version:              versionXML,
		Description:          "SQL Server 数据库备份驱动，支持 sqlcmd 命令备份",
		SupportedActions:     []string{backupTypeXML, actionRestore, actionList, actionDelete, "validate", actionInfo, "register", "unregister", "verify-status", "delete-invalid", actionDeleteAll},
		SupportedBackupTypes: []BackupType{BackupTypePhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewMSSQLBackup(config)
	})
}
