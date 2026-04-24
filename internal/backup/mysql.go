package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"db-backup-restore/pkg/utils"
)

// MySQLBackup 实现 DatabaseBackup 接口，针对 MySQL 数据库
type MySQLBackup struct {
	BaseBackup
	env           []string // 环境变量
	mysqlPath     string   // mysql 命令路径
	mysqldumpPath string   // mysqldump 命令路径
}

// NewMySQLBackup 创建 MySQL 备份实例
func NewMySQLBackup(config *DBConfig) (*MySQLBackup, error) {
	if config.Type != "mysql" {
		return nil, errors.New("config.Type 必须是 mysql")
	}

	mysqlPath := "mysql"
	mysqldumpPath := "mysqldump"

	if val, ok := config.Extra["MYSQL_BIN_PATH"]; ok && val != "" {
		mysqlPath = filepath.Join(val, "mysql")
		if filepath.Ext(mysqlPath) == "" {
			mysqlPath += ".exe"
		}
		mysqldumpPath = filepath.Join(val, "mysqldump")
		if filepath.Ext(mysqldumpPath) == "" {
			mysqldumpPath += ".exe"
		}
	}

	return &MySQLBackup{
		BaseBackup:    BaseBackup{config: config},
		mysqlPath:     mysqlPath,
		mysqldumpPath: mysqldumpPath,
	}, nil
}

// buildDumpArgs 构建 mysqldump 命令参数
func (m *MySQLBackup) buildDumpArgs(opts BackupOptions) []string {
	var args []string

	if m.config.Host != "" {
		args = append(args, "-h", m.config.Host)
	}
	if m.config.Port != 0 {
		args = append(args, "-P", strconv.Itoa(m.config.Port))
	}
	if m.config.User != "" {
		args = append(args, "-u", m.config.User)
	}
	if m.config.Password != "" {
		args = append(args, "-p"+m.config.Password)
	}

	if opts.Compression {
		args = append(args, "--compression-algorithms=zlib")
	}

	if opts.Type == BackupLogical {
		args = append(args, "--single-transaction")
		args = append(args, "--quick")
		args = append(args, "--lock-tables=false")
	}

	return args
}

// buildRestoreArgs 构建 mysql 命令参数（用于恢复）
func (m *MySQLBackup) buildRestoreArgs() []string {
	var args []string

	if m.config.Host != "" {
		args = append(args, "-h", m.config.Host)
	}
	if m.config.Port != 0 {
		args = append(args, "-P", strconv.Itoa(m.config.Port))
	}
	if m.config.User != "" {
		args = append(args, "-u", m.config.User)
	}
	if m.config.Password != "" {
		args = append(args, "-p"+m.config.Password)
	}

	return args
}

// execSQL 执行 SQL 命令（通过 mysql）
func (m *MySQLBackup) execSQL(ctx context.Context, sqlText string) (string, error) {
	utils.Infof("\n========== SQL 命令开始 ==========\n%s\n========== SQL 命令结束 ==========", sqlText)

	args := m.buildRestoreArgs()
	args = append(args, "-e", sqlText)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	output, err := utils.ExecCommand(ctx, cmd)

	utils.Infof("\n========== SQL 执行输出开始 ==========\n%s\n========== SQL 执行输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("mysql 执行失败: %w", err)
	}
	return output, nil
}

// execMysqldump 执行 mysqldump 命令，直接写入文件
func (m *MySQLBackup) execMysqldump(ctx context.Context, args []string, outputFile string) error {
	utils.Infof("\n========== mysqldump 命令开始 ==========\n%s %s\n========== mysqldump 命令结束 ==========", m.mysqldumpPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, m.mysqldumpPath, args...)

	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建备份文件失败: %w", err)
	}
	defer file.Close()

	cmd.Stdout = file

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		stderrOutput, _ := utils.ConvertGBKToUTF8(stderrBytes)
		return fmt.Errorf("mysqldump 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	stderrOutput, _ := utils.ConvertGBKToUTF8(stderrBytes)
	if stderrOutput != "" {
		utils.Warnf("mysqldump 警告: %s", stderrOutput)
	}

	utils.Infof("mysqldump 完成，输出文件: %s", outputFile)
	return nil
}

// execMySQLFromFile 从文件执行 SQL（用于还原）
func (m *MySQLBackup) execMySQLFromFile(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	utils.Infof("\n========== MySQL 还原开始 ==========\n数据库: %s\n========== MySQL 还原命令结束 ==========", databaseName)

	args := m.buildRestoreArgs()
	args = append(args, databaseName)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	cmd.Stdin = inputFile
	output, err := utils.ExecCommand(ctx, cmd)

	utils.Infof("\n========== MySQL 还原输出开始 ==========\n%s\n========== MySQL 还原输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("mysql 还原失败: %w", err)
	}
	return output, nil
}

