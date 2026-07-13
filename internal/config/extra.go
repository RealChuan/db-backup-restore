package config

import (
	"fmt"
	"sort"
	"strings"
)

// 备份模式常量
const (
	backupModeLogical  = "logical"
	backupModePhysical = "physical"
)

// Extra 参数键名常量
const (
	extraMySQLBinPath      = "MYSQL_BIN_PATH"
	extraXtrabackupBinPath = "XTRABACKUP_BIN_PATH"
	extraDataDir           = "DATA_DIR"
	extraServiceName       = "SERVICE_NAME"
	extraPGBinPath         = "PG_BIN_PATH"
	extraOracleHome        = "ORACLE_HOME"
	extraOracleSID         = "ORACLE_SID"
	extraInstance          = "INSTANCE"
	extraAuthType          = "AUTH_TYPE"
	authTypeWindows        = "windows"
	authTypeSQL            = "sql"
	extraDamengHome        = "DM_HOME"
	extraDamengInstance    = "DM_INSTANCE"
	extraDamengDataDir     = "DM_DATA_DIR"
	extraAutoGhostCleanup  = "AUTO_GHOST_CLEANUP"
	extraDefaultTrue       = "true"
)

// ExtraFieldDef 定义 extra 参数的字段规范。
type ExtraFieldDef struct {
	Key         string // 配置键名（大写，与 JSON 中一致）
	Required    bool   // 是否必填
	Description string // 中文说明
	Default     string // 默认值（空字符串表示无默认值）
}

// ExtraSpec 定义某种数据库类型的 extra 参数规范。
type ExtraSpec struct {
	DBType      string          // 数据库类型
	Fields      []ExtraFieldDef // 字段定义
	Description string          // 整体说明
	Example     string          // JSON 示例
	BackupModes []string        // 支持的备份模式
}

// 各数据库类型的 Extra 规范定义。
var extraSpecs = map[string]ExtraSpec{
	dbTypeMySQL: {
		DBType:      dbTypeMySQL,
		Description: "MySQL 额外配置参数",
		BackupModes: []string{backupModeLogical, backupModePhysical},
		Fields: []ExtraFieldDef{
			{
				Key:         extraMySQLBinPath,
				Required:    false,
				Description: "MySQL 客户端工具目录（mysql、mysqldump 所在目录），为空则使用 PATH 中的命令",
			},
			{
				Key:         extraXtrabackupBinPath,
				Required:    false,
				Description: "XtraBackup 工具目录（xtrabackup 所在目录），物理备份时使用，为空则使用 PATH 中的命令",
			},
			{
				Key:         extraDataDir,
				Required:    false,
				Description: "MySQL 数据目录路径，物理备份还原时需要（如 /var/lib/mysql）",
			},
			{
				Key:         extraServiceName,
				Required:    false,
				Description: "MySQL 系统服务名称，物理备份还原时启停服务使用，为空则自动检测",
			},
		},
		Example: `{
  "MYSQL_BIN_PATH": "C:\\Program Files\\MySQL\\MySQL Server 8.0\\bin",
  "XTRABACKUP_BIN_PATH": "/usr/bin",
  "DATA_DIR": "/var/lib/mysql",
  "SERVICE_NAME": "mysql"
}`,
	},
	dbTypePostgreSQL: {
		DBType:      dbTypePostgreSQL,
		Description: "PostgreSQL 额外配置参数",
		BackupModes: []string{backupModeLogical, backupModePhysical},
		Fields: []ExtraFieldDef{
			{
				Key:         extraPGBinPath,
				Required:    false,
				Description: "PostgreSQL 客户端工具目录（pg_dump、pg_basebackup、pg_ctl 所在目录），为空则使用 PATH 中的命令",
			},
			{
				Key:         extraDataDir,
				Required:    false,
				Description: "PostgreSQL 数据目录路径，物理备份还原时需要（如 /var/lib/postgresql/18/main）",
			},
			{
				Key:         extraServiceName,
				Required:    false,
				Description: "PostgreSQL 系统服务名称，物理备份还原时启停服务使用，为空则自动检测",
			},
		},
		Example: `{
  "PG_BIN_PATH": "C:\\Program Files\\PostgreSQL\\18\\bin",
  "DATA_DIR": "C:\\Program Files\\PostgreSQL\\18\\data",
  "SERVICE_NAME": "postgresql-18"
}`,
	},
	dbTypeOracle: {
		DBType:      dbTypeOracle,
		Description: "Oracle 额外配置参数",
		BackupModes: []string{backupModePhysical},
		Fields: []ExtraFieldDef{
			{
				Key:         extraOracleHome,
				Required:    true,
				Description: "Oracle 安装目录（ORACLE_HOME 环境变量），RMAN 命令依赖此路径",
			},
			{
				Key:         extraOracleSID,
				Required:    true,
				Description: "Oracle 实例标识（ORACLE_SID 环境变量）",
			},
			{
				Key:         extraAutoGhostCleanup,
				Required:    false,
				Description: "是否在备份/还原前自动执行 RMAN 幽灵对象清理（CROSSCHECK + DELETE EXPIRED + DELETE OBSOLETE），默认 true",
				Default:     extraDefaultTrue,
			},
		},
		Example: `{
  "ORACLE_HOME": "/opt/oracle/product/19c/dbhome_1",
  "ORACLE_SID": "ORCL",
  "AUTO_GHOST_CLEANUP": "true"
}`,
	},
	dbTypeMSSQL: {
		DBType:      dbTypeMSSQL,
		Description: "SQL Server 额外配置参数",
		BackupModes: []string{backupModePhysical},
		Fields: []ExtraFieldDef{
			{
				Key:         extraInstance,
				Required:    false,
				Description: "SQL Server 命名实例名称，默认实例留空",
			},
			{
				Key:         extraAuthType,
				Required:    false,
				Description: "认证方式：windows（Windows 身份验证）或 sql（SQL Server 身份验证，默认）",
				Default:     authTypeSQL,
			},
		},
		Example: `{
  "INSTANCE": "SQLEXPRESS",
  "AUTH_TYPE": "windows"
}`,
	},
	dbTypeDameng: {
		DBType:      dbTypeDameng,
		Description: "达梦数据库额外配置参数",
		BackupModes: []string{backupModeLogical, backupModePhysical},
		Fields: []ExtraFieldDef{
			{
				Key:         extraDamengHome,
				Required:    true,
				Description: "达梦安装目录（DM_HOME 环境变量），dexp/dimp/dmrman 工具依赖此路径",
			},
			{
				Key:         extraDamengInstance,
				Required:    false,
				Description: "达梦实例名，多实例场景需指定，默认使用 Database 字段值",
			},
			{
				Key:         extraDamengDataDir,
				Required:    false,
				Description: "达梦数据目录路径，物理备份还原时需要（如 /opt/dmdbms/data/DAMENG）",
			},
			{
				Key:         extraAutoGhostCleanup,
				Required:    false,
				Description: "是否在物理备份前自动清理归档目录中不属于当前实例的幽灵归档文件，默认 true",
				Default:     extraDefaultTrue,
			},
		},
		Example: `{
  "DM_HOME": "/opt/dmdbms",
  "DM_INSTANCE": "DMSERVER",
  "DM_DATA_DIR": "/opt/dmdbms/data/DAMENG",
  "AUTO_GHOST_CLEANUP": "true"
}`,
	},
}

