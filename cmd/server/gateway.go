package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/spf13/cobra"

	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start a RESTful gRPC-gateway proxy server",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		grpcHost, err := cmd.Flags().GetString("grpc-host")
		if err != nil {
			log.Fatalf("failed to parse grpc host")
		}
		grpcPort, err := cmd.Flags().GetUint16("grpc-port")
		if err != nil {
			log.Fatalf("failed to parse grpc port")
		}
		bindPort, err := cmd.Flags().GetUint16("bind-port")
		if err != nil {
			log.Fatalf("failed to parse bind port")
		}

		ropts := []runtime.ServeMuxOption{
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{}),
		}

		mux := runtime.NewServeMux(ropts...)
		opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

		err = pb.RegisterExchangeHandlerFromEndpoint(
			ctx, mux, fmt.Sprintf("%s:%d", grpcHost, grpcPort), opts,
		)
		if err != nil {
			log.Fatalf("failed to register endpoint")
		}

		log.Printf("starting gateway server on port %d\n", bindPort)
		http.ListenAndServe(fmt.Sprintf(":%d", bindPort), mux)
	},
}

func init() {
	gatewayCmd.Flags().String("grpc-host", "localhost", "gRPC port of the service")
	gatewayCmd.Flags().Uint16("grpc-port", 50051, "gRPC port of the service")
	gatewayCmd.Flags().Uint16("bind-port", 3000, "bind to port")
	serverCmd.AddCommand(gatewayCmd)
}
