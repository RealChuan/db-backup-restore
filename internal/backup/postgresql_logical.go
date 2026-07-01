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
	"github.com/RealChuan/db-backup-restore/pkg/shellexec"
)

// execSQL 执行 SQL 命令（通过 psql）
func (p *PostgreSQLBackup) execSQL(ctx context.Context, sqlStatement string) (string, error) {
	args := []string{"-c", sqlStatement}
	if p.config.Database != "" {
		args = append(args, "-d", p.config.Database)
	}
	cmdStr := p.psqlPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	output, err := shellexec.ExecCommandWithEncoding(cmd, false)
	if err != nil {
		logging.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("psql 执行失败: %w", err)
	}
	logging.LogCommand(cmdStr, output, false)
	return output, nil
}

// execPgDump 执行 pg_dump 命令
func (p *PostgreSQLBackup) execPgDump(ctx context.Context, args []string, outputFile string) error {
	cmdStr := p.pgDumpPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, p.pgDumpPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	isDirOutput := false
	for i, arg := range args {
		if arg == "-F" && i+1 < len(args) && args[i+1] == "d" {
			isDirOutput = true
			break
		}
	}

	if !isDirOutput {
		file, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("创建备份文件失败: %w", err)
		}
		defer file.Close()
		cmd.Stdout = file
	}

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
	stderrOutput := string(stderrBytes)

	if err := cmd.Wait(); err != nil {
		logging.LogCommand(cmdStr, stderrOutput, true)
		return fmt.Errorf("pg_dump 执行失败: %w, stderr: %s", err, stderrOutput)
	}

	if stderrOutput != "" {
		logging.LogCommand(cmdStr, stderrOutput, false)
	}

	logging.InfoCtx(ctx, "pg_dump 完成", "output", outputFile)
	return nil
}

// execPsqlFromFile 从文件执行 SQL（用于还原）
func (p *PostgreSQLBackup) execPsqlFromFile(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	args := []string{"-d", databaseName}
	cmdStr := p.psqlPath + " " + strings.Join(args, " ")
	logging.LogCommandInfo(cmdStr)

	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)
	cmd.Stdin = inputFile
	output, err := shellexec.ExecCommandWithEncoding(cmd, false)
	if err != nil {
		logging.LogCommand(cmdStr, output, true)
		return output, fmt.Errorf("psql 还原失败: %w", err)
	}
	logging.LogCommand(cmdStr, output, false)
	return output, nil
}

// backupSingleDatabaseLogical 逻辑备份单个数据库
func (p *PostgreSQLBackup) backupSingleDatabaseLogical(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
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

	args := []string{
		"-F", "p",
		"--clean",
		"--if-exists",
		"-d", databaseName,
		"-f", backupPath,
	}

	if err := p.execPgDump(ctx, args, backupPath); err != nil {
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
func (p *PostgreSQLBackup) backupMultipleDatabasesLogical(ctx context.Context, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

		singleResult, err := p.backupSingleDatabaseLogical(ctx, backupDir, dbName, nil)
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
func (p *PostgreSQLBackup) backupAllDatabasesLogical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := p.ListDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return p.backupMultipleDatabasesLogical(ctx, backupDir, databases, callback)
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

	if !opts.Overwrite {
		if err := p.createDatabaseIfNotExists(ctx, databaseName); err != nil {
			return nil, fmt.Errorf("创建数据库失败: %w", err)
		}
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		return nil, fmt.Errorf("打开备份文件失败: %w", err)
	}
	defer inputFile.Close()

	_, err = p.execPsqlFromFile(ctx, databaseName, inputFile)
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

	return databases, nil
}

// createDatabaseIfNotExists 如果数据库不存在则创建
func (p *PostgreSQLBackup) createDatabaseIfNotExists(ctx context.Context, databaseName string) error {
	if err := sanitizeDatabaseName(databaseName); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}

	existsSQL := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s';", databaseName)
	output, err := p.execSQL(ctx, existsSQL)
	if err != nil {
		return err
	}

	if !strings.Contains(output, "1") {
		createSQL := fmt.Sprintf("CREATE DATABASE \"%s\";", databaseName)
		_, err = p.execSQL(ctx, createSQL)
		if err != nil {
			return err
		}
	}

	return nil
}
