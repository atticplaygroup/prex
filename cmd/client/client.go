package client

import (
	"encoding/base64"
	"fmt"
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
	seed, err := bip39.NewSeedWithErrorChecking(mnemonic, field)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(seed)[:64], nil
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
