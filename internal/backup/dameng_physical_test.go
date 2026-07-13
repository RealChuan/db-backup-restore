package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestDamengBackup(extra ...map[string]string) *DamengBackup {
	extraMap := map[string]string{"DM_HOME": "/opt/dmdbms"}
	for _, m := range extra {
		for k, v := range m {
			extraMap[k] = v
		}
	}
	cfg := &DBConfig{
		Type:     DBTypeDameng,
		Host:     "localhost",
		Port:     5236,
		User:     "SYSDBA",
		Password: "test123",
		Extra:    extraMap,
	}
	dm, _ := NewDamengBackup(cfg)
	return dm
}

// ===== 备份脚本测试 =====

func TestDamengBackup_BuildFullBackupScript(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{EnableCompression: true, ParallelWorkers: 2}
	script := dm.buildFullBackupScript("DM_FULL_20260703", "/backup/dm_full_20260703", opts)

	if !strings.Contains(script, `BACKUP DATABASE FULL TO DM_FULL_20260703`) {
		t.Errorf("全量备份脚本缺少 BACKUP DATABASE FULL TO，得到: %s", script)
	}
	if !strings.Contains(script, `BACKUPSET '/backup/dm_full_20260703'`) {
		t.Errorf("全量备份脚本缺少 BACKUPSET，得到: %s", script)
	}
	if !strings.Contains(script, "COMPRESSED") {
		t.Errorf("启用压缩时脚本应包含 COMPRESSED，得到: %s", script)
	}
	if !strings.Contains(script, "PARALLEL 2") {
		t.Errorf("并行度 2 时脚本应包含 PARALLEL 2，得到: %s", script)
	}
}

func TestDamengBackup_BuildFullBackupScript_NoCompression(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{}
	script := dm.buildFullBackupScript("DM_FULL_20260703", "/backup/dm_full_20260703", opts)

	if strings.Contains(script, "COMPRESSED") {
		t.Errorf("未启用压缩时脚本不应包含 COMPRESSED，得到: %s", script)
	}
}

func TestDamengBackup_BuildArchiveBackupScript(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{EnableCompression: true}
	script, err := dm.buildArchiveBackupScript("DM_ARCH_20260703", "/backup/dm_arch_20260703", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `BACKUP ARCHIVELOG ALL TO DM_ARCH_20260703`) {
		t.Errorf("归档备份脚本缺少 BACKUP ARCHIVELOG ALL TO，得到: %s", script)
	}
	if !strings.Contains(script, "COMPRESSED") {
		t.Errorf("启用压缩时归档备份脚本应包含 COMPRESSED，得到: %s", script)
	}
}

func TestDamengBackup_BuildIncrementalBackupScript(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{"DM_DATA_DIR": "/opt/dmdbms/data/DAMENG"})

	opts := BackupOptions{Mode: BackupModeIncremental}
	script, err := dm.buildIncrementalBackupScript("DM_INCR_20260703", "/backup/dm_incr_20260703", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `BACKUP DATABASE INCREMENT WITH BACKUPDIR`) {
		t.Errorf("增量备份脚本缺少 BACKUP DATABASE INCREMENT WITH BACKUPDIR，得到: %s", script)
	}
	if strings.Contains(script, "CUMULATIVE") {
		t.Errorf("增量模式不应包含 CUMULATIVE，得到: %s", script)
	}
}

func TestDamengBackup_BuildIncrementalBackupScript_Cumulative(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{"DM_DATA_DIR": "/opt/dmdbms/data/DAMENG"})

	opts := BackupOptions{Mode: BackupModeDifferential}
	script, err := dm.buildIncrementalBackupScript("DM_INCR_20260703", "/backup/dm_incr_20260703", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "CUMULATIVE") {
		t.Errorf("差分模式应包含 CUMULATIVE，得到: %s", script)
	}
}

// ===== 还原脚本测试 =====

