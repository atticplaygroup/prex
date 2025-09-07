package api

import (
	"context"
	"crypto/ed25519"
	"strconv"
	"strings"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	headerAuthorize = "authorization"
)

func parseBearer(authString string) (string, error) {
	expectedScheme := "bearer"
	scheme, token, found := strings.Cut(authString, " ")
	if !found {
		return "", status.Error(codes.Unauthenticated, "Bad authorization string")
	}
	if !strings.EqualFold(scheme, expectedScheme) {
		return "", status.Error(codes.Unauthenticated, "Request unauthenticated with "+expectedScheme)
	}
	return token, nil
}

func ParseHeaderJwt(
	authString string,
	claims jwt.Claims,
	jwtSecret ed25519.PublicKey,
	headerField string,
	withValdidation bool,
) (interface{}, error) {
	tokenString, err := parseBearer(authString)
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid bearer token: %v",
			err,
		)
	}
	options := []jwt.ParserOption{}
	if !withValdidation {
		// TODO: add jwt.WithAudience and WithIssuer to check them
		options = append(options, jwt.WithoutClaimsValidation())
	}
	token, err := jwt.ParseWithClaims(
		tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Validate the algorithm
			if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
				return nil, status.Errorf(
					codes.Unauthenticated,
					"invalid signing method %s",
					token.Method.Alg(),
				)
			}
			return jwtSecret, nil
		},
		options...,
	)
	if err != nil || !token.Valid {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid token of %s: %v",
			headerField,
			err,
		)
	}
	return token.Claims, nil
}

type AuthClaims struct {
	AccountId int64
}

func ParseAuthToken(authString string, jwtSecret ed25519.PublicKey, withValidation bool) (*AuthClaims, error) {
	rawAuthclaims, err := ParseHeaderJwt(authString, &jwt.RegisteredClaims{}, jwtSecret, headerAuthorize, withValidation)
	if err != nil {
		return nil, err
	}
	authClaims, ok := rawAuthclaims.(*jwt.RegisteredClaims)
	if !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"failed to parse auth claims: %+v",
			rawAuthclaims,
		)
	}
	subject, err := authClaims.GetSubject()
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"cannot find subject in jwt",
		)
	}
	accountId, err := strconv.Atoi(subject)
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"subject is not parsable as account id int64",
		)
	}
	return &AuthClaims{AccountId: int64(accountId)}, nil
}

func protoValidation(req any, v protovalidate.Validator) error {
	m, ok := req.(proto.Message)
	if !ok {
		return status.Error(
			codes.InvalidArgument,
			"failed to parse proto",
		)
	}
	if err := v.Validate(m); err != nil {
		return status.Errorf(
			codes.InvalidArgument,
			"failed to parse proto: %s",
			err.Error(),
		)
	}
	return nil
}

func NewGrpcValidationInterceptor(v protovalidate.Validator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := protoValidation(req, v); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func NewConnectValidationInterceptor(v protovalidate.Validator) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			if err := protoValidation(req.Any(), v); err != nil {
				return nil, err
			}
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func NewConnectAuthInterceptor(
	publicKey ed25519.PublicKey,
) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			for _, authNotRequired := range []string{
				pb.ExchangeService_Deposit_FullMethodName,
				pb.ExchangeService_Login_FullMethodName,
				pb.ExchangeService_Ping_FullMethodName,
				pb.ExchangeService_ListPaymentMethods_FullMethodName,
				pb.ExchangeService_GetChallenge_FullMethodName,
			} {
				if req.Spec().Procedure == authNotRequired {
					return next(ctx, req)
				}
			}

			authClaims, err := ParseAuthToken(req.Header().Get(headerAuthorize), publicKey, true)
			if err != nil || authClaims.AccountId <= 0 {
				return nil, status.Errorf(
					codes.Unauthenticated,
					"account id is invalid: %v",
					err,
				)
			}
			return next(context.WithValue(ctx, utils.KEY_ACCOUNT_ID, authClaims.AccountId), req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}

func NewGrpcAuthInterceptor(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		headerField := headerAuthorize
		vals := metadata.ValueFromIncomingContext(ctx, headerField)
		if len(vals) == 0 {
			return nil, status.Error(codes.Unauthenticated, "Request unauthenticated with "+headerField)
		}
		authClaims, err := ParseAuthToken(vals[0], jwtSecret, true)
		if err != nil || authClaims.AccountId <= 0 {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"account id is invalid: %v",
				err,
			)
		}
		return handler(context.WithValue(ctx, utils.KEY_ACCOUNT_ID, authClaims.AccountId), req)
	}
}

func AuthMiddlewareSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	for _, authNotRequired := range []string{
		pb.ExchangeService_Deposit_FullMethodName,
		pb.ExchangeService_Login_FullMethodName,
		pb.ExchangeService_Ping_FullMethodName,
		pb.ExchangeService_ListPaymentMethods_FullMethodName,
		pb.ExchangeService_GetChallenge_FullMethodName,
	} {
		if callMeta.FullMethod() == authNotRequired {
			return false
		}
	}
	return true
}
