package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"db-backup-restore/internal/backup"
	"db-backup-restore/pkg/utils"
)

// 基础目录路径
const baseBackupDir = "c:\\work\\database_backup"

// 操作类型常量
const (
	ActionBackup        = "backup"
	ActionRestore       = "restore"
	ActionList          = "list"
	ActionDelete        = "delete"
	ActionValidate      = "validate"
	ActionInfo          = "info"
	ActionRegister      = "register"
	ActionUnregister    = "unregister"
	ActionVerifyStatus  = "verify-status"
	ActionDeleteInvalid = "delete-invalid"
	ActionDeleteAll     = "delete-all"
)

// 数据库类型常量
const (
	DBTypeMySQL  = "mysql"
	DBTypeOracle = "oracle"
	DBTypeMSSQL  = "mssql"
)

func main() {
	// 解析命令行参数
	args := parseArgs()

	// 验证必要参数
	if err := validateArgs(args); err != nil {
		utils.Fatalf("参数验证失败: %v", err)
	}

	// 获取数据库配置
	cfg, err := getDBConfig(args.dbType)
	if err != nil {
		utils.Fatalf("获取数据库配置失败: %v", err)
	}

	// 创建数据库备份实例
	db, err := backup.NewBackup(cfg)
	if err != nil {
		utils.Fatalf("创建数据库备份实例失败: %v", err)
	}
	defer db.Close()

	// 创建上下文（带超时）
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	// 执行操作
	if err := executeAction(ctx, db, args); err != nil {
		utils.Fatalf("操作执行失败: %v", err)
	}

	utils.Info("操作完成")
}

// Args 命令行参数结构
type Args struct {
	dbType           string
	action           string
	backupType       string
	parallelism      int
	compression      bool
	description      string
	pointInTime      string
	backupTag        string
	targetDB         string
	deleteIdentifier string
	validateBackupID string
	infoBackupID     string
	registerPath     string
	unregisterID     string
}

// parseArgs 解析命令行参数
func parseArgs() *Args {
	args := &Args{}

	flag.StringVar(&args.dbType, "db-type", "mysql", "数据库类型: mysql, oracle, mssql")
	flag.StringVar(&args.action, "action", "",
		"操作类型: backup, restore, list, delete, validate, info, "+
			"register, unregister, verify-status, delete-invalid, delete-all")

	// 备份参数
	flag.StringVar(&args.backupType, "backup-type", "full", "备份类型: full, incremental, differential, logical, physical")
	flag.IntVar(&args.parallelism, "parallelism", 2, "并行度")
	flag.BoolVar(&args.compression, "compression", true, "是否压缩")
	flag.StringVar(&args.description, "description", "", "备份描述")

	// 还原参数
	flag.StringVar(&args.pointInTime, "point-in-time", "", "时间点恢复，格式: 2006-01-02T15:04:05")
	flag.StringVar(&args.backupTag, "backup-tag", "", "备份标签（Oracle: 标签名, MSSQL/MySQL: 备份文件路径）")
	flag.StringVar(&args.targetDB, "target-db", "", "还原的目标数据库名")

	// 其他参数
	flag.StringVar(&args.deleteIdentifier, "delete-identifier", "", "删除备份的标识符")
	flag.StringVar(&args.validateBackupID, "validate-id", "", "验证备份的ID")
	flag.StringVar(&args.infoBackupID, "info-id", "", "获取备份信息的ID")
	flag.StringVar(&args.registerPath, "register-path", "", "注册备份的路径")
	flag.StringVar(&args.unregisterID, "unregister-id", "", "移除备份记录的ID")

	flag.Parse()
	return args
}

// 错误信息常量
const (
	errMsgActionRequired = "必须指定操作类型: -action " +
		"backup|restore|list|delete|validate|info|" +
		"register|unregister|verify-status|delete-invalid|delete-all"
	errMsgInvalidAction         = "无效的操作类型: %s"
	errMsgUnsupportedDBType     = "不支持的数据库类型: %s，支持的类型: mysql, oracle, mssql"
	errMsgDeleteIdentifierEmpty = "必须指定删除备份的标识符: -delete-identifier"
	errMsgValidateIDEmpty       = "必须指定验证备份的ID: -validate-id"
	errMsgInfoIDEmpty           = "必须指定获取备份信息的ID: -info-id"
	errMsgRegisterPathEmpty     = "必须指定注册备份的路径: -register-path"
	errMsgUnregisterIDEmpty     = "必须指定移除备份记录的ID: -unregister-id"
	errMsgBackupResultNil       = "备份结果为空"
)

