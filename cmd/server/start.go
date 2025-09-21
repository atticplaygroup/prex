package server

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/atticplaygroup/prex/internal/api"
	"github.com/atticplaygroup/prex/internal/config"
	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "start the Prex gRPC server",
	Run: func(cmd *cobra.Command, args []string) {
		envPath, err := cmd.Flags().GetString("environment")
		if err != nil {
			log.Fatalf("failed to get environment config file")
		}
		conf := config.LoadConfig(envPath)
		log.Printf("conf: %+v", conf)
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", conf.PrexGrpcPort))
		if err != nil {
			log.Fatalf("failed to listen port: %v\n", err)
		}

		ctx := context.Background()
		pool, err := pgxpool.New(ctx, conf.TestDbUrl)
		if err != nil {
			log.Fatalf("failed to connect to db: %v\n", err)
		}
		defer pool.Close()
		db.New(pool)
		store1 := store.NewStore(pool)

		server, err := api.NewServer(
			conf,
			*store1,
		)
		if err != nil {
			log.Fatalf("failed to init server: %v\n", err)
		}

		s := api.NewGrpcServer(server)
		pb.RegisterExchangeServiceServer(s, server.ExchangeServiceServer)
		go func() {
			defer s.GracefulStop()
			<-ctx.Done()
		}()
		s.Serve(l)
	},
}

func init() {
	startCmd.Flags().StringP("environment", "e", ".env", "environment file to load configs")
	serverCmd.AddCommand(startCmd)
}
