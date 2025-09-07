package utils

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	db "github.com/atticplaygroup/prex/internal/db/sqlc"
	pb "github.com/atticplaygroup/prex/pkg/proto/gen/go/exchange/v1"
	"github.com/block-vision/sui-go-sdk/models"
	"github.com/google/uuid"
	"github.com/ssoready/hyrumtoken"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type key int

const (
	KEY_PARAMS key = 1
)

const (
	RESOURCE_PATTERN_WITHDRAW        = "accounts/%d/withdraws/%d"
	RESOURCE_PATTERN_ORDER           = "accounts/%d/sell-orders/%d"
	RESOURCE_PATTERN_FULFILLED_ORDER = "services/%d/fulfilled-orders/%d"
	RESOURCE_PATTERN_SERVICE         = "services/%d"
)

type ResourceInfo struct {
	AccountId        *int64
	ServiceId        *int64
	WithdrawId       *int64
	FulfilledOrderId *int64
	SellOrderId      *int64
}

func ParseResourceName(name string, fields []string) ([]int64, error) {
	idSegments, err := parseResourceName(name, fields)
	if err != nil || len(idSegments) != len(fields) {
		return nil, fmt.Errorf(
			"cannot parse resource name: %v",
			err,
		)
	}
	for _, id := range idSegments {
		if id <= 0 {
			return nil, fmt.Errorf(
				"some id is non positive: %v",
				idSegments,
			)
		}
	}
	return idSegments, nil
}

func parseResourceName(name string, prefixes []string) ([]int64, error) {
	pattern := strings.Join(prefixes, `/(\d+)/`) + `/(\d+)`
	r := regexp.MustCompile(pattern)
	matches := r.FindStringSubmatch(name)

	if len(matches) == len(prefixes)+1 {
		ret := make([]int64, 0)
		for _, m := range matches[1:] {
			segmentId, err := strconv.Atoi(m)
			if err != nil {
				return nil, fmt.Errorf("failed to parse segment: %v", err)
			}
			ret = append(ret, int64(segmentId))
		}
		return ret, nil
	} else {
		return nil, fmt.Errorf("resource name %s not matching pattern %s", name, pattern)
	}
}

func FormatAccount(account db.Account) pb.Account {
	return pb.Account{
		Name:       fmt.Sprintf("/accounts/%d", account.AccountID),
		AccountId:  account.AccountID,
		Username:   account.Username,
		ExpireTime: timestamppb.New(account.ExpireTime.Time),
		Balance:    account.Balance,
	}
}

func BytesToHexWithPrefix(data []byte) string {
	hexString := hex.EncodeToString(data)
	return "0x" + hexString
}

func Uuid2bytes(uuidStr string) ([]byte, error) {
	u, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, err
	}

	return u.MarshalBinary()
}

func HexToBytes32(hexStr string) ([]byte, error) {
	if bytes, err := hexToBytes(hexStr, 32); err != nil {
		return nil, err
	} else {
		return bytes, nil
	}
}

func hexToBytes(hexStr string, arrayLength uint8) ([]byte, error) {
	if (len(hexStr) != 2*int(arrayLength)+2) || hexStr[:2] != "0x" {
		return nil, fmt.Errorf(
			"expected hex string for 32 bytes beginning with 0x but got %s", hexStr,
		)
	}

	byteSlice, err := hex.DecodeString(hexStr[2:])
	if err != nil {
		return nil, err
	}
	return byteSlice, nil
}

func ConcatBytes(byteSlices ...[]byte) []byte {
	var result []byte
	for _, b := range byteSlices {
		result = append(result, b...)
	}
	return result
}

type PageToken struct {
	ID         int64     `json:"id"`
	ExpireTime time.Time `json:"exp"`
}

type IPagination interface {
	GetPageSize() int32
	GetSkip() int32
	GetPageToken() string
}

type Pagination struct {
	PageSize int32
	Skip     int32
	StartID  int64
}

