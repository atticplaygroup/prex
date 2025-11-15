package api

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/atticplaygroup/prex/internal/auth"
	"github.com/atticplaygroup/prex/internal/config"
	"github.com/atticplaygroup/prex/internal/payment"
	"github.com/atticplaygroup/prex/internal/store"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type Server struct {
	pb.ExchangeServiceServer
	// exchange.UnimplementedExchangeServer
	config        config.Config
	store         store.Store
	redisClient   *redis.Client
	auth          auth.Auth
	paymentClient *payment.SuiPaymentClient
}

func (s *Server) Ping(ctx context.Context, _ *connect.Request[pb.PingRequest]) (*connect.Response[pb.PingResponse], error) {
	return connect.NewResponse(&pb.PingResponse{
		Pong: "pong",
	}), nil
}

func (s *Server) ListPaymentMethods(
	ctx context.Context,
	req *connect.Request[pb.ListPaymentMethodsRequest],
) (*connect.Response[pb.ListPaymentMethodsResponse], error) {
	return connect.NewResponse(&pb.ListPaymentMethodsResponse{
		// TODO: Support other options
		PaymentMethods: []*pb.PaymentMethod{
			{
				Coin:        *pb.PaymentCoin_PAYMENT_COIN_SUI.Enum(),
				Environment: *pb.PaymentEnvironment_PAYMENT_ENVIRONMENT_DEVNET.Enum(),
				Address:     s.config.WalletSigner.Address,
			},
			{
				Coin:        *pb.PaymentCoin_PAYMENT_COIN_SUI.Enum(),
				Environment: *pb.PaymentEnvironment_PAYMENT_ENVIRONMENT_LOCALNET.Enum(),
				Address:     s.config.WalletSigner.Address,
			},
		},
	}), nil
}

func NewServer(config config.Config, store store.Store) (*Server, error) {
	paymentClient, err := payment.NewSuiPaymentClient(config.SuiNetwork, &config.WalletSigner)
	if err != nil {
		log.Fatalf("failed to initialize sui client: %v", err)
	}
	authentication, err := auth.NewAuth(config)
	if err != nil {
		log.Fatalf("Cannot initialize auth: %v", err)
	}
	server := &Server{
		config:        config,
		store:         store,
		auth:          *authentication,
		paymentClient: paymentClient,
		redisClient: redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", config.RedisHost, config.RedisPort),
		}),
	}
	ctx := context.Background()
	if err := server.redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("cannot connect to redis: %v", err)
	}
	return server, nil
}

func NewGrpcServer(server *Server) *grpc.Server {
	privateKey := server.GetConfig().TokenSigningPrivateKey
	publicKey := privateKey.Public().(ed25519.PublicKey)
	validator, err := protovalidate.New()
	if err != nil {
		log.Fatalf("failed to initialize validator: %s", err.Error())
	}
	// var freeQuotaRedisRateLimiter func(context.Context, interceptors.CallMeta) bool
	selectors := []grpc.UnaryServerInterceptor{
		selector.UnaryServerInterceptor(
			NewGrpcAuthInterceptor(publicKey),
			selector.MatchFunc(AuthMiddlewareSelector),
		),
		NewGrpcValidationInterceptor(validator),
	}
	return grpc.NewServer(
		grpc.ChainUnaryInterceptor(selectors...),
	)
}

func (s *Server) GetRedisClient() *redis.Client {
	return s.redisClient
}

func (s *Server) GetConfig() *config.Config {
	return &s.config
}
