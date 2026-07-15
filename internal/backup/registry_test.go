package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// stubDriver 是测试用的最小 DatabaseBackup 实现，仅用于验证注册表逻辑。
// BaseBackup 提供 NotSupported 默认实现的方法（RegisterBackup/UnregisterBackup/
// VerifyBackupStatus/DeleteInvalidBackups/ValidateBackup/ListDatabases/
// EnableArchiveLogMode/DisableArchiveLogMode/Close），其余方法在此显式实现。
type stubDriver struct {
	*BaseBackup
}

func newStubDriver(cfg *DBConfig) (DatabaseBackup, error) {
	return &stubDriver{BaseBackup: NewBaseBackup(cfg)}, nil
}

func (s *stubDriver) Backup(ctx context.Context, _ BackupOptions, _ ProgressCallback) (*BackupResult, error) {
	return nil, NewNotSupportedError(ctx, "Backup", s.GetConfig().Type)
}

func (s *stubDriver) Restore(ctx context.Context, _ RestoreOptions, _ ProgressCallback) (*RestoreResult, error) {
	return nil, NewNotSupportedError(ctx, "Restore", s.GetConfig().Type)
}

func (s *stubDriver) ListBackups(ctx context.Context, _ ...BackupOptions) ([]BackupInfo, error) {
	return nil, NewNotSupportedError(ctx, "ListBackups", s.GetConfig().Type)
}

func (s *stubDriver) DeleteBackup(ctx context.Context, _ string, _ ...BackupOptions) error {
	return NewNotSupportedError(ctx, "DeleteBackup", s.GetConfig().Type)
}

func (s *stubDriver) GetBackupInfo(ctx context.Context, _ string, _ ...BackupOptions) (map[string]string, error) {
	return nil, NewNotSupportedError(ctx, "GetBackupInfo", s.GetConfig().Type)
}

func (s *stubDriver) DeleteAllBackups(ctx context.Context, _ ...BackupOptions) error {
	return NewNotSupportedError(ctx, "DeleteAllBackups", s.GetConfig().Type)
}

// stubFactory 返回固定的工厂函数，用于注册测试驱动
func stubFactory(cfg *DBConfig) (DatabaseBackup, error) {
	return newStubDriver(cfg)
}

// registerTestDriver 注册一个测试驱动并返回其名称，测试结束自动注销
func registerTestDriver(t *testing.T, name string) {
	t.Helper()
	if err := RegisterDriver(DriverMetadata{
		Name:        name,
		Version:     "test",
		Description: "stub driver for registry tests",
	}, stubFactory); err != nil {
		t.Fatalf("注册测试驱动 %q 失败: %v", name, err)
	}
	t.Cleanup(func() { UnregisterDriver(name) })
}

func TestRegisterDriver_Errors(t *testing.T) {
	tests := []struct {
		name     string
		metadata DriverMetadata
		factory  DriverFactory
		wantSub  string
	}{
		{
			name:     "nil factory",
			metadata: DriverMetadata{Name: "nil-factory-driver"},
			factory:  nil,
			wantSub:  "nil factory",
		},
		{
			name:     "empty name",
			metadata: DriverMetadata{Name: ""},
			factory:  stubFactory,
			wantSub:  "empty driver name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterDriver(tt.metadata, tt.factory)
			if err == nil {
				t.Fatal("期望返回错误，但返回了 nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("错误信息应包含 %q, got: %v", tt.wantSub, err)
			}
		})
	}
}

func TestRegisterDriver_DuplicateName(t *testing.T) {
	const dupName = "test-duplicate-driver"
	registerTestDriver(t, dupName)

	err := RegisterDriver(DriverMetadata{
		Name:        dupName,
		Version:     "v2",
		Description: "duplicate",
	}, stubFactory)
	if err == nil {
		t.Fatal("重复注册应返回错误")
	}
	if !strings.Contains(err.Error(), "called twice") {
		t.Errorf("错误信息应指示重复注册, got: %v", err)
	}
}

func TestRegisterDriver_Success(t *testing.T) {
	const name = "test-register-success"
	registerTestDriver(t, name)

	// 验证可以查询到
	meta, ok := GetDriverMetadata(name)
	if !ok {
		t.Fatal("注册后应能查询到驱动元数据")
	}
	if meta.Version != "test" {
		t.Errorf("Version = %q, want %q", meta.Version, "test")
	}
}

func TestUnregisterDriver(t *testing.T) {
	const name = "test-unregister-driver"
	registerTestDriver(t, name)

	UnregisterDriver(name)

	if _, ok := GetDriverMetadata(name); ok {
		t.Error("注销后不应能查询到驱动")
	}
	// 再次注销不存在的驱动不应 panic
	UnregisterDriver(name)
}

func TestGetDriverMetadata_NotExists(t *testing.T) {
	_, ok := GetDriverMetadata("non-existent-driver-xyz")
	if ok {
		t.Error("不存在的驱动应返回 ok=false")
	}
}

