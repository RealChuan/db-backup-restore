package backup

import (
	"context"
	"errors"
	"testing"
)

func TestBaseBackup_ListDatabases_NotSupported(t *testing.T) {
	b := NewBaseBackup(&DBConfig{Type: DBTypeOracle})
	_, err := b.ListDatabases(context.Background())
	if err == nil {
		t.Fatal("期望返回错误，但返回了 nil")
	}

	var be *BackupError
	if !errors.As(err, &be) {
		t.Fatalf("期望 *BackupError，实际 %T", err)
	}

	if be.Type != ErrorTypeNotSupported {
		t.Errorf("Type = %v, want %v", be.Type, ErrorTypeNotSupported)
	}
	if be.Op != "ListDatabases" {
		t.Errorf("Op = %v, want ListDatabases", be.Op)
	}
	if be.DBType != DBTypeOracle {
		t.Errorf("DBType = %v, want %v", be.DBType, DBTypeOracle)
	}
}
