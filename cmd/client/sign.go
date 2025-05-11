package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var signCmd = &cobra.Command{
	Use:   "sign",
	Short: "Sign a personal message",
	Long:  "",
	Run:   sign,
}

func init() {
	signCmd.Flags().StringP("message", "m", "", "Base64 encoded message")
	clientCmd.AddCommand(signCmd)
}

func sign(cmd *cobra.Command, args []string) {
	messageStr, err := cmd.Flags().GetString("message")
	if err != nil || messageStr == "" {
		log.Fatal("cannot parse message")
	}

	message, err := base64.StdEncoding.DecodeString(messageStr)
	if err != nil {
		log.Fatal(err)
	}
	signature, err := conf.Account.KeyPair.SignPersonalMessageV1(string(message))
	if err != nil {
		log.Fatal(err)
	}

	signatuerEncoded, err := json.Marshal(signature)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(signatuerEncoded))
}
