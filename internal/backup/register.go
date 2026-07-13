package backup

import "fmt"

// RegisterAllDrivers 显式注册所有数据库驱动
// 必须在 main.go 中调用，替代各驱动的 init() 函数
func RegisterAllDrivers() error {
	if err := registerMySQLDriver(); err != nil {
		return fmt.Errorf("注册 MySQL 驱动失败: %w", err)
	}
	if err := registerPostgreSQLDriver(); err != nil {
		return fmt.Errorf("注册 PostgreSQL 驱动失败: %w", err)
	}
	if err := registerOracleDriver(); err != nil {
		return fmt.Errorf("注册 Oracle 驱动失败: %w", err)
	}
	if err := registerMSSQLDriver(); err != nil {
		return fmt.Errorf("注册 MSSQL 驱动失败: %w", err)
	}
	if err := registerDamengDriver(); err != nil {
		return fmt.Errorf("注册达梦驱动失败: %w", err)
	}
	return nil
}
