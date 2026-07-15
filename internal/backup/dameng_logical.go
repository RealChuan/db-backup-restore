package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RealChuan/db-backup-restore/internal/logging"
)

// ListDatabases 通过 disql 查询用户模式列表
func (d *DamengBackup) ListDatabases(ctx context.Context) ([]string, error) {
	output, err := d.execSQL(ctx, "SELECT USERNAME FROM DBA_USERS WHERE ACCOUNT_STATUS = 'OPEN';")
	if err != nil {
		return nil, fmt.Errorf("获取模式列表失败: %w", err)
	}

	var databases []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" || name == "USERNAME" || name == "行号" {
			continue
		}
		// 排除系统模式
		isSystem := false
		for _, sysSchema := range DamengSystemSchemas {
			if strings.EqualFold(name, sysSchema) {
				isSystem = true
				break
			}
		}
		if !isSystem {
			databases = append(databases, name)
		}
	}

	return databases, nil
}

// backupLogical 执行达梦逻辑备份（dexp）
func (d *DamengBackup) backupLogical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	databaseName := d.config.Database
	databases := d.parseDatabaseNames(databaseName)

	if len(databases) == 0 {
		// FULL 模式
		return d.backupFullLogical(ctx, backupDir, callback)
	}
	// SCHEMAS 模式
	return d.backupLogicalAll(ctx, backupDir, databases, callback)
}

// backupFullLogical 全库逻辑备份
func (d *DamengBackup) backupFullLogical(ctx context.Context, backupDir string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if callback != nil {
		callback(0, "开始达梦全库逻辑备份...")
	}

	timestamp := time.Now().Format("20060102_150405")
	backupFileName := fmt.Sprintf("dameng_full_%s.dmp", timestamp)
	backupPath := filepath.Join(backupDir, backupFileName)

	if _, err := sanitizeBackupPath(backupPath); err != nil {
		return nil, fmt.Errorf("invalid backup path: %w", err)
	}

	args := d.buildDexpArgs(backupPath, timestamp, backupModeFULL)

	logging.InfoCtx(ctx, "达梦全库逻辑备份", "output_file", backupPath)
	output, err := d.execDump(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("全库逻辑备份失败: %w, 输出: %s", err, output)
	}

	if callback != nil {
		callback(100, "全库逻辑备份完成")
	}

	if info, err := os.Stat(backupPath); err == nil {
		result.BackupFile = backupPath
		result.BackupSize = info.Size()
	} else {
		result.BackupFile = backupPath
	}

	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Metadata["backup_mode"] = backupModeFULL

	return result, nil
}

// backupLogicalAll 按模式逻辑备份
func (d *DamengBackup) backupLogicalAll(ctx context.Context, backupDir string, schemas []string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if len(schemas) == 1 {
		return d.backupLogicalSingle(ctx, backupDir, schemas[0], callback)
	}

	// 多模式：逐个备份
	var backupFiles []string
	var totalSize int64

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份 %d 个模式...", len(schemas)))
	}

	for i, schema := range schemas {
		if err := sanitizeDatabaseName(schema); err != nil {
			logging.WarnCtx(ctx, "模式名校验失败，跳过", "schema", schema, "error", err)
			continue
		}

		if callback != nil {
			percent := float64(i) / float64(len(schemas)) * 100
			callback(percent, fmt.Sprintf("正在备份模式 %s (%d/%d)", schema, i+1, len(schemas)))
		}

		singleResult, err := d.backupLogicalSingle(ctx, backupDir, schema, nil)
		if err != nil {
			logging.WarnCtx(ctx, "逻辑备份模式失败，继续备份其他模式", "schema", schema, "error", err)
			continue
		}

		backupFiles = append(backupFiles, singleResult.BackupFile)
		totalSize += singleResult.BackupSize
	}

	if callback != nil {
		callback(100, "逻辑备份完成")
	}

	if len(backupFiles) == 0 {
		return nil, errors.New("没有成功逻辑备份任何模式")
	}

	result.BackupFile = strings.Join(backupFiles, ",")
	result.BackupSize = totalSize
	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Metadata["backup_mode"] = backupModeSCHEMAS

	return result, nil
}

