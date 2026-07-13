package backup

import (
	"strings"
	"testing"
)

func TestDamengBackup_BuildDexpArgs_Full(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)

	opts := BackupOptions{EnableCompression: true, ParallelWorkers: 4}
	args := dm.buildDexpArgs("/backup/dameng_full_20260703.dmp", "20260703_120000", opts, "FULL")

	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "USERID=SYSDBA/test123@localhost:5236") {
		t.Errorf("缺少 USERID 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "FILE=/backup/dameng_full_20260703.dmp") {
		t.Errorf("缺少 FILE 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "FULL=Y") {
		t.Errorf("缺少 FULL=Y 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "ROWS=Y") {
		t.Errorf("缺少 ROWS=Y 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "FEEDBACK=1000") {
		t.Errorf("缺少 FEEDBACK=1000 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "COMPRESS=Y") {
		t.Errorf("缺少 COMPRESS=Y 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "PARALLEL=4") {
		t.Errorf("缺少 PARALLEL=4 参数，得到: %s", argsStr)
	}
}

func TestDamengBackup_BuildDexpArgs_Schemas(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)

	opts := BackupOptions{}
	args := dm.buildDexpArgs("/backup/schema1_20260703.dmp", "20260703_120000", opts, "SCHEMAS", "SCHEMA1")

	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "SCHEMAS=SCHEMA1") {
		t.Errorf("缺少 SCHEMAS=SCHEMA1 参数，得到: %s", argsStr)
	}
	if strings.Contains(argsStr, "FULL=Y") {
		t.Errorf("SCHEMAS 模式不应包含 FULL=Y，得到: %s", argsStr)
	}
	if strings.Contains(argsStr, "COMPRESS=Y") {
		t.Errorf("未启用压缩不应包含 COMPRESS=Y，得到: %s", argsStr)
	}
}

func TestDamengBackup_BuildDimpArgs_Full(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)

	opts := RestoreOptions{}
	args := dm.buildDimpArgs("/backup/dameng_full.dmp", "20260703_120000", opts)

	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "FILE=/backup/dameng_full.dmp") {
		t.Errorf("缺少 FILE 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "FULL=Y") {
		t.Errorf("默认还原应包含 FULL=Y，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "TABLE_EXISTS_ACTION=REPLACE") {
		t.Errorf("缺少 TABLE_EXISTS_ACTION=REPLACE 参数，得到: %s", argsStr)
	}
	if strings.Contains(argsStr, "DESTROY=Y") {
		t.Errorf("dimp V8 不支持 DESTROY 参数，得到: %s", argsStr)
	}
	if strings.Contains(argsStr, "IGNORE=Y") {
		t.Errorf("TABLE_EXISTS_ACTION 已替代 IGNORE，不应同时使用 IGNORE=Y，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "COMPILE=Y") {
		t.Errorf("缺少 COMPILE=Y 参数，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "FEEDBACK=1000") {
		t.Errorf("缺少 FEEDBACK=1000 参数，得到: %s", argsStr)
	}
}

func TestDamengBackup_BuildDimpArgs_WithSchema(t *testing.T) {
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Extra:    map[string]string{"DM_HOME": "/opt/dmdbms"},
	}
	dm, _ := NewDamengBackup(cfg)

	opts := RestoreOptions{TargetDatabaseName: "TARGET_SCHEMA"}
	args := dm.buildDimpArgs("/backup/schema1.dmp", "20260703_120000", opts)

	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "SCHEMAS=TARGET_SCHEMA") {
		t.Errorf("缺少 SCHEMAS=TARGET_SCHEMA 参数，得到: %s", argsStr)
	}
	if strings.Contains(argsStr, "FULL=Y") {
		t.Errorf("指定目标模式时不应包含 FULL=Y，得到: %s", argsStr)
	}
	if !strings.Contains(argsStr, "TABLE_EXISTS_ACTION=REPLACE") {
		t.Errorf("指定目标模式时也应包含 TABLE_EXISTS_ACTION=REPLACE，得到: %s", argsStr)
	}
}
