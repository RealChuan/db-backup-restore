package backup

const (
	DBTypeMySQL      = "mysql"
	DBTypePostgreSQL = "postgresql"
	DBTypeOracle     = "oracle"
	DBTypeMSSQL      = "mssql"

	versionXML      = "1.0.0"
	backupTypeXML   = "backup"
	actionDelete    = "delete"
	actionDeleteAll = "delete-all"
	actionInfo      = "info"
	actionList      = "list"
	actionRestore   = "restore"
	defaultHost     = "localhost"
)

var MySQLSystemDatabases = []string{
	"information_schema",
	DBTypeMySQL,
	"performance_schema",
	"sys",
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
