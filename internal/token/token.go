package token

import (
	"encoding/json"
	"fmt"
	"math/big"
)

type TokenPolicy interface {
	ParseAndVerifyQuantity(argJson string) (*int64, map[string]int64, error)
	MarshalConfig() string
	// GetPolicyType() string
}

const (
	PRODUCT_TOKEN_POLICY_NAME = "exchange.prex.proto.ProductTokenPolicy"
)

type ProductTokenPolicy struct {
	UnitPrice int64 `json:"unit_price,omitempty"`
}

func UnmarshalFromConfig(policyType string, configStr string) (TokenPolicy, error) {
	if policyType == PRODUCT_TOKEN_POLICY_NAME {
		var tokenPolicy ProductTokenPolicy
		if err := json.Unmarshal([]byte(configStr), &tokenPolicy); err != nil {
			return nil, err
		} else {
			return &tokenPolicy, nil
		}
	} else {
		return nil, fmt.Errorf("got unknown token policy: %s", policyType)
	}
}

func NewProductTokenPolicy(unitPrice int64) (TokenPolicy, error) {
	return &ProductTokenPolicy{
		UnitPrice: unitPrice,
	}, nil
}

func (p *ProductTokenPolicy) MarshalConfig() string {
	configStr, err := json.Marshal(*p)
	if err != nil {
		fmt.Printf("MarshalConfig err: %v", err)
	}
	return string(configStr)
}

func (p *ProductTokenPolicy) ParseAndVerifyQuantity(argJson string) (*int64, map[string]int64, error) {
	var claims map[string]int64
	err := json.Unmarshal([]byte(argJson), &claims)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse argJson")
	}

	total := new(big.Int).SetInt64(p.UnitPrice)
	for k, dim := range claims {
		if dim <= 0 {
			return nil, nil, fmt.Errorf("dimension %s is not positive", k)
		}
		total = new(big.Int).Mul(total, new(big.Int).SetInt64(dim))
	}
	if !total.IsInt64() {
		return nil, nil, fmt.Errorf("total quantity %d exceeds int64", total)
	}
	ret := total.Int64()
	return &ret, claims, nil
}
