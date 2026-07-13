package backup

const (
	DBTypeMySQL      = "mysql"
	DBTypePostgreSQL = "postgresql"
	DBTypeOracle     = "oracle"
	DBTypeMSSQL      = "mssql"
	DBTypeDameng     = "dameng"

	versionXML         = "1.0.0"
	backupTypeXML      = "backup"
	actionDelete       = "delete"
	actionDeleteAll    = "delete-all"
	actionInfo         = "info"
	actionList         = "list"
	actionRestore      = "restore"
	actionValidate     = "validate"
	actionVerifyStatus = "verify-status"
	defaultHost        = "localhost"

	// 备份模式常量
	backupModeFULL    = "FULL"
	backupModeSCHEMAS = "SCHEMAS"

	// MySQL 系统数据库名
	mysqlSysDB = "sys"

	// 平台常量
	platformWindows = "windows"
)

var MySQLSystemDatabases = []string{
	"information_schema",
	DBTypeMySQL,
	"performance_schema",
	mysqlSysDB,
}

var PostgreSQLSystemDatabases = []string{
	"postgres",
	"template0",
	"template1",
}

var MSSQLSystemDatabases = []string{
	"master",
	"model",
	"msdb",
	"tempdb",
	"distribution",
	"reportserver",
	"reportservertempdb",
}

// DamengSystemSchemas 达梦内置系统模式，逻辑备份时自动排除。
// 包含: SYSSSO(系统安全员), SYSAUDITOR(审计员), SYS(系统模式), CTISYS(全文检索)。
// 注意: SYSDBA 不在此列表中，因为 SYSDBA 模式下可能存在业务数据，不应自动排除。
var DamengSystemSchemas = []string{
	"SYSSSO",
	"SYSAUDITOR",
	"SYS",
	"CTISYS",
}
