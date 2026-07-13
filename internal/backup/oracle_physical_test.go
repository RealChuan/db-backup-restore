package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestOracleBackup(t *testing.T) *OracleBackup {
	t.Helper()
	cfg := &DBConfig{
		Type:     DBTypeOracle,
		Host:     "localhost",
		Port:     1521,
		User:     "sys",
		Password: "test123",
		Database: "ORCL",
		Extra:    map[string]string{"ORACLE_HOME": "/opt/oracle/product/19c/dbhome_1", "ORACLE_SID": "ORCL"},
	}
	o, err := NewOracleBackup(cfg)
	if err != nil {
		t.Fatalf("NewOracleBackup failed: %v", err)
	}
	return o
}

// testAbsBackupDir 返回跨平台的绝对备份路径，用于测试
func testAbsBackupDir() string {
	// 使用 os.TempDir 作为前缀保证路径在所有平台上均为绝对路径
	return filepath.Join(os.TempDir(), "backup", "oracle")
}

func TestOracleBackup_BuildFullRestoreScript_Default(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{RestoreMode: RestoreModeFull}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SHUTDOWN IMMEDIATE") {
		t.Error("full restore script missing SHUTDOWN IMMEDIATE")
	}
	if !strings.Contains(script, "STARTUP MOUNT") {
		t.Error("full restore script missing STARTUP MOUNT")
	}
	if !strings.Contains(script, "RESTORE DATABASE;") {
		t.Error("full restore script missing RESTORE DATABASE")
	}
	if !strings.Contains(script, "RECOVER DATABASE;") {
		t.Error("full restore script missing RECOVER DATABASE")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN;") {
		t.Error("default full restore should open without RESETLOGS")
	}
	if strings.Contains(script, "RESETLOGS") {
		t.Error("default full restore should NOT contain RESETLOGS")
	}
}

func TestOracleBackup_BuildFullRestoreScript_WithTAG(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode:      RestoreModeFull,
		BackupIdentifier: "TAG20260703T120000",
	}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE DATABASE FROM TAG='TAG20260703T120000'") {
		t.Errorf("tag-based restore script missing RESTORE DATABASE FROM TAG, got: %s", script)
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN;") {
		t.Error("tag-based restore without PITR should open without RESETLOGS")
	}
}

func TestOracleBackup_BuildFullRestoreScript_WithPITR(t *testing.T) {
	o := newTestOracleBackup(t)
	pitrTime := time.Date(2026, 7, 3, 10, 30, 0, 0, time.Local)
	opts := RestoreOptions{
		RestoreMode:         RestoreModeFull,
		RecoveryPointInTime: pitrTime,
	}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SET UNTIL TIME") {
		t.Error("PITR restore should contain SET UNTIL TIME")
	}
	if !strings.Contains(script, "2026-07-03 10:30:00") {
		t.Error("PITR restore should contain formatted time")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("PITR restore should use RESETLOGS")
	}
}

func TestOracleBackup_BuildFullRestoreScript_WithSCN(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode: RestoreModeFull,
		RecoverySCN: "123456789",
	}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SET UNTIL SCN 123456789") {
		t.Errorf("SCN restore should contain SET UNTIL SCN, got: %s", script)
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("SCN restore should use RESETLOGS")
	}
}

func TestOracleBackup_BuildFullRestoreScript_InvalidSCN(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode: RestoreModeFull,
		RecoverySCN: "not_a_number",
	}
	_, err := o.buildFullRestoreScript(opts)
	if err == nil {
		t.Error("invalid SCN should return error")
	}
}

func TestOracleBackup_BuildIncrementalRestoreScript_Default(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{RestoreMode: RestoreModeIncremental}
	script, err := o.buildIncrementalRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE DATABASE;") {
		t.Error("incremental restore script missing RESTORE DATABASE")
	}
	if !strings.Contains(script, "RECOVER DATABASE;") {
		t.Error("incremental restore without NoRedo should use RECOVER DATABASE")
	}
	if strings.Contains(script, "NOREDO") {
		t.Error("incremental restore without NoRedo should NOT contain NOREDO")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("incremental restore should use RESETLOGS")
	}
}

func TestOracleBackup_BuildIncrementalRestoreScript_NoRedo(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode: RestoreModeIncremental,
		NoRedo:      true,
	}
	script, err := o.buildIncrementalRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RECOVER DATABASE NOREDO") {
		t.Errorf("incremental restore with NoRedo should contain NOREDO, got: %s", script)
	}
}

