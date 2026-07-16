package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileSystemBackupManager_DeleteBackup_WithLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件和关联日志文件（精确后缀匹配，无时间戳）
	backupFile := filepath.Join(tmpDir, "dameng_full_20260703.dmp")
	logFile := backupFile + ".log"
	restoreLogFile := backupFile + ".restore.log"

	if err := os.WriteFile(backupFile, []byte("backup data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("backup log"), 0o644); err != nil {
		t.Fatalf("创建日志文件失败: %v", err)
	}
	if err := os.WriteFile(restoreLogFile, []byte("restore log"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 验证备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 验证日志文件已删除
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("日志文件 .log 应被删除")
	}
	if _, err := os.Stat(restoreLogFile); !os.IsNotExist(err) {
		t.Error("还原日志文件 .restore.log 应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_WithTimestampedLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件和带时间戳后缀的日志文件（与 dexp/dimp 实际输出一致）
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710_120000.dmp")
	backupLog := backupFile + "_20260710_120000.log"
	restoreLog1 := backupFile + "_20260710_140000.restore.log"
	restoreLog2 := backupFile + "_20260711_090000.restore.log"

	if err := os.WriteFile(backupFile, []byte("backup data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(backupLog, []byte("backup log"), 0o644); err != nil {
		t.Fatalf("创建备份日志文件失败: %v", err)
	}
	if err := os.WriteFile(restoreLog1, []byte("restore log 1"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件1失败: %v", err)
	}
	if err := os.WriteFile(restoreLog2, []byte("restore log 2"), 0o644); err != nil {
		t.Fatalf("创建还原日志文件2失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 验证备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 验证带时间戳的备份日志已删除
	if _, err := os.Stat(backupLog); !os.IsNotExist(err) {
		t.Error("带时间戳的备份日志文件 .log 应被删除")
	}
	// 验证多次还原产生的日志文件都已删除
	if _, err := os.Stat(restoreLog1); !os.IsNotExist(err) {
		t.Error("带时间戳的还原日志文件1 .restore.log 应被删除")
	}
	if _, err := os.Stat(restoreLog2); !os.IsNotExist(err) {
		t.Error("带时间戳的还原日志文件2 .restore.log 应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_MixedLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 同时存在精确后缀和带时间戳的日志文件
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710.dmp")
	exactLog := backupFile + ".log"
	exactRestoreLog := backupFile + ".restore.log"
	tsLog := backupFile + "_20260710_120000.log"
	tsRestoreLog := backupFile + "_20260710_140000.restore.log"

	for _, f := range []string{backupFile, exactLog, exactRestoreLog, tsLog, tsRestoreLog} {
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("创建文件失败 %s: %v", f, err)
		}
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 所有文件都应被删除
	for _, f := range []string{backupFile, exactLog, exactRestoreLog, tsLog, tsRestoreLog} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("文件应被删除: %s", f)
		}
	}
}

func TestFileSystemBackupManager_DeleteBackup_NoLogSuffixes(t *testing.T) {
	tmpDir := t.TempDir()

	backupFile := filepath.Join(tmpDir, "test.sql")
	logFile := backupFile + ".log"

	if err := os.WriteFile(backupFile, []byte("sql data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}
	if err := os.WriteFile(logFile, []byte("log data"), 0o644); err != nil {
		t.Fatalf("创建日志文件失败: %v", err)
	}

	// 不配置 logFileSuffixes，日志文件不应被删除
	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.sql"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
	// 日志文件仍存在（未配置 logFileSuffixes）
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Error("日志文件不应被删除（未配置 logFileSuffixes）")
	}
}

func TestFileSystemBackupManager_DeleteAllBackups_WithLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建多个备份文件和关联日志文件（带时间戳后缀）
	backup1 := filepath.Join(tmpDir, "dameng_full_001.dmp")
	backup2 := filepath.Join(tmpDir, "dameng_full_002.dmp")

	for _, f := range []string{backup1, backup2} {
		if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
			t.Fatalf("创建备份文件失败: %v", err)
		}
		if err := os.WriteFile(f+"_20260710_120000.log", []byte("log"), 0o644); err != nil {
			t.Fatalf("创建日志文件失败: %v", err)
		}
		if err := os.WriteFile(f+"_20260710_140000.restore.log", []byte("rlog"), 0o644); err != nil {
			t.Fatalf("创建还原日志文件失败: %v", err)
		}
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteAllBackups(context.TODO(), tmpDir); err != nil {
		t.Fatalf("DeleteAllBackups 失败: %v", err)
	}

	// 所有备份文件和日志文件都应被删除
	for _, f := range []string{
		backup1, backup2,
		backup1 + "_20260710_120000.log", backup2 + "_20260710_120000.log",
		backup1 + "_20260710_140000.restore.log", backup2 + "_20260710_140000.restore.log",
	} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("文件应被删除: %s", f)
		}
	}
}

func TestFileSystemBackupManager_DeleteBackup_LogFileNotExist_NoError(t *testing.T) {
	tmpDir := t.TempDir()

	// 只创建备份文件，不创建日志文件
	backupFile := filepath.Join(tmpDir, "dameng_full.dmp")
	if err := os.WriteFile(backupFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	// 日志文件不存在时不应报错
	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 不应因缺少日志文件而报错: %v", err)
	}

	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_GlobNotMatchingUnrelatedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建备份文件
	backupFile := filepath.Join(tmpDir, "dameng_full_20260710.dmp")
	if err := os.WriteFile(backupFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	// 创建另一个备份的日志文件（不应被删除）
	unrelatedBackup := filepath.Join(tmpDir, "dameng_full_20260711.dmp")
	unrelatedLog := unrelatedBackup + "_20260711_120000.log"
	if err := os.WriteFile(unrelatedLog, []byte("unrelated log"), 0o644); err != nil {
		t.Fatalf("创建无关日志文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir,
		WithLogicalGlob("*.dmp"),
		WithLogFileSuffixes(".log", ".restore.log"))

	if err := mgr.DeleteBackup(context.TODO(), filepath.Base(backupFile), tmpDir); err != nil {
		t.Fatalf("DeleteBackup 失败: %v", err)
	}

	// 备份文件已删除
	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}

	// 无关日志文件不应被删除
	if _, err := os.Stat(unrelatedLog); os.IsNotExist(err) {
		t.Error("无关备份的日志文件不应被删除")
	}
}

func TestFileSystemBackupManager_ListBackups(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	createMixedBackups(t, tmpDir)

	mgr := NewFileSystemBackupManager(tmpDir)
	backups, err := mgr.ListBackups(context.TODO(), tmpDir)
	if err != nil {
		t.Fatalf("ListBackups 失败: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("期望 3 个备份, 得到 %d", len(backups))
	}

	// 验证按 BackupID 排序
	if backups[0].BackupID != "db_backup_001.sql" {
		t.Errorf("第一个备份 ID = %q, 期望 db_backup_001.sql", backups[0].BackupID)
	}

	t.Run("逻辑备份", func(t *testing.T) {
		t.Parallel()
		var found bool
		for _, b := range backups {
			if b.BackupID == "db_backup_001.sql" {
				found = true
				if b.BackupType != string(BackupTypeLogical) {
					t.Errorf("备份类型 = %q, 期望 %q", b.BackupType, BackupTypeLogical)
				}
				if b.Size != int64(len("sql data")) {
					t.Errorf("大小 = %d, 期望 %d", b.Size, len("sql data"))
				}
			}
		}
		if !found {
			t.Error("未找到逻辑备份 db_backup_001.sql")
		}
	})

	t.Run("物理备份", func(t *testing.T) {
		t.Parallel()
		var found bool
		for _, b := range backups {
			if b.BackupID == "db_physical" {
				found = true
				if b.BackupType != string(BackupTypePhysical) {
					t.Errorf("备份类型 = %q, 期望 %q", b.BackupType, BackupTypePhysical)
				}
			}
		}
		if !found {
			t.Error("未找到物理备份 db_physical")
		}
	})
}

// createMixedBackups 在 dir 下创建逻辑备份文件和物理备份目录
func createMixedBackups(t *testing.T, dir string) {
	t.Helper()

	// 逻辑备份文件
	logical1 := filepath.Join(dir, "db_backup_001.sql")
	logical2 := filepath.Join(dir, "db_backup_002.sql.gz")
	if err := os.WriteFile(logical1, []byte("sql data"), 0o644); err != nil {
		t.Fatalf("创建逻辑备份文件失败: %v", err)
	}
	if err := os.WriteFile(logical2, []byte("gz data"), 0o644); err != nil {
		t.Fatalf("创建逻辑备份文件失败: %v", err)
	}

	// 物理备份目录
	physicalDir := filepath.Join(dir, "db_physical")
	if err := os.MkdirAll(physicalDir, 0o755); err != nil {
		t.Fatalf("创建物理备份目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(physicalDir, "data.bin"), []byte("physical"), 0o644); err != nil {
		t.Fatalf("创建物理备份文件失败: %v", err)
	}
}

func TestFileSystemBackupManager_ListBackups_EmptyDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mgr := NewFileSystemBackupManager(tmpDir)

	backups, err := mgr.ListBackups(context.TODO(), tmpDir)
	if err != nil {
		t.Fatalf("ListBackups 空目录不应报错: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("空目录应返回 0 个备份, 得到 %d", len(backups))
	}
}

func TestFileSystemBackupManager_ListBackups_NoBackupDir(t *testing.T) {
	t.Parallel()

	mgr := NewFileSystemBackupManager("")

	backups, err := mgr.ListBackups(context.TODO(), "")
	if err == nil {
		t.Fatal("未指定备份目录时应返回错误")
	}
	if backups != nil {
		t.Errorf("错误时应返回 nil, 得到 %v", backups)
	}
}

func TestFileSystemBackupManager_GetBackupInfo_LogicalFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	backupFile := filepath.Join(tmpDir, "test_backup.sql")
	if err := os.WriteFile(backupFile, []byte("backup content"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir)

	info, err := mgr.GetBackupInfo(context.TODO(), filepath.Base(backupFile), tmpDir)
	if err != nil {
		t.Fatalf("GetBackupInfo 失败: %v", err)
	}

	if info["path"] != backupFile {
		t.Errorf("path = %q, 期望 %q", info["path"], backupFile)
	}
	if info["size"] != "14" {
		t.Errorf("size = %q, 期望 14", info["size"])
	}
	if info["backup_type"] != string(BackupTypeLogical) {
		t.Errorf("backup_type = %q, 期望 %q", info["backup_type"], BackupTypeLogical)
	}
	if info["mod_time"] == "" {
		t.Error("mod_time 不应为空")
	}
}

func TestFileSystemBackupManager_GetBackupInfo_PhysicalDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	physicalDir := filepath.Join(tmpDir, "db_physical")
	if err := os.MkdirAll(physicalDir, 0o755); err != nil {
		t.Fatalf("创建物理备份目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(physicalDir, "data.bin"), []byte("physical data"), 0o644); err != nil {
		t.Fatalf("创建物理备份文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir)

	info, err := mgr.GetBackupInfo(context.TODO(), "db_physical", tmpDir)
	if err != nil {
		t.Fatalf("GetBackupInfo 失败: %v", err)
	}

	if info["backup_type"] != string(BackupTypePhysical) {
		t.Errorf("backup_type = %q, 期望 %q", info["backup_type"], BackupTypePhysical)
	}
	if info["size"] == "" {
		t.Error("物理备份 size 不应为空")
	}
}

func TestFileSystemBackupManager_GetBackupInfo_NotExist(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mgr := NewFileSystemBackupManager(tmpDir)

	_, err := mgr.GetBackupInfo(context.TODO(), "nonexistent.sql", tmpDir)
	if err == nil {
		t.Fatal("不存在的备份应返回错误")
	}
}

func TestFileSystemBackupManager_GetBackupInfo_EmptyID(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	mgr := NewFileSystemBackupManager(tmpDir)

	_, err := mgr.GetBackupInfo(context.TODO(), "", tmpDir)
	if err == nil {
		t.Fatal("空 backupID 应返回错误")
	}
}

func TestFileSystemBackupManager_DeleteBackup_AbsolutePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	backupFile := filepath.Join(tmpDir, "abs_backup.sql")
	if err := os.WriteFile(backupFile, []byte("data"), 0o644); err != nil {
		t.Fatalf("创建备份文件失败: %v", err)
	}

	mgr := NewFileSystemBackupManager(tmpDir)

	// 使用绝对路径删除
	if err := mgr.DeleteBackup(context.TODO(), backupFile, ""); err != nil {
		t.Fatalf("DeleteBackup 绝对路径失败: %v", err)
	}

	if _, err := os.Stat(backupFile); !os.IsNotExist(err) {
		t.Error("备份文件应被删除")
	}
}

func TestFileSystemBackupManager_DeleteBackup_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// 创建一个在 tmpDir 之外的文件
	outsideFile := filepath.Join(filepath.Dir(tmpDir), "outside_target.sql")
	if err := os.WriteFile(outsideFile, []byte("sensitive"), 0o644); err != nil {
		t.Fatalf("创建外部文件失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outsideFile) })

	mgr := NewFileSystemBackupManager(tmpDir)

	// 尝试用路径遍历删除 tmpDir 之外的文件
	identifier := filepath.Join("..", "outside_target.sql")
	err := mgr.DeleteBackup(context.TODO(), identifier, tmpDir)
	if err == nil {
		t.Fatal("路径遍历攻击应被拒绝")
	}

	// 文件不应被删除
	if _, err := os.Stat(outsideFile); os.IsNotExist(err) {
		t.Error("外部文件不应被路径遍历删除")
	}
}
