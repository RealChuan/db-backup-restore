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

	"db-backup-restore/pkg/utils"
)

// backupSingleDatabaseLogical 逻辑备份单个数据库
func (m *MySQLBackup) backupSingleDatabaseLogical(ctx context.Context, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份数据库 %s...", databaseName))
	}

	backupFileName := GenerateBackupFilename(databaseName, "sql")
	backupPath := filepath.Join(backupDir, backupFileName)

	args := m.buildDumpCommandArgs()
	args = append(args, databaseName)

	if err := m.execMySQLDump(ctx, args, backupPath); err != nil {
		result.Error = fmt.Errorf("逻辑备份失败: %w", err)
		return result, result.Error
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
	result.Success = true

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
			utils.Warnf("逻辑备份数据库 %s 失败: %v, 继续备份其他数据库", dbName, err)
			continue
		}

		backupFiles = append(backupFiles, singleResult.BackupFile)
		totalSize += singleResult.BackupSize
	}

	if callback != nil {
		callback(100, "逻辑备份完成")
	}

	if len(backupFiles) == 0 {
		result.Error = errors.New("没有成功逻辑备份任何数据库")
		return result, result.Error
	}

	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = totalSize
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// backupAllDatabasesLogical 逻辑备份所有数据库
func (m *MySQLBackup) backupAllDatabasesLogical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := m.getAllDatabases(ctx)
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
		result.Error = errors.New("必须通过 --backup-identifier 参数指定备份文件路径")
		return result, result.Error
	}

	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		result.Error = fmt.Errorf("备份文件不存在: %s", backupFile)
		return result, result.Error
	}

	databaseName := opts.TargetDatabaseName
	if databaseName == "" {
		databaseName = ExtractDatabaseName(backupFile)
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		result.Error = fmt.Errorf("打开备份文件失败: %w", err)
		return result, result.Error
	}
	defer inputFile.Close()

	_, err = m.execMySQLFromFile(ctx, databaseName, inputFile)
	if err != nil {
		result.Error = fmt.Errorf("逻辑还原失败: %w", err)
		return result, result.Error
	}

	if callback != nil {
		callback(100, "逻辑还原完成")
	}

	result.Duration = time.Since(startTime)
	result.Success = true

	return result, nil
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
	utils.LogCommandInfo(cmdStr)

	args := m.buildConnectionArgs()
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
	args := m.buildConnectionArgs()
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