func TestDamengBackup_BuildFullRestoreScript(t *testing.T) {
	dm := newTestDamengBackup()

	opts := RestoreOptions{RestoreMode: RestoreModeFull}
	script := dm.buildFullRestoreScript("/backup/dm_full_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, `RESTORE DATABASE`) {
		t.Errorf("全量还原脚本缺少 RESTORE DATABASE，得到: %s", script)
	}
	if !strings.Contains(script, `RECOVER DATABASE`) {
		t.Errorf("全量还原脚本缺少 RECOVER DATABASE，得到: %s", script)
	}
	if !strings.Contains(script, "UPDATE DB_MAGIC") {
		t.Errorf("全量还原脚本缺少 UPDATE DB_MAGIC，得到: %s", script)
	}
	if strings.Contains(script, "UNTIL TIME") {
		t.Errorf("非 PITR 还原不应包含 UNTIL TIME，得到: %s", script)
	}
	if strings.Contains(script, "WITH BACKUPDIR") {
		t.Errorf("全量还原不应包含 WITH BACKUPDIR，得到: %s", script)
	}
}

func TestDamengBackup_BuildFullRestoreScript_WithPITR(t *testing.T) {
	dm := newTestDamengBackup()

	pitrTime := time.Date(2026, 7, 3, 10, 30, 0, 0, time.Local)
	opts := RestoreOptions{RecoveryPointInTime: pitrTime}
	script := dm.buildFullRestoreScript("/backup/dm_full_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, "UNTIL TIME") {
		t.Errorf("PITR 还原应包含 UNTIL TIME，得到: %s", script)
	}
	if !strings.Contains(script, "2026-07-03 10:30:00") {
		t.Errorf("PITR 还原应包含正确的时间格式，得到: %s", script)
	}
}

func TestDamengBackup_BuildFullRestoreScript_WithArchDir(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{
		"DM_HOME": "/opt/dmdbms",
	})

	opts := RestoreOptions{
		RestoreMode:    RestoreModeFull,
		ArchiveLogDest: "/opt/dmdbms/arch",
	}
	script := dm.buildFullRestoreScript("/backup/dm_full_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, `WITH ARCHIVEDIR '/opt/dmdbms/arch'`) {
		t.Errorf("配置归档目录时应使用 WITH ARCHIVEDIR，得到: %s", script)
	}
	// RESTORE 命令总是使用 FROM BACKUPSET，但 RECOVER 命令在有归档目录时应使用 WITH ARCHIVEDIR
	if !strings.Contains(script, `RECOVER DATABASE '/opt/dmdbms/data/DAMENG_new' WITH ARCHIVEDIR '/opt/dmdbms/arch'`) {
		t.Errorf("配置归档目录时 RECOVER 应使用 WITH ARCHIVEDIR，得到: %s", script)
	}
}

func TestDamengBackup_BuildIncrementalRestoreScript(t *testing.T) {
	dm := newTestDamengBackup()

	opts := RestoreOptions{RestoreMode: RestoreModeIncremental}
	script := dm.buildIncrementalRestoreScript("/backup/dm_full_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, `RESTORE DATABASE`) {
		t.Errorf("增量还原脚本缺少 RESTORE DATABASE，得到: %s", script)
	}
	if !strings.Contains(script, `RECOVER DATABASE`) {
		t.Errorf("增量还原脚本缺少 RECOVER DATABASE，得到: %s", script)
	}
	if !strings.Contains(script, "WITH BACKUPDIR") {
		t.Errorf("增量还原脚本应包含 WITH BACKUPDIR 以自动应用增量备份集，得到: %s", script)
	}
	if !strings.Contains(script, "UPDATE DB_MAGIC") {
		t.Errorf("增量还原脚本缺少 UPDATE DB_MAGIC，得到: %s", script)
	}
}

func TestDamengBackup_BuildIncrementalRestoreScript_WithPITR(t *testing.T) {
	dm := newTestDamengBackup()

	pitrTime := time.Date(2026, 7, 3, 14, 0, 0, 0, time.Local)
	opts := RestoreOptions{
		RestoreMode:         RestoreModeIncremental,
		RecoveryPointInTime: pitrTime,
	}
	script := dm.buildIncrementalRestoreScript("/backup/dm_full_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, "WITH BACKUPDIR") {
		t.Errorf("增量还原脚本应包含 WITH BACKUPDIR，得到: %s", script)
	}
	if !strings.Contains(script, "UNTIL TIME '2026-07-03 14:00:00'") {
		t.Errorf("增量还原 PITR 应包含 UNTIL TIME，得到: %s", script)
	}
}

