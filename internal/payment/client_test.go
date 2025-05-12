package payment_test

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/atticplaygroup/prex/internal/payment"
	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/block-vision/sui-go-sdk/sui"

	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/signer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func transferAndWait(
	ctx context.Context,
	cli sui.ISuiAPI,
	senderMnemonic string,
	recipientAddress string,
	coinObjectId string,
	amount int,
) (string, error) {
	signerAccount, err := signer.NewSignertWithMnemonic(senderMnemonic)
	if err != nil {
		log.Fatalf("cannot init singer: %v", err)
	}

	priKey := signerAccount.PriKey

	rsp, err := cli.TransferSui(ctx, models.TransferSuiRequest{
		Signer:      signerAccount.Address,
		SuiObjectId: coinObjectId,
		GasBudget:   "100000000",
		Recipient:   recipientAddress,
		Amount:      strconv.Itoa(amount),
	})

	if err != nil {
		return "", err
	}
	rsp2, err := cli.SignAndExecuteTransactionBlock(ctx, models.SignAndExecuteTransactionBlockRequest{
		TxnMetaData: rsp,
		PriKey:      priKey,
		Options: models.SuiTransactionBlockOptions{
			ShowInput:    true,
			ShowRawInput: true,
			ShowEffects:  true,
		},
		RequestType: "WaitForLocalExecution",
	})
	if err != nil {
		log.Fatalf("failed to execute transaction: %v", err)
	}
	return rsp2.Digest, nil
}

type MockEpochGetter struct{}

func (c *MockEpochGetter) GetCurrentEpoch(ctx context.Context) (int, error) {
	return 42, nil
}

var _ = Describe("Transfer with Sui", Label("sui"), func() {
	senderMnemonic := "trend people tourist grunt essay hungry indicate wedding step roast heart park"
	sender, err := signer.NewSignertWithMnemonic(senderMnemonic)
	if err != nil {
		log.Fatal(err)
	}
	platformMnemonic := "thought unaware clump fork ring hawk cloud outside reject crack photo toy"
	platform, err := signer.NewSignertWithMnemonic(platformMnemonic)
	if err != nil {
		log.Fatal(err)
	}
	recipientMnemonic := "gold tide pretty world express kitchen must road rival priority curtain benefit"
	recipient, err := signer.NewSignertWithMnemonic(recipientMnemonic)
	if err != nil {
		log.Fatal(err)
	}

	suiNetwork := os.Getenv("SUI_NETWORK")
	if suiNetwork == "" {
		suiNetwork = "devnet"
	}
	client, err := payment.NewSuiPaymentClient(suiNetwork, platform)
	if err != nil {
		log.Fatal(err)
	}
	It("should discover successful deposit", func() {
		ctx := context.Background()
		header := map[string]string{}
		err = sui.RequestSuiFromFaucet(utils.GetSuiFaucetHost(suiNetwork), sender.Address, header)
		Expect(err).To(BeNil())

		amount := 1_000_000

		nonExistDigest := "CeVpDXKKU3Gs89efej9pKiYYQyTzifE2BDxWwquUaUht"

		_, err := client.CheckDeposit(ctx, nonExistDigest, 100)
		Expect(err).To((Not(BeNil())))

		coins, err := client.SuiClient.SuiXGetAllCoins(ctx, models.SuiXGetAllCoinsRequest{
			Owner: sender.Address,
			Limit: 1,
		})
		Expect(err).To(BeNil())
		Expect(len(coins.Data)).To(Equal(1))

		digest, err := transferAndWait(
			ctx, client.SuiClient, senderMnemonic, client.Signer.Address,
			coins.Data[0].CoinObjectId, amount,
		)
		Expect(err).To(BeNil())

		// Wait for transaction to finalize
		time.Sleep(5 * time.Second)

		receipt, err := client.CheckDeposit(ctx, digest, 100)
		Expect(err).To(BeNil())
		Expect(receipt.Address).To(Equal(sender.Address))
		Expect(receipt.Amount).To(BeEquivalentTo(amount))
	})

	It("should return pending not found withdrawal", func() {
		ctx := context.Background()
		client1, err := payment.NewSuiPaymentClient(suiNetwork, platform)
		client1.SetEpochGetter(&MockEpochGetter{})
		Expect(err).To(BeNil())
		status, err := client1.CheckTransactionStatus(
			ctx, "CeVpDXKKU3Gs89efej9pKiYYQyTzifE2BDxWwquUaUht")
		Expect(err).To(BeNil())
		Expect(status).To(Equal(payment.PENDING))
	})

	It("should return success for successful withdrawal on check transaction status", func() {
		ctx := context.Background()
		addressTo := "0xe789fb3f9e6e0736b648f3f33ff60bc0e4583583b2142cb2665bcc520635aac0"
		transferInfo := payment.TransferInfo{
			Address: addressTo,
			Amount:  10_000_000,
		}
		withdrawTx, err := client.PrepareWithdrawTransaction(
			ctx, []payment.TransferInfo{transferInfo}, 8_000_000,
		)
		Expect(err).To(BeNil())
		dryRunResult, err := client.SuiClient.SuiDryRunTransactionBlock(
			ctx, models.SuiDryRunTransactionBlockRequest{
				TxBytes: withdrawTx.TxBytes,
			})
		Expect(err).To(BeNil())
		digest := dryRunResult.Effects.TransactionDigest
		Expect(digest).To(Not(Equal("")))
		txResult, err := client.SuiClient.SignAndExecuteTransactionBlock(
			ctx, models.SignAndExecuteTransactionBlockRequest{
				TxnMetaData: *withdrawTx,
				PriKey:      platform.PriKey,
				RequestType: "WaitForLocalExecution",
			})
		Expect(err).To(BeNil())
		Expect(txResult.Digest).To(Equal(digest))

		time.Sleep(5 * time.Second)
		status, err := client.CheckTransactionStatus(ctx, digest)
		Expect(err).To(BeNil())
		Expect(status).To(Equal(payment.SUCCESS))
	})

	It("should withdraw multiple recipients", func() {
		ctx := context.Background()
		header := map[string]string{}
		err = sui.RequestSuiFromFaucet(utils.GetSuiFaucetHost(suiNetwork), client.Signer.Address, header)
		Expect(err).To(BeNil())

		transferInfo := make([]payment.TransferInfo, 0)
		transferInfo = append(transferInfo, payment.TransferInfo{
			Amount:  10_000_000,
			Address: sender.Address,
		})
		transferInfo = append(transferInfo, payment.TransferInfo{
			Amount:  20_000_000,
			Address: recipient.Address,
		})
		suiTx, err := client.PrepareWithdrawTransaction(ctx, transferInfo, 1_000_000)
		Expect(err).To(BeNil())
		digest, err := client.Withdraw(ctx, suiTx)
		Expect(err).To(BeNil())
		var q struct {
			TransactionBlock struct {
				Sender struct {
					Address string
				}
			} `graphql:"transactionBlock(digest: $digest)"`
		}

		variables := map[string]any{
			"digest": digest,
		}
		time.Sleep(5 * time.Second)
		err = client.GqlClient.Query(ctx, &q, variables)
		Expect(err).To(BeNil())
		Expect(q.TransactionBlock.Sender.Address).To(Equal(client.Signer.Address))
	})
})