// Backup 执行 MySQL 备份
func (m *MySQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	backupDir := opts.TargetPath
	if backupDir == "" {
		result.Error = errors.New("必须通过 -target-path 参数指定备份路径")
		return result, result.Error
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		result.Error = err
		return result, err
	}

	databaseName := m.config.Database

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
func (m *MySQLBackup) parseDatabaseNames(databaseName string) []string {
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
func (m *MySQLBackup) backupSingleDatabase(ctx context.Context, opts BackupOptions, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始备份数据库 %s...", databaseName))
	}

	backupFileName := fmt.Sprintf("%s_%s.sql", databaseName, time.Now().Format("20060102_150405"))
	backupPath := filepath.Join(backupDir, backupFileName)

	var args []string

	if opts.Type == BackupLogical || opts.Type == BackupFull || opts.Type == BackupPhysical {
		args = m.buildDumpArgs(opts)
		args = append(args, databaseName)

		if err := m.execMysqldump(ctx, args, backupPath); err != nil {
			result.Error = fmt.Errorf("备份失败: %w", err)
			return result, result.Error
		}
	} else {
		result.Error = errors.New("MySQL 仅支持 full、logical 和 physical 备份类型")
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
func (m *MySQLBackup) backupMultipleDatabases(ctx context.Context, opts BackupOptions, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

// backupAllDatabases 备份所有数据库
func (m *MySQLBackup) backupAllDatabases(ctx context.Context, opts BackupOptions, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.getAllDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return m.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
}

// getAllDatabases 获取所有数据库（排除系统数据库）
func (m *MySQLBackup) getAllDatabases(ctx context.Context) ([]string, error) {
	output, err := m.execSQL(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	var databases []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "Database" {
			continue
		}
		if line == "information_schema" || line == "mysql" || line == "performance_schema" || line == "sys" {
			continue
		}
		utils.Infof("发现数据库: |%s|", line)

		databases = append(databases, line)
	}

	return databases, nil
}

// Restore 执行 MySQL 还原
func (m *MySQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
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

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		result.Error = fmt.Errorf("备份文件不存在: %s", backupFile)
		return result, result.Error
	}

	databaseName := opts.TargetDB
	if databaseName == "" {
		databaseName = m.extractDatabaseName(backupFile)
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		result.Error = fmt.Errorf("打开备份文件失败: %w", err)
		return result, result.Error
	}
	defer inputFile.Close()

	_, err = m.execMySQLFromFile(ctx, databaseName, inputFile)
	if err != nil {
		result.Error = fmt.Errorf("还原失败: %w", err)
		return result, result.Error
	}

	if callback != nil {
		callback(100, "还原完成")
	}

	result.Duration = time.Since(startTime)
	result.Success = true

	return result, nil
}

// extractDatabaseName 从备份文件名中提取数据库名
func (m *MySQLBackup) extractDatabaseName(backupFile string) string {
	baseName := filepath.Base(backupFile)
	re := regexp.MustCompile(`^(.+)_(\d{8})_(\d{6})\.sql$`)
	if matches := re.FindStringSubmatch(baseName); len(matches) > 1 {
		return matches[1]
	}
	return filepath.Base(backupFile)
}

// ListBackups 列出所有备份（从文件系统）
func (m *MySQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	backupDir := m.getBackupDir(opts)
	if backupDir == "" {
		return nil, errors.New("必须通过 opts.TargetPath 指定备份目录")
	}

	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return nil, fmt.Errorf("列出备份失败: %w", err)
	}

	var backups []BackupInfo
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		bi := BackupInfo{
			BackupID:       filepath.Base(file),
			CompletionTime: info.ModTime(),
			Size:           info.Size(),
			BackupPath:     file,
		}
		backups = append(backups, bi)
	}

	return backups, nil
}

// DeleteBackup 删除指定备份
func (m *MySQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	var backupPath string
	if filepath.IsAbs(identifier) {
		backupPath = identifier
	} else {
		backupDir := m.getBackupDir(opts)
		if backupDir == "" {
			return errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, identifier)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("删除备份失败: %w", err)
	}
	return nil
}

// ValidateBackup 验证备份有效性
func (m *MySQLBackup) ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	utils.Warnf("MySQL 逻辑备份文件无法完全验证有效性")
	return nil
}

// GetBackupInfo 获取指定备份的详细信息
func (m *MySQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	if filepath.IsAbs(backupID) {
		backupPath = backupID
	} else {
		backupDir := m.getBackupDir(opts)
		if backupDir == "" {
			return nil, errors.New("必须通过 opts.TargetPath 指定备份目录或提供绝对路径")
		}
		backupPath = filepath.Join(backupDir, backupID)
	}

	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("获取备份信息失败: %w", err)
	}

	result := make(map[string]string)
	result["path"] = backupPath
	result["size"] = strconv.FormatInt(info.Size(), 10)
	result["mod_time"] = info.ModTime().Format(time.RFC3339)
	result["backup_type"] = "LOGICAL"

	return result, nil
}

// RegisterBackup 将指定路径的备份文件注册到备份目录库
func (m *MySQLBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	utils.Warnf("MySQL 不使用备份目录库，备份文件直接存储在文件系统中")
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (m *MySQLBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	utils.Warnf("MySQL 不支持取消注册备份功能")
	return nil
}

// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
func (m *MySQLBackup) VerifyBackupStatus(ctx context.Context) error {
	utils.Warnf("MySQL 不支持检查备份状态功能")
	return nil
}

// DeleteInvalidBackups 删除无效的备份记录
func (m *MySQLBackup) DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error {
	utils.Warnf("MySQL 逻辑备份文件无法验证有效性，跳过删除无效备份操作")
	return nil
}

// DeleteAllBackups 删除所有备份
func (m *MySQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	backupDir := m.getBackupDir(opts)
	if backupDir == "" {
		return errors.New("必须通过 opts.TargetPath 指定备份目录")
	}

	files, err := filepath.Glob(filepath.Join(backupDir, "*.sql*"))
	if err != nil {
		return fmt.Errorf("列出备份失败: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			utils.Warnf("删除备份失败: %v", err)
		}
	}
	return nil
}

// getBackupDir 从选项中获取备份目录
func (m *MySQLBackup) getBackupDir(opts []BackupOptions) string {
	if len(opts) > 0 && opts[0].TargetPath != "" {
		return opts[0].TargetPath
	}
	return ""
}

// Close 释放资源
func (m *MySQLBackup) Close() error {
	return nil
}
