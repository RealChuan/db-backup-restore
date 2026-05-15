package backup

const (
	DBTypeMySQL      = "mysql"
	DBTypePostgreSQL = "postgresql"
	DBTypeOracle     = "oracle"
	DBTypeMSSQL      = "mssql"
)

var MySQLSystemDatabases = []string{
	"information_schema",
	"mysql",
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
