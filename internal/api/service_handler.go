package api

import (
	"context"
	"database/sql"
	"fmt"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	"github.com/atticplaygroup/prex/internal/token"
	"github.com/atticplaygroup/prex/internal/utils"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type RemoveServiceRequest struct {
	ServiceId int64 `json:"service_id" binding:"required"`
}

func (s *Server) DeleteService(
	ctx context.Context, req *pb.DeleteServiceRequest,
) (*emptypb.Empty, error) {
	idSegments, err := utils.ParseResourceName(
		req.GetName(), []string{"services"},
	)
	if err != nil || len(idSegments) != 1 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	_, err = s.store.RemoveService(ctx, idSegments[0])
	if err == sql.ErrNoRows {
		return nil, status.Error(
			codes.NotFound,
			"service not found",
		)
	} else if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to delete from db: %v",
			err,
		)
	}
	return &emptypb.Empty{}, err
}

func (s *Server) CreateService(
	ctx context.Context,
	req *pb.CreateServiceRequest,
) (*pb.CreateServiceResponse, error) {
	globalIdBytes, err := utils.Uuid2bytes(req.Service.GetGlobalId())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"cannot parse global_id to uuid: %v",
			err,
		)
	}
	var tokenPolicy token.TokenPolicy
	if req.Service.GetProductTokenPolicy() != nil {
		tokenPolicy, err = token.NewProductTokenPolicy(
			req.Service.GetProductTokenPolicy().GetUnitPrice(),
		)
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"cannit init token policy: %v",
				err,
			)
		}
	} else {
		return nil, status.Error(
			codes.InvalidArgument,
			"unknown token policy",
		)
	}

	service, err := s.store.CreateService(ctx, db.CreateServiceParams{
		ServiceGlobalID: pgtype.UUID{
			Bytes: [16]byte(globalIdBytes),
			Valid: true,
		},
		DisplayName:       req.Service.GetDisplayName(),
		TokenPolicyConfig: tokenPolicy.MarshalConfig(),
		TokenPolicyType:   string(req.Service.GetProductTokenPolicy().ProtoReflect().Descriptor().FullName()),
	})
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to insert db: %v",
			err,
		)
	}
	s.tokenPolicies[service.ServiceID] = &tokenPolicy
	return &pb.CreateServiceResponse{
		Service: &pb.Service{
			Name:        fmt.Sprintf(utils.RESOURCE_PATTERN_SERVICE, service.ServiceID),
			TokenPolicy: req.Service.GetTokenPolicy(),
			GlobalId:    req.Service.GetGlobalId(),
			DisplayName: req.Service.GetDisplayName(),
		},
	}, nil
}

func (s *Server) CreateServicesFromDb(
	ctx context.Context,
) error {
	offset := int32(0)
	limit := int32(50)
	currentId := int64(0)
	for {
		services, err := s.store.ListServices(ctx, db.ListServicesParams{
			Offset:    offset,
			ServiceID: currentId,
			Limit:     limit,
		})
		if err != nil {
			return nil
		}
		if len(services) == 0 {
			return nil
		}
		for _, service := range services {
			tokenPolicy, err := token.UnmarshalFromConfig(
				service.TokenPolicyType,
				service.TokenPolicyConfig,
			)
			if err != nil {
				return err
			}
			s.tokenPolicies[service.ServiceID] = &tokenPolicy
		}
		currentId = services[len(services)-1].ServiceID + 1
	}
}

func (s *Server) encodeService(service *db.Service) (*pb.Service, error) {
	globalId, err := service.ServiceGlobalID.UUIDValue()
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"cannot parse service global id of service %d",
			service.ServiceID,
		)
	}
	policy, ok := (*s.tokenPolicies[service.ServiceID]).(*token.ProductTokenPolicy)
	if ok {
		return &pb.Service{
			Name: fmt.Sprintf(utils.RESOURCE_PATTERN_SERVICE, service.ServiceID),
			TokenPolicy: &pb.Service_ProductTokenPolicy{
				ProductTokenPolicy: &pb.ProductTokenPolicy{
					UnitPrice: policy.UnitPrice,
				},
			},
			GlobalId:    globalId.String(),
			DisplayName: service.DisplayName,
		}, nil
	} else {
		return nil, status.Error(
			codes.Internal,
			"got unknown token policy from db",
		)
	}
}

func (s *Server) ListServices(
	ctx context.Context,
	req *pb.ListServicesRequest,
) (*pb.ListServicesResponse, error) {
	parsedPagination, err := utils.ParsePagination(req)
	if err != nil {
		return nil, err
	}
	services, err := s.store.ListServices(ctx, db.ListServicesParams{
		Offset:    parsedPagination.Skip,
		ServiceID: parsedPagination.StartID,
		Limit:     parsedPagination.PageSize,
	})
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"cannot list services from db",
		)
	}
	if len(services) == 0 {
		return &pb.ListServicesResponse{}, nil
	}
	var ret []*pb.Service
	for _, service := range services {
		svc, err := s.encodeService(&service)
		if err != nil {
			return nil, err
		}
		ret = append(ret, svc)
	}
	if len(services) == int(parsedPagination.PageSize) {
		return &pb.ListServicesResponse{
			Services:      ret,
			NextPageToken: utils.GeneratePageToken(services[len(services)-1].ServiceID + 1),
		}, nil
	}
	return &pb.ListServicesResponse{
		Services: ret,
	}, nil
}
