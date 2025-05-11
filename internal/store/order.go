package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/utils"
)

type MatchOrderTxParams struct {
	db.MatchOneOrderParams
}

func (s *Store) DoMatchOrderTx(
	ctx context.Context,
	qtx *db.Queries,
	arg MatchOrderTxParams,
) (*db.FulfilledOrder, error) {
	fulfilledOrder, err := qtx.MatchOneOrder(ctx, arg.MatchOneOrderParams)
	if err != nil {
		return nil, fmt.Errorf("match one order failed: %v", err)
	}
	dealTotalPrice := fulfilledOrder.DealPrice * fulfilledOrder.DealQuantity
	if _, err := qtx.ChangeBalance(ctx, db.ChangeBalanceParams{
		AccountID:     fulfilledOrder.BuyerID,
		BalanceChange: -dealTotalPrice,
	}); err != nil {
		return nil, err
	}
	if _, err := qtx.ChangeBalance(ctx, db.ChangeBalanceParams{
		AccountID:     fulfilledOrder.SellerID,
		BalanceChange: dealTotalPrice,
	}); err != nil {
		return nil, err
	}
	return &fulfilledOrder, nil
}

func (s *Store) MatchOrderTx(ctx context.Context, arg MatchOrderTxParams) (*db.FulfilledOrder, error) {
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	qtx := s.Queries.WithTx(tx)
	fulfilledOrder, err := s.DoMatchOrderTx(ctx, qtx, arg)
	if err != nil {
		return nil, err
	}
	return fulfilledOrder, tx.Commit(context.Background())
}

type ClaimOrderTxParams struct {
	db.NewClaimOrderParams
	AccountID int64
}

type Claim struct {
	OrderClaimId       int64
	AudienceAddress    string
	Expiration         time.Time
	OrderId            int64
	OrderFulfillmentId int64
	SellerId           int64
	ServiceId          int64
	Quantity           int64
}

func (s *Store) ClaimOrderTx(ctx context.Context, arg ClaimOrderTxParams) (*Claim, error) {
	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	qtx := s.Queries.WithTx(tx)

	fulfilledOrder, err := qtx.QueryFulfilledOrder(
		ctx,
		db.QueryFulfilledOrderParams{
			BuyerID:            arg.AccountID,
			OrderFulfillmentID: arg.NewClaimOrderParams.OrderFulfillmentID,
		},
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf(
			"not found: buyer_id = %d and order_fulfillment_id = %d: %v",
			arg.AccountID,
			arg.NewClaimOrderParams.OrderFulfillmentID,
			err,
		)
	} else if err != nil {
		return nil, err
	}
	updatedFulfilledOrder, err := qtx.ClaimFulfilledOrderOfQuantity(
		ctx,
		db.ClaimFulfilledOrderOfQuantityParams{
			OrderFulfillmentID: arg.NewClaimOrderParams.OrderFulfillmentID,
			ClaimQuantity:      arg.NewClaimOrderParams.ClaimQuantity,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"fail to update remaining quantity to claim quantity of %d: %v",
			arg.NewClaimOrderParams.ClaimQuantity,
			err,
		)
	}
	claimedOrder, err := qtx.NewClaimOrder(
		ctx,
		arg.NewClaimOrderParams,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"fail to insert claimed orders: %v",
			err,
		)
	}

	_, err = qtx.QueryService(
		ctx,
		fulfilledOrder.ServiceID,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf(
			"service %d has been deactivated: %v",
			fulfilledOrder.ServiceID,
			err,
		)
	} else if err != nil {
		return nil, err
	}

	return &Claim{
		OrderClaimId:       claimedOrder.OrderClaimID,
		AudienceAddress:    utils.BytesToHexWithPrefix(arg.AudienceAddress),
		Expiration:         updatedFulfilledOrder.ServiceExpireTime.Time,
		OrderId:            updatedFulfilledOrder.OrderID,
		OrderFulfillmentId: updatedFulfilledOrder.OrderFulfillmentID,
		SellerId:           updatedFulfilledOrder.SellerID,
		ServiceId:          updatedFulfilledOrder.ServiceID,
		Quantity:           claimedOrder.ClaimQuantity,
	}, tx.Commit(context.Background())
}
