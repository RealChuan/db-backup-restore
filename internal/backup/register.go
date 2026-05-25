package backup

// RegisterAllDrivers 显式注册所有数据库驱动
// 必须在 main.go 中调用，替代各驱动的 init() 函数
func RegisterAllDrivers() {
	registerMySQLDriver()
	registerPostgreSQLDriver()
	registerOracleDriver()
	registerMSSQLDriver()
}
