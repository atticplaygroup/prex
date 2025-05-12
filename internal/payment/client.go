package payment

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shurcooL/graphql"

	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/block-vision/sui-go-sdk/sui"
)

type IEpochGetter interface {
	GetCurrentEpoch(context.Context) (int, error)
}

type EpochGetter struct {
	suiClient sui.ISuiAPI
}

type SuiPaymentClient struct {
	SuiClient   sui.ISuiAPI
	GqlClient   *graphql.Client
	Signer      signer.Signer
	epochGetter IEpochGetter
}

func NewSuiPaymentClient(network string, walletSigner *signer.Signer) (*SuiPaymentClient, error) {
	var clientUrl, graphqlClientUrl string
	switch network {
	case "localnet":
		clientUrl = "http://127.0.0.1:9000"
		graphqlClientUrl = "http://127.0.0.1:9125"
	case "mainnet":
		clientUrl = "https://fullnode.mainnet.sui.io"
		graphqlClientUrl = "https://sui-mainnet.mystenlabs.com/graphql"
	case "testnet":
		clientUrl = "https://fullnode.testnet.sui.io"
		graphqlClientUrl = "https://sui-testnet.mystenlabs.com/graphql"
	case "devnet":
		clientUrl = "https://fullnode.devnet.sui.io"
		graphqlClientUrl = "https://sui-devnet.mystenlabs.com/graphql"
	default:
		return nil, fmt.Errorf("unknown network %s", network)
	}

	suiClient := sui.NewSuiClient(clientUrl)
	ret := SuiPaymentClient{
		SuiClient: suiClient,
		GqlClient: graphql.NewClient(graphqlClientUrl, nil),
		Signer:    *walletSigner,
	}
	ret.epochGetter = &EpochGetter{suiClient: suiClient}
	return &ret, nil
}

func (c *SuiPaymentClient) GetCurrentEpoch(ctx context.Context) (int, error) {
	return c.epochGetter.GetCurrentEpoch(ctx)
}

func (c *SuiPaymentClient) SetEpochGetter(eg IEpochGetter) {
	c.epochGetter = eg
}

func (c *EpochGetter) GetCurrentEpoch(ctx context.Context) (int, error) {
	systemSummary, err := c.suiClient.SuiXGetLatestSuiSystemState(ctx)
	if err != nil {
		return 0, err
	}
	epochNum, err := strconv.Atoi(systemSummary.Epoch)
	if err != nil {
		return 0, err
	}
	return epochNum, nil
}

type TransactionStatus int

const (
	UNKNOWN TransactionStatus = 0
	SUCCESS TransactionStatus = 1
	FAIL    TransactionStatus = 2
	PENDING TransactionStatus = 3
)

func (c *SuiPaymentClient) CheckTransactionStatus(
	ctx context.Context,
	digest string,
) (TransactionStatus, error) {
	_, err := c.SuiClient.SuiGetTransactionBlock(
		ctx, models.SuiGetTransactionBlockRequest{
			Digest: digest,
		})
	if err != nil {
		if strings.Contains(err.Error(), "Could not find the referenced transaction") {
			return PENDING, nil
		} else {
			return UNKNOWN, fmt.Errorf("failed to get current epoch: %v", err)
		}
	}
	return SUCCESS, nil
}

type TransferInfo struct {
	Address string
	Amount  int64
}