func TestOracleBackup_BuildIncrementalRestoreScript_WithSCN(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode: RestoreModeIncremental,
		RecoverySCN: "999888",
	}
	script, err := o.buildIncrementalRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SET UNTIL SCN 999888") {
		t.Errorf("incremental restore with SCN should contain SET UNTIL SCN, got: %s", script)
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_Default(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{RestoreMode: RestoreModeArchive}
	script, err := o.buildArchiveRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE DATABASE;") {
		t.Error("archive restore script missing RESTORE DATABASE")
	}
	if !strings.Contains(script, "RESTORE ARCHIVELOG ALL") {
		t.Error("default archive restore should use RESTORE ARCHIVELOG ALL")
	}
	if !strings.Contains(script, "RECOVER DATABASE;") {
		t.Error("archive restore script missing RECOVER DATABASE")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("archive restore should use RESETLOGS")
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_WithSCN(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode: RestoreModeArchive,
		RecoverySCN: "555666",
	}
	script, err := o.buildArchiveRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SET UNTIL SCN 555666") {
		t.Errorf("archive restore with SCN should contain SET UNTIL SCN, got: %s", script)
	}
	if !strings.Contains(script, "RESTORE ARCHIVELOG ALL") {
		t.Error("archive restore with SCN should still restore all archivelogs")
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_WithSeqRange(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode:     RestoreModeArchive,
		ArchiveFromSeq:  "100",
		ArchiveUntilSeq: "200",
	}
	script, err := o.buildArchiveRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE ARCHIVELOG FROM SEQUENCE 100 UNTIL SEQUENCE 200") {
		t.Errorf("archive restore with seq range should contain FROM/UNTIL SEQUENCE, got: %s", script)
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_WithFromSeqOnly(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode:    RestoreModeArchive,
		ArchiveFromSeq: "50",
	}
	script, err := o.buildArchiveRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE ARCHIVELOG FROM SEQUENCE 50") {
		t.Errorf("archive restore with from-seq should contain FROM SEQUENCE, got: %s", script)
	}
	if strings.Contains(script, "UNTIL SEQUENCE") {
		t.Error("archive restore with only from-seq should NOT contain UNTIL SEQUENCE")
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_InvalidSeq(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode:     RestoreModeArchive,
		ArchiveFromSeq:  "not_a_number",
		ArchiveUntilSeq: "200",
	}
	_, err := o.buildArchiveRestoreScript(opts)
	if err == nil {
		t.Error("invalid sequence number should return error")
	}
}

func TestOracleBackup_BuildArchiveRestoreScript_WithTAG(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := RestoreOptions{
		RestoreMode:      RestoreModeArchive,
		BackupIdentifier: "TAG20260703T120000",
	}
	script, err := o.buildArchiveRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "RESTORE DATABASE FROM TAG='TAG20260703T120000'") {
		t.Error("archive restore with TAG should contain RESTORE DATABASE FROM TAG")
	}
}

func TestOracleBackup_BuildBackupScript_FullMode(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeFull}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "BACKUP DATABASE PLUS ARCHIVELOG") {
		t.Errorf("full backup script missing BACKUP DATABASE PLUS ARCHIVELOG, got: %s", script)
	}
}

func TestOracleBackup_BuildBackupScript_IncrementalMode(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeIncremental}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "BACKUP INCREMENTAL LEVEL 1 DATABASE") {
		t.Errorf("incremental backup script missing INCREMENTAL LEVEL 1, got: %s", script)
	}
	if strings.Contains(script, "CUMULATIVE") {
		t.Error("incremental mode should NOT contain CUMULATIVE")
	}
}

func TestOracleBackup_BuildBackupScript_DifferentialMode(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeDifferential}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "CUMULATIVE") {
		t.Errorf("differential backup script should contain CUMULATIVE, got: %s", script)
	}
}

func TestOracleBackup_BuildBackupScript_WithCompression(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{EnableCompression: true}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "CONFIGURE COMPRESSION ALGORITHM 'MEDIUM'") {
		t.Errorf("compressed backup script missing COMPRESSION, got: %s", script)
	}
}

func TestOracleBackup_BuildBackupScript_InvalidPath(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeFull}
	_, err := o.buildBackupScript(opts, "")
	if err == nil {
		t.Error("empty backup path should return error")
	}
}

