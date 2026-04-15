package main

import (
	"context"
	"flag"
	"time"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

// 基础目录路径
const baseBackupDir = "c:\\work\\database_backup"

func main() {
	// 解析命令行参数
	dbType := flag.String("db-type", "oracle", "数据库类型: oracle, mssql")
	action := flag.String("action", "", "操作类型: backup, restore, list, delete, validate, info")

	// 备份参数
	backupType := flag.String("backup-type", "full", "备份类型: full, incremental, differential, logical, physical")
	backupParallelism := flag.Int("parallelism", 2, "并行度")
	backupCompression := flag.Bool("compression", true, "是否压缩")
	backupDescription := flag.String("description", "", "备份描述")

	// 还原参数
	restorePointInTime := flag.String("point-in-time", "", "时间点恢复，格式: 2006-01-02T15:04:05")
	restoreBackupTag := flag.String("backup-tag", "", "备份标签（Oracle: 标签名, MSSQL: 备份文件路径）")
	restoreTargetDB := flag.String("target-db", "", "还原的目标数据库名")

	// 其他参数
	deleteIdentifier := flag.String("delete-identifier", "", "删除备份的标识符")
	validateBackupID := flag.String("validate-id", "", "验证备份的ID")
	infoBackupID := flag.String("info-id", "", "获取备份信息的ID")
	registerPath := flag.String("register-path", "", "注册备份的路径")
	unregisterID := flag.String("unregister-id", "", "移除备份记录的ID")

	flag.Parse()

	// 检查必须的参数
	if *action == "" {
		utils.Fatalf("必须指定操作类型: -action backup|restore|list|delete|validate|info")
	}

	// 根据数据库类型获取配置（写死的配置）
	cfg := getDBConfig(*dbType)

	// 根据数据库类型生成备份路径
	backupTarget := baseBackupDir + "\\" + *dbType + "\\backup"
	archiveLogDest := baseBackupDir + "\\" + *dbType + "\\archivelog"

	// 使用工厂方法创建数据库备份实例
	db, err := backup.NewBackup(cfg)
	if err != nil {
		utils.Fatalf("创建数据库备份实例失败: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// 根据操作类型执行相应的操作
	switch *action {
	case "backup":
		execBackup(ctx, db, *backupType, *backupParallelism, *backupCompression, backupTarget, *backupDescription, archiveLogDest)
	case "restore":
		execRestore(ctx, db, *restorePointInTime, *restoreBackupTag, *restoreBackupTag, *restoreTargetDB)
	case "list":
		execListBackups(ctx, db)
	case "delete":
		execDeleteBackup(ctx, db, *deleteIdentifier)
	case "validate":
		execValidateBackup(ctx, db, *validateBackupID)
	case "info":
		execGetBackupInfo(ctx, db, *infoBackupID)
	case "register":
		execRegisterBackup(ctx, db, *registerPath)
	case "unregister":
		execUnregisterBackup(ctx, db, *unregisterID)
	case "verify-status":
		execVerifyBackupStatus(ctx, db)
	case "delete-invalid":
		execDeleteInvalidBackups(ctx, db)
	default:
		utils.Fatalf("无效的操作类型: %s", *action)
	}
}

// execBackup 执行备份操作
func execBackup(ctx context.Context, db backup.DatabaseBackup, backupType string, parallelism int, compression bool, target string, description string, archiveLogDest string) {
	utils.Info("=== 开始备份 ===")

	// 解析备份类型
	var backupTypeEnum backup.BackupType
	switch backupType {
	case "full":
		backupTypeEnum = backup.BackupFull
	case "incremental":
		backupTypeEnum = backup.BackupIncremental
	case "differential":
		backupTypeEnum = backup.BackupDifferential
	case "logical":
		backupTypeEnum = backup.BackupLogical
	case "physical":
		backupTypeEnum = backup.BackupPhysical
	default:
		utils.Fatalf("无效的备份类型: %s", backupType)
	}

	backupOpts := backup.BackupOptions{
		Type:           backupTypeEnum,
		Parallelism:    parallelism,
		Compression:    compression,
		TargetPath:     target,
		Description:    description,
		ArchiveLogDest: archiveLogDest,
		Timeout:        2 * time.Hour,
	}

	result, err := db.Backup(ctx, backupOpts, func(percent float64, msg string) {
		utils.Infof("备份进度: %.2f%% - %s", percent, msg)
	})
	if err != nil {
		utils.Fatalf("备份失败: %v", err)
	}

	if result == nil {
		utils.Fatalf("备份结果为空")
	}

	utils.Infof("备份成功: 文件=%s, 大小=%s, 耗时=%v",
		result.BackupFile, utils.FormatFileSize(result.BackupSize), result.Duration)
	if result.Metadata["backup_set_key"] != "" {
		utils.Infof("备份集ID: %s", result.Metadata["backup_set_key"])
	}
}

// execRestore 执行还原操作
func execRestore(ctx context.Context, db backup.DatabaseBackup, pointInTimeStr string, backupID string, backupTag string, targetDB string) {
	utils.Info("=== 开始还原 ===")

	restoreOpts := backup.RestoreOptions{
		BackupTag: backupTag,
		TargetDB:  targetDB,
		Overwrite: true,
	}

	// 解析时间点
	if pointInTimeStr != "" {
		// 尝试解析RFC3339格式（带时区）
		pointInTime, err := time.Parse(time.RFC3339, pointInTimeStr)
		if err != nil {
			// 尝试解析不带时区的格式
			pointInTime, err = time.Parse("2006-01-02T15:04:05", pointInTimeStr)
			if err != nil {
				utils.Fatalf("无效的时间格式: %v", err)
			}
		}
		restoreOpts.PointInTime = pointInTime
	}

	restoreResult, err := db.Restore(ctx, restoreOpts, func(percent float64, msg string) {
		utils.Infof("还原进度: %.2f%% - %s", percent, msg)
	})
	if err != nil {
		utils.Fatalf("还原失败: %v", err)
	}

	utils.Infof("还原成功, 耗时=%v", restoreResult.Duration)
	if restoreResult.RestoredToSCN != "" {
		utils.Infof("恢复到SCN=%s", restoreResult.RestoredToSCN)
	}
}

// execListBackups 列出所有备份
func execListBackups(ctx context.Context, db backup.DatabaseBackup) {
	utils.Info("=== 列出所有备份 ===")

	backups, err := db.ListBackups(ctx)
	if err != nil {
		utils.Fatalf("列出备份失败: %v", err)
	}

	for _, b := range backups {
		if b.BackupPath != "" {
			utils.Infof("ID=%s, 类型=%s, 完成时间=%s, 大小=%s, 状态=%s, 路径=%s",
				b.BackupID, b.BackupType, b.CompletionTime.Format("2006-01-02 15:04:05"),
				utils.FormatFileSize(b.Size), b.Status, b.BackupPath)
		} else {
			utils.Infof("ID=%s, 类型=%s, 完成时间=%s, 大小=%s, 状态=%s",
				b.BackupID, b.BackupType, b.CompletionTime.Format("2006-01-02 15:04:05"),
				utils.FormatFileSize(b.Size), b.Status)
		}
	}
}

// execDeleteBackup 删除指定备份
func execDeleteBackup(ctx context.Context, db backup.DatabaseBackup, identifier string) {
	if identifier == "" {
		utils.Fatalf("必须指定删除备份的标识符: -delete-identifier")
	}

	utils.Infof("=== 删除备份: %s ===", identifier)

	err := db.DeleteBackup(ctx, identifier)
	if err != nil {
		utils.Fatalf("删除备份失败: %v", err)
	}

	utils.Info("删除成功")
}

// execValidateBackup 验证备份
func execValidateBackup(ctx context.Context, db backup.DatabaseBackup, backupID string) {
	if backupID == "" {
		utils.Fatalf("必须指定验证备份的ID: -validate-id")
	}

	utils.Infof("=== 验证备份: %s ===", backupID)

	err := db.ValidateBackup(ctx, backupID)
	if err != nil {
		utils.Fatalf("验证失败: %v", err)
	}

	utils.Info("备份有效")
}

// execGetBackupInfo 获取备份信息
func execGetBackupInfo(ctx context.Context, db backup.DatabaseBackup, backupID string) {
	if backupID == "" {
		utils.Fatalf("必须指定获取备份信息的ID: -info-id")
	}

	utils.Infof("=== 获取备份信息: %s ===", backupID)

	info, err := db.GetBackupInfo(ctx, backupID)
	if err != nil {
		utils.Fatalf("获取备份信息失败: %v", err)
	}

	for key, value := range info {
		utils.Infof("%s: %s", key, value)
	}
}

// execRegisterBackup 注册备份到备份目录库
func execRegisterBackup(ctx context.Context, db backup.DatabaseBackup, backupPath string) {
	if backupPath == "" {
		utils.Fatalf("必须指定注册备份的路径: -register-path")
	}

	utils.Infof("=== 注册备份: %s ===", backupPath)

	err := db.RegisterBackup(ctx, backupPath)
	if err != nil {
		utils.Fatalf("注册备份失败: %v", err)
	}

	utils.Info("注册成功")
}

// execUnregisterBackup 从备份目录库中移除备份记录
func execUnregisterBackup(ctx context.Context, db backup.DatabaseBackup, backupID string) {
	if backupID == "" {
		utils.Fatalf("必须指定移除备份记录的ID: -unregister-id")
	}

	utils.Infof("=== 移除备份记录: %s ===", backupID)

	err := db.UnregisterBackup(ctx, backupID)
	if err != nil {
		utils.Fatalf("移除备份记录失败: %v", err)
	}

	utils.Info("移除成功")
}

// execVerifyBackupStatus 检查备份状态
func execVerifyBackupStatus(ctx context.Context, db backup.DatabaseBackup) {
	utils.Info("=== 检查备份状态 ===")

	err := db.VerifyBackupStatus(ctx)
	if err != nil {
		utils.Fatalf("检查备份状态失败: %v", err)
	}

	utils.Info("检查完成")
}

// execDeleteInvalidBackups 删除无效备份
func execDeleteInvalidBackups(ctx context.Context, db backup.DatabaseBackup) {
	utils.Info("=== 删除无效备份 ===")

	err := db.DeleteInvalidBackups(ctx)
	if err != nil {
		utils.Fatalf("删除无效备份失败: %v", err)
	}

	utils.Info("删除成功")
}

// getDBConfig 根据数据库类型获取写死的配置
func getDBConfig(dbType string) *backup.DBConfig {
	switch dbType {
	case "oracle":
		return &backup.DBConfig{
			Type:     "oracle",
			Database: "orcl",
			Extra: map[string]string{
				"ORACLE_HOME": "c:\\windows.x64_193000_db_home",
				"ORACLE_SID":  "ORCL",
			},
		}
	case "mssql":
		return &backup.DBConfig{
			Type:     "mssql",
			Host:     "localhost",
			Port:     1433,
			User:     "",
			Password: "",
			Database: "",
			Extra:    map[string]string{"AUTH_TYPE": "windows"},
		}
	default:
		utils.Fatalf("不支持的数据库类型: %s", dbType)
		return nil
	}
}