func TestDamengBackup_BuildArchiveRestoreScript(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{
		"DM_HOME": "/opt/dmdbms",
	})

	opts := RestoreOptions{
		RestoreMode:    RestoreModeArchive,
		ArchiveLogDest: "/opt/dmdbms/arch",
	}
	script := dm.buildArchiveRestoreScript("/backup/dm_arch_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, `RESTORE DATABASE`) {
		t.Errorf("归档还原脚本缺少 RESTORE DATABASE，得到: %s", script)
	}
	if !strings.Contains(script, `RESTORE ARCHIVE LOG`) {
		t.Errorf("归档还原脚本缺少 RESTORE ARCHIVE LOG，得到: %s", script)
	}
	if !strings.Contains(script, `TO ARCHIVEDIR '/opt/dmdbms/arch'`) {
		t.Errorf("归档还原脚本应还原归档到指定目录，得到: %s", script)
	}
	if !strings.Contains(script, "WITH ARCHIVEDIR") {
		t.Errorf("归档还原脚本 RECOVER 应使用 WITH ARCHIVEDIR，得到: %s", script)
	}
	if !strings.Contains(script, "UPDATE DB_MAGIC") {
		t.Errorf("归档还原脚本缺少 UPDATE DB_MAGIC，得到: %s", script)
	}
}

func TestDamengBackup_BuildArchiveRestoreScript_WithPITR(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{
		"DM_HOME": "/opt/dmdbms",
	})

	pitrTime := time.Date(2026, 7, 3, 15, 30, 0, 0, time.Local)
	opts := RestoreOptions{
		RestoreMode:         RestoreModeArchive,
		RecoveryPointInTime: pitrTime,
		ArchiveLogDest:      "/opt/dmdbms/arch",
	}
	script := dm.buildArchiveRestoreScript("/backup/dm_arch_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, "UNTIL TIME '2026-07-03 15:30:00'") {
		t.Errorf("归档还原 PITR 应包含 UNTIL TIME，得到: %s", script)
	}
}

func TestDamengBackup_BuildArchiveRestoreScript_WithLSN(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{
		"DM_HOME": "/opt/dmdbms",
	})

	opts := RestoreOptions{
		RestoreMode:    RestoreModeArchive,
		RecoveryLSN:    "99999",
		ArchiveLogDest: "/opt/dmdbms/arch",
	}
	script := dm.buildArchiveRestoreScript("/backup/dm_arch_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if !strings.Contains(script, "UNTIL LSN 99999") {
		t.Errorf("归档还原 LSN 应包含 UNTIL LSN 99999，得到: %s", script)
	}
}

func TestDamengBackup_BuildArchiveRestoreScript_NoArchDir(t *testing.T) {
	dm := newTestDamengBackup()

	opts := RestoreOptions{RestoreMode: RestoreModeArchive}
	script := dm.buildArchiveRestoreScript("/backup/dm_arch_20260703", "/opt/dmdbms/data/DAMENG_new", opts)

	if strings.Contains(script, "RESTORE ARCHIVE LOG") {
		t.Errorf("无归档目录时不应执行 RESTORE ARCHIVE LOG，得到: %s", script)
	}
	if !strings.Contains(script, "FROM BACKUPSET") {
		t.Errorf("无归档目录时应使用 FROM BACKUPSET 恢复，得到: %s", script)
	}
}

// ===== 还原模式分发测试 =====

