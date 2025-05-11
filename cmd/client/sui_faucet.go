package client

import (
	"log"

	"github.com/block-vision/sui-go-sdk/sui"
	"github.com/spf13/cobra"
)

func getSuiFaucetHost(network string) string {
	switch network {
	case "devnet":
		return "https://faucet.devnet.sui.io"
	case "testnet":
		return "https://faucet.testnet.sui.io"
	case "localnet":
		return "http://127.0.0.1:9123"
	default:
	}
	log.Fatalf("faucet network should in [devnet, testnet, localnet]")
	return ""
}

func getSuiFullNodeHost(network string) string {
	switch network {
	case "mainnet":
		return "https://fullnode.mainnet.sui.io:443"
	case "devnet":
		return "https://fullnode.devnet.sui.io:443"
	case "testnet":
		return "https://fullnode.testnet.sui.io:443"
	case "localnet":
		return "http://127.0.0.1:9000"
	default:
	}
	log.Fatalf("faucet network should in [mainnet, devnet, testnet, localnet]")
	return ""
}

var faucetCmd = &cobra.Command{
	Use:   "sui-faucet",
	Short: "Request Sui coins from faucet",
	Run: func(cmd1 *cobra.Command, args []string) {
		network, err := cmd1.Flags().GetString("network")
		if err != nil {
			log.Fatalf("cannot get network: %v", err)
		}
		faucetHost := getSuiFaucetHost(network)
		recipient := conf.Account.KeyPair.Address
		_, err = sui.GetFaucetHost(network)
		if err != nil {
			log.Fatalf("get faucet host failed: %v", err)
		}
		header := map[string]string{}
		err = sui.RequestSuiFromFaucet(faucetHost, recipient, header)
		if err != nil {
			log.Fatalf("request faucet failed: %v", err)
		}
	},
}

func init() {
	faucetCmd.Flags().StringP("network", "n", "devnet", "which network's faucet to request")
	clientCmd.AddCommand(faucetCmd)
}
