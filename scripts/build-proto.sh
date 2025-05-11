mkdir -p pkg/proto/gen/go pkg/proto/gen/openapi

protoc -I ./pkg/proto \
    -I ../googleapis \
    -I ../protobuf/src \
    --go_out=./pkg/proto/gen/go \
    --go_opt=paths=source_relative \
    --go-grpc_out=./pkg/proto/gen/go \
    --go-grpc_opt=paths=source_relative \
    --grpc-gateway_out=./pkg/proto/gen/go \
    --grpc-gateway_opt=paths=source_relative \
    --openapiv2_out ./pkg/proto/gen/openapi \
    --openapiv2_opt use_go_templates=true \
    ./pkg/proto/exchange/exchange.proto
