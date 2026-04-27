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

// PostgreSQLBackup 实现 DatabaseBackup 接口，针对 PostgreSQL 数据库
type PostgreSQLBackup struct {
	BaseBackup
	psqlPath      string   // psql 命令路径
	pgDumpPath    string   // pg_dump 命令路径
	pgDumpallPath string   // pg_dumpall 命令路径
	env           []string // 环境变量
}

// NewPostgreSQLBackup 创建 PostgreSQL 备份实例
func NewPostgreSQLBackup(config *DBConfig) (*PostgreSQLBackup, error) {
	if config.Type != "postgresql" {
		return nil, errors.New("config.Type 必须是 postgresql")
	}

	psqlPath := "psql"
	pgDumpPath := "pg_dump"
	pgDumpallPath := "pg_dumpall"

	if val, ok := config.Extra["PG_BIN_PATH"]; ok && val != "" {
		psqlPath = filepath.Join(val, "psql")
		if filepath.Ext(psqlPath) == "" {
			psqlPath += ".exe"
		}
		pgDumpPath = filepath.Join(val, "pg_dump")
		if filepath.Ext(pgDumpPath) == "" {
			pgDumpPath += ".exe"
		}
		pgDumpallPath = filepath.Join(val, "pg_dumpall")
		if filepath.Ext(pgDumpallPath) == "" {
			pgDumpallPath += ".exe"
		}
	}

	env := []string{
		fmt.Sprintf("PGHOST=%s", config.Host),
		fmt.Sprintf("PGPORT=%d", config.Port),
		fmt.Sprintf("PGUSER=%s", config.User),
		fmt.Sprintf("PGPASSWORD=%s", config.Password),
		fmt.Sprintf("PGDATABASE=%s", config.Database),
	}

	if config.SSLMode != "" {
		env = append(env, fmt.Sprintf("PGSSLMODE=%s", config.SSLMode))
	}

	return &PostgreSQLBackup{
		BaseBackup:    BaseBackup{config: config},
		psqlPath:      psqlPath,
		pgDumpPath:    pgDumpPath,
		pgDumpallPath: pgDumpallPath,
		env:           env,
	}, nil
}

// buildDumpArgs 构建 pg_dump 命令参数
func (p *PostgreSQLBackup) buildDumpArgs(opts BackupOptions) []string {
	var args []string

	if opts.Type == BackupPhysical {
		args = append(args, "-F", "d")
		if opts.Compression {
			level := opts.CompressionLevel
			if level <= 0 || level > 9 {
				level = 6
			}
			args = append(args, "-Z", strconv.Itoa(level))
		}
		if opts.Parallelism > 1 {
			args = append(args, "-j", strconv.Itoa(opts.Parallelism))
		}
	} else {
		args = append(args, "-F", "p")
		args = append(args, "--clean")
		args = append(args, "--if-exists")
	}

	return args
}

// buildRestoreArgs 构建 psql 命令参数（用于恢复）
func (p *PostgreSQLBackup) buildRestoreArgs() []string {
	var args []string
	return args
}

// execSQL 执行 SQL 命令（通过 psql）
func (p *PostgreSQLBackup) execSQL(ctx context.Context, sqlText string) (string, error) {
	utils.Infof("\n========== SQL 命令开始 ==========\n%s\n========== SQL 命令结束 ==========", sqlText)

	args := []string{"-c", sqlText}
	if p.config.Database != "" {
		args = append(args, "-d", p.config.Database)
	}

	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)

	output, err := utils.ExecCommandWithEncoding(ctx, cmd, false)

	utils.Infof("\n========== SQL 执行输出开始 ==========\n%s\n========== SQL 执行输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("psql 执行失败: %w", err)
	}
	return output, nil
}

// execPgDump 执行 pg_dump 命令
func (p *PostgreSQLBackup) execPgDump(ctx context.Context, args []string, outputFile string) error {
	utils.Infof("\n========== pg_dump 命令开始 ==========\n%s %s\n========== pg_dump 命令结束 ==========", p.pgDumpPath, strings.Join(args, " "))

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

	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("pg_dump 执行失败: %w, stderr: %s", err, string(stderrBytes))
	}

	if string(stderrBytes) != "" {
		utils.Warnf("pg_dump 警告: %s", string(stderrBytes))
	}

	utils.Infof("pg_dump 完成，输出: %s", outputFile)
	return nil
}

