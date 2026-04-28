package config

import (
	"encoding/json"
	"os"

	"db-backup-restore/internal/backup"
)

type DatabaseConfig struct {
	Type     string            `json:"type"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	User     string            `json:"user"`
	Password string            `json:"password"`
	Database string            `json:"database"`
	SSLMode  string            `json:"ssl_mode"`
	Extra    map[string]string `json:"extra"`
}

type Config struct {
	BaseBackupDir string                    `json:"base_backup_dir"`
	Databases     map[string]DatabaseConfig `json:"databases"`
}

func LoadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (cfg *Config) GetDBConfig(dbType string) (*backup.DBConfig, error) {
	dbCfg, ok := cfg.Databases[dbType]
	if !ok {
		return nil, os.ErrNotExist
	}

	return &backup.DBConfig{
		Type:     dbCfg.Type,
		Host:     dbCfg.Host,
		Port:     dbCfg.Port,
		User:     dbCfg.User,
		Password: dbCfg.Password,
		Database: dbCfg.Database,
		SSLMode:  dbCfg.SSLMode,
		Extra:    dbCfg.Extra,
	}, nil
}
