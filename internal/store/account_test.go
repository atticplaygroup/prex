package store_test

import (
	"context"
	"fmt"

	_ "github.com/lib/pq" // PostgreSQL driver
	"golang.org/x/crypto/bcrypt"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Checking deposit into database", Label("db"), func() {
	password := "passw0rd4test"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		Fail(fmt.Sprintf("Failed to hash password: %v", err))
	}

	BeforeEach(func() {
		RefreshDb(StoreTestDb, Migrations)
	})

	When("user deposits", func() {
		It("should deposit successfully", func() {
			ctx := context.Background()
			s := *StoreInstance
			account1, err := s.UpsertAccountTx(ctx, &store.UpsertAccountTxParams{
				Digest: "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhpJ",
				UpsertAccountParams: db.UpsertAccountParams{
					Username: "test_user_1",
					Password: string(hashedPassword),
					Balance:  1_000_000,
					Ttl: pgtype.Interval{
						Microseconds: 3600 * 24 * 30 * 1000,
						Valid:        true,
					},
					Privilege: "user",
				},
			})
			Expect(err).To(BeNil())
			Expect(account1.AccountID).To(Not(BeNil()))

			account, err := s.QueryBalance(ctx, account1.AccountID)
			Expect(err).To(BeNil())
			Expect(account.AccountID).To(Equal(account1.AccountID))
			Expect(account.Balance).To(Equal(int64(1_000_000)))

			newAccount, err := s.UpsertAccountTx(ctx, &store.UpsertAccountTxParams{
				Digest: "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhp1",
				UpsertAccountParams: db.UpsertAccountParams{
					Username: "test_user_1",
					Password: string(hashedPassword),
					Balance:  2_000_000,
					Ttl: pgtype.Interval{
						Microseconds: 0,
						Valid:        true,
					},
					Privilege: "user",
				},
			})
			Expect(err).To(BeNil())
			Expect(newAccount.AccountID).To(Equal(account1.AccountID))
			newAccount1, err := s.QueryBalance(ctx, account1.AccountID)
			Expect(err).To(BeNil())
			Expect(newAccount1.Balance).To(Equal(int64(3_000_000)))
		})
	})

	When("user duplicates deposit with the same digest", func() {
		It("should reject the deposit", func() {
			ctx := context.Background()
			s := *StoreInstance
			params := &store.UpsertAccountTxParams{
				Digest: "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhpJ",
				UpsertAccountParams: db.UpsertAccountParams{
					Username: "test_user_1",
					Password: string(hashedPassword),
					Balance:  1_000_000,
					Ttl: pgtype.Interval{
						Microseconds: 3600 * 24 * 30 * 1000,
						Valid:        true,
					},
					Privilege: "user",
				},
			}
			accountId, err := s.UpsertAccountTx(ctx, params)
			Expect(err).To(BeNil())
			Expect(accountId).To(Not(BeNil()))

			newAccountId, err := s.UpsertAccountTx(ctx, params)
			Expect(err).To(MatchError(ContainSubstring("duplicate key value violates unique constraint")))
			Expect(newAccountId).To(BeNil())
		})
	})
})