func TestOracleBackup_RestoreDispatch(t *testing.T) {
	o := newTestOracleBackup(t)

	tests := []struct {
		name        string
		opts        RestoreOptions
		wantContain string
		wantAbsent  string
	}{
		{
			name:        "full mode",
			opts:        RestoreOptions{RestoreMode: RestoreModeFull},
			wantContain: "RECOVER DATABASE;",
			wantAbsent:  "NOREDO",
		},
		{
			name:        "incremental mode",
			opts:        RestoreOptions{RestoreMode: RestoreModeIncremental},
			wantContain: "RECOVER DATABASE;",
			wantAbsent:  "NOREDO",
		},
		{
			name:        "incremental mode with noredo",
			opts:        RestoreOptions{RestoreMode: RestoreModeIncremental, NoRedo: true},
			wantContain: "RECOVER DATABASE NOREDO",
			wantAbsent:  "",
		},
		{
			name:        "archive mode",
			opts:        RestoreOptions{RestoreMode: RestoreModeArchive},
			wantContain: "RESTORE ARCHIVELOG ALL",
			wantAbsent:  "NOREDO",
		},
		{
			name:        "controlfile mode",
			opts:        RestoreOptions{RestoreMode: RestoreModeControlFile},
			wantContain: "RESTORE CONTROLFILE FROM AUTOBACKUP",
			wantAbsent:  "SHUTDOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var script string
			var err error
			switch tt.opts.RestoreMode {
			case RestoreModeFull:
				script, err = o.buildFullRestoreScript(tt.opts)
			case RestoreModeIncremental:
				script, err = o.buildIncrementalRestoreScript(tt.opts)
			case RestoreModeArchive:
				script, err = o.buildArchiveRestoreScript(tt.opts)
			case RestoreModeControlFile:
				script, err = o.buildControlFileRestoreScript(tt.opts)
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantContain != "" && !strings.Contains(script, tt.wantContain) {
				t.Errorf("expected script to contain %q, got: %s", tt.wantContain, script)
			}
			if tt.wantAbsent != "" && strings.Contains(script, tt.wantAbsent) {
				t.Errorf("expected script NOT to contain %q, got: %s", tt.wantAbsent, script)
			}
		})
	}
}

// ===== Level0/Archive/TAG+PITR 新功能测试 =====

func TestOracleBackup_BuildBackupScript_Level0Mode(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeLevel0}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "BACKUP INCREMENTAL LEVEL 0 DATABASE") {
		t.Errorf("Level 0 备份脚本应包含 BACKUP INCREMENTAL LEVEL 0 DATABASE，got: %s", script)
	}
	if strings.Contains(script, "LEVEL 1") {
		t.Error("Level 0 备份脚本不应包含 LEVEL 1")
	}
}

func TestOracleBackup_BuildBackupScript_ArchiveMode(t *testing.T) {
	o := newTestOracleBackup(t)
	opts := BackupOptions{Mode: BackupModeArchive}
	script, err := o.buildBackupScript(opts, testAbsBackupDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "BACKUP ARCHIVELOG ALL") {
		t.Errorf("归档备份脚本应包含 BACKUP ARCHIVELOG ALL，got: %s", script)
	}
	// 独立归档备份不应包含数据文件备份
	if strings.Contains(script, "BACKUP DATABASE") {
		t.Error("归档备份脚本不应包含 BACKUP DATABASE")
	}
	if strings.Contains(script, "BACKUP CURRENT CONTROLFILE") {
		t.Error("归档备份脚本不应包含控制文件备份")
	}
}

func TestOracleBackup_BuildFullRestoreScript_TAGWithPITR(t *testing.T) {
	o := newTestOracleBackup(t)

	pitrTime := time.Date(2026, 7, 3, 10, 30, 0, 0, time.Local)
	opts := RestoreOptions{
		RestoreMode:         RestoreModeFull,
		BackupIdentifier:    "TAG20260703T120000",
		RecoveryPointInTime: pitrTime,
	}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `RESTORE DATABASE FROM TAG='TAG20260703T120000'`) {
		t.Errorf("TAG+PITR 组合还原应包含 RESTORE DATABASE FROM TAG，got: %s", script)
	}
	if !strings.Contains(script, "SET UNTIL TIME") {
		t.Error("TAG+PITR 组合还原应包含 SET UNTIL TIME")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("TAG+PITR 组合还原应使用 RESETLOGS")
	}
}

