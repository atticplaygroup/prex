package api

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Token struct {
	*jwt.RegisteredClaims
	Quantity int64       `json:"quantity"`
	Usage    pb.JwtUsage `json:"usage"`
}

func (s *Server) generateJwt(audience string, quantity int64) (string, error) {
	claims := &Token{
		// No "sub" encoded inside token needed
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    s.config.TokenSigningKeyId,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.config.TokenTtl)),
			ID:        uuid.NewString(),
		},
		Quantity: quantity,
		Usage:    pb.JwtUsage_JWT_USAGE_CREATE_SESSION,
	}

	token := jwt.NewWithClaims(&jwt.SigningMethodEd25519{}, claims)
	token.Header["kid"] = s.config.TokenSigningKeyId
	jwt, err := token.SignedString(s.config.TokenSigningPrivateKey)
	if err != nil {
		return "", status.Errorf(
			codes.InvalidArgument,
			"failed to sign jwt: %v",
			err,
		)
	}
	return jwt, err
}

func (s *Server) BuyToken(
	ctx context.Context,
	connectReq *connect.Request[pb.BuyTokenRequest],
) (*connect.Response[pb.BuyTokenResponse], error) {
	req := connectReq.Msg
	accountId, ok := ctx.Value(utils.KEY_ACCOUNT_ID).(int64)
	if !ok {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"failed to get account id",
		)
	}
	tx, err := s.store.GetConn().Begin(context.Background())
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to begin transaction: %v",
			err,
		)
	}
	defer tx.Rollback(context.Background())
	qtx := s.store.Queries.WithTx(tx)
	_, err = s.store.BuyTokenTx(ctx, qtx, &store.BuyTokenTxParams{
		Req:     req,
		BuyerID: accountId,
	})
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to execute transaction: %v",
			err,
		)
	}
	jwt, err := s.generateJwt(req.GetAudience(), req.GetAmount())
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to generate jwt: %v",
			err,
		)
	}
	if err = tx.Commit(context.Background()); err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to commit transaction: %v",
			err,
		)
	}
	return connect.NewResponse(&pb.BuyTokenResponse{
		Token: jwt,
	}), nil
}
