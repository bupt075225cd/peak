package domain

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm/logger"
)

func TestOpenDBSQLite(t *testing.T) {
	db, err := OpenDB(DialectSQLite, filepath.Join(t.TempDir(), "t.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
}

func TestOpenDBUnsupported(t *testing.T) {
	if _, err := OpenDB("oracle", "dsn", logger.Silent); err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
}

func TestMigrate(t *testing.T) {
	db, err := OpenDB(DialectSQLite, filepath.Join(t.TempDir(), "m.db"), logger.Silent)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 迁移后应能创建一条 User。
	if err := db.Create(&User{Account: "u1", Name: "n1"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func TestDBDialectValues(t *testing.T) {
	if DialectMySQL != "mysql" || DialectPostgres != "postgres" || DialectSQLite != "sqlite" {
		t.Fatal("unexpected dialect constants")
	}
}