// validateArgs 验证命令行参数
func validateArgs(args *Args) error {
	if args.action == "" {
		return errors.New(errMsgActionRequired)
	}

	validActions := map[string]bool{
		ActionBackup:        true,
		ActionRestore:       true,
		ActionList:          true,
		ActionDelete:        true,
		ActionValidate:      true,
		ActionInfo:          true,
		ActionRegister:      true,
		ActionUnregister:    true,
		ActionVerifyStatus:  true,
		ActionDeleteInvalid: true,
		ActionDeleteAll:     true,
	}

	if !validActions[args.action] {
		return fmt.Errorf(errMsgInvalidAction, args.action)
	}

	validDBTypes := map[string]bool{
		DBTypeMySQL:  true,
		DBTypeOracle: true,
		DBTypeMSSQL:  true,
	}

	if !validDBTypes[args.dbType] {
		return fmt.Errorf(errMsgUnsupportedDBType, args.dbType)
	}

	return nil
}

// executeAction 根据操作类型执行相应的操作
func executeAction(ctx context.Context, db backup.DatabaseBackup, args *Args) error {
	backupTarget := baseBackupDir + "\\" + args.dbType + "\\backup"
	archiveLogDest := baseBackupDir + "\\" + args.dbType + "\\archivelog"

	backupOpts := backup.BackupOptions{
		TargetPath:     backupTarget,
		ArchiveLogDest: archiveLogDest,
	}

	switch args.action {
	case ActionBackup:
		return execBackup(ctx, db, args, backupTarget, archiveLogDest)
	case ActionRestore:
		return execRestore(ctx, db, args)
	case ActionList:
		return execListBackups(ctx, db, backupOpts)
	case ActionDelete:
		return execDeleteBackup(ctx, db, args.deleteIdentifier, backupOpts)
	case ActionValidate:
		return execValidateBackup(ctx, db, args.validateBackupID, backupOpts)
	case ActionInfo:
		return execGetBackupInfo(ctx, db, args.infoBackupID, backupOpts)
	case ActionRegister:
		return execRegisterBackup(ctx, db, args.registerPath)
	case ActionUnregister:
		return execUnregisterBackup(ctx, db, args.unregisterID)
	case ActionVerifyStatus:
		return execVerifyBackupStatus(ctx, db)
	case ActionDeleteInvalid:
		return execDeleteInvalidBackups(ctx, db, backupOpts)
	case ActionDeleteAll:
		return execDeleteAllBackups(ctx, db, backupOpts)
	default:
		return fmt.Errorf("无效的操作类型: %s", args.action)
	}
}

// execBackup 执行备份操作
func execBackup(ctx context.Context, db backup.DatabaseBackup, args *Args, target, archiveLogDest string) error {
	utils.Info("=== 开始备份 ===")

	// 解析备份类型
	backupTypeEnum, err := parseBackupType(args.backupType)
	if err != nil {
		return err
	}

	backupOpts := backup.BackupOptions{
		Type:           backupTypeEnum,
		Parallelism:    args.parallelism,
		Compression:    args.compression,
		TargetPath:     target,
		Description:    args.description,
		ArchiveLogDest: archiveLogDest,
		Timeout:        2 * time.Hour,
	}

	result, err := db.Backup(ctx, backupOpts, func(percent float64, msg string) {
		utils.Infof("备份进度: %.2f%% - %s", percent, msg)
	})
	if err != nil {
		return fmt.Errorf("备份失败: %w", err)
	}

	if result == nil {
		return errors.New(errMsgBackupResultNil)
	}

	utils.Infof("备份成功: 文件=%s, 大小=%s, 耗时=%v",
		result.BackupFile, utils.FormatFileSize(result.BackupSize), result.Duration)

	if result.Metadata["backup_set_key"] != "" {
		utils.Infof("备份集ID: %s", result.Metadata["backup_set_key"])
	}

	return nil
}

