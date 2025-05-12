package client

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/block-vision/sui-go-sdk/models"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/block-vision/sui-go-sdk/sui"
	"github.com/spf13/cobra"
)

var suiTransferCmd = &cobra.Command{
	Use:   "sui-transfer",
	Short: "Transfer Sui coins to recipient",
	Run: func(cmd1 *cobra.Command, args []string) {
		network, err := cmd1.Flags().GetString("network")
		if err != nil {
			log.Fatalf("cannot get network: %v", err)
		}
		recipient, err := cmd1.Flags().GetString("recipient")
		if err != nil || recipient == "" {
			log.Fatalf("cannot get recipient: %v", err)
		}
		coinObjectId, err := cmd1.Flags().GetString("coin")
		if err != nil || coinObjectId == "" {
			log.Fatalf("cannot get coinObjectId: %v", err)
		}
		amount, err := cmd1.Flags().GetInt("amount")
		if err != nil || amount <= 0 {
			log.Fatalf("cannot get coinObjectId: %v", err)
		}

		var ctx = context.Background()
		var cli = sui.NewSuiClient(utils.GetSuiFullNodeHost(network))

		signerAccount, err := signer.NewSignertWithMnemonic(conf.Account.Mnemonic)
		if err != nil {
			log.Fatalf("cannot init singer: %v", err)
		}

		priKey := signerAccount.PriKey
		fmt.Printf("signerAccount.Address: %s\n", signerAccount.Address)

		rsp, err := cli.TransferSui(ctx, models.TransferSuiRequest{
			Signer:      signerAccount.Address,
			SuiObjectId: coinObjectId,
			GasBudget:   "100000000",
			Recipient:   recipient,
			Amount:      strconv.Itoa(amount),
		})

		if err != nil {
			fmt.Println(err.Error())
			return
		}
		_, err = cli.SignAndExecuteTransactionBlock(ctx, models.SignAndExecuteTransactionBlockRequest{
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
	},
}

func init() {
	suiTransferCmd.Flags().StringP("network", "n", "devnet", "which network's faucet to request")
	suiTransferCmd.Flags().StringP("recipient", "r", "", "recipient address")
	suiTransferCmd.Flags().String("coin", "", "coin object's Sui address")
	suiTransferCmd.Flags().IntP("amount", "a", 1_000_000_000, "the amount of coins to transfer")
	clientCmd.AddCommand(suiTransferCmd)
}
