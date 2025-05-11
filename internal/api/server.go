package api

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"

	"github.com/atticplaygroup/prex/internal/auth"
	"github.com/atticplaygroup/prex/internal/config"
	"github.com/atticplaygroup/prex/internal/payment"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/token"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/ratelimit"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Server struct {
	pb.ExchangeServer
	// exchange.UnimplementedExchangeServer
	config        config.Config
	store         store.Store
	redisClient   *redis.Client
	auth          auth.Auth
	paymentClient *payment.SuiPaymentClient
	tokenPolicies map[int64]*token.TokenPolicy
	dbState       *DbState
}

func (s *Server) Ping(ctx context.Context, _ *emptypb.Empty) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Pong: "pong",
	}, nil
}

func (s *Server) ListPaymentMethods(
	ctx context.Context,
	req *pb.ListPaymentMethodsRequest,
) (*pb.ListPaymentMethodsResponse, error) {
	return &pb.ListPaymentMethodsResponse{
		// TODO: Support other options
		PaymentMethods: []*pb.PaymentMethod{
			{
				Coin:        *pb.PaymentCoin_SUI.Enum(),
				Environment: *pb.PaymentEnvironment_DEVNET.Enum(),
				Address:     s.config.WalletSigner.Address,
			},
		},
	}, nil
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
		tokenPolicies: make(map[int64]*token.TokenPolicy),
	}
	ctx := context.Background()
	if err := server.redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("cannot connect to redis: %v", err)
	}
	dbState := server.InitDb(ctx)
	server.dbState = &dbState
	fmt.Printf("dbState: %+v\n", dbState)
	if err = server.CreateServicesFromDb(ctx); err != nil {
		log.Fatal(err)
	}
	return server, nil
}

func NewGrpcServer(server *Server) *grpc.Server {
	privateKey := server.GetConfig().TokenSigningPrivateKey
	publicKey := privateKey.Public().(ed25519.PublicKey)
	// var freeQuotaRedisRateLimiter func(context.Context, interceptors.CallMeta) bool
	selectors := []grpc.UnaryServerInterceptor{
		selector.UnaryServerInterceptor(
			AuthMiddleware(publicKey),
			selector.MatchFunc(AuthMiddlewareSelector),
		),
		selector.UnaryServerInterceptor(
			grpcauth.UnaryServerInterceptor(AdminAuthMiddleware(
				[]int64{server.dbState.AdminAccountId}, publicKey)),
			selector.MatchFunc(AdminAuthMiddlewareSelector),
		),
	}
	if server.config.EnablePrexQuotaLimiter {
		selectors = append(selectors, selector.UnaryServerInterceptor(
			ratelimit.UnaryServerInterceptor(&FreeQuotaRedisRateLimiter{
				server.GetRedisClient(),
				server.GetConfig().FreeQuotaRefreshPeriod,
				server.dbState.FreeQuotaServiceId,
				publicKey,
			}),
			selector.MatchFunc(FreeQuotaRedisRateLimiterSelector),
		))
		selectors = append(selectors, selector.UnaryServerInterceptor(
			ratelimit.UnaryServerInterceptor(&QuotaRedisLimiter{
				Rdb:       server.GetRedisClient(),
				jwtSecret: publicKey,
			}),
			selector.MatchFunc(SoldQuotaRedisLimiterSelector),
		))
		selectors = append(selectors, selector.UnaryServerInterceptor(
			ActiveQuotaCheckTokenMiddleware(publicKey),
			selector.MatchFunc(func(ctx context.Context, callMeta interceptors.CallMeta) bool {
				return callMeta.FullMethod() == pb.Exchange_ActivateQuotaToken_FullMethodName
			}),
		))
	}
	if server.config.EnableServiceRegistrationWhitelist {
		selectors = append(selectors, selector.UnaryServerInterceptor(
			grpcauth.UnaryServerInterceptor(AdminAuthMiddleware(
				// TODO: support other users than admin to register services
				[]int64{server.dbState.AdminAccountId},
				publicKey,
			)),
			selector.MatchFunc(ServiceRegisterAuthMiddlewareSelector),
		))
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
