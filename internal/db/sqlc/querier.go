// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0

package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type Querier interface {
	AddDepositRecord(ctx context.Context, arg AddDepositRecordParams) (Deposit, error)
	CancelOrder(ctx context.Context, arg CancelOrderParams) (ActiveOrder, error)
	CancelWithdrawalById(ctx context.Context, withdrawalID int64) (CancelWithdrawalByIdRow, error)
	ChangeBalance(ctx context.Context, arg ChangeBalanceParams) (Account, error)
	ClaimFulfilledOrderOfQuantity(ctx context.Context, arg ClaimFulfilledOrderOfQuantityParams) (FulfilledOrder, error)
	CleanExpiredFulfilledOrders(ctx context.Context) ([]FulfilledOrder, error)
	CleanInactiveOrders(ctx context.Context) ([]ActiveOrder, error)
	// 'processing' withdrawals must wait being marked to avoid losing money
	CleanOldWithdrawals(ctx context.Context, cleanTime pgtype.Timestamptz) ([]ProcessingWithdrawal, error)
	CreateOrder(ctx context.Context, arg CreateOrderParams) (ActiveOrder, error)
	CreateService(ctx context.Context, arg CreateServiceParams) (Service, error)
	DeleteInvalidAccounts(ctx context.Context) ([]int64, error)
	FindServiceByGlobalId(ctx context.Context, serviceGlobalID pgtype.UUID) (Service, error)
	GetAccount(ctx context.Context, username string) (Account, error)
	ListFulfilledOrders(ctx context.Context, arg ListFulfilledOrdersParams) ([]FulfilledOrder, error)
	ListFulfilledOrdersByService(ctx context.Context, arg ListFulfilledOrdersByServiceParams) ([]FulfilledOrder, error)
	ListProcessingWithdrawals(ctx context.Context, arg ListProcessingWithdrawalsParams) ([]ProcessingWithdrawal, error)
	ListServices(ctx context.Context, arg ListServicesParams) ([]Service, error)
	ListWithdrawals(ctx context.Context, arg ListWithdrawalsParams) ([]Withdrawal, error)
	MatchOneOrder(ctx context.Context, arg MatchOneOrderParams) (FulfilledOrder, error)
	NewClaimOrder(ctx context.Context, arg NewClaimOrderParams) (ClaimedOrder, error)
	ProcessWithdrawals(ctx context.Context, arg ProcessWithdrawalsParams) ([]Withdrawal, error)
	QueryBalance(ctx context.Context, accountID int64) (Account, error)
	QueryBalanceForShare(ctx context.Context, accountID int64) (Account, error)
	QueryFulfilledOrder(ctx context.Context, arg QueryFulfilledOrderParams) (FulfilledOrder, error)
	QueryService(ctx context.Context, serviceID int64) (Service, error)
	RemoveService(ctx context.Context, serviceID int64) (Service, error)
	SelectCandidateWithdrawals(ctx context.Context, retrieveCount int32) ([]Withdrawal, error)
	SetWithdrawalBatch(ctx context.Context, arg SetWithdrawalBatchParams) (ProcessingWithdrawal, error)
	SetWithdrawalSuccess(ctx context.Context, transactionDigest string) (ProcessingWithdrawal, error)
	StartWithdrawal(ctx context.Context, arg StartWithdrawalParams) (Withdrawal, error)
	UpsertAccount(ctx context.Context, arg UpsertAccountParams) (Account, error)
}

var _ Querier = (*Queries)(nil)