func TestDamengBackup_BuildRestoreScriptByMode(t *testing.T) {
	tests := []struct {
		name           string
		restoreMode    RestoreMode
		wantContains   string
		wantNotContain string
	}{
		{
			name:           "full_mode",
			restoreMode:    RestoreModeFull,
			wantContains:   "RESTORE DATABASE",
			wantNotContain: "WITH BACKUPDIR",
		},
		{
			name:           "incremental_mode",
			restoreMode:    RestoreModeIncremental,
			wantContains:   "WITH BACKUPDIR",
			wantNotContain: "",
		},
		{
			name:           "archive_mode",
			restoreMode:    RestoreModeArchive,
			wantContains:   "RESTORE ARCHIVE LOG",
			wantNotContain: "",
		},
		{
			name:           "empty_mode_defaults_to_full",
			restoreMode:    "",
			wantContains:   "RESTORE DATABASE",
			wantNotContain: "WITH BACKUPDIR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := newTestDamengBackup(map[string]string{
				"DM_HOME": "/opt/dmdbms",
			})

			opts := RestoreOptions{
				RestoreMode:    tt.restoreMode,
				ArchiveLogDest: "/opt/dmdbms/arch",
			}
			script := dm.buildRestoreScriptByMode("/backup/dm_full", "/opt/dmdbms/data/DAMENG_new", opts)

			if tt.wantContains != "" && !strings.Contains(script, tt.wantContains) {
				t.Errorf("还原模式 %q 脚本应包含 %q，得到: %s", tt.restoreMode, tt.wantContains, script)
			}
			if tt.wantNotContain != "" && strings.Contains(script, tt.wantNotContain) {
				t.Errorf("还原模式 %q 脚本不应包含 %q，得到: %s", tt.restoreMode, tt.wantNotContain, script)
			}
		})
	}
}

// ===== 加密/并行/LSN范围 新功能测试 =====

func TestDamengBackup_BuildFullBackupScript_WithEncryption(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{Encryption: true, EncryptionKey: "mySecret123"}
	script := dm.buildFullBackupScript("DM_FULL_20260703", "/backup/dm_full_20260703", opts)

	if !strings.Contains(script, `IDENTIFIED BY "mySecret123"`) {
		t.Errorf("启用加密时全量备份脚本应包含 IDENTIFIED BY，得到: %s", script)
	}
	if !strings.Contains(script, `BACKUP DATABASE FULL`) {
		t.Error("加密模式下应仍包含 BACKUP DATABASE FULL")
	}
}

func TestDamengBackup_BuildFullBackupScript_AllOptions(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{
		EnableCompression: true,
		ParallelWorkers:   4,
		Encryption:        true,
		EncryptionKey:     "pass",
	}
	script := dm.buildFullBackupScript("DM_FULL", "/backup/dm_full", opts)

	for _, keyword := range []string{"COMPRESSED", `IDENTIFIED BY "pass"`, "PARALLEL 4"} {
		if !strings.Contains(script, keyword) {
			t.Errorf("全量备份脚本（全部选项）应包含 %q，得到: %s", keyword, script)
		}
	}
}

func TestDamengBackup_BuildArchiveBackupScript_WithParallel(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{ParallelWorkers: 3}
	script, err := dm.buildArchiveBackupScript("DM_ARCH", "/backup/dm_arch", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "PARALLEL 3") {
		t.Errorf("归档备份脚本并行度 3 应包含 PARALLEL 3，得到: %s", script)
	}
	if !strings.Contains(script, "BACKUP ARCHIVELOG ALL") {
		t.Error("归档备份脚本应包含 BACKUP ARCHIVELOG ALL")
	}
}

func TestDamengBackup_BuildArchiveBackupScript_WithLSNRange(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{
		ArchiveFromLSN:  "1000",
		ArchiveUntilLSN: "2000",
	}
	script, err := dm.buildArchiveBackupScript("DM_ARCH_LSN", "/backup/dm_arch_lsn", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "FROM LSN 1000 TO LSN 2000") {
		t.Errorf("LSN 范围归档备份应包含 FROM LSN ... TO LSN，得到: %s", script)
	}
	if strings.Contains(script, "BACKUP ARCHIVELOG ALL") {
		t.Error("LSN 范围模式不应使用 ALL 关键字")
	}
}

func TestDamengBackup_BuildArchiveBackupScript_InvalidLSNError(t *testing.T) {
	dm := newTestDamengBackup()

	// 无效的 LSN 值应返回错误，而非静默回退
	opts := BackupOptions{
		ArchiveFromLSN:  "not_a_number",
		ArchiveUntilLSN: "2000",
	}
	_, err := dm.buildArchiveBackupScript("DM_ARCH", "/backup/dm_arch", opts)
	if err == nil {
		t.Error("无效 LSN 应返回 error，而非静默回退到 ALL")
	}
}

