package client

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/spf13/cobra"
)

type AccountInfo struct {
	SecretSeed string `json:"secret_seed"`
	// PrivateKey string `json:"private_key"`
	Mnemonic  string `json:"mnemonics"`
	PublicKey string `json:"public_key"`
	Address   string `json:"address"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Print information of the account",
	Long:  "Print out secret seed, private key, mnemonic, public key and address of the account",
	Run: func(cmd *cobra.Command, args []string) {
		accountInfo := AccountInfo{
			SecretSeed: utils.BytesToHexWithPrefix(conf.Account.SecretSeed),
			// PrivateKey: bech32.Encode(conf.Account.KeyPair.PriKey),
			Mnemonic:  conf.Account.Mnemonic,
			PublicKey: utils.BytesToHexWithPrefix(conf.Account.KeyPair.PubKey),
			Address:   conf.Account.KeyPair.Address,
			Username:  conf.Account.Username,
			Password:  conf.Account.Password,
		}
		output, err := json.Marshal(accountInfo)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(string(output))
	},
}

func init() {
	clientCmd.AddCommand(accountCmd)
}
