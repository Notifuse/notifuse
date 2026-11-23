package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Notifuse/notifuse/internal/domain"
)

// systemTxRunner runs functions in transactions on the system database.
type systemTxRunner struct {
	systemDB *sql.DB
}

// NewSystemTxRunner returns the domain.SystemTxRunner of the system database.
func NewSystemTxRunner(db *sql.DB) domain.SystemTxRunner {
	return &systemTxRunner{systemDB: db}
}

// WithSystemTx runs fn in one READ COMMITTED transaction. See domain.SystemTxRunner for
// when it commits: nil and CommitAndFail commit, anything else rolls back, and a failed
// commit is reported in place of the business error.
func (r *systemTxRunner) WithSystemTx(ctx context.Context, fn func(tx domain.DBExecutor) error) (err error) {
	tx, err := r.systemDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && err == nil {
				err = fmt.Errorf("failed to roll back transaction: %w", rollbackErr)
			}
		}
	}()

	fnErr := fn(tx)
	business, commitAnyway := domain.AsCommitAndFail(fnErr)
	if fnErr != nil && !commitAnyway {
		return fnErr
	}
	if err := tx.Commit(); err != nil {
		committed = true // Commit ends the transaction whatever it returns.
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	if commitAnyway {
		return business
	}
	return nil
}
