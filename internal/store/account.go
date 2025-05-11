package store

import (
	"context"
	"fmt"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
)

type UpsertAccountTxParams struct {
	db.UpsertAccountParams
	Digest string
	Epoch  int64
}

func (s *Store) DoUpsertAccountWithTx(
	ctx context.Context,
	qtx *db.Queries,
	arg *UpsertAccountTxParams,
) (*db.Account, error) {
	if arg.Balance < 0 || arg.Ttl.Microseconds < 0 || arg.Ttl.Days < 0 || arg.Ttl.Months < 0 {
		return nil, fmt.Errorf("expect deposit balance and ttl to be non negative but got %d and %v", arg.Balance, arg.Ttl)
	}
	account, err := qtx.UpsertAccount(ctx, arg.UpsertAccountParams)
	if err != nil {
		return nil, err
	}
	if _, err = qtx.AddDepositRecord(ctx, db.AddDepositRecordParams{
		AccountID:         account.AccountID,
		TransactionDigest: arg.Digest,
		Epoch:             arg.Epoch,
	}); err != nil {
		return nil, fmt.Errorf("AddDepositRecord failed: %v", err)
	}
	return &account, nil
}

func (s *Store) UpsertAccountTx(ctx context.Context, arg *UpsertAccountTxParams) (*db.Account, error) {
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	qtx := s.Queries.WithTx(tx)
	account, err := s.DoUpsertAccountWithTx(ctx, qtx, arg)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(context.Background()); err != nil {
		return nil, err
	}
	return account, nil
}
