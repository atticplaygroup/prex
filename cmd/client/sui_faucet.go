package client

import (
	"log"

	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/spf13/cobra"
)

var faucetCmd = &cobra.Command{
	Use:   "sui-faucet",
	Short: "Request Sui coins from faucet",
	Run: func(cmd1 *cobra.Command, args []string) {
		network, err := cmd1.Flags().GetString("network")
		if err != nil {
			log.Fatalf("cannot get network: %v", err)
		}
		recipient := conf.Account.KeyPair.Address
		err = utils.RequestSuiFromFaucet(network, recipient)
		if err != nil {
			log.Fatalf("request faucet failed: %v", err)
		}
	},
}

func init() {
	faucetCmd.Flags().StringP("network", "n", "devnet", "which network's faucet to request")
	clientCmd.AddCommand(faucetCmd)
}
