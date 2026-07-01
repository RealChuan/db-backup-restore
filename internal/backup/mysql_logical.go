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
	"strconv"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
)

// backupSingleDatabaseLogical 逻辑备份单个数据库
func (m *MySQLBackup) backupSingleDatabaseLogical(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := sanitizeDatabaseName(databaseName); err != nil {
		return nil, fmt.Errorf("invalid database name: %w", err)
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份数据库 %s...", databaseName))
	}

	backupFileName := GenerateBackupFilename(databaseName, "sql")
	backupPath := filepath.Join(backupDir, backupFileName)

	if _, err := sanitizeBackupPath(backupDir); err != nil {
		return nil, fmt.Errorf("invalid backup directory: %w", err)
	}

	args := m.buildDumpCommandArgs()
	args = append(args, databaseName)

	if err := m.execMySQLDump(ctx, args, backupPath); err != nil {
		return nil, fmt.Errorf("逻辑备份失败: %w", err)
	}

	if callback != nil {
		callback(100, "逻辑备份完成")
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

// backupMultipleDatabasesLogical 逻辑备份多个数据库
func (m *MySQLBackup) backupMultipleDatabasesLogical(ctx context.Context, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	var backupFiles []string
	var totalSize int64

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份 %d 个数据库...", len(databases)))
	}

	for i, dbName := range databases {
		if callback != nil {
			percent := float64(i) / float64(len(databases)) * 100
			callback(percent, fmt.Sprintf("正在逻辑备份数据库 %s (%d/%d)", dbName, i+1, len(databases)))
		}

		singleResult, err := m.backupSingleDatabaseLogical(ctx, backupDir, dbName, nil)
		if err != nil {
			logging.WarnCtx(ctx, "逻辑备份数据库失败，继续备份其他数据库", "db", dbName, "error", err)
			continue
		}

		backupFiles = append(backupFiles, singleResult.BackupFile)
		totalSize += singleResult.BackupSize
	}

	if callback != nil {
		callback(100, "逻辑备份完成")
	}

	if len(backupFiles) == 0 {
		return nil, errors.New("没有成功逻辑备份任何数据库")
	}

	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = totalSize
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()

	return result, nil
}

// backupAllDatabasesLogical 逻辑备份所有数据库
func (m *MySQLBackup) backupAllDatabasesLogical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return m.backupMultipleDatabasesLogical(ctx, backupDir, databases, callback)
}

// restoreLogical 执行 MySQL 逻辑还原
func (m *MySQLBackup) restoreLogical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行逻辑还原...")
	}

	var backupFile string
	if opts.BackupIdentifier != "" {
		backupFile = opts.BackupIdentifier
	}

	if backupFile == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份文件路径")
	}

	cleanFile, err := sanitizeBackupPath(backupFile, ".sql")
	if err != nil {
		return nil, fmt.Errorf("invalid backup file path: %w", err)
	}
	backupFile = cleanFile

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("备份文件不存在: %s", backupFile)
	}

	databaseName := opts.TargetDatabaseName
	if databaseName == "" {
		databaseName = ExtractDatabaseName(backupFile)
	}

	if err := sanitizeDatabaseName(databaseName); err != nil {
		return nil, fmt.Errorf("invalid target database name: %w", err)
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		return nil, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer inputFile.Close()

	_, err = m.execMySQLFromFile(ctx, databaseName, inputFile)
	if err != nil {
		return nil, fmt.Errorf("逻辑还原失败: %w", err)
	}

	if callback != nil {
		callback(100, "逻辑还原完成")
	}

	result.Duration = time.Since(startTime)
	result.TargetDatabase = databaseName

	return result, nil
}

// ListDatabases 获取所有数据库（排除系统数据库）。
func (m *MySQLBackup) ListDatabases(ctx context.Context) ([]string, error) {
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
		if line == "information_schema" || line == DBTypeMySQL || line == "performance_schema" || line == "sys" {
			continue
		}
		databases = append(databases, line)
	}

	return databases, nil
}

// buildConnectionArgs 构建 mysql/mysqldump 命令连接参数
func (m *MySQLBackup) buildConnectionArgs() []string {
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

// buildDumpCommandArgs 构建 mysqldump 命令参数
func (m *MySQLBackup) buildDumpCommandArgs() []string {
	args := m.buildConnectionArgs()

	args = append(args, "--single-transaction")
	args = append(args, "--quick")
	args = append(args, "--lock-tables=false")

	return args
}

// execSQL 执行 SQL 命令（通过 mysql）
func (m *MySQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	cmdStr := m.mysqlPath + " " + strings.Join(m.buildConnectionArgs(), " ") + " -e " + sqlStatement
	logging.LogCommandInfo(cmdStr)

	args := m.buildConnectionArgs()
	args = append(args, "-e", sqlStatement)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	output, err := shellexec.ExecCommand(cmd)
	if err != nil {
		logging.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("mysql 执行失败: %w", err)
	}
	logging.LogCommand(cmdStr, output, false)
	return output, nil
}

// execMySQLDump 执行 mysqldump 命令，直接写入文件
func (m *MySQLBackup) execMySQLDump(ctx context.Context, args []string, outputFile string) error {
	cmdStr := m.mysqldumpPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

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

	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		return fmt.Errorf("读取命令错误输出失败: %w", err)
	}

	stderrOutput, _ := shellexec.ConvertGBKToUTF8(stderrBytes)
	if err := cmd.Wait(); err != nil {
		logging.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("mysqldump 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		logging.LogCommand(cmdStr, stderrOutput, false)
	}

	logging.InfoCtx(ctx, "mysqldump 完成", "output_file", outputFile)
	return nil
}

// execMySQLFromFile 从文件执行 SQL（用于还原）
func (m *MySQLBackup) execMySQLFromFile(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	args := m.buildConnectionArgs()
	args = append(args, databaseName)
	cmdStr := m.mysqlPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	cmd.Stdin = inputFile
	output, err := shellexec.ExecCommand(cmd)
	if err != nil {
		logging.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("mysql 还原失败: %w", err)
	}
	logging.LogCommand(cmdStr, output, false)
	return output, nil
}
