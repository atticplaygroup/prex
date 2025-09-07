package store

import (
	"context"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
)

type IStore interface {
	db.Querier // Querier gives access sqlc-generated methods
	UpsertAccountTx(ctx context.Context, arg UpsertAccountTxParams) (*int64, error)
	WithdrawTx(ctx context.Context, arg WithdrawTxParams) (*int64, error)
	CancelWithdrawTx(ctx context.Context, arg CancelWithdrawTxParams) error
}

type Store struct {
	*db.Queries // implements Querier
	db          *pgx.Conn
}

func (s *Store) GetConn() *pgx.Conn {
	return s.db
}

func NewStore(pgdb *pgx.Conn) *Store {
	return &Store{
		db:      pgdb,
		Queries: db.New(pgdb),
	}
}
