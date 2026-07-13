package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateExtra_MySQL(t *testing.T) {
	t.Run("有效配置", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeMySQL,
			Host:  defaultHost,
			Port:  3306,
			Extra: map[string]string{extraMySQLBinPath: "/usr/bin"},
		}
		if errs := cfg.ValidateExtra(); len(errs) > 0 {
			t.Errorf("期望无错误，得到: %v", errs)
		}
	})

	t.Run("空Extra有效", func(t *testing.T) {
		cfg := &DBConfig{Type: dbTypeMySQL, Host: defaultHost, Port: 3306, Extra: map[string]string{}}
		if errs := cfg.ValidateExtra(); len(errs) > 0 {
			t.Errorf("期望无错误，得到: %v", errs)
		}
	})

	t.Run("未知字段报错", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeMySQL,
			Host:  defaultHost,
			Port:  3306,
			Extra: map[string]string{"UNKNOWN_KEY": "value"},
		}
		errs := cfg.ValidateExtra()
		if len(errs) == 0 {
			t.Fatal("期望有错误，但返回空")
		}
		if !strings.Contains(errs[0].Error(), "UNKNOWN_KEY") {
			t.Errorf("错误信息应包含 UNKNOWN_KEY，得到: %v", errs[0])
		}
	})
}

func TestValidateExtra_Oracle(t *testing.T) {
	t.Run("缺少必填项", func(t *testing.T) {
		cfg := &DBConfig{Type: dbTypeOracle, Host: defaultHost, Port: 1521, Extra: map[string]string{}}
		errs := cfg.ValidateExtra()
		if len(errs) < 2 {
			t.Fatalf("期望至少2个错误，得到 %d 个: %v", len(errs), errs)
		}
		errMsg := strings.Join(func() []string {
			var s []string
			for _, e := range errs {
				s = append(s, e.Error())
			}
			return s
		}(), " ")
		if !strings.Contains(errMsg, extraOracleHome) {
			t.Errorf("缺少 %s 必填项错误", extraOracleHome)
		}
		if !strings.Contains(errMsg, extraOracleSID) {
			t.Errorf("缺少 %s 必填项错误", extraOracleSID)
		}
	})

	t.Run("完整配置有效", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeOracle,
			Host:  defaultHost,
			Port:  1521,
			Extra: map[string]string{extraOracleHome: "/opt/oracle", extraOracleSID: "ORCL"},
		}
		if errs := cfg.ValidateExtra(); len(errs) > 0 {
			t.Errorf("期望无错误，得到: %v", errs)
		}
	})
}

func TestValidateExtra_MSSQL(t *testing.T) {
	cfg := &DBConfig{
		Type:  dbTypeMSSQL,
		Host:  defaultHost,
		Port:  1433,
		Extra: map[string]string{extraAuthType: authTypeWindows},
	}
	if errs := cfg.ValidateExtra(); len(errs) > 0 {
		t.Errorf("期望无错误，得到: %v", errs)
	}
}

func TestValidateExtra_PostgreSQL(t *testing.T) {
	cfg := &DBConfig{
		Type:  dbTypePostgreSQL,
		Host:  defaultHost,
		Port:  5432,
		Extra: map[string]string{extraPGBinPath: "/usr/bin", extraDataDir: "/var/lib/pgsql"},
	}
	if errs := cfg.ValidateExtra(); len(errs) > 0 {
		t.Errorf("期望无错误，得到: %v", errs)
	}
}

func TestValidateExtra_UnknownType(t *testing.T) {
	cfg := &DBConfig{Type: "unknown", Host: defaultHost, Port: 3306, Extra: map[string]string{"ANY_KEY": "value"}}
	if errs := cfg.ValidateExtra(); len(errs) > 0 {
		t.Errorf("未知类型应跳过校验，得到: %v", errs)
	}
}

