package backup

import (
	"context"
	"os"
	"testing"
)

func TestNewDamengBackup_InvalidType(t *testing.T) {
	cfg := &DBConfig{Type: "mysql"}
	_, err := NewDamengBackup(cfg)
	if err == nil {
		t.Error("期望类型校验失败，但返回了 nil")
	}
}

func TestNewDamengBackup_MissingDMHome(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test",
		Extra:    map[string]string{},
	}
	// 清除环境变量避免干扰
	origDMHome := os.Getenv("DM_HOME")
	os.Unsetenv("DM_HOME")
	defer func() {
		if origDMHome != "" {
			os.Setenv("DM_HOME", origDMHome)
		}
	}()

	_, err := NewDamengBackup(cfg)
	if err == nil {
		t.Error("缺少 DM_HOME 应报错")
	}
}

func TestNewDamengBackup_ValidConfig(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Database: "DMSERVER",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}

	dm, err := NewDamengBackup(cfg)
	if err != nil {
		t.Fatalf("有效配置不应报错，得到: %v", err)
	}
	if dm.dmHome != "/opt/dmdbms" {
		t.Errorf("dmHome = %v, want /opt/dmdbms", dm.dmHome)
	}
	if dm.dmInstance != "DMSERVER" {
		t.Errorf("dmInstance = %v, want DMSERVER", dm.dmInstance)
	}
}

func TestNewDamengBackup_InstanceFromExtra(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Database: "DB1",
		Extra: map[string]string{
			"DM_HOME":     "/opt/dmdbms",
			"DM_INSTANCE": "CUSTOM_INSTANCE",
		},
	}

	dm, err := NewDamengBackup(cfg)
	if err != nil {
		t.Fatalf("有效配置不应报错，得到: %v", err)
	}
	if dm.dmInstance != "CUSTOM_INSTANCE" {
		t.Errorf("dmInstance = %v, want CUSTOM_INSTANCE (Extra 优先于 Database)", dm.dmInstance)
	}
}

func TestDamengBackup_BuildConnectionString(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "192.168.1.100",
		Port:     5237,
		User:     "SYSDBA",
		Password: "MyPass123",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)

	got := dm.buildConnectionString()
	want := "SYSDBA/MyPass123@192.168.1.100:5237"
	if got != want {
		t.Errorf("buildConnectionString() = %v, want %v", got, want)
	}

	masked := dm.buildConnectionStringMasked()
	wantMasked := "SYSDBA/***@192.168.1.100:5237"
	if masked != wantMasked {
		t.Errorf("buildConnectionStringMasked() = %v, want %v", masked, wantMasked)
	}
}

func TestDamengBackup_GetServiceName(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms", "DM_INSTANCE": "DMSERVER"},
	}
	dm, _ := NewDamengBackup(cfg)
	if got := dm.getServiceName(); got != "DmServiceDMSERVER" {
		t.Errorf("getServiceName() = %v, want DmServiceDMSERVER", got)
	}
}

func TestDamengBackup_NotSupportedMethods(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)
	ctx := context.Background()

	t.Run("RegisterBackup", func(t *testing.T) {
		err := dm.RegisterBackup(ctx, "/tmp/test")
		if err == nil {
			t.Error("期望 NotSupported 错误")
		}
	})

	t.Run("UnregisterBackup", func(t *testing.T) {
		err := dm.UnregisterBackup(ctx, "test")
		if err == nil {
			t.Error("期望 NotSupported 错误")
		}
	})
}
