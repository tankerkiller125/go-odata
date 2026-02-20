package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "github.com/mattn/go-sqlite3"
)

type transactionHookEntity struct {
	ObservedTx *sql.Tx
	ObservedOK bool
}

func (e *transactionHookEntity) ODataBeforeCreate(ctx context.Context, _ *http.Request) error {
	e.ObservedTx, e.ObservedOK = TransactionFromContext(ctx)
	return nil
}

func TestCallHookIncludesTransactionInContext(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	req := httptest.NewRequest("POST", "/entities", nil)
	entity := &transactionHookEntity{}

	if err := gormDB.Transaction(func(gormTx *gorm.DB) error {
		// Extract *sql.Tx from GORM transaction
		var sqlTx *sql.Tx
		if gormTx.Statement != nil && gormTx.Statement.ConnPool != nil {
			if tx, ok := gormTx.Statement.ConnPool.(*sql.Tx); ok {
				sqlTx = tx
			}
		}
		if sqlTx == nil {
			t.Fatal("Failed to extract *sql.Tx from GORM transaction")
		}
		
		hookReq := requestWithTransaction(req, sqlTx)
		if err := callHook(entity, "ODataBeforeCreate", hookReq); err != nil {
			return err
		}
		if !entity.ObservedOK {
			t.Fatalf("transaction was not available to hook")
		}
		if entity.ObservedTx != sqlTx {
			t.Fatalf("hook received unexpected transaction pointer")
		}
		return nil
	}); err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
}
