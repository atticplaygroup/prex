package api

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/payment"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/block-vision/sui-go-sdk/models"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) CreateWithdraw(ctx context.Context, req *pb.CreateWithdrawRequest) (*pb.Withdrawal, error) {
	// TODO: change priority fee mechanism to second price
	chainAddressBytes, err := hex.DecodeString(req.GetWithdrawal().AddressTo[2:])
	if err != nil || len(chainAddressBytes) != 32 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse address: %v",
			err,
		)
	}
	idSegments, err := utils.ParseResourceName(
		req.GetParent(), []string{"accounts"},
	)
	if err != nil || len(idSegments) != 1 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	accountId := idSegments[0]
	withdrawal, err := s.store.WithdrawTx(ctx, store.WithdrawTxParams{
		WithdrawAll: req.GetWithdrawAll(),
		StartWithdrawalParams: db.StartWithdrawalParams{
			AccountID:       accountId,
			WithdrawAddress: chainAddressBytes,
			Amount:          req.GetWithdrawal().GetAmount(),
			PriorityFee:     req.GetWithdrawal().GetPriorityFee(),
		},
	})
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to execute withdrawTx",
		)
	}
	return &pb.Withdrawal{
		Name:        fmt.Sprintf(utils.RESOURCE_PATTERN_WITHDRAW, accountId, withdrawal.WithdrawalID),
		AddressTo:   req.GetWithdrawal().GetAddressTo(),
		Amount:      withdrawal.Amount,
		PriorityFee: withdrawal.PriorityFee,
	}, nil
}

func (s *Server) BatchProcessWithdraws(
	ctx context.Context,
	req *pb.BatchProcessWithdrawsRequest,
) (*pb.BatchProcessWithdrawsResponse, error) {
	if req.GetLimit() > s.config.WithdrawRecipientCount || req.GetLimit() <= 0 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"limit exceeds valid range (0, %d]",
			s.config.WithdrawRecipientCount,
		)
	}

	tx, err := s.store.GetConn().Begin(ctx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to init transaction: %v",
			err,
		)
	}
	defer tx.Rollback(context.Background())
	qtx := s.store.Queries.WithTx(tx)
	withdrawals, err := qtx.SelectCandidateWithdrawals(
		ctx, int32(req.GetLimit()))
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to select withdraw candidates db",
		)
	}
	if len(withdrawals) == 0 {
		return &pb.BatchProcessWithdrawsResponse{
			BatchSize: 0,
		}, nil
	}

	transferInfo := make([]payment.TransferInfo, 0)
	withdrawIds := make([]int64, 0)
	totalPriorityFee := int64(0)
	for _, withdrawal := range withdrawals {
		transferInfo = append(transferInfo, payment.TransferInfo{
			Address: hex.EncodeToString(withdrawal.WithdrawAddress),
			Amount:  withdrawal.Amount,
		})
		withdrawIds = append(withdrawIds, withdrawal.WithdrawalID)
		totalPriorityFee += withdrawal.PriorityFee
	}
	suiTx, err := s.paymentClient.PrepareWithdrawTransaction(ctx, transferInfo, totalPriorityFee)
	if err != nil {
		return nil, err
	}
	dryRunResult, err := s.paymentClient.SuiClient.SuiDryRunTransactionBlock(
		ctx, models.SuiDryRunTransactionBlockRequest{
			TxBytes: suiTx.TxBytes,
		})
	if err != nil || dryRunResult.Effects.TransactionDigest == "" {
		return nil, fmt.Errorf("failed to calculate transaction digest: %v", err)
	}

	processingWithdrawal, err := qtx.SetWithdrawalBatch(
		ctx, db.SetWithdrawalBatchParams{
			TransactionDigest:      dryRunResult.Effects.TransactionDigest,
			TransactionBytesBase64: suiTx.TxBytes,
			TotalPriorityFee:       totalPriorityFee,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to set withdrawal batch: %v", err)
	}
	_, err = qtx.ProcessWithdrawals(ctx, db.ProcessWithdrawalsParams{
		WithdrawalIds: withdrawIds,
		ProcessingWithdrawalID: pgtype.Int8{
			Int64: processingWithdrawal.ProcessingWithdrawalID,
			Valid: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update withdraw status: %v", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit database change: %v", err)
	}

	digest, err := s.paymentClient.Withdraw(ctx, suiTx)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to call payment: %v",
			err,
		)
	}
	return &pb.BatchProcessWithdrawsResponse{Digest: digest}, nil
}

func (s *Server) BatchMarkWithdraws(
	ctx context.Context,
	req *pb.BatchMarkWithdrawsRequest,
) (*pb.BatchMarkWithdrawsResponse, error) {
	if req.GetLimit() > s.config.WithdrawCheckStatusCount || req.GetLimit() <= 0 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"limit exceeds valid range (0, %d]",
			s.config.WithdrawCheckStatusCount,
		)
	}
	processingWithdrawals, err := s.store.ListProcessingWithdrawals(
		ctx,
		db.ListProcessingWithdrawalsParams{
			Limit:  req.GetLimit(),
			Offset: 0,
		})
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to list processing withdrawals from db: %v",
			err,
		)
	}
	successChan := make(chan int64)
	var wg sync.WaitGroup
	for _, withdraw := range processingWithdrawals {
		transactionDigestHex := withdraw.TransactionDigest
		wg.Add(1)
		go func() {
			defer wg.Done()
			transactionStatus, err := s.paymentClient.CheckTransactionStatus(
				ctx,
				withdraw.TransactionDigest,
			)
			if err != nil {
				slog.ErrorContext(
					ctx,
					fmt.Sprintf("check transaction status failed for transactionDigest %s: %v",
						transactionDigestHex,
						err,
					))
				return
			}
			switch transactionStatus {
			case payment.SUCCESS:
				if processingWithdrawal, err := s.store.SetWithdrawalSuccess(
					ctx,
					withdraw.TransactionDigest,
				); err != nil {
					slog.ErrorContext(
						ctx,
						fmt.Sprintf("set %s success in db failed: %v", transactionDigestHex, err))
				} else {
					successChan <- processingWithdrawal.ProcessingWithdrawalID
				}
			case payment.PENDING:
				slog.InfoContext(ctx, fmt.Sprintf("%s is pending", transactionDigestHex))
			default:
				slog.ErrorContext(
					ctx,
					fmt.Sprintf(
						"got undefined status for %s: %d",
						transactionDigestHex,
						transactionStatus,
					))
			}
		}()
	}

	wg.Wait()
	close(successChan)

	successIds := make([]int64, 0)
	for id := range successChan {
		successIds = append(successIds, id)
	}
	return &pb.BatchMarkWithdrawsResponse{
		SuccessWithdrawIds: successIds,
	}, nil
}
