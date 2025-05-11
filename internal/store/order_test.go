package store_test

import (
	"context"
	"fmt"
	"log"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/token"
	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/jackc/pgx/v5/pgtype"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var serviceId *int64
var sellerAccount, buyerAccount, poorBuyerAccount *db.Account

var _ = Describe("Match multiple orders", Label("db"), Ordered, func() {
	password := "passw0rd4test"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		Fail(fmt.Sprintf("Failed to hash password: %v", err))
	}

	s := *StoreInstance
	var firstOrder, secondOrder *db.ActiveOrder
	BeforeAll(func() {
		RefreshDb(StoreTestDb, Migrations)

		ctx := context.Background()
		s := store.NewStore(Conn)
		firstParameter := &store.UpsertAccountTxParams{
			Digest: "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhpJ",
			UpsertAccountParams: db.UpsertAccountParams{
				Username: "test_user_1",
				Password: string(hashedPassword),
				Balance:  1_000_000,
				Ttl: pgtype.Interval{
					Microseconds: 10 * 24 * 3600 * 1000 * 1000,
					Valid:        true,
				},
				Privilege: "user",
			},
		}
		sellerAccount, err = s.UpsertAccountTx(ctx, firstParameter)
		if err != nil {
			Fail(fmt.Sprintf("Failed to create seller: %v", err))
		}

		secondParameter := firstParameter
		secondParameter.Digest = "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhp1"
		secondParameter.Username = "test_user_2"
		buyerAccount, err = s.UpsertAccountTx(ctx, secondParameter)
		if err != nil {
			Fail(fmt.Sprintf("Failed to create buyer: %v", err))
		}

		globalId1, err := utils.Uuid2bytes("86f9379b-ad74-4320-b503-5834c5167ec9")
		if err != nil {
			Fail(fmt.Sprintf("Failed to create uuid 1: %v", err))
		}
		service, err := s.CreateService(context.Background(), db.CreateServiceParams{
			ServiceGlobalID: pgtype.UUID{
				Bytes: [16]byte(globalId1),
				Valid: true,
			},
			DisplayName:       "sample service 1",
			TokenPolicyType:   token.PRODUCT_TOKEN_POLICY_NAME,
			TokenPolicyConfig: `{"unit_price":1}`,
		})
		if err != nil {
			Fail(fmt.Sprintf("Failed to create service: %v", err))
		}
		serviceId = &service.ServiceID

		thirdParameter := secondParameter
		thirdParameter.Digest = "DdzbG47u5MDUrSVArmVYmvnpDvspgKeAXxzgq2cNnhp2"
		thirdParameter.Username = "test_user_3"
		thirdParameter.Balance = 100
		poorBuyerAccount, err = s.UpsertAccountTx(ctx, thirdParameter)
		if err != nil {
			Fail(fmt.Sprintf("Failed to create poor buyer: %v", err))
		}

		globalId2, err := utils.Uuid2bytes("50e7d2fb-ccd7-48c2-aba5-ac285f67dfc1")
		if err != nil {
			Fail(fmt.Sprintf("Failed to create uuid 2: %v", err))
		}
		service, err = s.CreateService(context.Background(), db.CreateServiceParams{
			ServiceGlobalID: pgtype.UUID{
				Bytes: [16]byte(globalId2),
				Valid: true,
			},
			DisplayName:       "sample service 2",
			TokenPolicyType:   token.PRODUCT_TOKEN_POLICY_NAME,
			TokenPolicyConfig: `{"unit_price":1}`,
		})
		if err != nil {
			Fail(fmt.Sprintf("Failed to create service: %v", err))
		}
		serviceId = &service.ServiceID
	})

	It("should init 2 accounts successfully", func() {
		Expect(sellerAccount.AccountID).To(Not(BeNil()))
		Expect(buyerAccount.AccountID).To(Not(BeNil()))
		Expect(buyerAccount.AccountID).To(Not(Equal(sellerAccount.AccountID)))
	})

	When("create orders when db is empty", func() {
		It("should create successfully", func() {
			firstOrder1, err := s.CreateOrder(context.Background(), db.CreateOrderParams{
				SellerID:  sellerAccount.AccountID,
				ServiceID: *serviceId,
				AskPrice:  100,
				Quantity:  30,
				OrderExpireTime: pgtype.Timestamptz{
					Time:  time.Now().Add(time.Duration(20 * 24 * time.Hour)),
					Valid: true,
				},
				ServiceExpireTime: pgtype.Timestamptz{
					Time:  time.Now().Add(time.Duration(20 * 24 * time.Hour)),
					Valid: true,
				},
			})
			Expect(err).To(BeNil())
			firstOrder = &firstOrder1
			secondOrder1, err := s.CreateOrder(context.Background(), db.CreateOrderParams{
				SellerID:  sellerAccount.AccountID,
				ServiceID: *serviceId,
				AskPrice:  80,
				Quantity:  50,
				OrderExpireTime: pgtype.Timestamptz{
					Time:  time.Now().Add(time.Duration(20 * 24 * time.Hour)),
					Valid: true,
				},
				ServiceExpireTime: pgtype.Timestamptz{
					Time:  time.Now().Add(time.Duration(20 * 24 * time.Hour)),
					Valid: true,
				},
			})
			Expect(err).To(BeNil())
			secondOrder = &secondOrder1
		})
	})

	When("deal price is more than its balance", func() {
		It("should fail to bid", func() {
			ctx := context.Background()
			_, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      100,
					BuyerID:       poorBuyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   10,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(MatchError(ContainSubstring("violates check constraint \"accounts_balance_check\"")))
			poorBuyer, err := s.QueryBalance(ctx, poorBuyerAccount.AccountID)
			Expect(err).To(BeNil())
			Expect(poorBuyer.Balance).To(BeEquivalentTo(100))
		})
	})

	When("buyer matches order with too low bids", func() {
		It("should fail to match any order", func() {
			ctx := context.Background()
			_, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      70,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   10,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(MatchError(ContainSubstring("no rows in result set")))
			buyer, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			Expect(buyer.Balance).To(BeEquivalentTo(1_000_000))
		})
	})

	When("buyer matches order with service expiration time too late", func() {
		It("should fail to match any order", func() {
			ctx := context.Background()
			_, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      70,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   10,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now().Add(20 * 24 * time.Hour), Valid: true},
				},
			})
			Expect(err).To(MatchError(ContainSubstring("no rows in result set")))
			buyer, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			Expect(buyer.Balance).To(BeEquivalentTo(1_000_000))
		})
	})

	When("buyer matches order with proper bid and quantity", func() {
		It("should succeed to match with ask price", func() {
			ctx := context.Background()
			buyerAccountBefore, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceBefore := buyerAccountBefore.Balance
			sellerAccountBefore, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceBefore := sellerAccountBefore.Balance

			order, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      90,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   10,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(BeNil())
			Expect(order.DealPrice).To(BeEquivalentTo(80))
			Expect(order.DealQuantity).To(BeEquivalentTo(10))
			Expect(order.BuyerID).To(Equal(buyerAccount.AccountID))
			Expect(order.SellerID).To(Equal(sellerAccount.AccountID))
			Expect(order.OrderID).To(Equal(secondOrder.OrderID))

			buyerAccountAfter, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceAfter := buyerAccountAfter.Balance
			sellerAccountAfter, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceAfter := sellerAccountAfter.Balance
			Expect(sellerBalanceAfter - sellerBalanceBefore).To(BeEquivalentTo(800))
			Expect(buyerBalanceAfter - buyerBalanceBefore).To(BeEquivalentTo(-800))
		})
	})

	When("buyer matches order with proper bid but exceeding quantity", func() {
		It("should succeed to match with an order but not fully fulfill quantity requirement", func() {
			ctx := context.Background()
			buyerAccountBefore, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceBefore := buyerAccountBefore.Balance
			sellerAccountBefore, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceBefore := sellerAccountBefore.Balance

			order, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      90,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   100,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(BeNil())
			Expect(order.DealPrice).To(BeEquivalentTo(80))
			Expect(order.DealQuantity).To(BeEquivalentTo(40))
			Expect(order.BuyerID).To(Equal(buyerAccount.AccountID))
			Expect(order.SellerID).To(Equal(sellerAccount.AccountID))
			Expect(order.OrderID).To(Equal(secondOrder.OrderID))

			buyerAccountAfter, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceAfter := buyerAccountAfter.Balance
			sellerAccountAfter, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceAfter := sellerAccountAfter.Balance
			Expect(sellerBalanceAfter - sellerBalanceBefore).To(BeEquivalentTo(3200))
			Expect(buyerBalanceAfter - buyerBalanceBefore).To(BeEquivalentTo(-3200))
		})
	})

	var orderToClaim db.FulfilledOrder

	When("continue to match with higher price", func() {
		It("should succeed to match", func() {
			ctx := context.Background()
			buyerAccountBefore, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceBefore := buyerAccountBefore.Balance
			sellerAccountBefore, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceBefore := sellerAccountBefore.Balance

			order, err := s.MatchOrderTx(ctx, store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      120,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   60,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(BeNil())
			Expect(order.DealPrice).To(BeEquivalentTo(100))
			Expect(order.DealQuantity).To(BeEquivalentTo(30))
			Expect(order.BuyerID).To(Equal(buyerAccount.AccountID))
			Expect(order.SellerID).To(Equal(sellerAccount.AccountID))
			Expect(order.OrderID).To(Equal(firstOrder.OrderID))

			buyerAccountAfter, err := s.QueryBalance(ctx, buyerAccount.AccountID)
			Expect(err).To(BeNil())
			buyerBalanceAfter := buyerAccountAfter.Balance
			sellerAccountAfter, err := s.QueryBalance(ctx, sellerAccount.AccountID)
			Expect(err).To(BeNil())
			sellerBalanceAfter := sellerAccountAfter.Balance
			Expect(sellerBalanceAfter - sellerBalanceBefore).To(BeEquivalentTo(3000))
			Expect(buyerBalanceAfter - buyerBalanceBefore).To(BeEquivalentTo(-3000))

			orderToClaim = *order
		})
	})

	When("match when there is no order with non-zero quantity", func() {
		It("should fail to match", func() {
			_, err := s.MatchOrderTx(context.Background(), store.MatchOrderTxParams{
				db.MatchOneOrderParams{
					BidPrice:      120,
					BuyerID:       buyerAccount.AccountID,
					ServiceID:     *serviceId,
					BidQuantity:   10,
					MinExpireTime: pgtype.Timestamptz{Time: time.Now(), Valid: true},
				},
			})
			Expect(err).To(MatchError(ContainSubstring("no rows in result set")))
		})
	})

	recipientAddress := "0xd22cd969ddf3a93c992d2a586afc80d966ee0f078a3af9de049eb06284f2b51a"
	audienceAddress, err := utils.HexToBytes32(recipientAddress)
	if err != nil {
		log.Fatalf("audience address %s decode failed: %v", recipientAddress, err)
	}

	When("claim other user's fulfilled order", func() {
		It("should fail", func() {
			_, err := s.ClaimOrderTx(context.Background(), store.ClaimOrderTxParams{
				AccountID: sellerAccount.AccountID,
				NewClaimOrderParams: db.NewClaimOrderParams{
					OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
					AudienceAddress:    audienceAddress,
					ClaimQuantity:      25,
				},
			})
			Expect(err).To(Not(BeNil()))
		})
	})

	When("claim an exceeding quantity of a fulfilled order", func() {
		It("should fail", func() {
			_, err := s.ClaimOrderTx(context.Background(), store.ClaimOrderTxParams{
				AccountID: orderToClaim.BuyerID,
				NewClaimOrderParams: db.NewClaimOrderParams{
					OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
					AudienceAddress:    audienceAddress,
					ClaimQuantity:      40,
				},
			})
			Expect(err).To(Not(BeNil()))
		})
	})

	When("claim a proper quantity of a fulfilled order", func() {
		It("should succeed", func() {
			claim, err := s.ClaimOrderTx(context.Background(), store.ClaimOrderTxParams{
				AccountID: orderToClaim.BuyerID,
				NewClaimOrderParams: db.NewClaimOrderParams{
					OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
					AudienceAddress:    audienceAddress,
					ClaimQuantity:      10,
				},
			})
			Expect(err).To(BeNil())
			Expect(claim.Quantity).To(BeEquivalentTo(10))
			Expect(claim.ServiceId).To(Equal(*serviceId))
			Expect(claim.OrderFulfillmentId).To(Equal(orderToClaim.OrderFulfillmentID))
			Expect(claim.OrderId).To(Equal(orderToClaim.OrderID))
			Expect(claim.SellerId).To(Equal(orderToClaim.SellerID))
			Expect(claim.AudienceAddress).To(Equal(recipientAddress))
		})
	})

	When("claim again with an exceeding quantity", func() {
		It("should fail", func() {
			_, err := s.ClaimOrderTx(context.Background(), store.ClaimOrderTxParams{
				AccountID: orderToClaim.BuyerID,
				NewClaimOrderParams: db.NewClaimOrderParams{
					OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
					AudienceAddress:    audienceAddress,
					ClaimQuantity:      25,
				},
			})
			Expect(err).To(Not(BeNil()))
		})
	})

	When("claim a proper quantity of a fulfilled order", func() {
		It("should succeed", func() {
			claim, err := s.ClaimOrderTx(context.Background(), store.ClaimOrderTxParams{
				AccountID: orderToClaim.BuyerID,
				NewClaimOrderParams: db.NewClaimOrderParams{
					OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
					AudienceAddress:    audienceAddress,
					ClaimQuantity:      20,
				},
			})
			Expect(err).To(BeNil())
			Expect(claim.Quantity).To(BeEquivalentTo(20))
			Expect(claim.ServiceId).To(Equal(*serviceId))
			Expect(claim.OrderFulfillmentId).To(Equal(orderToClaim.OrderFulfillmentID))
			Expect(claim.OrderId).To(Equal(orderToClaim.OrderID))
			Expect(claim.SellerId).To(Equal(orderToClaim.SellerID))
			Expect(claim.AudienceAddress).To(Equal(recipientAddress))

			updatedOrder, err := s.QueryFulfilledOrder(context.Background(), db.QueryFulfilledOrderParams{
				BuyerID:            orderToClaim.BuyerID,
				OrderFulfillmentID: orderToClaim.OrderFulfillmentID,
			})
			Expect(err).To(BeNil())
			Expect(updatedOrder.RemainingQuantity).To(BeEquivalentTo(0))
			Expect(updatedOrder.DealQuantity).To(BeEquivalentTo(30))
		})
	})
})
