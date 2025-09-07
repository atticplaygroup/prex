package config

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/mr-tron/base58"

	"github.com/atticplaygroup/prex/internal/utils"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/spf13/viper"
)

type Config struct {
	PaymentServiceUrl      string  `mapstructure:"PAYMENT_SERVICE_URL"`
	AccountTtlPrice        float64 `mapstructure:"ACCOUNT_TTL_PRICE"`
	MaxExpirationExtension int64   `mapstructure:"MAX_EXPIRATION_EXTENTION"`

	WithdrawRecipientCount   int32  `mapstructure:"WITHDRAW_RECIPIENT_COUNT"`
	WithdrawCheckStatusCount int32  `mapstructure:"WITHDRAW_CHECK_STATUS_COUNT"`
	WalletMnemonic           string `mapstructure:"WALLET_MNEMONIC"`
	WalletSigner             signer.Signer
	SuiNetwork               string `mapstructure:"SUI_NETWORK"`
	TokenSigningSeed         string `mapstructure:"TOKEN_SIGNING_SEED"`
	TokenSigningPrivateKey   ed25519.PrivateKey
	TokenSigningKeyId        string

	TestDbUrl            string `mapstructure:"TEST_DB_URL"`
	TestMigrateSourceUrl string `mapstructure:"TEST_MIGRATE_SOURCE_URL"`

	JwtSecret          string        `mapstructure:"JWT_SECRET"`
	AdminUsername      string        `mapstructure:"ADMIN_USERNAME"`
	AdminPassword      string        `mapstructure:"ADMIN_PASSWORD"`
	MessageAuthTimeout time.Duration `mapstructure:"MESSAGE_AUTH_TIMEOUT"`
	MaxDepositEpochGap int64         `mapstructure:"MAX_DEPOSIT_EPOCH_GAP"`
	SessionTimeout     time.Duration `mapstructure:"SESSION_TIMEOUT"`

	FreeQuotaRefreshPeriod time.Duration `mapstructure:"FREE_QUOTA_REFRESH_PERIOD"`
	TokenTtl               time.Duration `mapstructure:"TOKEN_TTL"`

	RedisHost string `mapstructure:"redis_host"`
	RedisPort uint16 `mapstructure:"redis_port"`

	PrexGrpcPort uint16 `mapstructure:"PREX_GRPC_PORT"`

	EnablePrexQuotaLimiter             bool `mapstructure:"ENABLE_PREX_QUOTA_LIMITER"`
	EnableServiceRegistrationWhitelist bool `mapstructure:"ENABLE_SERVICE_REGISTRATION_WHITELIST"`
}

func LoadConfig(path string) (config Config) {
	viper.AddConfigPath(filepath.Dir(path))
	viper.SetConfigName(filepath.Base(path))
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("config: %v", err)
	}

	seed, err := utils.HexToBytes32(config.TokenSigningSeed)
	if err != nil {
		log.Fatalf("failed to parse TokenSigningSeed %s: %v", config.TokenSigningSeed, err)
	}
	if len(seed) != ed25519.SeedSize {
		log.Fatalf("expect seed to have len %d but got %d: %v", ed25519.SeedSize, len(seed), seed)
	}
	config.TokenSigningPrivateKey = ed25519.NewKeyFromSeed(seed)
	buf := []byte{0xed, 0x01}
	buf = append(buf, []byte(config.TokenSigningPrivateKey.Public().(ed25519.PublicKey))...)
	config.TokenSigningKeyId = fmt.Sprintf("did:key:z%s", base58.Encode(buf))
	fmt.Printf("did: %s\n", config.TokenSigningKeyId)

	signer, err := signer.NewSignertWithMnemonic(config.WalletMnemonic)
	if err != nil {
		log.Fatalf("failed to load mnemonic: %v", err)
	}
	config.WalletSigner = *signer

	return
}
