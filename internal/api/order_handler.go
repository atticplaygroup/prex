package api

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/store"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Server) CreateSellOrder(
	ctx context.Context, req *pb.CreateSellOrderRequest,
) (*pb.CreateSellOrderResponse, error) {
	idSegments, err := utils.ParseResourceName(req.Parent, []string{"services"})
	if err != nil || len(idSegments) != 1 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	serviceId := idSegments[0]
	if req.GetAccountId() != req.GetSellOrder().GetSellerId() {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"account id %d != seller id %d",
			req.GetAccountId(),
			req.GetSellOrder().GetSellerId(),
		)
	}
	order, err := s.store.CreateOrder(ctx, db.CreateOrderParams{
		SellerID:  req.GetAccountId(),
		ServiceID: serviceId,
		AskPrice:  req.GetSellOrder().GetAskPrice(),
		Quantity:  req.GetSellOrder().GetQuantity(),
		OrderExpireTime: pgtype.Timestamptz{
			Time:  req.GetSellOrder().GetServiceExpireTime().AsTime(),
			Valid: true,
		},
		ServiceExpireTime: pgtype.Timestamptz{
			Time:  req.GetSellOrder().GetServiceExpireTime().AsTime(),
			Valid: true,
		},
	})
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create order in db: %v",
			err,
		)
	}

	return &pb.CreateSellOrderResponse{
		SellOrder: &pb.SellOrder{
			Name: fmt.Sprintf(
				utils.RESOURCE_PATTERN_ORDER,
				order.SellerID,
				order.OrderID,
			),
			SellerId:          order.SellerID,
			ServiceId:         order.ServiceID,
			AskPrice:          order.AskPrice,
			Quantity:          order.Quantity,
			OrderExpireTime:   timestamppb.New(order.OrderExpireTime.Time),
			ServiceExpireTime: timestamppb.New(order.ServiceExpireTime.Time),
		},
	}, nil
}

func (s *Server) DeleteSellOrder(
	ctx context.Context,
	req *pb.DeleteSellOrderRequest,
) (*emptypb.Empty, error) {
	idSegments, err := utils.ParseResourceName(
		req.GetName(), []string{"services", "sell-orders"},
	)
	if err != nil || len(idSegments) != 2 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	_, err = s.store.CancelOrder(ctx, db.CancelOrderParams{
		OrderID:  idSegments[1],
		SellerID: req.GetAccountId(),
	})
	if err == sql.ErrNoRows {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"order not found or not under your login account",
		)
	} else if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to delete in db",
		)
	}
	return nil, nil
}

func (s *Server) MatchOrder(
	ctx context.Context, req *pb.MatchOrderRequest,
) (*pb.MatchOrderResponse, error) {
	idSegments, err := utils.ParseResourceName(
		req.GetParent(), []string{"services"},
	)
	if err != nil || len(idSegments) != 1 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	orderFulfillment, err := s.store.MatchOrderTx(
		ctx,
		store.MatchOrderTxParams{
			MatchOneOrderParams: db.MatchOneOrderParams{
				BidPrice:  req.GetBidPrice(),
				BuyerID:   req.GetAccountId(),
				ServiceID: idSegments[0],
				MinExpireTime: pgtype.Timestamptz{
					Time:  req.GetMinExpireTime().AsTime(),
					Valid: true,
				},
				BidQuantity: req.GetQuantity(),
			},
		},
	)

	if err != nil {
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "no rows in result set") {
			return nil, status.Error(
				codes.NotFound,
				"no order matched",
			)
		}
		return nil, status.Errorf(
			codes.Internal,
			"db update failed: %v",
			err,
		)
	}
	return &pb.MatchOrderResponse{
		FulfilledOrder: &pb.FulfilledOrder{
			Name: fmt.Sprintf(
				utils.RESOURCE_PATTERN_FULFILLED_ORDER,
				orderFulfillment.ServiceID,
				orderFulfillment.OrderFulfillmentID,
			),
			ServiceId:         orderFulfillment.ServiceID,
			BuyerId:           orderFulfillment.BuyerID,
			SellerId:          orderFulfillment.SellerID,
			SellOrderId:       orderFulfillment.OrderID,
			DealPrice:         orderFulfillment.DealPrice,
			DealQuantity:      orderFulfillment.DealQuantity,
			DealTime:          timestamppb.New(orderFulfillment.DealTime.Time),
			RemainingQuantity: orderFulfillment.RemainingQuantity,
			ServiceExpireTime: timestamppb.New(orderFulfillment.ServiceExpireTime.Time),
		},
	}, nil
}

