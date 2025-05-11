package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) isFreeService(name string) (bool, error) {
	idSegments, err := utils.ParseResourceName(
		name, []string{"services", "fulfilled-orders"},
	)
	if err != nil || len(idSegments) != 2 {
		return false, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	return idSegments[0] == s.dbState.FreeQuotaServiceId, nil
}

func (s *Server) ClaimFreeToken(
	ctx context.Context, req *pb.ClaimTokenRequest,
) (*pb.ClaimTokenResponse, error) {
	if isFree, err := s.isFreeService(req.GetName()); err != nil {
		return nil, err
	} else if isFree {
		return s.doClaimToken(ctx, req)
	} else {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"this api only allows to claim free quota",
		)
	}
}

func (s *Server) ClaimToken(
	ctx context.Context, req *pb.ClaimTokenRequest,
) (*pb.ClaimTokenResponse, error) {
	if isFree, err := s.isFreeService(req.GetName()); err != nil {
		return nil, err
	} else if !isFree {
		return s.doClaimToken(ctx, req)
	} else {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"this api only allows to claim sold quota",
		)
	}
}

func (s *Server) doClaimToken(
	ctx context.Context, req *pb.ClaimTokenRequest,
) (*pb.ClaimTokenResponse, error) {
	idSegments, err := utils.ParseResourceName(
		req.GetName(), []string{"services", "fulfilled-orders"},
	)
	if err != nil || len(idSegments) != 2 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}

	quantity, tokenClaims, err := (*s.tokenPolicies[idSegments[0]]).ParseAndVerifyQuantity(req.GetArgJson())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"cannot parse claims: %v",
			err,
		)
	}
	audienceAddress, err := utils.HexToBytes32(req.GetAudience())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"cannot parse audience address: %v",
			err,
		)
	}
	claim, err := s.store.ClaimOrderTx(
		ctx,
		store.ClaimOrderTxParams{
			AccountID: req.GetAccountId(),
			NewClaimOrderParams: db.NewClaimOrderParams{
				OrderFulfillmentID: idSegments[1],
				AudienceAddress:    audienceAddress,
				ClaimQuantity:      *quantity,
			},
		},
	)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to claim order: %v",
			err,
		)
	}

	var expiration time.Time
	if claim.ServiceId == s.dbState.FreeQuotaServiceId {
		expiration = time.Now().Add(s.config.FreeQuotaRefreshPeriod)
	} else {
		expiration = claim.Expiration
	}
	claims := jwt.MapClaims{
		// No "sub" encoded inside token to allow OTC trading
		"aud":                  claim.AudienceAddress,
		"exp":                  jwt.NewNumericDate(expiration),
		"order_id":             claim.OrderId,
		"order_fulfillment_id": claim.OrderFulfillmentId,
		"seller_id":            claim.SellerId,
		"service_id":           claim.ServiceId,
		"quota_quantity":       claim.Quantity,
		"jti":                  strconv.Itoa(int(claim.OrderClaimId)),
	}
	for k, v := range tokenClaims {
		if _, ok := claims[k]; ok {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"custom fields conflict with predefined: %s",
				k,
			)
		}
		claims[k] = v
	}

	token := jwt.NewWithClaims(&jwt.SigningMethodEd25519{}, claims)
	token.Header["kid"] = s.config.TokenSigningKeyId
	jwt, err := token.SignedString(s.config.TokenSigningPrivateKey)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to sign jwt: %v",
			err,
		)
	}
	return &pb.ClaimTokenResponse{
		Token: jwt,
	}, nil
}

func (s *Server) ActivateQuotaToken(
	ctx context.Context, req *pb.ActivateQuotaTokenRequest,
) (*pb.ActivateQuotaTokenResponse, error) {
	quotaKey := fmt.Sprintf("quota/%d/%d", req.GetServiceId(), req.GetOrderClaimId())
	quotaValue := fmt.Sprintf("%d", req.GetQuotaQuantity())
	var ttl time.Duration
	if req.GetServiceId() == s.dbState.FreeQuotaServiceId {
		ttl = s.config.FreeQuotaRefreshPeriod
	} else if req.GetServiceId() == s.dbState.SoldQuotaServiceId {
		ttl = time.Until(req.GetExpireAt().AsTime())
	} else {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"no quota to activate for service %d",
			req.GetServiceId(),
		)
	}
	if ttl <= 0 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"ttl must be positive but got %v",
			ttl,
		)
	}
	if err := s.redisClient.Set(ctx, quotaKey, quotaValue, ttl).Err(); err != nil {
		return nil, status.Error(codes.Internal, "failed to set quota")
	}
	return &pb.ActivateQuotaTokenResponse{
		Success: true,
	}, nil
}
