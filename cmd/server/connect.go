package server

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"net/http"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/atticplaygroup/prex/internal/api"
	"github.com/atticplaygroup/prex/internal/config"
	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1/exchangeconnect"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "start the Prex Connect protocol server",
	Run: func(cmd *cobra.Command, args []string) {
		envPath, err := cmd.Flags().GetString("environment")
		if err != nil {
			log.Fatalf("failed to get environment config file")
		}
		conf := config.LoadConfig(envPath)
		log.Printf("conf: %+v", conf)

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

		validator, err := protovalidate.New()
		if err != nil {
			log.Fatalf("failed to initialize validator: %s", err.Error())
		}
		privateKey := server.GetConfig().TokenSigningPrivateKey
		publicKey := privateKey.Public().(ed25519.PublicKey)

		mux := http.NewServeMux()
		path, handler := exchangeconnect.NewExchangeServiceHandler(
			server,
			connect.WithInterceptors(
				api.NewConnectAuthInterceptor(publicKey),
				api.NewConnectValidationInterceptor(validator),
			),
		)
		mux.Handle(path, handler)
		c := getCorsConfig()
		http.ListenAndServe(
			fmt.Sprintf("127.0.0.1:%d", conf.PrexGrpcPort),
			h2c.NewHandler(c.Handler(mux), &http2.Server{}),
		)
	},
}

func init() {
	connectCmd.Flags().StringP("environment", "e", ".env", "environment file to load configs")
	serverCmd.AddCommand(connectCmd)
}
