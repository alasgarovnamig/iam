// Package postgres implements the repository ports on PostgreSQL (pgx).
// Every unit of work runs inside a transaction whose Row-Level Security
// context is set via SET LOCAL, so tenant isolation is enforced by the
// database in addition to application filters.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sr-alasgarov/wallet-gateway/internal/ports"
)

type txKey struct{}

// TxManager implements ports.TxManager on a pgx pool.
type TxManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

func (m *TxManager) WithTx(ctx context.Context, dbc ports.DBContext, fn func(ctx context.Context) error) error {
	// Nested call: reuse the existing transaction (same RLS context).
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if dbc.Platform {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.is_platform', 'true', true)`); err != nil {
			return fmt.Errorf("set rls context: %w", err)
		}
	} else {
		if dbc.TenantID == "" {
			return errors.New("tenant transaction requires a tenant id")
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, dbc.TenantID); err != nil {
			return fmt.Errorf("set rls context: %w", err)
		}
	}
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// queryer returns the transaction bound to ctx. Repositories refuse to run
// outside a transaction: that would bypass the RLS context.
func queryer(ctx context.Context) (pgx.Tx, error) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return nil, errors.New("no transaction in context; use TxManager.WithTx")
	}
	return tx, nil
}
