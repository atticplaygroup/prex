package client

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/atticplaygroup/prex/cmd"
	"github.com/block-vision/sui-go-sdk/signer"
	"github.com/mr-tron/base58"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Command line interface to interact with Prex",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		var err error
		conf, err = getConfig(cmd)
		if err != nil {
			log.Fatalf("parse config failed: %v", err)
		}
	},
}

type AccountConfig struct {
	Mnemonic string `yaml:"mnemonic"`
}

type Config struct {
	Account AccountConfig `yaml:"account"`
}

type ParsedAccount struct {
	Mnemonic   string
	SecretSeed []byte
	KeyPair    *signer.Signer

	Username string
	Password string
}

type ParsedConfig struct {
	Account ParsedAccount
}

var conf *ParsedConfig

func deriveFromMnemonic(mnemonic string, field string) (string, error) {
	// Some random public value. NOT safe. Demo purpose only. Generate and store a per account
	// random value in a serious client.
	argon2Salt := []byte{
		0x5e, 0xc5, 0x57, 0xbd, 0x6f, 0x5d, 0xbb, 0xa2,
		0xf2, 0xba, 0x8c, 0xf7, 0x31, 0xc6, 0xc2, 0x5b,
		0xb8, 0x2a, 0x5e, 0x94, 0x52, 0x10, 0xae, 0x6e,
		0xe7, 0xe1, 0xa2, 0x06, 0xff, 0xa8, 0xe7, 0x5d,
	}
	ikm := argon2.IDKey([]byte(mnemonic), argon2Salt, 3, 64*1024, 1, 32)
	salt := argon2Salt
	info := []byte(field)
	h := hkdf.New(sha256.New, ikm, salt, info)
	okm := make([]byte, 32)
	if _, err := io.ReadFull(h, okm); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(okm), nil
}

func parseConfig(config *Config) (*ParsedConfig, error) {
	seed, err := bip39.NewSeedWithErrorChecking(config.Account.Mnemonic, "")
	if err != nil {
		return nil, err
	}
	derivedSeed, err := signer.DeriveForPath("m/44'/784'/0'/0'/0'", seed)
	if err != nil {
		return nil, err
	}
	keypair := signer.NewSigner(derivedSeed.Key)
	buf := []byte{0xed, 0x01}
	buf = append(buf, []byte(keypair.PubKey)...)
	username := fmt.Sprintf("did:key:z%s", base58.Encode(buf))
	password, err := deriveFromMnemonic(config.Account.Mnemonic, "password")
	if err != nil {
		return nil, fmt.Errorf("cannot deriver password: %v", err)
	}
	return &ParsedConfig{
		Account: ParsedAccount{
			Mnemonic:   config.Account.Mnemonic,
			SecretSeed: derivedSeed.Key,
			KeyPair:    keypair,
			Username:   username,
			Password:   password,
		},
	}, nil
}

func readConfig(configPath string) (*ParsedConfig, error) {
	config := &Config{}
	yamlData, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If the file does not exist, create a default config
			if err := createDefaultConfig(configPath); err != nil {
				return nil, err
			}
			// Read the default config
			return readConfig(configPath)
		}
		return nil, err
	}
	err = yaml.Unmarshal(yamlData, config)
	if err != nil {
		return nil, err
	}
	parsedConfig, err := parseConfig(config)
	if err != nil {
		return nil, err
	}
	return parsedConfig, nil
}

func createDefaultConfig(configPath string) error {
	// Create the parent directory if it does not exist
	parentDir := filepath.Dir(configPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("cannot create default config file: %v", err)
	}
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return err
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return err
	}
	// Create the default YAML file
	defaultConfig := &Config{
		Account: AccountConfig{Mnemonic: mnemonic},
	}
	yamlData, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, yamlData, 0644)
}

func getConfig(cmd *cobra.Command) (*ParsedConfig, error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}
	return readConfig(configPath)
}

func getDefaultConfigPath() string {
	defaultConfigPath, ok := os.LookupEnv("PREX_CONFIG_PATH")
	if ok {
		return defaultConfigPath
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		defaultConfigPath = ""
	} else {
		defaultConfigPath = path.Join(homeDir, ".prex", "config.yaml")
	}
	return defaultConfigPath
}

func init() {
	flags := clientCmd.PersistentFlags()
	flags.StringP("config", "c", getDefaultConfigPath(), "Config yaml path")
	cmd.RootCmd.AddCommand(clientCmd)
}
