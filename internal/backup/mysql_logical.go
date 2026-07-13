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
)

// execSQL 执行 SQL 命令（通过 mysql）
func (m *MySQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "mysql", "script", sqlStatement)
	args := m.buildConnectionArgs()
	args = append(args, "-e", sqlStatement)
	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	return runCapture(ctx, "mysql", cmd, withGBKConversion())
}

// execDump 执行 mysqldump 命令，直接写入文件
func (m *MySQLBackup) execDump(ctx context.Context, args []string, outputFile string) error {
	cmd := exec.CommandContext(ctx, m.mysqldumpPath, args...)
	return runToFile(ctx, "mysqldump", cmd, outputFile, withGBKConversion())
}

// execRestore 从文件执行 SQL（用于还原）
func (m *MySQLBackup) execRestore(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	args := m.buildConnectionArgs()
	args = append(args, databaseName)
	cmd := exec.CommandContext(ctx, m.mysqlPath, args...)
	return runCapture(ctx, "mysql", cmd, withGBKConversion(), withStdin(inputFile))
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
		isSystem := false
		for _, sysDb := range MySQLSystemDatabases {
			if line == sysDb {
				isSystem = true
				break
			}
		}
		if isSystem {
			continue
		}
		databases = append(databases, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取输出失败: %w", err)
	}

	return databases, nil
}

// backupLogicalSingle 逻辑备份单个数据库
func (m *MySQLBackup) backupLogicalSingle(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := sanitizeDatabaseName(databaseName); err != nil {
		return nil, fmt.Errorf("无效的数据库名: %w", err)
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份数据库 %s...", databaseName))
	}

	backupFileName := GenerateBackupFilename(databaseName, "sql")
	backupPath := filepath.Join(backupDir, backupFileName)

	if _, err := sanitizeBackupPath(backupPath); err != nil {
		return nil, fmt.Errorf("无效的备份路径: %w", err)
	}

	args := m.buildDumpCommandArgs()
	args = append(args, databaseName)

	if err := m.execDump(ctx, args, backupPath); err != nil {
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

// backupLogicalMultiple 逻辑备份多个数据库
func (m *MySQLBackup) backupLogicalMultiple(ctx context.Context, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

		singleResult, err := m.backupLogicalSingle(ctx, backupDir, dbName, nil)
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

// backupLogicalAll 逻辑备份所有数据库
func (m *MySQLBackup) backupLogicalAll(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return m.backupLogicalMultiple(ctx, backupDir, databases, callback)
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
		return nil, fmt.Errorf("无效的备份文件路径: %w", err)
	}
	backupFile = cleanFile

	if _, err := os.Stat(backupFile); err != nil {
		return nil, fmt.Errorf("备份文件不可访问: %w", err)
	}

	databaseName := opts.TargetDatabaseName
	if databaseName == "" {
		databaseName = ExtractDatabaseName(backupFile)
	}

	if err := sanitizeDatabaseName(databaseName); err != nil {
		return nil, fmt.Errorf("无效的目标数据库名: %w", err)
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		return nil, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer inputFile.Close()

	_, err = m.execRestore(ctx, databaseName, inputFile)
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