func (c *SuiPaymentClient) PrepareWithdrawTransaction(
	ctx context.Context, info []TransferInfo, gasBudget int64,
) (*models.TxnMetaData, error) {
	coins, err := c.SuiClient.SuiXGetAllCoins(ctx, models.SuiXGetAllCoinsRequest{
		Owner: c.Signer.Address,
	})
	if err != nil {
		return nil, err
	}
	myCoins := make([]string, 0)
	for _, coin := range coins.Data {
		myCoins = append(myCoins, coin.CoinObjectId)
	}
	recipients := make([]string, 0)
	splitAmounts := make([]string, 0)
	for _, t := range info {
		splitAmounts = append(splitAmounts, strconv.Itoa(int(t.Amount)))
		recipients = append(recipients, t.Address)
	}
	gasPrice, err := c.SuiClient.SuiXGetReferenceGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference gas price: %v", err)
	}
	if int64(gasPrice) < 0 || int64(gasPrice) > gasBudget {
		return nil, fmt.Errorf("gas price %d is higher than budget %d", gasPrice, gasBudget)
	}
	batchTx, err := c.SuiClient.PaySui(ctx, models.PaySuiRequest{
		Signer:      c.Signer.Address,
		SuiObjectId: myCoins,
		Recipient:   recipients,
		Amount:      splitAmounts,
		// FIXME: how to use gas budget? Why will setting it higher break?
		GasBudget: strconv.Itoa(1_000_000),
	})
	if err != nil {
		return nil, err
	}
	return &batchTx, nil
}

func (c *SuiPaymentClient) Withdraw(
	ctx context.Context, batchTx *models.TxnMetaData,
) (string, error) {
	dryRunResult, err := c.SuiClient.SuiDryRunTransactionBlock(
		ctx, models.SuiDryRunTransactionBlockRequest{
			TxBytes: batchTx.TxBytes,
		})
	if err != nil {
		return "nil", fmt.Errorf("failed to calculate transaction digest: %v", err)
	}
	rsp, err := c.SuiClient.SignAndExecuteTransactionBlock(
		ctx, models.SignAndExecuteTransactionBlockRequest{
			TxnMetaData: *batchTx,
			PriKey:      c.Signer.PriKey,
			RequestType: "WaitForLocalExecution",
		})
	if err != nil {
		return "", err
	}
	// Sanity check
	if dryRunResult.Effects.TransactionDigest != rsp.Digest {
		return "", fmt.Errorf("digest mismatch with dry run: %s vs %s", rsp.Digest, dryRunResult.Effects.TransactionDigest)
	}
	return rsp.Digest, nil
}

type DepositTransferInfo struct {
	*TransferInfo
	Epoch int64
}

func (c *SuiPaymentClient) CheckDeposit(ctx context.Context, digest string, maxGapEpochs int) (*DepositTransferInfo, error) {
	currentEpoch, err := c.epochGetter.GetCurrentEpoch(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current epoch: %v", err)
	}
	var q struct {
		TransactionBlock struct {
			Sender struct {
				Address string
			}
			Effects struct {
				BalanceChanges struct {
					Nodes []struct {
						Owner struct {
							Address string
						}
						Amount   string
						CoinType struct {
							Repr string
						}
					}
				}
				Timestamp string
				Epoch     struct {
					EpochId int
				}
			}
		} `graphql:"transactionBlock(digest: $digest)"`
	}

	variables := map[string]any{
		"digest": digest,
	}
	err = c.GqlClient.Query(ctx, &q, variables)
	if err != nil {
		return nil, err
	}
	if q.TransactionBlock.Sender.Address == "" {
		return nil, fmt.Errorf("no transaction found for digest")
	}
	if q.TransactionBlock.Effects.Epoch.EpochId+maxGapEpochs < currentEpoch {
		return nil, fmt.Errorf("deposit too late")
	}
	for _, node := range q.TransactionBlock.Effects.BalanceChanges.Nodes {
		if node.Amount != "" && node.Owner.Address == c.Signer.Address {
			amount, err := strconv.Atoi(node.Amount)
			if err != nil {
				return nil, fmt.Errorf("failed to parse amount: %s", node.Amount)
			}
			// FIXME: currently only the first matching effect is considered. User will lose money if they deposit with multiple effects.
			return &DepositTransferInfo{
				TransferInfo: &TransferInfo{
					Amount:  int64(amount),
					Address: q.TransactionBlock.Sender.Address,
				},
				Epoch: int64(q.TransactionBlock.Effects.Epoch.EpochId),
			}, nil
		}
	}
	return nil, fmt.Errorf("no valid transaction found")
}