// backupLogicalSingle 单模式逻辑备份
func (d *DamengBackup) backupLogicalSingle(ctx context.Context, backupDir, schema string, callback ProgressCallback) (*BackupResult, error) {
	startTime := time.Now()
	result := &BackupResult{
		StartTime: startTime,
		Metadata:  make(map[string]string),
	}

	if err := sanitizeDatabaseName(schema); err != nil {
		return nil, fmt.Errorf("invalid schema name: %w", err)
	}

	if callback != nil {
		callback(0, fmt.Sprintf("开始逻辑备份模式 %s...", schema))
	}

	timestamp := time.Now().Format("20060102_150405")
	backupFileName := fmt.Sprintf("%s_%s.dmp", schema, timestamp)
	backupPath := filepath.Join(backupDir, backupFileName)

	if _, err := sanitizeBackupPath(backupPath); err != nil {
		return nil, fmt.Errorf("invalid backup path: %w", err)
	}

	args := d.buildDexpArgs(backupPath, timestamp, backupModeSCHEMAS, schema)

	logging.InfoCtx(ctx, "达梦模式逻辑备份", "schema", schema, "output_file", backupPath)
	output, err := d.execDump(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("模式逻辑备份失败: %w, 输出: %s", err, output)
	}

	if callback != nil {
		callback(100, "模式逻辑备份完成")
	}

	if info, err := os.Stat(backupPath); err == nil {
		result.BackupFile = backupPath
		result.BackupSize = info.Size()
	} else {
		result.BackupFile = backupPath
	}

	result.Duration = time.Since(startTime)
	result.EndTime = time.Now()
	result.Metadata["backup_mode"] = backupModeSCHEMAS

	return result, nil
}

// buildDexpArgs 构建 dexp 命令参数
func (d *DamengBackup) buildDexpArgs(outputFile string, timestamp string, mode string, modeValue ...string) []string {
	args := []string{
		fmt.Sprintf("USERID=%s", d.buildConnectionString()),
		fmt.Sprintf("FILE=%s", outputFile),
		fmt.Sprintf("LOG=%s_%s.log", outputFile, timestamp),
		"ROWS=Y",
		"FEEDBACK=1000",
	}

	switch mode {
	case backupModeFULL:
		args = append(args, "FULL=Y")
	case backupModeSCHEMAS:
		if len(modeValue) > 0 {
			args = append(args, fmt.Sprintf("SCHEMAS=%s", modeValue[0]))
		}
	}

	extra := d.config.GetExtraTyped()
	if extra.EnableCompression() {
		args = append(args, "COMPRESS=Y")
		args = append(args, "COMPRESS_LEVEL=2")
	}

	if extra.ParallelWorkers() > 1 {
		args = append(args, fmt.Sprintf("PARALLEL=%d", extra.ParallelWorkers()))
	}

	return args
}

// restoreLogical 执行达梦逻辑还原（dimp）
func (d *DamengBackup) restoreLogical(ctx context.Context, opts RestoreOptions, callback ProgressCallback) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{}

	backupFile := opts.BackupIdentifier
	if backupFile == "" {
		return nil, errors.New("必须通过 --backup-identifier 参数指定备份文件路径")
	}

	cleanFile, err := sanitizeBackupPath(backupFile, ".dmp")
	if err != nil {
		return nil, fmt.Errorf("invalid backup file path: %w", err)
	}
	backupFile = cleanFile

	if _, err := os.Stat(backupFile); err != nil {
		return nil, fmt.Errorf("备份文件不可访问: %w", err)
	}

	// 校验 REMAP_SCHEMA 参数格式
	if opts.RemapSchema != "" {
		if err := validateRemapSchema(opts.RemapSchema); err != nil {
			return nil, fmt.Errorf("REMAP_SCHEMA 参数无效: %w", err)
		}
	}

	if callback != nil {
		callback(0, "开始执行逻辑还原...")
	}

	args := d.buildDimpArgs(backupFile, time.Now().Format("20060102_150405"), opts)

	logging.InfoCtx(ctx, "达梦逻辑还原", "backup_file", backupFile)
	output, err := d.execRestore(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("逻辑还原失败: %w, 输出: %s", err, output)
	}

	if callback != nil {
		callback(100, "逻辑还原完成")
	}

	result.Duration = time.Since(startTime)
	if opts.TargetDatabaseName != "" {
		result.TargetDatabase = opts.TargetDatabaseName
	}

	return result, nil
}

// buildDimpArgs 构建 dimp 命令参数
func (d *DamengBackup) buildDimpArgs(backupFile string, timestamp string, opts RestoreOptions) []string {
	args := []string{
		fmt.Sprintf("USERID=%s", d.buildConnectionString()),
		fmt.Sprintf("FILE=%s", backupFile),
		fmt.Sprintf("LOG=%s_%s.restore.log", backupFile, timestamp),
		"TABLE_EXISTS_ACTION=REPLACE",
		"COMPILE=Y",
		"FEEDBACK=1000",
	}

	switch {
	case opts.RemapSchema != "":
		// REMAP_SCHEMA 模式：将源模式数据映射到目标模式，需配合 FULL=Y 使用
		args = append(args, "FULL=Y")
		args = append(args, fmt.Sprintf("REMAP_SCHEMA=%s", opts.RemapSchema))
	case opts.TargetDatabaseName != "":
		args = append(args, fmt.Sprintf("SCHEMAS=%s", opts.TargetDatabaseName))
	default:
		args = append(args, "FULL=Y")
	}

	return args
}
