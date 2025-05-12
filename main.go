//go:generate sqlc generate
//go:generate protoc -I ./pkg/proto -I ./pkg/proto/googleapis --go_out=./pkg/proto/gen/go --go_opt=paths=source_relative --go-grpc_out=./pkg/proto/gen/go --go-grpc_opt=paths=source_relative --grpc-gateway_out=./pkg/proto/gen/go --grpc-gateway_opt=paths=source_relative --openapiv2_out ./pkg/proto/gen/openapi --openapiv2_opt use_go_templates=true ./pkg/proto/exchange/exchange.proto

package main

import (
	"github.com/atticplaygroup/prex/cmd"
	_ "github.com/atticplaygroup/prex/cmd/client"
	_ "github.com/atticplaygroup/prex/cmd/server"
)

func main() {
	cmd.Execute()
}
