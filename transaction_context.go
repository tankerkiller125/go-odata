package odata

import (
	"context"
	"database/sql"

	"github.com/nlstn/go-odata/internal/handlers"
)

// TransactionFromContext returns the active *sql.Tx transaction stored for hook execution.
// Hooks invoked by the entity and collection write handlers can opt into the shared
// transaction by calling this helper with the context they receive.
//
// During the migration from GORM to database/sql, this function signature has changed
// from returning (*gorm.DB, bool) to (*sql.Tx, bool). Update your hook implementations
// to use the standard database/sql transaction API.
//
// TEMPORARY: During Phase 1, the internal implementation still uses GORM transactions,
// but the public API exposes *sql.Tx. In Phase 2, the internal implementation will
// be fully migrated to database/sql.
func TransactionFromContext(ctx context.Context) (*sql.Tx, bool) {
	// During migration, extract GORM DB and convert to sql.Tx
	gormDB, ok := handlers.TransactionFromContext(ctx)
	if !ok {
		return nil, false
	}

	// Extract the underlying sql.Tx from GORM's Statement
	// This is a temporary bridge during migration
	if gormDB.Statement != nil && gormDB.Statement.ConnPool != nil {
		if tx, ok := gormDB.Statement.ConnPool.(*sql.Tx); ok {
			return tx, true
		}
	}

	// If we can't extract the sql.Tx, return nil
	// This shouldn't happen in normal operation but provides a safe fallback
	return nil, false
}
