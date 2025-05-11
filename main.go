package main

import (
	"github.com/atticplaygroup/prex/cmd"
	_ "github.com/atticplaygroup/prex/cmd/client"
	_ "github.com/atticplaygroup/prex/cmd/server"
)

func main() {
	cmd.Execute()
}
