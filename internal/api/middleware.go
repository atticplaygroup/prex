package api

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	headerAuthorize = "authorization"
	headerQuota     = "x-prex-quota"
)

// Adapted from grpcauth.AuthFromMD but allows other than authorization header
func TokenFromMD(ctx context.Context, headerField string) (string, error) {
	expectedScheme := "bearer"
	vals := metadata.ValueFromIncomingContext(ctx, headerField)
	if len(vals) == 0 {
		return "", status.Error(codes.Unauthenticated, "Request unauthenticated with "+expectedScheme)
	}
	scheme, token, found := strings.Cut(vals[0], " ")
	if !found {
		return "", status.Error(codes.Unauthenticated, "Bad authorization string")
	}
	if !strings.EqualFold(scheme, expectedScheme) {
		return "", status.Error(codes.Unauthenticated, "Request unauthenticated with "+expectedScheme)
	}
	return token, nil
}

func ParseHeaderJwt(
	ctx context.Context,
	claims jwt.Claims,
	jwtSecret ed25519.PublicKey,
	headerField string,
	withValdidation bool,
) (interface{}, error) {
	tokenString, err := TokenFromMD(ctx, headerField)
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

type QuotaTokenClaims struct {
	ServiceID     int64 `json:"service_id"`
	QuotaQuantity int64 `json:"quota_quantity"`
	*jwt.RegisteredClaims
}

func ParseQuotaToken(ctx context.Context, jwtSecret ed25519.PublicKey) (*QuotaTokenClaims, error) {
	rawQuotaClaims, err := ParseHeaderJwt(ctx, &QuotaTokenClaims{}, jwtSecret, headerQuota, true)
	if err != nil {
		return nil, err
	}
	quotaClaims, ok := rawQuotaClaims.(*QuotaTokenClaims)
	if !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"failed to parse quota token claims",
		)
	}
	tokenId := quotaClaims.ID
	if tokenId == "" {
		return nil, status.Error(codes.Internal, "jti should not be empty")
	}
	return quotaClaims, nil
}

type AuthClaims struct {
	AccountId int64
}

func ParseAuthToken(ctx context.Context, jwtSecret ed25519.PublicKey, withValidation bool) (*AuthClaims, error) {
	rawAuthclaims, err := ParseHeaderJwt(ctx, &jwt.RegisteredClaims{}, jwtSecret, headerAuthorize, withValidation)
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

type Resource interface {
	GetName() string
}

type ResourceParent interface {
	GetParent() string
}

type RequestWithAccount interface {
	GetAccountId() int64
}

func checkResourceField(req any, filedName string, value int64) error {
	if resourceRequest, ok := req.(Resource); ok {
		if resourceRequest == nil {
			return nil
		}
		if idSegments, err := utils.ParseResourceName(
			resourceRequest.GetName(), []string{filedName}); err != nil {
			return nil
		} else {
			if len(idSegments) != 1 {
				return fmt.Errorf("idSegment parse failed: %v", idSegments)
			}
			if idSegments[0] != value {
				return fmt.Errorf("%s in resource parent mismatch: %d vs %d", filedName, idSegments[0], value)
			}
			return nil
		}
	}
	return nil
}

func checkResourceParentField(req any, filedName string, value int64) error {
	if resourceRequest, ok := req.(ResourceParent); ok {
		if resourceRequest == nil {
			return nil
		}
		if idSegments, err := utils.ParseResourceName(
			resourceRequest.GetParent(), []string{filedName}); err != nil {
			return nil
		} else {
			if len(idSegments) != 1 {
				return fmt.Errorf("idSegment parse failed: %v", idSegments)
			}
			if idSegments[0] != value {
				return fmt.Errorf("%s in resource parent mismatch: %d vs %d", filedName, idSegments[0], value)
			}
			return nil
		}
	}
	return nil
}

func QuotaTokenValidityMiddleware(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		quotaClaims, err := ParseQuotaToken(ctx, jwtSecret)
		if err != nil || quotaClaims.ServiceID <= 0 {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"service id is invalid: %v",
				err,
			)
		}
		if err = checkResourceField(req, "services", quotaClaims.ServiceID); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %d not matching that in resource name: %v",
				quotaClaims.ServiceID,
				err,
			)
		}
		if err = checkResourceParentField(req, "services", quotaClaims.ServiceID); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %d not matching that in resource parent name: %v",
				quotaClaims.ServiceID,
				err,
			)
		}
		return handler(ctx, req)
	}
}

