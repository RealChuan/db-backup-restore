package app

import (
	"context"
	"errors"
	"testing"

	"github.com/RealChuan/db-backup-restore/internal/backup"
	"github.com/RealChuan/db-backup-restore/internal/config"
)

// fakeDriver 用于测试的假数据库驱动，仅实现 ListDatabases 的可控行为，
// 其余方法通过 BaseBackup 默认实现返回「不支持」。
type fakeDriver struct {
	*backup.BaseBackup
	databases []string
	listErr   error
}

func newFakeDriver(cfg *backup.DBConfig, databases []string, listErr error) *fakeDriver {
	return &fakeDriver{
		BaseBackup: backup.NewBaseBackup(cfg),
		databases:  databases,
		listErr:    listErr,
	}
}

// ListDatabases 返回预设的数据库列表或错误
func (f *fakeDriver) ListDatabases(_ context.Context) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.databases, nil
}

// 以下方法为满足 DatabaseBackup 接口而存在，测试中不使用
func (f *fakeDriver) Backup(ctx context.Context, _ backup.BackupOptions, _ backup.ProgressCallback) (*backup.BackupResult, error) {
	return nil, backup.NewNotSupportedError(ctx, "Backup", f.GetConfig().Type)
}

func (f *fakeDriver) Restore(ctx context.Context, _ backup.RestoreOptions, _ backup.ProgressCallback) (*backup.RestoreResult, error) {
	return nil, backup.NewNotSupportedError(ctx, "Restore", f.GetConfig().Type)
}

func (f *fakeDriver) ListBackups(ctx context.Context, _ ...backup.BackupOptions) ([]backup.BackupInfo, error) {
	return nil, backup.NewNotSupportedError(ctx, "ListBackups", f.GetConfig().Type)
}

func (f *fakeDriver) DeleteBackup(ctx context.Context, _ string, _ ...backup.BackupOptions) error {
	return backup.NewNotSupportedError(ctx, "DeleteBackup", f.GetConfig().Type)
}

func (f *fakeDriver) GetBackupInfo(ctx context.Context, _ string, _ ...backup.BackupOptions) (map[string]string, error) {
	return nil, backup.NewNotSupportedError(ctx, "GetBackupInfo", f.GetConfig().Type)
}

func (f *fakeDriver) DeleteAllBackups(ctx context.Context, _ ...backup.BackupOptions) error {
	return backup.NewNotSupportedError(ctx, "DeleteAllBackups", f.GetConfig().Type)
}

const fakeDriverName = "fake-test-driver"

func TestManagerApp_ListDatabases_Success(t *testing.T) {
	if err := backup.RegisterDriver(backup.DriverMetadata{
		Name:             fakeDriverName,
		Version:          "test",
		Description:      "fake driver for testing",
		SupportedActions: []string{"list_databases"},
	}, func(cfg *backup.DBConfig) (backup.DatabaseBackup, error) {
		return newFakeDriver(cfg, []string{"db_alpha", "db_beta", "db_gamma"}, nil), nil
	}); err != nil {
		t.Fatalf("注册驱动失败: %v", err)
	}
	defer backup.UnregisterDriver(fakeDriverName)

	cfg := &config.Config{
		Databases: map[string]config.DBConfig{
			fakeDriverName: {Type: fakeDriverName, Host: "localhost"},
		},
	}

	app := NewManagerApp(cfg)
	result, err := app.ListDatabases(context.Background(), fakeDriverName)
	if err != nil {
		t.Fatalf("ListDatabases() error = %v", err)
	}

	if !result.Success {
		t.Fatal("期望 Success=true")
	}
	if result.Operation != "list_databases" {
		t.Errorf("Operation = %q, want %q", result.Operation, "list_databases")
	}
	if result.Message != "共 3 个数据库" {
		t.Errorf("Message = %q, want %q", result.Message, "共 3 个数据库")
	}

	dbs, ok := result.Data["databases"].([]interface{})
	if !ok {
		t.Fatalf("Data[databases] type = %T, want []interface{}", result.Data["databases"])
	}
	if len(dbs) != 3 {
		t.Fatalf("databases 数量 = %d, want 3", len(dbs))
	}
	want := []string{"db_alpha", "db_beta", "db_gamma"}
	for i, db := range dbs {
		if db != want[i] {
			t.Errorf("databases[%d] = %q, want %q", i, db, want[i])
		}
	}
}

func TestManagerApp_ListDatabases_NotSupported(t *testing.T) {
	const notSupName = fakeDriverName + "-nosup"
	if err := backup.RegisterDriver(backup.DriverMetadata{
		Name:        notSupName,
		Version:     "test",
		Description: "fake driver returning not-supported",
	}, func(cfg *backup.DBConfig) (backup.DatabaseBackup, error) {
		listErr := backup.NewNotSupportedError(context.Background(), "ListDatabases", cfg.Type)
		return newFakeDriver(cfg, nil, listErr), nil
	}); err != nil {
		t.Fatalf("注册驱动失败: %v", err)
	}
	defer backup.UnregisterDriver(notSupName)

	cfg := &config.Config{
		Databases: map[string]config.DBConfig{
			notSupName: {Type: notSupName, Host: "localhost"},
		},
	}

	app := NewManagerApp(cfg)
	_, err := app.ListDatabases(context.Background(), notSupName)
	if err == nil {
		t.Fatal("期望返回错误，但返回了 nil")
	}

	var be *backup.BackupError
	if !errors.As(err, &be) {
		t.Fatalf("期望 *BackupError，实际 %T", err)
	}
	if be.Type != backup.ErrorTypeNotSupported {
		t.Errorf("Type = %v, want %v", be.Type, backup.ErrorTypeNotSupported)
	}
}