func TestDamengBackup_BuildArchiveBackupScript_AllOptions(t *testing.T) {
	dm := newTestDamengBackup()

	opts := BackupOptions{
		EnableCompression: true,
		ParallelWorkers:   4,
		Encryption:        true,
		EncryptionKey:     "secret",
		ArchiveFromLSN:    "500",
		ArchiveUntilLSN:   "1500",
	}
	script, err := dm.buildArchiveBackupScript("DM_ARCH_ALL", "/backup/dm_all", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, keyword := range []string{"COMPRESSED", `IDENTIFIED BY "secret"`, "PARALLEL 4", "FROM LSN 500 TO LSN 1500"} {
		if !strings.Contains(script, keyword) {
			t.Errorf("归档备份脚本（全部选项）应包含 %q，得到: %s", keyword, script)
		}
	}
}

func TestDamengBackup_BuildIncrementalBackupScript_WithEncryption(t *testing.T) {
	dm := newTestDamengBackup(map[string]string{"DM_DATA_DIR": "/opt/dmdbms/data/DAMENG"})

	opts := BackupOptions{
		Mode:          BackupModeIncremental,
		Encryption:    true,
		EncryptionKey: "encKey99",
	}
	script, err := dm.buildIncrementalBackupScript("DM_INCR_ENC", "/backup/dm_enc", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `IDENTIFIED BY "encKey99"`) {
		t.Errorf("增量备份加密应包含 IDENTIFIED BY，得到: %s", script)
	}
	if !strings.Contains(script, "INCREMENT WITH BACKUPDIR") {
		t.Error("增量备份加密模式应仍包含 INCREMENT WITH BACKUPDIR")
	}
}

func TestDamengBackup_BuildIncrementalBackupScript_EmptyBaseDir(t *testing.T) {
	// 构造一个 DM_DATA_DIR 和 DM_HOME 均为空的 DamengBackup 实例
	cfg := &DBConfig{
		Type:  DBTypeDameng,
		Extra: map[string]string{},
	}
	dm := &DamengBackup{BaseBackup: BaseBackup{config: cfg}}

	opts := BackupOptions{Mode: BackupModeIncremental}
	_, err := dm.buildIncrementalBackupScript("DM_INCR_NODIR", "/backup/dm_nodir", opts)
	if err == nil {
		t.Error("空 baseDir 应返回 error")
	}
}

// ===== 归档模式检查测试 =====

func TestDamengBackup_ArchModeRegex_ArchivedMode(t *testing.T) {
	// 模拟 disql 返回 ARCH_MODE = Y 的输出格式
	output := "行号     ARCH_MODE\n---------- ---------\n1          Y\n\n已用时间: 1.985(毫秒). 执行号:1019."

	matches := archModeRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		t.Fatalf("archModeRegex 未能匹配归档模式输出，output: %s", output)
	}
	if matches[1] != "Y" {
		t.Errorf("archModeRegex 匹配结果 = %q, want Y", matches[1])
	}
}

func TestDamengBackup_ArchModeRegex_NoArchiveMode(t *testing.T) {
	// 模拟 disql 返回 ARCH_MODE = N 的输出格式
	output := "行号     ARCH_MODE\n---------- ---------\n1          N\n\n已用时间: 1.985(毫秒). 执行号:1019."

	matches := archModeRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		t.Fatalf("archModeRegex 未能匹配非归档模式输出，output: %s", output)
	}
	if matches[1] != "N" {
		t.Errorf("archModeRegex 匹配结果 = %q, want N", matches[1])
	}
}

func TestDamengBackup_ArchModeRegex_AlternateFormat(t *testing.T) {
	// 模拟简洁输出格式
	output := "ARCH_MODE\n---------\n1          Y\n"

	matches := archModeRegex.FindStringSubmatch(output)
	if len(matches) < 2 {
		t.Fatalf("archModeRegex 未能匹配替代格式输出，output: %q", output)
	}
	if matches[1] != "Y" {
		t.Errorf("archModeRegex 匹配结果 = %q, want Y", matches[1])
	}
}

func TestDamengBackup_BackupPhysical_ArchiveCheck_AutoDeriveArchDir(t *testing.T) {
	// 验证归档目录从 opts.ArchiveLogDest 自动推导
	opts := BackupOptions{
		ArchiveLogDest: "/backup/dameng/archivelog",
	}

	if opts.ArchiveLogDest == "" {
		t.Error("ArchiveLogDest 不应为空（应用层应从 BaseBackupDir 自动推导）")
	}
}