// parseBackupType 解析备份类型
func parseBackupType(backupType string) (backup.BackupType, error) {
	switch backupType {
	case "full":
		return backup.BackupFull, nil
	case "incremental":
		return backup.BackupIncremental, nil
	case "differential":
		return backup.BackupDifferential, nil
	case "logical":
		return backup.BackupLogical, nil
	case "physical":
		return backup.BackupPhysical, nil
	default:
		return "", fmt.Errorf("无效的备份类型: %s", backupType)
	}
}

// execRestore 执行还原操作
func execRestore(ctx context.Context, db backup.DatabaseBackup, args *Args) error {
	utils.Info("=== 开始还原 ===")

	restoreOpts := backup.RestoreOptions{
		BackupTag: args.backupTag,
		TargetDB:  args.targetDB,
		Overwrite: true,
	}

	// 解析时间点
	if args.pointInTime != "" {
		pointInTime, err := parseTime(args.pointInTime)
		if err != nil {
			return fmt.Errorf("无效的时间格式: %w", err)
		}
		restoreOpts.PointInTime = pointInTime
	}

	result, err := db.Restore(ctx, restoreOpts, func(percent float64, msg string) {
		utils.Infof("还原进度: %.2f%% - %s", percent, msg)
	})
	if err != nil {
		return fmt.Errorf("还原失败: %w", err)
	}

	utils.Infof("还原成功, 耗时=%v", result.Duration)

	if result.RestoredToSCN != "" {
		utils.Infof("恢复到SCN=%s", result.RestoredToSCN)
	}

	return nil
}

// parseTime 解析时间字符串
func parseTime(timeStr string) (time.Time, error) {
	// 尝试解析RFC3339格式（带时区）
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return t, nil
	}

	// 尝试解析不带时区的格式
	return time.Parse("2006-01-02T15:04:05", timeStr)
}

// execListBackups 列出所有备份
func execListBackups(ctx context.Context, db backup.DatabaseBackup, opts backup.BackupOptions) error {
	utils.Info("=== 列出所有备份 ===")

	backups, err := db.ListBackups(ctx, opts)
	if err != nil {
		return fmt.Errorf("列出备份失败: %w", err)
	}

	if len(backups) == 0 {
		utils.Info("未找到备份")
		return nil
	}

	for _, b := range backups {
		logBackupInfo(b)
	}

	return nil
}

// logBackupInfo 记录备份信息（单行输出）
func logBackupInfo(b backup.BackupInfo) {
	var output string
	output = fmt.Sprintf("ID=%s, 完成时间=%s",
		b.BackupID, b.CompletionTime.Format("2006-01-02T15:04:05"))
	if b.BackupType != "" {
		output += fmt.Sprintf(", 类型=%s", b.BackupType)
	}
	if b.Size > 0 {
		output += fmt.Sprintf(", 大小=%s", utils.FormatFileSize(b.Size))
	}
	if b.Status != "" {
		output += fmt.Sprintf(", 状态=%s", b.Status)
	}
	if b.Tag != "" {
		output += fmt.Sprintf(", 标签=%s", b.Tag)
	}
	if b.DeviceType != "" {
		output += fmt.Sprintf(", 设备类型=%s", b.DeviceType)
	}
	if b.BackupPath != "" {
		output += fmt.Sprintf(", 路径=%s", b.BackupPath)
	}
	utils.Info(output)
}

// execDeleteBackup 删除指定备份
func execDeleteBackup(ctx context.Context, db backup.DatabaseBackup, identifier string, opts backup.BackupOptions) error {
	if identifier == "" {
		return errors.New(errMsgDeleteIdentifierEmpty)
	}

	utils.Infof("=== 删除备份: %s ===", identifier)

	if err := db.DeleteBackup(ctx, identifier, opts); err != nil {
		return fmt.Errorf("删除备份失败: %w", err)
	}

	utils.Info("删除成功")
	return nil
}

// execValidateBackup 验证备份
func execValidateBackup(ctx context.Context, db backup.DatabaseBackup, backupID string, opts backup.BackupOptions) error {
	if backupID == "" {
		return errors.New(errMsgValidateIDEmpty)
	}

	utils.Infof("=== 验证备份: %s ===", backupID)

	if err := db.ValidateBackup(ctx, backupID, opts); err != nil {
		return fmt.Errorf("验证失败: %w", err)
	}

	utils.Info("备份有效")
	return nil
}

