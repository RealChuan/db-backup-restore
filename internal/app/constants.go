package app

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