func TestOracleBackup_BuildFullRestoreScript_TAGWithSCN(t *testing.T) {
	o := newTestOracleBackup(t)

	opts := RestoreOptions{
		RestoreMode:      RestoreModeFull,
		BackupIdentifier: "TAG20260703T120000",
		RecoverySCN:      "123456789",
	}
	script, err := o.buildFullRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `RESTORE DATABASE FROM TAG='TAG20260703T120000'`) {
		t.Errorf("TAG+SCN 组合还原应包含 RESTORE DATABASE FROM TAG，got: %s", script)
	}
	if !strings.Contains(script, "SET UNTIL SCN 123456789") {
		t.Error("TAG+SCN 组合还原应包含 SET UNTIL SCN")
	}
}

func TestOracleBackup_BuildControlFileRestoreScript(t *testing.T) {
	o := newTestOracleBackup(t)

	opts := RestoreOptions{RestoreMode: RestoreModeControlFile}
	script, err := o.buildControlFileRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "STARTUP NOMOUNT") {
		t.Error("控制文件还原应使用 STARTUP NOMOUNT（而非 SHUTDOWN + STARTUP MOUNT）")
	}
	if !strings.Contains(script, "RESTORE CONTROLFILE FROM AUTOBACKUP") {
		t.Error("控制文件还原应包含 RESTORE CONTROLFILE FROM AUTOBACKUP")
	}
	if !strings.Contains(script, "ALTER DATABASE MOUNT") {
		t.Error("控制文件还原应包含 ALTER DATABASE MOUNT")
	}
	if !strings.Contains(script, "RESTORE DATABASE") {
		t.Error("控制文件还原应包含后续的 RESTORE DATABASE")
	}
	if !strings.Contains(script, "ALTER DATABASE OPEN RESETLOGS") {
		t.Error("控制文件还原应使用 RESETLOGS 打开数据库")
	}
}

func TestOracleBackup_BuildControlFileRestoreScript_WithTAG(t *testing.T) {
	o := newTestOracleBackup(t)

	opts := RestoreOptions{
		RestoreMode:      RestoreModeControlFile,
		BackupIdentifier: "TAG20260703T120000",
	}
	script, err := o.buildControlFileRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, `RESTORE CONTROLFILE FROM TAG='TAG20260703T120000'`) {
		t.Errorf("指定 TAG 的控制文件还原应包含 RESTORE CONTROLFILE FROM TAG，got: %s", script)
	}
	if strings.Contains(script, "RESTORE CONTROLFILE FROM AUTOBACKUP") {
		t.Error("指定 TAG 时不应使用 AUTOBACKUP")
	}
}

func TestOracleBackup_BuildControlFileRestoreScript_WithPITR(t *testing.T) {
	o := newTestOracleBackup(t)

	pitrTime := time.Date(2026, 7, 3, 10, 30, 0, 0, time.Local)
	opts := RestoreOptions{
		RestoreMode:         RestoreModeControlFile,
		RecoveryPointInTime: pitrTime,
	}
	script, err := o.buildControlFileRestoreScript(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(script, "SET UNTIL TIME") {
		t.Error("控制文件还原 + PITR 应包含 SET UNTIL TIME")
	}
}

func TestOracleBackup_BuildControlFileRestoreScript_InvalidSCN(t *testing.T) {
	o := newTestOracleBackup(t)

	opts := RestoreOptions{
		RestoreMode: RestoreModeControlFile,
		RecoverySCN: "invalid",
	}
	_, err := o.buildControlFileRestoreScript(opts)
	if err == nil {
		t.Error("invalid SCN in controlfile restore should return error")
	}
}

func TestOracleBackup_CrosscheckAndCleanup_ScriptContent(t *testing.T) {
	o := newTestOracleBackup(t)
	script := o.buildCrosscheckCleanupScript()

	if !strings.Contains(script, "CROSSCHECK BACKUP;") {
		t.Error("幽灵清理脚本应包含 CROSSCHECK BACKUP")
	}
	if !strings.Contains(script, "CROSSCHECK ARCHIVELOG ALL;") {
		t.Error("幽灵清理脚本应包含 CROSSCHECK ARCHIVELOG ALL")
	}
	if !strings.Contains(script, "DELETE NOPROMPT EXPIRED BACKUP;") {
		t.Error("幽灵清理脚本应包含 DELETE NOPROMPT EXPIRED BACKUP")
	}
	if !strings.Contains(script, "DELETE NOPROMPT EXPIRED ARCHIVELOG ALL;") {
		t.Error("幽灵清理脚本应包含 DELETE NOPROMPT EXPIRED ARCHIVELOG ALL")
	}
	if !strings.Contains(script, "DELETE NOPROMPT OBSOLETE;") {
		t.Error("幽灵清理脚本应包含 DELETE NOPROMPT OBSOLETE")
	}
}