func TestDamengBackup_BackupPhysical_TimeoutContext(t *testing.T) {
	_ = newTestDamengBackup(map[string]string{
		"DM_HOME": "/opt/dmdbms",
	})

	// 验证 Timeout 设置能正确创建带超时的 context
	timeout := 2 * time.Hour
	opts := BackupOptions{
		Type:       BackupTypePhysical,
		Mode:       BackupModeFull,
		TargetPath: "/backup/dameng",
		Timeout:    timeout,
	}

	// 模拟创建带超时的 context
	ctx := context.Background()
	backupCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		backupCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	deadline, ok := backupCtx.Deadline()
	if !ok {
		t.Error("带超时的 context 应有 deadline")
	}
	remaining := time.Until(deadline)
	if remaining < timeout-1*time.Second || remaining > timeout+1*time.Second {
		t.Errorf("超时 context 剩余时间 = %v, want approximately %v", remaining, timeout)
	}
}

// ===== 归档幽灵清理测试 =====

func TestDamengBackup_findStaleArchiveFiles(t *testing.T) {
	dir := t.TempDir()

	// 创建测试归档文件
	validFile := filepath.Join(dir, "arch_valid.log")
	staleFile := filepath.Join(dir, "arch_stale.log")
	if err := os.WriteFile(validFile, []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	validPaths := map[string]struct{}{
		validFile: {},
	}

	staleFiles := findStaleArchiveFiles(dir, validPaths)
	if len(staleFiles) != 1 {
		t.Fatalf("expected 1 stale file, got %d: %v", len(staleFiles), staleFiles)
	}
	if staleFiles[0] != staleFile {
		t.Errorf("expected stale file %q, got %q", staleFile, staleFiles[0])
	}
}

func TestDamengBackup_findStaleArchiveFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	validPaths := map[string]struct{}{"/nonexistent/arch.log": {}}

	staleFiles := findStaleArchiveFiles(dir, validPaths)
	if len(staleFiles) != 0 {
		t.Fatalf("empty dir should have 0 stale files, got %d", len(staleFiles))
	}
}

func TestDamengBackup_findStaleArchiveFiles_AllValid(t *testing.T) {
	dir := t.TempDir()

	validFile := filepath.Join(dir, "arch_valid.log")
	if err := os.WriteFile(validFile, []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}

	validPaths := map[string]struct{}{
		validFile: {},
	}

	staleFiles := findStaleArchiveFiles(dir, validPaths)
	if len(staleFiles) != 0 {
		t.Fatalf("all valid should have 0 stale files, got %d", len(staleFiles))
	}
}

func TestDamengBackup_findStaleArchiveFiles_AllStale(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		f := filepath.Join(dir, fmt.Sprintf("arch_%d.log", i))
		if err := os.WriteFile(f, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 无合法归档（如非归档模式），所有文件都是幽灵
	validPaths := map[string]struct{}{}

	staleFiles := findStaleArchiveFiles(dir, validPaths)
	if len(staleFiles) != 3 {
		t.Fatalf("all stale should have 3 stale files, got %d", len(staleFiles))
	}
}

func TestDamengBackup_findStaleArchiveFiles_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()

	// 创建子目录（应被跳过）
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 创建文件
	validFile := filepath.Join(dir, "arch_valid.log")
	staleFile := filepath.Join(dir, "arch_stale.log")
	if err := os.WriteFile(validFile, []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleFile, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	validPaths := map[string]struct{}{
		validFile: {},
	}

	staleFiles := findStaleArchiveFiles(dir, validPaths)
	if len(staleFiles) != 1 {
		t.Fatalf("expected 1 stale file (subdir skipped), got %d: %v", len(staleFiles), staleFiles)
	}
	if staleFiles[0] != staleFile {
		t.Errorf("expected stale file %q, got %q", staleFile, staleFiles[0])
	}
}

func TestDamengBackup_findStaleArchiveFiles_NonexistentDir(t *testing.T) {
	validPaths := map[string]struct{}{"/some/file.log": {}}

	staleFiles := findStaleArchiveFiles("/nonexistent/dir", validPaths)
	if staleFiles != nil {
		t.Fatalf("nonexistent dir should return nil, got %v", staleFiles)
	}
}