// execPsqlFromFile 从文件执行 SQL（用于还原）
func (p *PostgreSQLBackup) execPsqlFromFile(ctx context.Context, databaseName string, inputFile io.Reader) (string, error) {
	utils.Infof("\n========== PostgreSQL 还原开始 ==========\n数据库: %s\n========== PostgreSQL 还原命令结束 ==========", databaseName)

	args := []string{"-d", databaseName}

	cmd := exec.CommandContext(ctx, p.psqlPath, args...)
	cmd.Env = append(os.Environ(), p.env...)
	cmd.Stdin = inputFile
	output, err := utils.ExecCommandWithEncoding(ctx, cmd, false)

	utils.Infof("\n========== PostgreSQL 还原输出开始 ==========\n%s\n========== PostgreSQL 还原输出结束 ==========", output)
	if err != nil {
		return output, fmt.Errorf("psql 还原失败: %w", err)
	}
	return output, nil
}

// Backup 执行 PostgreSQL 备份
func (p *PostgreSQLBackup) Backup(ctx context.Context, opts BackupOptions, callback ProgressCallback) (*BackupResult, error) {
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

	databaseName := p.config.Database

	databases := p.parseDatabaseNames(databaseName)

	if len(databases) == 0 {
		return p.backupAllDatabases(ctx, opts, backupDir, callback)
	}

	if len(databases) == 1 {
		return p.backupSingleDatabase(ctx, opts, backupDir, databases[0], callback)
	}

	return p.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
}

// parseDatabaseNames 解析数据库名称（支持逗号分隔的多个数据库）
func (p *PostgreSQLBackup) parseDatabaseNames(databaseName string) []string {
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
func (p *PostgreSQLBackup) backupSingleDatabase(ctx context.Context, opts BackupOptions, backupDir, databaseName string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始备份数据库 %s...", databaseName))
	}

	timeStr := time.Now().Format("20060102_150405")

	var backupPath string
	if opts.Type == BackupPhysical {
		backupPath = filepath.Join(backupDir, fmt.Sprintf("%s_%s", databaseName, timeStr))
	} else {
		backupFileName := fmt.Sprintf("%s_%s.sql", databaseName, timeStr)
		backupPath = filepath.Join(backupDir, backupFileName)
	}

	var args []string

	if opts.Type == BackupLogical || opts.Type == BackupFull || opts.Type == BackupPhysical {
		args = p.buildDumpArgs(opts)
		args = append(args, "-d", databaseName, "-f", backupPath)

		if opts.Type == BackupPhysical {
			if err := os.RemoveAll(backupPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("删除旧备份目录失败: %w", err)
			}
			if err := os.MkdirAll(backupPath, 0755); err != nil {
				return nil, fmt.Errorf("创建备份目录失败: %w", err)
			}
		}

		if err := p.execPgDump(ctx, args, backupPath); err != nil {
			result.Error = fmt.Errorf("备份失败: %w", err)
			return result, result.Error
		}
	} else {
		result.Error = errors.New("PostgreSQL 仅支持 full、logical 和 physical 备份类型")
		return result, result.Error
	}

	if callback != nil {
		callback(100, "备份完成")
	}

	if info, err := os.Stat(backupPath); err == nil {
		result.BackupFile = backupPath
		if info.IsDir() {
			result.BackupSize = p.getDirSize(backupPath)
		} else {
			result.BackupSize = info.Size()
		}
	} else {
		result.BackupFile = backupPath
	}

	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Success = true

	return result, nil
}

// getDirSize 计算目录大小
func (p *PostgreSQLBackup) getDirSize(path string) int64 {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		utils.Warnf("计算目录大小失败: %v", err)
	}
	return size
}

// backupMultipleDatabases 备份多个数据库
func (p *PostgreSQLBackup) backupMultipleDatabases(ctx context.Context, opts BackupOptions, backupDir string, databases []string, callback ProgressCallback) (*BackupResult, error) {
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

		singleResult, err := p.backupSingleDatabase(ctx, opts, backupDir, dbName, nil)
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
func (p *PostgreSQLBackup) backupAllDatabases(ctx context.Context, opts BackupOptions, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databases, err := p.getAllDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库列表失败: %w", err)
	}

	if len(databases) == 0 {
		return nil, errors.New("未找到数据库")
	}

	return p.backupMultipleDatabases(ctx, opts, backupDir, databases, callback)
}

