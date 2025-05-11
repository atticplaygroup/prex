package server

import (
	"github.com/atticplaygroup/prex/cmd"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Prex server commands",
}

func init() {
	cmd.RootCmd.AddCommand(serverCmd)
}