func GeneratePageToken(nextID int64) string {
	var key [32]byte
	return hyrumtoken.Marshal(&key, PageToken{
		ID:         nextID,
		ExpireTime: time.Now().Add(time.Duration(PAGE_TOKEN_DEFAULT_EXP_SECONDS) * time.Second),
	})
}

func ParsePagination(req IPagination) (*Pagination, error) {
	var pageSize, skip int32
	var startServiceId int64
	if req.GetPageSize() < 0 || req.GetSkip() < 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			"Invalid page_size or skip",
		)
	}
	if req.GetPageSize() <= 0 {
		pageSize = SERVICE_DEFAULT_PAGE_SIZE
	} else if req.GetPageSize() > SERVICE_MAX_PAGE_SIZE {
		pageSize = SERVICE_MAX_PAGE_SIZE
	} else {
		pageSize = req.GetPageSize()
	}
	if req.GetSkip() > 0 {
		skip = req.GetSkip()
	}
	if req.GetPageToken() != "" {
		var key [32]byte
		var parsedPageToken PageToken
		err := hyrumtoken.Unmarshal(&key, req.GetPageToken(), &parsedPageToken)
		if err != nil {
			return nil, status.Error(
				codes.InvalidArgument,
				"cannot parse page_token",
			)
		}
		if parsedPageToken.ExpireTime.Before(time.Now()) {
			return nil, status.Error(
				codes.PermissionDenied,
				"cannot page_token has expired",
			)
		}
		if parsedPageToken.ID < 0 {
			return nil, status.Error(
				codes.InvalidArgument,
				"page_token.id < 0",
			)
		}
		startServiceId = parsedPageToken.ID
	} else {
		startServiceId = 0
	}
	return &Pagination{
		StartID:  startServiceId,
		Skip:     skip,
		PageSize: pageSize,
	}, nil
}

func GetSuiFaucetHost(network string) string {
	switch network {
	case "devnet":
		return "https://faucet.devnet.sui.io"
	case "mainnet":
		return "https://faucet.mainnet.sui.io"
	case "testnet":
		return "https://faucet.testnet.sui.io"
	case "localnet":
		return "http://127.0.0.1:9123"
	default:
	}
	log.Fatalf("faucet network should in [devnet, testnet, localnet]")
	return ""
}

func GetSuiFullNodeHost(network string) string {
	switch network {
	case "mainnet":
		return "https://fullnode.mainnet.sui.io:443"
	case "devnet":
		return "https://fullnode.devnet.sui.io:443"
	case "testnet":
		return "https://fullnode.testnet.sui.io:443"
	case "localnet":
		return "http://127.0.0.1:9000"
	default:
	}
	log.Fatalf("faucet network should in [mainnet, devnet, testnet, localnet]")
	return ""
}

const (
	faucetUriGasV0 = "/gas"
	faucetUriGasV1 = "/v1/gas"
	faucetUriGasV2 = "/v2/gas"
)

func RequestSuiFromFaucet(network, recipientAddress string) error {
	faucetHost := GetSuiFaucetHost(network)
	body := models.FaucetRequest{
		FixedAmountRequest: &models.FaucetFixedAmountRequest{
			Recipient: recipientAddress,
		},
	}

	err := faucetRequest(faucetHost+faucetUriGasV2, body, map[string]string{})

	return err
}

// Copied from https://github.com/block-vision/sui-go-sdk/blob/main/sui/faucet_api.go because it does not support v2 faucet
func faucetRequest(faucetUrl string, body interface{}, headers map[string]string) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body error: %s", err.Error())
	}

	req, err := http.NewRequest(http.MethodPost, faucetUrl, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("create request error: %s", err.Error())
	}

	req.Header.Set("Content-Type", "application/json")

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request faucet error: %s", err.Error())
	}

	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body error: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("request faucet failed, statusCode: %d, err: %+v", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