// getAllDatabases 获取所有数据库（排除系统数据库）
func (p *PostgreSQLBackup) getAllDatabases(ctx context.Context) ([]string, error) {
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
		utils.Infof("发现数据库: |%s|", line)

		databases = append(databases, line)
	}

	return databases, nil
}

// Restore 执行 PostgreSQL 还原
func (p *PostgreSQLBackup) Restore(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	if callback != nil {
		callback(0, "开始执行还原...")
	}

	var backupFile string
	if opts.BackupTag != "" {
		backupFile = opts.BackupTag
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
		databaseName = p.extractDatabaseName(backupFile)
	}

	if !opts.Overwrite {
		if err := p.createDatabaseIfNotExists(ctx, databaseName); err != nil {
			result.Error = fmt.Errorf("创建数据库失败: %w", err)
			return result, result.Error
		}
	}

	inputFile, err := os.Open(backupFile)
	if err != nil {
		result.Error = fmt.Errorf("打开备份文件失败: %w", err)
		return result, result.Error
	}
	defer inputFile.Close()

	_, err = p.execPsqlFromFile(ctx, databaseName, inputFile)
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

// createDatabaseIfNotExists 如果数据库不存在则创建
func (p *PostgreSQLBackup) createDatabaseIfNotExists(ctx context.Context, databaseName string) error {
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

// extractDatabaseName 从备份文件名中提取数据库名
func (p *PostgreSQLBackup) extractDatabaseName(backupFile string) string {
	baseName := filepath.Base(backupFile)
	re := regexp.MustCompile(`^(.+)_(\d{8})_(\d{6})\.sql$`)
	if matches := re.FindStringSubmatch(baseName); len(matches) > 1 {
		return matches[1]
	}
	return filepath.Base(backupFile)
}

// ListBackups 列出所有备份（从文件系统）
func (p *PostgreSQLBackup) ListBackups(ctx context.Context, opts ...BackupOptions) ([]BackupInfo, error) {
	backupDir := p.getBackupDir(opts)
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
func (p *PostgreSQLBackup) DeleteBackup(ctx context.Context, identifier string, opts ...BackupOptions) error {
	var backupPath string
	if filepath.IsAbs(identifier) {
		backupPath = identifier
	} else {
		backupDir := p.getBackupDir(opts)
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
func (p *PostgreSQLBackup) ValidateBackup(ctx context.Context, backupID string, opts ...BackupOptions) error {
	utils.Warnf("PostgreSQL 逻辑备份文件无法完全验证有效性")
	return nil
}

// GetBackupInfo 获取指定备份的详细信息
func (p *PostgreSQLBackup) GetBackupInfo(ctx context.Context, backupID string, opts ...BackupOptions) (map[string]string, error) {
	if backupID == "" {
		return nil, errors.New("必须指定备份文件路径")
	}

	var backupPath string
	if filepath.IsAbs(backupID) {
		backupPath = backupID
	} else {
		backupDir := p.getBackupDir(opts)
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
func (p *PostgreSQLBackup) RegisterBackup(ctx context.Context, backupPath string) error {
	utils.Warnf("PostgreSQL 不使用备份目录库，备份文件直接存储在文件系统中")
	return nil
}

// UnregisterBackup 从备份目录库中移除指定备份
func (p *PostgreSQLBackup) UnregisterBackup(ctx context.Context, backupID string) error {
	utils.Warnf("PostgreSQL 不支持取消注册备份功能")
	return nil
}

// VerifyBackupStatus 检查备份文件的状态并更新备份目录库
func (p *PostgreSQLBackup) VerifyBackupStatus(ctx context.Context) error {
	utils.Warnf("PostgreSQL 不支持检查备份状态功能")
	return nil
}

// DeleteInvalidBackups 删除无效的备份记录
func (p *PostgreSQLBackup) DeleteInvalidBackups(ctx context.Context, opts ...BackupOptions) error {
	utils.Warnf("PostgreSQL 逻辑备份文件无法验证有效性，跳过删除无效备份操作")
	return nil
}

// DeleteAllBackups 删除所有备份
func (p *PostgreSQLBackup) DeleteAllBackups(ctx context.Context, opts ...BackupOptions) error {
	backupDir := p.getBackupDir(opts)
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
func (p *PostgreSQLBackup) getBackupDir(opts []BackupOptions) string {
	if len(opts) > 0 && opts[0].TargetPath != "" {
		return opts[0].TargetPath
	}
	return ""
}

// Close 释放资源
func (p *PostgreSQLBackup) Close() error {
	return nil
}
