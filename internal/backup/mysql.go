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
		mysqlPath = utils.AddExeExt(filepath.Join(val, "mysql"))
		mysqldumpPath = utils.AddExeExt(filepath.Join(val, "mysqldump"))
	}

	return &MySQLBackup{
		BaseBackup:    BaseBackup{config: config},
		mysqlPath:     mysqlPath,
		mysqldumpPath: mysqldumpPath,
	}, nil
}

// buildDumpCommandArgs 构建 mysqldump 命令参数
func (m *MySQLBackup) buildDumpCommandArgs(opts BackupOptions) []string {
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

	if opts.EnableCompression {
		args = append(args, "--compression-algorithms=zlib")
	}

	if opts.Type == BackupLogical {
		args = append(args, "--single-transaction")
		args = append(args, "--quick")
		args = append(args, "--lock-tables=false")
	}

	return args
}

// buildRestoreCommandArgs 构建 mysql 命令连接参数（用于恢复）
// 返回包含主机、端口、用户名、密码等连接信息的参数列表
func (m *MySQLBackup) buildRestoreCommandArgs() []string {
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
func (m *MySQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	cmdStr := m.mysqlPath + " " + strings.Join(m.buildRestoreCommandArgs(), " ") + " -e " + sqlStatement
	utils.LogCommandInfo(cmdStr)

	args := m.buildRestoreCommandArgs()
	args = append(args, "-e", sqlStatement)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	output, err := utils.ExecCommand(ctx, cmd)

	if err != nil {
		utils.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("mysql 执行失败: %w", err)
	}
	utils.LogCommand(cmdStr, output, false)
	return output, nil
}

// execMySQLDump 执行 mysqldump 命令，直接写入文件
func (m *MySQLBackup) execMySQLDump(ctx context.Context, args []string, outputFile string) error {
	cmdStr := m.mysqldumpPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

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

	stderrOutput, _ := utils.ConvertGBKToUTF8(stderrBytes)
	if err := cmd.Wait(); err != nil {
		utils.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("mysqldump 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		utils.LogCommand(cmdStr, stderrOutput, false)
	}

	utils.Infof("mysqldump 完成，输出文件: %s", outputFile)
	return nil
}

// execMySQLFromFile 从文件执行 SQL（用于还原）
func (m *MySQLBackup) execMySQLFromFile(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	args := m.buildRestoreCommandArgs()
	args = append(args, databaseName)
	cmdStr := m.mysqlPath + " " + strings.Join(args, " ")
	utils.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	cmd.Stdin = inputFile
	output, err := utils.ExecCommand(ctx, cmd)

	if err != nil {
		utils.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("mysql 还原失败: %w", err)
	}
	utils.LogCommand(cmdStr, output, false)
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
		args = m.buildDumpCommandArgs(opts)
		args = append(args, databaseName)

		if err := m.execMySQLDump(ctx, args, backupPath); err != nil {
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
	if opts.BackupIdentifier != "" {
		backupFile = opts.BackupIdentifier
	}

	if backupFile == "" {
		result.Error = errors.New("必须通过 --backup-identifier 参数指定备份文件路径")
		return result, result.Error
	}

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		result.Error = fmt.Errorf("备份文件不存在: %s", backupFile)
		return result, result.Error
	}

	databaseName := opts.TargetDatabaseName
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

// init 自动注册 MySQL 驱动
func init() {
	RegisterDriver(DriverMetadata{
		Name:                 "mysql",
		Version:              "1.0.0",
		Description:          "MySQL 数据库备份驱动，支持 mysqldump 逻辑备份",
		SupportedActions:     []string{"backup", "restore", "list", "delete", "info", "delete-all"},
		SupportedBackupTypes: []BackupType{BackupFull, BackupLogical, BackupPhysical},
	}, func(config *DBConfig) (DatabaseBackup, error) {
		return NewMySQLBackup(config)
	})
}
