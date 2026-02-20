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
func TransactionFromContext(ctx context.Context) (*sql.Tx, bool) {
	return handlers.TransactionFromContext(ctx)
}
