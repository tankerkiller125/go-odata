package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/nlstn/go-odata/internal/trackchanges"
	"gorm.io/gorm"
)

type changeEvent struct {
	entity     interface{}
	changeType trackchanges.ChangeType
}

type pendingChangeEvent struct {
	handler *EntityHandler
	event   changeEvent
}

// transactionHandledError indicates the transaction already wrote an HTTP response
// and should simply be rolled back without additional error handling.
type transactionHandledError struct {
	err error
}

func (e *transactionHandledError) Error() string {
	if e == nil {
		return "transaction handled"
	}
	if e.err == nil {
		return "transaction handled"
	}
	return e.err.Error()
}

// newTransactionHandledError wraps an error indicating the response has been handled.
func newTransactionHandledError(err error) error {
	return &transactionHandledError{err: err}
}

// isTransactionHandled reports whether the error indicates the HTTP response was handled.
func isTransactionHandled(err error) bool {
	if err == nil {
		return false
	}
	var target *transactionHandledError
	return errors.As(err, &target)
}

func (h *EntityHandler) runInTransaction(ctx context.Context, r *http.Request, fn func(tx *sql.Tx, gormTx *gorm.DB, hookReq *http.Request) error) error {
	// Check if we're already in a transaction context
	if ctxTx, ok := TransactionFromContext(ctx); ok && ctxTx != nil {
		// We're in an existing transaction - use GORM with that context
		gormTx := h.db.WithContext(ctx)
		return fn(ctxTx, gormTx, requestWithTransaction(r, ctxTx))
	}

	// Start a new GORM transaction and extract the underlying *sql.Tx
	return h.db.WithContext(ctx).Transaction(func(gormTx *gorm.DB) error {
		// Extract the underlying *sql.Tx from GORM's transaction
		var sqlTx *sql.Tx
		if gormTx.Statement != nil && gormTx.Statement.ConnPool != nil {
			if tx, ok := gormTx.Statement.ConnPool.(*sql.Tx); ok {
				sqlTx = tx
			}
		}

		// If we couldn't extract the *sql.Tx, this is an error condition
		if sqlTx == nil {
			return errors.New("failed to extract *sql.Tx from GORM transaction")
		}

		// Pass both the *sql.Tx (for hooks) and *gorm.DB (for internal operations)
		return fn(sqlTx, gormTx, requestWithTransaction(r, sqlTx))
	})
}

func flushPendingChangeEvents(events []pendingChangeEvent) {
	for _, evt := range events {
		if evt.handler == nil {
			continue
		}
		evt.handler.recordChange(evt.event.entity, evt.event.changeType)
	}
}