func TestTypedExtra_GetString(t *testing.T) {
	t.Run("存在", func(t *testing.T) {
		cfg := &DBConfig{Extra: map[string]string{extraMySQLBinPath: "/usr/bin"}}
		if got := cfg.GetExtraTyped().GetString(extraMySQLBinPath); got != "/usr/bin" {
			t.Errorf("GetString = %v, want /usr/bin", got)
		}
	})

	t.Run("不存在返回空", func(t *testing.T) {
		cfg := &DBConfig{Extra: map[string]string{}}
		if got := cfg.GetExtraTyped().GetString("NOT_EXIST"); got != "" {
			t.Errorf("GetString 不存在的 key 应返回空字符串，得到: %v", got)
		}
	})
}

func TestTypedExtra_GetStringDefault(t *testing.T) {
	t.Run("不存在返回默认值", func(t *testing.T) {
		cfg := &DBConfig{Extra: map[string]string{}}
		if got := cfg.GetExtraTyped().GetStringDefault("NOT_EXIST", "fallback"); got != "fallback" {
			t.Errorf("GetStringDefault = %v, want fallback", got)
		}
	})

	t.Run("有值时返回实际值", func(t *testing.T) {
		cfg := &DBConfig{Extra: map[string]string{"KEY": "actual"}}
		if got := cfg.GetExtraTyped().GetStringDefault("KEY", "fallback"); got != "actual" {
			t.Errorf("GetStringDefault = %v, want actual", got)
		}
	})
}

