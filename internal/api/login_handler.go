package api

import (
	"context"
	"database/sql"

	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	account, err := s.store.GetAccount(ctx, req.GetUsername())
	if err != nil {
		if err == sql.ErrNoRows || err.Error() == "no rows in result set" {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"cannot find username %v",
				req.GetUsername(),
			)
		}
		return nil, status.Errorf(
			codes.Internal,
			"database access failed: %v",
			err,
		)
	}
	err = bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.GetPassword()))
	if err != nil {
		return nil, status.Error(
			codes.PermissionDenied,
			"username exists but password incorrect",
		)
	}
	jwt, err := s.auth.GenerateJWT(account.AccountID)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to generate jwt: %v",
			err,
		)
	}
	accountResponse := utils.FormatAccount(account)
	return &pb.LoginResponse{
		AccessToken: jwt,
		Account:     &accountResponse,
	}, nil
}