func AuthMiddleware(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	checkRequestWithAccount := func(req any, accountId int64) error {
		if req, ok := req.(RequestWithAccount); ok {
			if req == nil {
				return nil
			}
			actualAccountId := req.GetAccountId()
			if actualAccountId != accountId {
				return fmt.Errorf("account id in request mismatch: %d vs %d", actualAccountId, accountId)
			}
			return nil
		}
		return nil
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		authClaims, err := ParseAuthToken(ctx, jwtSecret, true)
		if err != nil || authClaims.AccountId <= 0 {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"account id is invalid: %v",
				err,
			)
		}
		if err = checkResourceField(req, "accounts", authClaims.AccountId); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %d not matching that in resource name: %v",
				authClaims.AccountId,
				err,
			)
		}
		if err = checkResourceParentField(req, "accounts", authClaims.AccountId); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %d not matching that in resource parent name: %v",
				authClaims.AccountId,
				err,
			)
		}
		if err = checkRequestWithAccount(req, authClaims.AccountId); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %d not matching that in request: %v",
				authClaims.AccountId,
				err,
			)
		}
		return handler(ctx, req)
	}
}

func ActiveQuotaCheckTokenMiddleware(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		reqParsed, ok := req.(*pb.ActivateQuotaTokenRequest)
		if !ok {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"[ActiveQuotaCheckTokenMiddleware] malformed request: %v",
				req,
			)
		}
		parsedToken, err := ParseQuotaToken(ctx, jwtSecret)
		if err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"invalid quota token: %v",
				err,
			)
		}
		if strconv.Itoa(int(reqParsed.GetOrderClaimId())) != parsedToken.ID {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"jti not matching: request %d vs header token %s",
				reqParsed.GetOrderClaimId(),
				parsedToken.ID,
			)
		}
		if reqParsed.GetServiceId() != parsedToken.ServiceID {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"service id not matching: request %d vs header token %d",
				reqParsed.GetServiceId(),
				parsedToken.ServiceID,
			)
		}
		if reqParsed.GetQuotaQuantity() != parsedToken.QuotaQuantity {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"quota quantity not matching: request %d vs header token %d",
				reqParsed.GetQuotaQuantity(),
				parsedToken.QuotaQuantity,
			)
		}
		if !reqParsed.GetExpireAt().AsTime().Equal(parsedToken.ExpiresAt.Time) {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"expire time not matching: request %v vs header token %v",
				reqParsed.GetExpireAt().AsTime(),
				parsedToken.ExpiresAt.Time,
			)
		}
		return handler(ctx, req)
	}
}

func AuthMiddlewareSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	for _, authNotRequired := range []string{
		pb.Exchange_Deposit_FullMethodName,
		pb.Exchange_Login_FullMethodName,
		pb.Exchange_Ping_FullMethodName,
		pb.Exchange_ListServices_FullMethodName,
		pb.Exchange_ListPaymentMethods_FullMethodName,
		pb.Exchange_GetChallenge_FullMethodName,
	} {
		if callMeta.FullMethod() == authNotRequired {
			return false
		}
	}
	return true
}

func AdminAuthMiddleware(adminIds []int64, jwtSecret ed25519.PublicKey) grpcauth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		authClaims, err := ParseAuthToken(ctx, jwtSecret, true)
		if err != nil || authClaims.AccountId <= 0 {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"account id is invalid: %v",
				err,
			)
		}
		found := false
		for _, adminId := range adminIds {
			if authClaims.AccountId == adminId {
				found = true
			}
		}
		if !found {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account_id = %d has no permission for this method",
				authClaims.AccountId,
			)
		}
		return ctx, nil
	}
}

func ServiceRegisterAuthMiddlewareSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	for _, authRequired := range []string{
		pb.Exchange_CreateService_FullMethodName,
		pb.Exchange_DeleteService_FullMethodName,
	} {
		if callMeta.FullMethod() == authRequired {
			return true
		}
	}
	return false
}

func AdminAuthMiddlewareSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	for _, authRequired := range []string{
		pb.Exchange_PruneAccounts_FullMethodName,
		pb.Exchange_BatchPruneFulfilledOrders_FullMethodName,
		pb.Exchange_BatchProcessWithdraws_FullMethodName,
		pb.Exchange_BatchMarkWithdraws_FullMethodName,
	} {
		if callMeta.FullMethod() == authRequired {
			return true
		}
	}
	return false
}

func IsReadOnlyMethod(methodName string) bool {
	for _, method := range []string{
		pb.Exchange_ListFulfilledOrders_FullMethodName,
		pb.Exchange_ListPaymentMethods_FullMethodName,
		pb.Exchange_ListServices_FullMethodName,
	} {
		if methodName == method {
			return true
		}
	}
	return false
}

func FreeQuotaRedisRateLimiterSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	return AuthMiddlewareSelector(ctx, callMeta) &&
		callMeta.FullMethod() == pb.Exchange_ClaimFreeToken_FullMethodName &&
		!IsReadOnlyMethod(callMeta.FullMethod())
}

func SoldQuotaRedisLimiterSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	return AuthMiddlewareSelector(ctx, callMeta) &&
		callMeta.FullMethod() != pb.Exchange_ClaimFreeToken_FullMethodName &&
		// TODO: claiming quota for external service should also cost sold quota.
		callMeta.FullMethod() != pb.Exchange_ClaimToken_FullMethodName &&
		callMeta.FullMethod() != pb.Exchange_ActivateQuotaToken_FullMethodName &&
		!IsReadOnlyMethod(callMeta.FullMethod())
}

type FreeQuotaRedisRateLimiter struct {
	Rdb                *redis.Client
	RefreshPeriod      time.Duration
	FreeQuotaServiceId int64
	jwtSecret          ed25519.PublicKey
}

func (l *FreeQuotaRedisRateLimiter) Limit(ctx context.Context) error {
	authClaims, err := ParseAuthToken(ctx, l.jwtSecret, true)
	if err != nil || authClaims.AccountId <= 0 {
		return status.Errorf(
			codes.Unauthenticated,
			"account id is invalid: %v",
			err,
		)
	}
	quotaKey := fmt.Sprintf("free-quota-rate-limit/%d", authClaims.AccountId)
	if ttl, err := l.Rdb.TTL(ctx, quotaKey).Result(); err != nil {
		return status.Error(
			codes.Internal,
			"failed to get cache for rate limit",
		)
	} else {
		if ttl.Seconds() == -1 {
			l.Rdb.Del(ctx, quotaKey).Err()
			return status.Error(
				codes.Internal,
				"bug: got unlimited ttl for rate limit",
			)
		} else if ttl.Seconds() >= 0 {
			return status.Errorf(
				codes.ResourceExhausted,
				"come back after %f seconds",
				ttl.Seconds(),
			)
		}
	}
	if err := l.Rdb.Set(ctx, quotaKey, "1", l.RefreshPeriod).Err(); err != nil {
		return status.Error(codes.Internal, "failed to set cache for rate limit")
	}
	return nil
}

type QuotaRedisLimiter struct {
	Rdb       *redis.Client
	jwtSecret ed25519.PublicKey
}

func (l *QuotaRedisLimiter) Limit(ctx context.Context) error {
	quotaClaims, err := ParseQuotaToken(ctx, l.jwtSecret)
	if err != nil {
		return status.Errorf(
			codes.PermissionDenied,
			"invalid quota token: %v",
			err,
		)
	}
	quotaKey := fmt.Sprintf("quota/%d/%s", quotaClaims.ServiceID, quotaClaims.ID)
	// quotaKey should be guaranteed to exist within valid time when token was issued
	// unless being used up and deleted
	if remaining, err := l.Rdb.DecrBy(ctx, quotaKey, utils.QUOTA_STEP).Result(); err != nil {
		return status.Error(
			codes.Internal,
			"check quota failed",
		)
	} else if remaining < 0 {
		_ = l.Rdb.Del(ctx, quotaKey).Err()
		return status.Error(
			codes.PermissionDenied,
			"token has been used up",
		)
	}
	return nil
}