func TestListDrivers_ReturnsSortedList(t *testing.T) {
	// 注册多个有明确顺序的驱动
	names := []string{"z-driver", "a-driver", "m-driver"}
	for _, n := range names {
		registerTestDriver(t, n)
	}

	all := ListDrivers()
	// 验证包含我们注册的驱动
	for _, n := range names {
		found := false
		for _, got := range all {
			if got == n {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListDrivers 未包含已注册的驱动 %q", n)
		}
	}
	// 验证返回的是排序列表
	for i := 1; i < len(all); i++ {
		if all[i-1] > all[i] {
			t.Errorf("ListDrivers 未排序: %v 在 %v 之前", all[i-1], all[i])
			break
		}
	}
}

func TestListDriverMetadata_ReturnsSortedList(t *testing.T) {
	names := []string{"meta-z", "meta-a", "meta-m"}
	for _, n := range names {
		registerTestDriver(t, n)
	}

	all := ListDriverMetadata()
	// 验证按 Name 排序
	for i := 1; i < len(all); i++ {
		if all[i-1].Name > all[i].Name {
			t.Errorf("ListDriverMetadata 未按 Name 排序: %q 在 %q 之前", all[i-1].Name, all[i].Name)
			break
		}
	}
	// 验证包含我们注册的驱动
	for _, n := range names {
		found := false
		for _, m := range all {
			if m.Name == n {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListDriverMetadata 未包含已注册的驱动 %q", n)
		}
	}
}

func TestNewBackup_Errors(t *testing.T) {
	tests := []struct {
		name    string
		config  *DBConfig
		wantSub string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantSub: "config 不能为空",
		},
		{
			name:    "empty type",
			config:  &DBConfig{Type: ""},
			wantSub: "必须指定数据库类型",
		},
		{
			name:    "unsupported type",
			config:  &DBConfig{Type: "unsupported-db-type-xyz"},
			wantSub: "不支持的数据库类型",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBackup(tt.config)
			if err == nil {
				t.Fatal("期望返回错误，但返回了 nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("错误信息应包含 %q, got: %v", tt.wantSub, err)
			}
		})
	}
}

func TestNewBackup_Success(t *testing.T) {
	const name = "test-newbackup-success"
	registerTestDriver(t, name)

	db, err := NewBackup(&DBConfig{Type: name})
	if err != nil {
		t.Fatalf("NewBackup 失败: %v", err)
	}
	if db == nil {
		t.Fatal("返回的 DatabaseBackup 不应为 nil")
	}
	// 验证返回的是 stubDriver 实例
	if _, ok := db.(*stubDriver); !ok {
		t.Errorf("返回类型 = %T, want *stubDriver", db)
	}
}

func TestValidateDriverConfig_Errors(t *testing.T) {
	tests := []struct {
		name    string
		config  *DBConfig
		wantSub string
	}{
		{
			name:    "nil config",
			config:  nil,
			wantSub: "config 不能为空",
		},
		{
			name:    "empty type",
			config:  &DBConfig{Type: ""},
			wantSub: "必须指定数据库类型",
		},
		{
			name:    "unsupported type",
			config:  &DBConfig{Type: "unsupported-db-type-xyz"},
			wantSub: "不支持的数据库类型",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDriverConfig(tt.config)
			if err == nil {
				t.Fatal("期望返回错误，但返回了 nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("错误信息应包含 %q, got: %v", tt.wantSub, err)
			}
		})
	}
}

func TestValidateDriverConfig_Success(t *testing.T) {
	const name = "test-validate-success"
	registerTestDriver(t, name)

	if err := ValidateDriverConfig(&DBConfig{Type: name}); err != nil {
		t.Errorf("ValidateDriverConfig 对已注册驱动应返回 nil, got: %v", err)
	}
}

// TestStubDriver_BaseBackupIntegration 验证 stubDriver 通过 BaseBackup 提供 NotSupported 默认实现
func TestStubDriver_BaseBackupIntegration(t *testing.T) {
	const name = "test-stub-integration"
	registerTestDriver(t, name)

	db, err := NewBackup(&DBConfig{Type: name})
	if err != nil {
		t.Fatalf("NewBackup 失败: %v", err)
	}

	// Close 应返回 nil（BaseBackup 默认实现）
	if err := db.Close(); err != nil {
		t.Errorf("Close() 应返回 nil, got: %v", err)
	}

	// ListDatabases 应返回 NotSupported 错误
	_, err = db.ListDatabases(context.Background())
	if err == nil {
		t.Fatal("ListDatabases 应返回 NotSupported 错误")
	}
	var be *BackupError
	if !errors.As(err, &be) {
		t.Fatalf("期望 *BackupError, 实际 %T", err)
	}
	if be.Type != ErrorTypeNotSupported {
		t.Errorf("Type = %v, want %v", be.Type, ErrorTypeNotSupported)
	}
}
