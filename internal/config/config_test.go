package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Run("有效配置文件", func(t *testing.T) {
		cfgContent := `{
			"base_backup_dir": "/tmp/backup",
			"databases": {
				"mysql": {
					"type": "mysql",
					"host": "localhost",
					"port": 3306,
					"user": "root",
					"password": "test",
					"database": "testdb"
				}
			}
		}`
		tmpFile := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(tmpFile, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg, err := LoadConfig(tmpFile)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if cfg.BaseBackupDir != "/tmp/backup" {
			t.Errorf("BaseBackupDir = %v, want /tmp/backup", cfg.BaseBackupDir)
		}
		if len(cfg.Databases) != 1 {
			t.Errorf("Databases count = %v, want 1", len(cfg.Databases))
		}
		mysqlCfg, ok := cfg.Databases[dbTypeMySQL]
		if !ok {
			t.Fatal("Databases[\"mysql\"] not found")
		}
		if mysqlCfg.Database != "testdb" {
			t.Errorf("Databases[\"mysql\"].Database = %v, want testdb", mysqlCfg.Database)
		}
	})

	t.Run("不存在的文件", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/config.json")
		if err == nil {
			t.Fatal("期望返回错误，但返回了 nil")
		}
	})

	t.Run("无效JSON", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(tmpFile, []byte("{invalid}"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadConfig(tmpFile)
		if err == nil {
			t.Fatal("期望返回错误，但返回了 nil")
		}
	})
}

func TestDBConfig_SetDefaults(t *testing.T) {
	t.Run("MySQL默认值", func(t *testing.T) {
		cfg := &DBConfig{Type: dbTypeMySQL}
		cfg.SetDefaults()
		if cfg.Host != defaultHost {
			t.Errorf("Host = %v, want localhost", cfg.Host)
		}
		if cfg.Port != 3306 {
			t.Errorf("Port = %v, want 3306", cfg.Port)
		}
		if cfg.SSLMode != "disable" {
			t.Errorf("SSLMode = %v, want disable", cfg.SSLMode)
		}
		if cfg.Extra == nil {
			t.Error("Extra should not be nil after SetDefaults")
		}
	})

	t.Run("PostgreSQL默认值", func(t *testing.T) {
		cfg := &DBConfig{Type: "postgresql"}
		cfg.SetDefaults()
		if cfg.Port != 5432 {
			t.Errorf("Port = %v, want 5432", cfg.Port)
		}
	})

	t.Run("已有值不被覆盖", func(t *testing.T) {
		cfg := &DBConfig{Type: "mysql", Host: "remotehost", Port: 3307}
		cfg.SetDefaults()
		if cfg.Host != "remotehost" {
			t.Errorf("Host = %v, want remotehost", cfg.Host)
		}
		if cfg.Port != 3307 {
			t.Errorf("Port = %v, want 3307", cfg.Port)
		}
	})
}

func TestGetDBConfig_NotFound(t *testing.T) {
	cfg := &Config{Databases: map[string]DBConfig{}}
	_, err := cfg.GetDBConfig("nonexistent")
	if err == nil {
		t.Fatal("期望返回错误，但返回了 nil")
	}
	if !errors.Is(err, ErrDBConfigNotFound) {
		t.Errorf("error = %v, want ErrDBConfigNotFound", err)
	}
}

func TestGetDBConfig_Found(t *testing.T) {
	cfg := &Config{
		Databases: map[string]DBConfig{
			dbTypeMySQL: {Type: dbTypeMySQL, Host: defaultHost, Port: 3306},
		},
	}
	dbCfg, err := cfg.GetDBConfig(dbTypeMySQL)
	if err != nil {
		t.Fatalf("GetDBConfig() error = %v", err)
	}
	if dbCfg.Type != dbTypeMySQL {
		t.Errorf("Type = %v, want mysql", dbCfg.Type)
	}
}
