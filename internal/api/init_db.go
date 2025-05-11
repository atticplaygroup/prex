package api

import (
	"context"
	"database/sql"
	"log"
	"math"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/utils"
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

func (s *Server) initDbService(
	ctx context.Context,
	uuid [16]byte,
	displayName string,
) int64 {
	pguuid := pgtype.UUID{Bytes: uuid, Valid: true}
	service, err := s.store.FindServiceByGlobalId(ctx, pguuid)
	if err == nil {
		return service.ServiceID
	} else if err != sql.ErrNoRows && err.Error() != "no rows in result set" {
		log.Fatalf("failed to find service by global id: %v", err)
	}
	service, err = s.store.CreateService(ctx, db.CreateServiceParams{
		ServiceGlobalID:   pguuid,
		DisplayName:       displayName,
		TokenPolicyConfig: `{"unit_price":1}`,
		TokenPolicyType:   "exchange.prex.proto.ProductTokenPolicy",
	})
	if err != nil {
		log.Fatalf("failed to create %s: %v", displayName, err)
	}
	return service.ServiceID
}

func (s *Server) initDbSoldServiceOrder(
	ctx context.Context,
	sellerId int64,
	serviceId int64,
) {
	VeryLongTtl := time.Duration(24*30*12*99) * time.Hour
	_, err := s.store.CreateOrder(ctx, db.CreateOrderParams{
		SellerID:  sellerId,
		ServiceID: serviceId,
		AskPrice:  0,
		Quantity:  math.MaxInt64,
		OrderExpireTime: pgtype.Timestamptz{
			Time:  time.Now().Add(VeryLongTtl),
			Valid: true,
		},
		ServiceExpireTime: pgtype.Timestamptz{
			Time:  time.Now().Add(VeryLongTtl),
			Valid: true,
		},
	})
	if err != nil {
		log.Fatalf("failed to create free quota sell order: %v", err)
	}
}

func (s *Server) initAdminQuota(ctx context.Context, adminAccountId, serviceId int64) {
	tmpAdminAccountId := s.initDbAdminAccount(ctx, "tmp_admin")
	s.initDbSoldServiceOrder(ctx, tmpAdminAccountId, serviceId)
	_, err := s.store.MatchOrderTx(ctx, store.MatchOrderTxParams{
		MatchOneOrderParams: db.MatchOneOrderParams{
			BidPrice:    0,
			BidQuantity: math.MaxInt64,
			ServiceID:   serviceId,
			BuyerID:     adminAccountId,
			MinExpireTime: pgtype.Timestamptz{
				Time:  time.Now(),
				Valid: true,
			},
		},
	})
	if err != nil {
		log.Fatalf("failed to get sold order for service %d: %v", serviceId, err)
	}
}

type DbState struct {
	AdminAccountId     int64
	FreeQuotaServiceId int64
	SoldQuotaServiceId int64
}

func (s *Server) InitDb(
	ctx context.Context,
) DbState {
	adminAccountId := s.initDbAdminAccount(ctx, s.config.AdminUsername)
	freeQuotaServiceUuid, _ := utils.Uuid2bytes("00000000-0000-0000-0000-000000000000")
	freeQuotaServiceId := s.initDbService(ctx, [16]byte(freeQuotaServiceUuid), "free quota service")
	soldQuotaServiceUuid, _ := utils.Uuid2bytes("00000000-0000-0000-0000-000000000001")
	soldQuotaServiceId := s.initDbService(ctx, [16]byte(soldQuotaServiceUuid), "sold quota service")
	s.initDbSoldServiceOrder(ctx, adminAccountId, freeQuotaServiceId)
	s.initAdminQuota(ctx, adminAccountId, soldQuotaServiceId)
	return DbState{
		AdminAccountId:     adminAccountId,
		FreeQuotaServiceId: freeQuotaServiceId,
		SoldQuotaServiceId: soldQuotaServiceId,
	}
}