// execGetBackupInfo 获取备份信息
func execGetBackupInfo(ctx context.Context, db backup.DatabaseBackup, backupID string, opts backup.BackupOptions) error {
	if backupID == "" {
		return errors.New(errMsgInfoIDEmpty)
	}

	utils.Infof("=== 获取备份信息: %s ===", backupID)

	info, err := db.GetBackupInfo(ctx, backupID, opts)
	if err != nil {
		return fmt.Errorf("获取备份信息失败: %w", err)
	}

	for key, value := range info {
		utils.Infof("  %s: %s", key, value)
	}

	return nil
}

// execRegisterBackup 注册备份到备份目录库
func execRegisterBackup(ctx context.Context, db backup.DatabaseBackup, backupPath string) error {
	if backupPath == "" {
		return errors.New(errMsgRegisterPathEmpty)
	}

	utils.Infof("=== 注册备份: %s ===", backupPath)

	if err := db.RegisterBackup(ctx, backupPath); err != nil {
		return fmt.Errorf("注册备份失败: %w", err)
	}

	utils.Info("注册成功")
	return nil
}

// execUnregisterBackup 从备份目录库中移除备份记录
func execUnregisterBackup(ctx context.Context, db backup.DatabaseBackup, backupID string) error {
	if backupID == "" {
		return errors.New(errMsgUnregisterIDEmpty)
	}

	utils.Infof("=== 移除备份记录: %s ===", backupID)

	if err := db.UnregisterBackup(ctx, backupID); err != nil {
		return fmt.Errorf("移除备份记录失败: %w", err)
	}

	utils.Info("移除成功")
	return nil
}

// execVerifyBackupStatus 检查备份状态
func execVerifyBackupStatus(ctx context.Context, db backup.DatabaseBackup) error {
	utils.Info("=== 检查备份状态 ===")

	if err := db.VerifyBackupStatus(ctx); err != nil {
		return fmt.Errorf("检查备份状态失败: %w", err)
	}

	utils.Info("检查完成")
	return nil
}

// execDeleteInvalidBackups 删除无效备份
func execDeleteInvalidBackups(ctx context.Context, db backup.DatabaseBackup, opts backup.BackupOptions) error {
	utils.Info("=== 删除无效备份 ===")

	if err := db.DeleteInvalidBackups(ctx, opts); err != nil {
		return fmt.Errorf("删除无效备份失败: %w", err)
	}

	utils.Info("删除成功")
	return nil
}

// execDeleteAllBackups 删除所有备份
func execDeleteAllBackups(ctx context.Context, db backup.DatabaseBackup, opts backup.BackupOptions) error {
	utils.Info("=== 删除所有备份 ===")

	if err := db.DeleteAllBackups(ctx, opts); err != nil {
		return fmt.Errorf("删除所有备份失败: %w", err)
	}

	utils.Info("删除成功")
	return nil
}

// getDBConfig 根据数据库类型获取配置
func getDBConfig(dbType string) (*backup.DBConfig, error) {
	switch dbType {
	case DBTypeMySQL:
		return &backup.DBConfig{
			Type:     DBTypeMySQL,
			Host:     "localhost",
			Port:     3306,
			User:     "root",
			Password: "123456",
			Database: "",
			Extra: map[string]string{
				"MYSQL_BIN_PATH": "C:\\Program Files\\MySQL\\MySQL Server 8.0\\bin",
			},
		}, nil

	case DBTypeOracle:
		return &backup.DBConfig{
			Type:     DBTypeOracle,
			Database: "orcl",
			Extra: map[string]string{
				"ORACLE_HOME": "c:\\windows.x64_193000_db_home",
				"ORACLE_SID":  "ORCL",
			},
		}, nil

	case DBTypeMSSQL:
		return &backup.DBConfig{
			Type:     DBTypeMSSQL,
			Host:     "localhost",
			Port:     1433,
			User:     "",
			Password: "",
			Database: "",
			Extra:    map[string]string{"AUTH_TYPE": "windows"},
		}, nil

	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbType)
	}
}