// GetExtraSpec 获取指定数据库类型的 Extra 参数规范。
func GetExtraSpec(dbType string) (ExtraSpec, bool) {
	spec, ok := extraSpecs[dbType]
	return spec, ok
}

// GetAllExtraSpecs 获取所有数据库类型的 Extra 参数规范。
// 返回深拷贝副本，调用方可安全修改而不影响内部状态。
func GetAllExtraSpecs() map[string]ExtraSpec {
	out := make(map[string]ExtraSpec, len(extraSpecs))
	for k, v := range extraSpecs {
		fields := make([]ExtraFieldDef, len(v.Fields))
		copy(fields, v.Fields)
		v.Fields = fields
		out[k] = v
	}
	return out
}

// ValidateExtra 校验 extra 参数是否合法，返回校验错误列表。
func (c *DBConfig) ValidateExtra() []error {
	spec, ok := extraSpecs[c.Type]
	if !ok {
		return nil
	}

	var errs []error
	fieldKeys := make(map[string]bool, len(spec.Fields))
	for _, f := range spec.Fields {
		fieldKeys[f.Key] = true

		val, exists := c.Extra[f.Key]
		if f.Required && (!exists || val == "") {
			errs = append(errs, fmt.Errorf("extra.%s 是必填项: %s", f.Key, f.Description))
		}
	}

	// 检查未知字段
	for key := range c.Extra {
		if !fieldKeys[key] {
			errs = append(errs, fmt.Errorf("extra.%s 不是 %s 的有效参数，有效参数: %s",
				key, c.Type, validKeysString(spec)))
		}
	}

	return errs
}

// GetExtraTyped 将 Extra map 解析为强类型访问器。
func (c *DBConfig) GetExtraTyped() *TypedExtra {
	return &TypedExtra{extra: c.Extra, dbType: c.Type}
}

// TypedExtra 提供对 Extra 参数的类型安全访问。
type TypedExtra struct {
	extra  map[string]string
	dbType string
}

// GetString 获取字符串类型的 extra 参数。
func (e *TypedExtra) GetString(key string) string {
	if e.extra == nil {
		return ""
	}
	return e.extra[key]
}

// GetStringDefault 获取字符串类型的 extra 参数，不存在则返回默认值。
func (e *TypedExtra) GetStringDefault(key, defaultVal string) string {
	if v := e.GetString(key); v != "" {
		return v
	}
	return defaultVal
}