func TestTypedExtra_GetBool(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"other", false},
	}
	for _, tt := range tests {
		cfg := &DBConfig{Extra: map[string]string{"FLAG": tt.value}}
		if got := cfg.GetExtraTyped().GetBool("FLAG"); got != tt.want {
			t.Errorf("GetBool(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestTypedExtra_IsSet(t *testing.T) {
	extra := (&DBConfig{Extra: map[string]string{"KEY": "value"}}).GetExtraTyped()
	if !extra.IsSet("KEY") {
		t.Error("IsSet(KEY) 应返回 true")
	}
	if extra.IsSet("NOT_EXIST") {
		t.Error("IsSet(NOT_EXIST) 应返回 false")
	}
}

func TestTypedExtra_NilExtra(t *testing.T) {
	extra := (&DBConfig{Extra: nil}).GetExtraTyped()
	if got := extra.GetString("ANY"); got != "" {
		t.Errorf("nil Extra 的 GetString 应返回空字符串，得到: %v", got)
	}
	if extra.IsSet("ANY") {
		t.Error("nil Extra 的 IsSet 应返回 false")
	}
}

func TestTypedExtra_MySQL(t *testing.T) {
	extra := (&DBConfig{
		Extra: map[string]string{
			extraMySQLBinPath:      "/usr/mysql",
			extraXtrabackupBinPath: "/usr/xtrabackup",
			extraDataDir:           "/var/lib/mysql",
			extraServiceName:       "mysqld",
		},
	}).GetExtraTyped()
	if got := extra.MySQLBinPath(); got != "/usr/mysql" {
		t.Errorf("MySQLBinPath() = %v, want /usr/mysql", got)
	}
	if got := extra.XtrabackupBinPath(); got != "/usr/xtrabackup" {
		t.Errorf("XtrabackupBinPath() = %v, want /usr/xtrabackup", got)
	}
	if got := extra.DataDir(); got != "/var/lib/mysql" {
		t.Errorf("DataDir() = %v, want /var/lib/mysql", got)
	}
	if got := extra.ServiceName(); got != "mysqld" {
		t.Errorf("ServiceName() = %v, want mysqld", got)
	}
}

func TestTypedExtra_PostgreSQL(t *testing.T) {
	extra := (&DBConfig{
		Extra: map[string]string{extraPGBinPath: "/usr/pgsql", extraDataDir: "/var/lib/pgsql"},
	}).GetExtraTyped()
	if got := extra.PGBinPath(); got != "/usr/pgsql" {
		t.Errorf("PGBinPath() = %v, want /usr/pgsql", got)
	}
	if got := extra.DataDir(); got != "/var/lib/pgsql" {
		t.Errorf("DataDir() = %v, want /var/lib/pgsql", got)
	}
}

func TestTypedExtra_Oracle(t *testing.T) {
	extra := (&DBConfig{
		Extra: map[string]string{extraOracleHome: "/opt/oracle", extraOracleSID: "ORCL"},
	}).GetExtraTyped()
	if got := extra.OracleHome(); got != "/opt/oracle" {
		t.Errorf("OracleHome() = %v, want /opt/oracle", got)
	}
	if got := extra.OracleSID(); got != "ORCL" {
		t.Errorf("OracleSID() = %v, want ORCL", got)
	}
}

func TestTypedExtra_MSSQL(t *testing.T) {
	t.Run("Windows认证", func(t *testing.T) {
		extra := (&DBConfig{
			Extra: map[string]string{extraInstance: "SQLEXPRESS", extraAuthType: authTypeWindows},
		}).GetExtraTyped()
		if got := extra.Instance(); got != "SQLEXPRESS" {
			t.Errorf("Instance() = %v, want SQLEXPRESS", got)
		}
		if !extra.IsWindowsAuth() {
			t.Error("IsWindowsAuth() 应返回 true")
		}
	})

	t.Run("默认SQL认证", func(t *testing.T) {
		extra := (&DBConfig{Extra: map[string]string{}}).GetExtraTyped()
		if extra.IsWindowsAuth() {
			t.Error("空 AUTH_TYPE 时 IsWindowsAuth() 应返回 false")
		}
		if got := extra.AuthType(); got != authTypeSQL {
			t.Errorf("AuthType() 默认应为 sql，得到: %v", got)
		}
	})
}

func TestGetExtraSpec(t *testing.T) {
	t.Run("获取MySQL规范", func(t *testing.T) {
		spec, ok := GetExtraSpec(dbTypeMySQL)
		if !ok {
			t.Fatal("GetExtraSpec(mysql) 应返回 true")
		}
		if spec.DBType != dbTypeMySQL {
			t.Errorf("DBType = %v, want mysql", spec.DBType)
		}
		if len(spec.Fields) == 0 {
			t.Error("Fields 不应为空")
		}
	})

	t.Run("获取未知类型", func(t *testing.T) {
		if _, ok := GetExtraSpec("unknown"); ok {
			t.Error("未知类型应返回 false")
		}
	})
}

func TestGetAllExtraSpecs(t *testing.T) {
	if specs := GetAllExtraSpecs(); len(specs) < 5 {
		t.Errorf("GetAllExtraSpecs() 应返回至少5种类型，得到 %d", len(specs))
	}
}

func TestExtraHelpMarkdown(t *testing.T) {
	result := ExtraHelpMarkdown()
	if result == "" {
		t.Fatal("ExtraHelpMarkdown() 不应返回空字符串")
	}
	for _, kw := range []string{"MySQL", "PostgreSQL", "Oracle", extraMySQLBinPath, extraOracleHome, extraPGBinPath, extraAuthType, "达梦", extraDamengHome} {
		if !strings.Contains(result, kw) {
			t.Errorf("ExtraHelpMarkdown() 结果应包含 %q", kw)
		}
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(tmpFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return tmpFile
}

func TestLoadConfig_ValidateExtra_OracleMissing(t *testing.T) {
	cfgContent := `{
		"base_backup_dir": "/tmp/backup",
		"databases": {
			"oracle": {
				"type": "oracle", "host": "localhost", "port": 1521,
				"user": "system", "password": "test", "database": "orcl", "extra": {}
			}
		}
	}`
	_, err := LoadConfig(writeTestConfig(t, cfgContent))
	if err == nil {
		t.Fatal("期望 Oracle 缺少必填项时报错")
	}
	if !strings.Contains(err.Error(), extraOracleHome) {
		t.Errorf("错误信息应包含 %s，得到: %v", extraOracleHome, err)
	}
}

func TestLoadConfig_ValidateExtra_UnknownKey(t *testing.T) {
	cfgContent := `{
		"base_backup_dir": "/tmp/backup",
		"databases": {
			"mysql": {
				"type": "mysql", "host": "localhost", "port": 3306,
				"user": "root", "password": "test", "extra": {"INVALID_KEY": "value"}
			}
		}
	}`
	_, err := LoadConfig(writeTestConfig(t, cfgContent))
	if err == nil {
		t.Fatal("期望未知 Extra 字段时报错")
	}
	if !strings.Contains(err.Error(), "INVALID_KEY") {
		t.Errorf("错误信息应包含 INVALID_KEY，得到: %v", err)
	}
}

func TestLoadConfig_ValidateExtra_Valid(t *testing.T) {
	cfgContent := `{
		"base_backup_dir": "/tmp/backup",
		"databases": {
			"mysql": {
				"type": "mysql", "host": "localhost", "port": 3306,
				"user": "root", "password": "test", "extra": {"MYSQL_BIN_PATH": "/usr/bin"}
			}
		}
	}`
	cfg, err := LoadConfig(writeTestConfig(t, cfgContent))
	if err != nil {
		t.Fatalf("有效配置不应报错，得到: %v", err)
	}
	mysqlCfg := cfg.Databases[dbTypeMySQL]
	if got := mysqlCfg.GetExtraTyped().MySQLBinPath(); got != "/usr/bin" {
		t.Errorf("MySQLBinPath() = %v, want /usr/bin", got)
	}
}

func TestValidateExtra_Dameng(t *testing.T) {
	t.Run("缺少必填项DM_HOME", func(t *testing.T) {
		cfg := &DBConfig{Type: dbTypeDameng, Host: defaultHost, Port: 5236, Extra: map[string]string{}}
		errs := cfg.ValidateExtra()
		if len(errs) == 0 {
			t.Fatal("期望至少1个错误，但返回空")
		}
		errMsg := errs[0].Error()
		if !strings.Contains(errMsg, extraDamengHome) {
			t.Errorf("错误信息应包含 %s，得到: %v", extraDamengHome, errMsg)
		}
	})

	t.Run("完整配置有效", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeDameng,
			Host:  defaultHost,
			Port:  5236,
			Extra: map[string]string{extraDamengHome: "/opt/dmdbms", extraDamengInstance: "DMSERVER"},
		}
		if errs := cfg.ValidateExtra(); len(errs) > 0 {
			t.Errorf("期望无错误，得到: %v", errs)
		}
	})

	t.Run("未知字段报错", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeDameng,
			Host:  defaultHost,
			Port:  5236,
			Extra: map[string]string{extraDamengHome: "/opt/dmdbms", "INVALID_KEY": "value"},
		}
		errs := cfg.ValidateExtra()
		if len(errs) == 0 {
			t.Fatal("期望有错误，但返回空")
		}
		if !strings.Contains(errs[0].Error(), "INVALID_KEY") {
			t.Errorf("错误信息应包含 INVALID_KEY，得到: %v", errs[0])
		}
	})

	t.Run("仅DM_HOME必填", func(t *testing.T) {
		cfg := &DBConfig{
			Type:  dbTypeDameng,
			Host:  defaultHost,
			Port:  5236,
			Extra: map[string]string{extraDamengHome: "/opt/dmdbms"},
		}
		if errs := cfg.ValidateExtra(); len(errs) > 0 {
			t.Errorf("仅 DM_HOME 时不应报错，得到: %v", errs)
		}
	})
}

func TestTypedExtra_Dameng(t *testing.T) {
	extra := (&DBConfig{
		Extra: map[string]string{
			extraDamengHome:     "/opt/dmdbms",
			extraDamengInstance: "DMSERVER",
			extraDamengDataDir:  "/opt/dmdbms/data/DAMENG",
		},
	}).GetExtraTyped()
	if got := extra.DamengHome(); got != "/opt/dmdbms" {
		t.Errorf("DamengHome() = %v, want /opt/dmdbms", got)
	}
	if got := extra.DamengInstance(); got != "DMSERVER" {
		t.Errorf("DamengInstance() = %v, want DMSERVER", got)
	}
	if got := extra.DamengDataDir(); got != "/opt/dmdbms/data/DAMENG" {
		t.Errorf("DamengDataDir() = %v, want /opt/dmdbms/data/DAMENG", got)
	}
}
