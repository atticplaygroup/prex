package store

import (
	"context"
	"fmt"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
)

type WithdrawTxParams struct {
	db.StartWithdrawalParams
	WithdrawAll bool
}

// Account is not deleted even if all balance is withdrawn. Account is only deleted when expire_time is reached
func (s *Store) WithdrawTx(ctx context.Context, arg WithdrawTxParams) (*db.Withdrawal, error) {
	if arg.PriorityFee < 0 {
		return nil, fmt.Errorf(
			"expect priority fee to be non negative but got %d", arg.PriorityFee)
	}
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	qtx := s.Queries.WithTx(tx)
	if arg.WithdrawAll {
		if account, err := qtx.QueryBalanceForShare(ctx, arg.AccountID); err != nil {
			return nil, err
		} else {
			arg.Amount = account.Balance - arg.PriorityFee
		}
	}
	if arg.Amount <= 0 {
		return nil, fmt.Errorf(
			"expect withdraw amount to be positive but got %d", arg.Amount,
		)
	}
	withdraw, err := qtx.StartWithdrawal(ctx, arg.StartWithdrawalParams)
	if err != nil {
		return nil, err
	}
	if _, err := qtx.ChangeBalance(ctx, db.ChangeBalanceParams{
		AccountID:     withdraw.AccountID,
		BalanceChange: -withdraw.Amount - withdraw.PriorityFee,
	}); err != nil {
		return nil, err
	}
	if err = tx.Commit(context.Background()); err != nil {
		return nil, err
	}
	return &withdraw, nil
}

type CancelWithdrawTxParams struct {
	WithdrawalId int64
}

func (s *Store) CancelWithdrawTx(ctx context.Context, arg CancelWithdrawTxParams) error {
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	qtx := s.Queries.WithTx(tx)
	withdraw, err := qtx.CancelWithdrawalById(ctx, arg.WithdrawalId)
	if err != nil {
		return err
	}
	if _, err := qtx.ChangeBalance(ctx, db.ChangeBalanceParams{
		AccountID:     withdraw.AccountID,
		BalanceChange: withdraw.Amount + withdraw.PriorityFee,
	}); err != nil {
		return err
	}
	return tx.Commit(context.Background())
}