// GetBool 获取布尔类型的 extra 参数（值为 "true"/"1"/"yes" 视为 true）。
func (e *TypedExtra) GetBool(key string) bool {
	v := e.GetString(key)
	return v == "true" || v == "1" || v == "yes"
}

// GetBoolDefault 获取布尔类型的 extra 参数，键未设置时返回默认值。
func (e *TypedExtra) GetBoolDefault(key string, defaultVal bool) bool {
	if !e.IsSet(key) {
		return defaultVal
	}
	return e.GetBool(key)
}

// IsSet 检查 extra 参数是否已设置。
func (e *TypedExtra) IsSet(key string) bool {
	if e.extra == nil {
		return false
	}
	_, ok := e.extra[key]
	return ok
}

// MySQL 便捷访问方法

// MySQLBinPath 返回 MySQL 客户端工具目录。
func (e *TypedExtra) MySQLBinPath() string {
	return e.GetString(extraMySQLBinPath)
}

// XtrabackupBinPath 返回 XtraBackup 工具目录。
func (e *TypedExtra) XtrabackupBinPath() string {
	return e.GetString(extraXtrabackupBinPath)
}

// DataDir 返回数据目录路径。
func (e *TypedExtra) DataDir() string {
	return e.GetString(extraDataDir)
}

// ServiceName 返回系统服务名称。
func (e *TypedExtra) ServiceName() string {
	return e.GetString(extraServiceName)
}

// PostgreSQL 便捷访问方法

// PGBinPath 返回 PostgreSQL 客户端工具目录。
func (e *TypedExtra) PGBinPath() string {
	return e.GetString(extraPGBinPath)
}

// Oracle 便捷访问方法

// OracleHome 返回 Oracle 安装目录。
func (e *TypedExtra) OracleHome() string {
	return e.GetString(extraOracleHome)
}

// OracleSID 返回 Oracle 实例标识。
func (e *TypedExtra) OracleSID() string {
	return e.GetString(extraOracleSID)
}

// MSSQL 便捷访问方法

// Instance 返回 SQL Server 命名实例名称。
func (e *TypedExtra) Instance() string {
	return e.GetString(extraInstance)
}

// AuthType 返回认证方式。
func (e *TypedExtra) AuthType() string {
	return e.GetStringDefault(extraAuthType, authTypeSQL)
}

// IsWindowsAuth 返回是否使用 Windows 身份验证。
func (e *TypedExtra) IsWindowsAuth() bool {
	return e.AuthType() == authTypeWindows
}

// 达梦便捷访问方法

// DamengHome 返回达梦安装目录。
func (e *TypedExtra) DamengHome() string { return e.GetString(extraDamengHome) }

// DamengInstance 返回达梦实例名。
func (e *TypedExtra) DamengInstance() string { return e.GetString(extraDamengInstance) }

// DamengDataDir 返回达梦数据目录路径。
func (e *TypedExtra) DamengDataDir() string { return e.GetString(extraDamengDataDir) }

// 幽灵清理便捷访问方法（Oracle 和达梦共用）

// AutoGhostCleanup 返回是否在备份/还原前自动执行幽灵对象清理，默认 true。
func (e *TypedExtra) AutoGhostCleanup() bool {
	return e.GetBoolDefault(extraAutoGhostCleanup, true)
}

// validKeysString 返回规范中所有有效键名的逗号分隔字符串。
func validKeysString(spec ExtraSpec) string {
	keys := make([]string, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		keys = append(keys, f.Key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// ExtraHelpMarkdown 生成所有数据库 extra 参数的 Markdown 格式帮助文档。
func ExtraHelpMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# 数据库 Extra 配置参数参考\n\n")
	sb.WriteString("不同数据库类型有不同的额外配置参数（`extra` 字段），以下为各类型的详细说明。\n\n")

	// 按固定顺序输出
	order := []string{dbTypeMySQL, dbTypePostgreSQL, dbTypeOracle, dbTypeMSSQL, dbTypeDameng}
	for _, dbType := range order {
		spec, ok := extraSpecs[dbType]
		if !ok {
			continue
		}

		fmt.Fprintf(&sb, "## %s\n\n", spec.Description)
		fmt.Fprintf(&sb, "支持的备份模式: %s\n\n", strings.Join(spec.BackupModes, ", "))

		sb.WriteString("| 参数 | 必填 | 默认值 | 说明 |\n")
		sb.WriteString("|------|------|--------|------|\n")
		for _, f := range spec.Fields {
			req := "否"
			if f.Required {
				req = "是"
			}
			def := "-"
			if f.Default != "" {
				def = fmt.Sprintf("`%s`", f.Default)
			}
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n", f.Key, req, def, f.Description)
		}

		fmt.Fprintf(&sb, "\n**配置示例:**\n```json\n%s\n```\n\n", spec.Example)
	}

	return sb.String()
}
