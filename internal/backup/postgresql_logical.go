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
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// execSQL 执行 SQL 命令（通过 psql）
func (p *PostgreSQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	logging.InfoCtx(ctx, "执行脚本", "tool", "psql", "script", sqlStatement)
	args := []string{"-c", sqlStatement}
	if p.config.Database != "" {
		args = append(args, "-d", p.config.Database)
	}
	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	return runCapture(ctx, "psql", cmd, withEnv(p.env))
}

// execDump 执行 pg_dump 命令
func (p *PostgreSQLBackup) execDump(ctx context.Context, args []string, outputFile string) error {
	isDirOutput := false
	for i, arg := range args {
		if arg == "-F" && i+1 < len(args) && args[i+1] == "d" {
			isDirOutput = true
			break
		}
	}
	cmd := exec.CommandContext(ctx, p.pgDumpPath, args...)
	if isDirOutput {
		_, err := runCapture(ctx, "pg_dump", cmd, withEnv(p.env))
		return err
	}
	return runToFile(ctx, "pg_dump", cmd, outputFile, withEnv(p.env))
}

// execRestore 从文件执行 SQL（用于还原）
func (p *PostgreSQLBackup) execRestore(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	args := []string{"-d", databaseName}
	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	return runCapture(ctx, "psql", cmd, withEnv(p.env), withStdin(inputFile))
}

// backupLogicalSingle 逻辑备份单个数据库
func (p *PostgreSQLBackup) backupLogicalSingle(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := sanitizeDatabaseName(databaseName); err != nil {
		return nil, fmt.Errorf("数据库名称无效: %w", err)
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份数据库 %s...", databaseName))
	}

	backupFileName := GenerateBackupFilename(databaseName, "sql")
	backupPath := filepath.Join(backupDir, backupFileName)

	if _, err := sanitizeBackupPath(backupPath); err != nil {
		return nil, fmt.Errorf("备份路径无效: %w", err)
	}

	args := []string{
		"-F", "p",
		"--clean",
		"--if-exists",
		"-d", databaseName,
		"-f", backupPath,
	}

	if err := p.execDump(ctx, args, backupPath); err != nil {
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
func (p *PostgreSQLBackup) backupLogicalMultiple(ctx context.Context, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

		singleResult, err := p.backupLogicalSingle(ctx, backupDir, dbName, nil)
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
func (p *PostgreSQLBackup) backupLogicalAll(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := p.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return p.backupLogicalMultiple(ctx, backupDir, databases, callback)
}

// restoreLogical 执行 PostgreSQL 逻辑还原
func (p *PostgreSQLBackup) restoreLogical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
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
		return nil, fmt.Errorf("备份文件路径无效: %w", err)
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
		return nil, fmt.Errorf("目标数据库名称无效: %w", err)
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		return nil, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer inputFile.Close()

	_, err = p.execRestore(ctx, databaseName, inputFile)
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
func (p *PostgreSQLBackup) ListDatabases(ctx context.Context) ([]string, error) {
	output, err := p.execSQL(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false;")
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	var databases []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line == "datname" {
			continue
		}
		if line == "postgres" {
			continue
		}
		if strings.Contains(line, "----------") {
			continue
		}
		if strings.Contains(line, "行记录") || strings.Contains(line, "rows") {
			continue
		}
		if strings.HasPrefix(line, "(") && strings.HasSuffix(line, ")") {
			continue
		}
		databases = append(databases, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取输出失败: %w", err)
	}

	return databases, nil
}
