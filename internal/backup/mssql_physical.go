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
)

// buildSQLCmd 构造 sqlcmd 命令。
// csvMode 为 true 时附加 CSV 格式参数（-h-1 -s, -w 65535）。
func (m *MSSQLBackup) buildSQLCmd(ctx context.Context, sqlStatement string, csvMode bool) *exec.Cmd {
	args := []string{"-S", m.buildServerArg()}
	if m.isWindowsAuth() {
		args = append(args, "-E")
	} else {
		args = append(args, "-U", m.config.User, "-P", m.config.Password)
	}
	args = append(args, "-C", "-Q", sqlStatement)

	if m.config.Database != "" {
		args = append(args, "-d", m.config.Database)
	}
	if csvMode {
		args = append(args, "-h-1", "-s,", "-w", "65535")
	}

	return exec.CommandContext(ctx, "sqlcmd", args...)
}

// execSQL 执行 SQL 命令（通过 sqlcmd），捕获输出。
func (m *MSSQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "sqlcmd", "script", sqlStatement)
	cmd := m.buildSQLCmd(ctx, sqlStatement, false)
	return runCapture(ctx, "sqlcmd", cmd, withEnv(m.env))
}

// execSQLWithCSV 执行 SQL 命令（通过 sqlcmd）并以 CSV 格式输出。
func (m *MSSQLBackup) execSQLWithCSV(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "sqlcmd", "script", sqlStatement)
	cmd := m.buildSQLCmd(ctx, sqlStatement, true)
	return runCapture(ctx, "sqlcmd", cmd, withEnv(m.env))
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

// backupLogicalSingle 备份单个数据库
func (m *MSSQLBackup) backupLogicalSingle(ctx context.Context, opts BackupOptions, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
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
		return nil, fmt.Errorf("构建备份脚本失败: %w", err)
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

// backupLogicalMultiple 备份多个数据库
func (m *MSSQLBackup) backupLogicalMultiple(ctx context.Context, opts BackupOptions, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

		// 复用 backupLogicalSingle
		singleResult, err := m.backupLogicalSingle(ctx, opts, backupDir, dbName, nil)
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

// backupLogicalAll 备份所有用户数据库
func (m *MSSQLBackup) backupLogicalAll(ctx context.Context, opts BackupOptions, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到用户数据库")
	}

	// 复用 backupLogicalMultiple
	return m.backupLogicalMultiple(ctx, opts, backupDir, databases, callback)
}

// checkDatabaseExists 检查指定数据库是否存在
func (m *MSSQLBackup) checkDatabaseExists(ctx context.Context, databaseName string) error {
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return fmt.Errorf("无效的数据库名: %w", err)
	}
	sqlScript := fmt.Sprintf(`
	IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = N'%s')
	BEGIN
	    RAISERROR('数据库 %s 不存在', 16, 1)
	END
	`, databaseName, databaseName)
	_, err := m.execSQL(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("数据库 %q 不存在或无法访问: %w", databaseName, err)
	}
	return nil
}

// buildBackupScript 根据选项生成备份脚本
func (m *MSSQLBackup) buildBackupScript(opts BackupOptions, _ string, databaseName, backupPath string) (string, error) {
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return "", fmt.Errorf("无效的数据库名: %w", err)
	}
	cleanPath, err := sanitizeBackupPath(backupPath, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("无效的备份路径: %w", err)
	}

	var script strings.Builder
	fmt.Fprintf(&script, "BACKUP DATABASE [%s] TO DISK = N'%s' WITH ", databaseName, cleanPath)

	if opts.EnableCompression {
		script.WriteString("COMPRESSION, ")
	}

	script.WriteString("STATS = 10")
	return script.String(), nil
}

// getDatabaseNameFromBackup 从备份文件中获取数据库名
func (m *MSSQLBackup) getDatabaseNameFromBackup(ctx context.Context, backupFile string) (string, error) {
	cleanPath, err := sanitizeBackupPath(backupFile, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("无效的备份文件路径: %w", err)
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
		return "", fmt.Errorf("无效的数据库名: %w", err)
	}
	// 安全校验：备份文件路径
	cleanPath, err := sanitizeBackupPath(backupFile, ".bak", ".trn")
	if err != nil {
		return "", fmt.Errorf("无效的备份文件路径: %w", err)
	}

	var script strings.Builder
	fmt.Fprintf(&script, "USE master; RESTORE DATABASE [%s] FROM DISK = N'%s' WITH ", databaseName, cleanPath)

	script.WriteString("REPLACE, ")

	if !opts.RecoveryPointInTime.IsZero() {
		timeStr := opts.RecoveryPointInTime.Format("2006-01-02 15:04:05")
		if _, err := sanitizeDateLiteral(timeStr); err != nil {
			return "", fmt.Errorf("无效的恢复时间点: %w", err)
		}
		fmt.Fprintf(&script, "STOPAT = '%s', ", timeStr)
	}

	script.WriteString("STATS = 10")
	return script.String(), nil
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

	// SQL Server 的 bs.type 字段表示备份模式，而非备份类型
	// MSSQL 仅支持物理备份，备份类型统一为 physical
	backupMode := mapMSSQLBackupMode(backupType)

	return BackupInfo{
		BackupID:       backupID,
		BackupType:     string(BackupTypePhysical),
		BackupMode:     backupMode,
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

// mapMSSQLBackupMode 将 SQL Server bs.type 值映射为标准 BackupMode 常量。
// SQL Server 类型: D=Database(Full), I=Differential, L=Log
func mapMSSQLBackupMode(mssqlType string) string {
	switch strings.ToUpper(strings.TrimSpace(mssqlType)) {
	case "FULL", "D":
		return string(BackupModeFull)
	case "INCREMENTAL", "DIFFERENTIAL", "I":
		return string(BackupModeIncremental)
	case "LOG", "L":
		return string(BackupModeArchive)
	default:
		return strings.ToLower(mssqlType)
	}
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
