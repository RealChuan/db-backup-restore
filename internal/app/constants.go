package app

import "path/filepath"

// 操作名称常量，用于 OperationResult.Operation 和 AuditLog。
const (
	OpBackup         = "backup"
	OpRestore        = "restore"
	OpList           = "list"
	OpListDatabases  = "list_databases"
	OpDelete         = "delete"
	OpValidate       = "validate"
	OpInfo           = "info"
	OpRegister       = "register"
	OpUnregister     = "unregister"
	OpVerifyStatus   = "verify_status"
	OpDeleteInvalid  = "delete_invalid"
	OpDeleteAll      = "delete_all"
	OpValidateConfig = "validate_config"
	OpListDrivers    = "list_drivers"
	OpEnableArchive  = "enable_archive"
	OpDisableArchive = "disable_archive"
)

// Data 字段键名常量，用于 OperationResult.Data 的 key。
const (
	DataKeyDuration     = "duration"
	DataKeySize         = "size"
	DataKeyDatabases    = "databases"
	DataKeyBackups      = "backups"
	DataKeyIdentifier   = "identifier"
	DataKeyID           = "id"
	DataKeyPath         = "path"
	DataKeyFile         = "file"
	DataKeyTargetDB     = "target_db"
	DataKeySCN          = "scn"
	DataKeyBackupSetKey = "backup_set_key"
)

// 消息常量，用于 OperationResult.Message。
const (
	MsgDeleteSuccess = "删除成功"
)

// backupDir 返回指定数据库和备份类型的备份目录路径。
// 路径格式：{baseBackupDir}/{dbType}/{typeDir}/backup
// 这是所有管理操作（列出、删除、验证等）的统一路径构造入口。
func backupDir(baseBackupDir, dbType, typeDir string) string {
	return filepath.Join(baseBackupDir, dbType, typeDir, "backup")
}

// archiveLogDir 返回指定数据库和备份类型的归档日志目录路径。
// 路径格式：{baseBackupDir}/{dbType}/{typeDir}/archivelog
// 这是备份和还原操作的归档日志路径统一构造入口。
func archiveLogDir(baseBackupDir, dbType, typeDir string) string {
	return filepath.Join(baseBackupDir, dbType, typeDir, "archivelog")
}
