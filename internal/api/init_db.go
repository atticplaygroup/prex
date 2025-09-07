package api

import (
	"context"
	"database/sql"
	"log"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) initDbAdminAccount(
	ctx context.Context,
	adminUsername string,
) int64 {
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(s.config.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("cannot hash admin password: %v", err)
	}
	veryLongTtl := pgtype.Interval{
		Months: 12 * 99,
		Valid:  true,
	}
	account1, err := s.store.GetAccount(ctx, adminUsername)
	if err == nil {
		return account1.AccountID
	} else if err != sql.ErrNoRows && err.Error() != "no rows in result set" {
		log.Fatalf("failed to find admin account by username `%s`: %v", adminUsername, err)
	}
	account, err := s.store.UpsertAccountTx(
		ctx, &store.UpsertAccountTxParams{
			UpsertAccountParams: db.UpsertAccountParams{
				Username:  adminUsername,
				Password:  string(hashedPassword),
				Privilege: "admin",
				Balance:   0,
				Ttl:       veryLongTtl,
			},
			// random string to avoid conflicts
			// TODO: make the format uniform as other digests
			Digest: uuid.NewString(),
			Epoch:  0,
		})
	if err != nil {
		log.Fatalf("failed to add admin account: %v", err)
	}
	return account.AccountID
}

type DbState struct {
	AdminAccountId int64
}

func (s *Server) InitDb(
	ctx context.Context,
) DbState {
	adminAccountId := s.initDbAdminAccount(ctx, s.config.AdminUsername)
	return DbState{
		AdminAccountId: adminAccountId,
	}
}
