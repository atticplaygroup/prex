package store_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Record withdrawal and set at waiting status", Label("db"), func() {
	Context("with positive deposit", Ordered, func() {
		password := "passw0rd4test"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			Fail(fmt.Sprintf("Failed to hash password: %v", err))
		}

		var withdrawalId *int64
		var processingWithdrawId *int64
		var account *db.Account
		ctx := context.Background()
		s := *StoreInstance

		BeforeAll(func() {
			RefreshDb(StoreTestDb, Migrations)
			account, err = s.UpsertAccountTx(ctx, &store.UpsertAccountTxParams{
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
			if err != nil {
				Fail(fmt.Sprintf("Failed to deposit: %v", err))
			}
		})

		withdrawToAddress := func(chainAddress string) (*int64, error) {
			chainAddressBytes, err := hex.DecodeString(chainAddress[2:])
			if err != nil {
				log.Fatalf("Failed to decode hex string: %v", err)
			}
			withdrawTxParams := store.WithdrawTxParams{
				StartWithdrawalParams: db.StartWithdrawalParams{
					AccountID:       account.AccountID,
					WithdrawAddress: chainAddressBytes,
					Amount:          500_000,
					PriorityFee:     100_000,
				},
				WithdrawAll: false,
			}
			withdrawal, err := s.WithdrawTx(ctx, withdrawTxParams)
			if err != nil {
				return nil, err
			}
			return &withdrawal.WithdrawalID, nil
		}

		When("user withdraws with proper amount", func() {
			It("should record the withdrawal in waiting status", func() {
				chainAddress := "0xe789fb3f9e6e0736b648f3f33ff60bc0e4583583b2142cb2665bcc520635aac0"
				accountBefore, err := s.QueryBalance(ctx, account.AccountID)
				balanceBefore := accountBefore.Balance
				Expect(err).To(BeNil())

				withdrawalId, err = withdrawToAddress(chainAddress)
				Expect(err).To(BeNil())

				accountAfter, err := s.QueryBalance(ctx, account.AccountID)
				balanceAfter := accountAfter.Balance
				Expect(err).To(BeNil())
				Expect(balanceBefore - balanceAfter).To(BeEquivalentTo(600_000))

				withdrawals, err := s.ListWithdrawals(ctx, db.ListWithdrawalsParams{
					Limit:     2,
					Offset:    0,
					AccountID: account.AccountID,
				})

				Expect(err).To(BeNil())
				Expect(withdrawals).To(HaveLen(1))
				Expect(withdrawals[0].WithdrawalID).To(Equal(*withdrawalId))
				Expect(withdrawals[0].ProcessingWithdrawalID.Valid).To(Not(BeTrue()))
			})

			When("withdraw to the same address before canceling", func() {
				It("should fail to withdraw", func() {
					chainAddress := "0xe789fb3f9e6e0736b648f3f33ff60bc0e4583583b2142cb2665bcc520635aac0"
					_, err := withdrawToAddress(chainAddress)
					Expect(err).To(MatchError(ContainSubstring(
						"duplicate key value violates unique constraint \"withdrawals_withdraw_address_key\"",
					)))
				})
			})

			When("withdraw another address but insufficient balance", func() {
				It("should fail to withdraw", func() {
					chainAddress := "0x4f89910d450a3654e82bc3f573e24dfcbbeed23cb3b87de4883e890bfd952473"
					_, err := withdrawToAddress(chainAddress)
					Expect(err).To(MatchError(ContainSubstring(
						"violates check constraint \"accounts_balance_check\"",
					)))
				})
			})
		})

		When("user then cancels the withdrawal request", func() {
			It("should success", func() {
				err := s.CancelWithdrawTx(ctx, store.CancelWithdrawTxParams{
					WithdrawalId: *withdrawalId,
				})
				Expect(err).To(BeNil())

				account, err := s.QueryBalance(ctx, account.AccountID)
				Expect(err).To(BeNil())
				Expect(account.Balance).To(BeEquivalentTo(1_000_000))
			})
		})

		transactionDigest := "CeVpDXKKU3Gs89efej9pKiYYQyTzifE2BDxWwquUaUht"

		When("user withdraw again and got processed", func() {
			It("should have withdrawal record in processing status", func() {
				chainAddress := "0x4f89910d450a3654e82bc3f573e24dfcbbeed23cb3b87de4883e890bfd952473"
				var err error
				withdrawalId, err = withdrawToAddress(chainAddress)
				Expect(err).To(BeNil())

				processingWithdrawal, err := s.SetWithdrawalBatch(
					ctx, db.SetWithdrawalBatchParams{
						TransactionDigest:      transactionDigest,
						TransactionBytesBase64: "mock=",
						TotalPriorityFee:       100,
					})
				processingWithdrawId = &processingWithdrawal.ProcessingWithdrawalID
				Expect(err).To(BeNil())
				withdraws, err := s.ProcessWithdrawals(ctx, db.ProcessWithdrawalsParams{
					ProcessingWithdrawalID: pgtype.Int8{
						Int64: processingWithdrawal.ProcessingWithdrawalID,
						Valid: true,
					},
					WithdrawalIds: []int64{*withdrawalId},
				})
				Expect(err).To(BeNil())
				Expect(withdraws).To(HaveLen(1))
				Expect(withdraws[0].WithdrawalID).To(Equal(*withdrawalId))
				Expect(withdraws[0].Amount).To(BeEquivalentTo(500_000))
				Expect(withdraws[0].ProcessingWithdrawalID.Value()).To(
					BeEquivalentTo(*processingWithdrawId))
			})
		})

		When("user cancels withdrawal but all are not in waiting status", func() {
			It("should fail to cancel", func() {
				err := s.CancelWithdrawTx(ctx, store.CancelWithdrawTxParams{
					WithdrawalId: *withdrawalId,
				})
				Expect(err).To(MatchError(ContainSubstring("no rows in result set")))

				account, err := s.QueryBalance(ctx, account.AccountID)
				Expect(err).To(BeNil())
				Expect(account.Balance).To(BeEquivalentTo(400_000))
			})
		})

		When("successful processed withdrawals are removed", func() {
			It("should also delete in withdrawls", func() {
				succeedWithdrawal, err := s.SetWithdrawalSuccess(ctx, transactionDigest)
				Expect(err).To(BeNil())
				Expect(succeedWithdrawal.ProcessingWithdrawalID).To(BeEquivalentTo(
					*processingWithdrawId,
				))
				withdrawIds, err := s.CleanOldWithdrawals(
					ctx, pgtype.Timestamptz{
						Time:  time.Now().Add(5 * time.Second),
						Valid: true,
					})
				Expect(err).To(BeNil())
				Expect(withdrawIds).To(HaveLen(1))
				Expect(withdrawIds[0].ProcessingWithdrawalID).To(
					BeEquivalentTo(*processingWithdrawId))

				withdrawals, err := s.ListWithdrawals(ctx, db.ListWithdrawalsParams{
					Limit:  100,
					Offset: 0,
				})
				Expect(err).To(BeNil())
				Expect(len(withdrawals)).To(BeEquivalentTo(0))
			})
		})
	})
})