func (s *Server) BatchPruneFulfilledOrders(
	ctx context.Context, req *pb.BatchPruneFulfilledOrdersRequest,
) (*pb.BatchPruneFulfilledOrdersResponse, error) {
	orders, err := s.store.CleanInactiveOrders(ctx)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to delete sell orders from db",
		)
	}
	fulfilledrders, err := s.store.CleanExpiredFulfilledOrders(ctx)
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to delete fulfilled orders from db",
		)
	}

	return &pb.BatchPruneFulfilledOrdersResponse{
		SellOrdersCleaned:      int64(len(orders)),
		FulfilledOrdersCleaned: int64(len(fulfilledrders)),
	}, nil
}

func (s *Server) encodeFulflledOrder(order *db.FulfilledOrder) (*pb.FulfilledOrder, error) {
	return &pb.FulfilledOrder{
		Name: fmt.Sprintf(
			utils.RESOURCE_PATTERN_FULFILLED_ORDER,
			order.ServiceID,
			order.OrderFulfillmentID,
		),
		ServiceId:         order.ServiceID,
		SellOrderId:       order.OrderID,
		BuyerId:           order.BuyerID,
		SellerId:          order.SellerID,
		DealPrice:         order.DealPrice,
		DealQuantity:      order.DealQuantity,
		DealTime:          timestamppb.New(order.DealTime.Time),
		RemainingQuantity: order.RemainingQuantity,
		ServiceExpireTime: timestamppb.New(order.ServiceExpireTime.Time),
	}, nil
}

func (s *Server) ListFulfilledOrders(
	ctx context.Context, req *pb.ListFulfilledOrdersRequest,
) (*pb.ListFulfilledOrdersResponse, error) {
	parsedPagination, err := utils.ParsePagination(req)
	if err != nil {
		return nil, err
	}
	var orders []db.FulfilledOrder
	if req.GetParent() == "" {
		orders, err = s.store.ListFulfilledOrders(
			ctx, db.ListFulfilledOrdersParams{
				BuyerID:            req.GetAccountId(),
				Limit:              parsedPagination.PageSize,
				Offset:             parsedPagination.Skip,
				OrderFulfillmentID: parsedPagination.StartID,
				RemainingQuantity:  req.GetMinRemainingQuantity(),
			},
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to query db: %v", err)
		}
	} else {
		idSegments, err := utils.ParseResourceName(
			req.GetParent(), []string{"services"},
		)
		if err != nil || len(idSegments) != 1 {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"failed to parse resource name: %v",
				err,
			)
		}
		orders, err = s.store.ListFulfilledOrdersByService(
			ctx, db.ListFulfilledOrdersByServiceParams{
				BuyerID:            req.GetAccountId(),
				Limit:              parsedPagination.PageSize,
				Offset:             parsedPagination.Skip,
				OrderFulfillmentID: parsedPagination.StartID,
				ServiceID:          idSegments[0],
				RemainingQuantity:  req.GetMinRemainingQuantity(),
			},
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to query db: %v", err)
		}
	}
	var ret []*pb.FulfilledOrder
	for _, order := range orders {
		o, err := s.encodeFulflledOrder(&order)
		if err != nil {
			return nil, err
		}
		ret = append(ret, o)
	}
	if len(orders) == int(parsedPagination.PageSize) {
		return &pb.ListFulfilledOrdersResponse{
			FulfilledOrders: ret,
			NextPageToken:   utils.GeneratePageToken(orders[len(orders)-1].OrderFulfillmentID + 1),
		}, nil
	}
	return &pb.ListFulfilledOrdersResponse{
		FulfilledOrders: ret,
	}, nil
}
