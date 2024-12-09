package main

import (
	"encoding/json"
	"fmt"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/jessevdk/go-flags"

	"github.com/urfave/cli"

	covcfg "github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/keyring"
)

type covenantKey struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public-key-hex"`
	BabylonAddr string `json:"babylon-address"`
}

var createKeyCommand = cli.Command{
	Name:      "create-key",
	ShortName: "ck",
	Usage:     "Create a Covenant account in the keyring.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  chainIdFlag,
			Usage: "The chainID of the consumer chain",
			Value: defaultChainID,
		},
		cli.StringFlag{
			Name:     keyNameFlag,
			Usage:    "The unique name of the covenant key",
			Required: true,
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "The pass phrase used to encrypt the keys",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  hdPathFlag,
			Usage: "The hd path used to derive the private key",
			Value: defaultHdPath,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "Select keyring's backend",
			Value: defaultKeyringBackend,
		},
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The home directory for the covenant",
			Value: covcfg.DefaultCovenantDir,
		},
	},
	Action: createKey,
}

func createKey(ctx *cli.Context) error {
	homePath := ctx.String(homeFlag)
	chainID := ctx.String(chainIdFlag)
	keyName := ctx.String(keyNameFlag)
	backend := ctx.String(keyringBackendFlag)
	passphrase := ctx.String(passphraseFlag)
	hdPath := ctx.String(hdPathFlag)
	keyBackend := ctx.String(keyringBackendFlag)

	// check the config file exists
	cfg, err := covcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load the config from %s: %w", covcfg.ConfigFile(homePath), err)
	}

	keyPair, err := keyring.CreateCovenantKey(
		homePath,
		chainID,
		keyName,
		backend,
		passphrase,
		hdPath,
	)
	if err != nil {
		return fmt.Errorf("failed to create covenant key: %w", err)
	}

	bip340Key := types.NewBIP340PubKeyFromBTCPK(keyPair.PublicKey)
	printRespJSON(
		&covenantKey{
			Name:        ctx.String(keyNameFlag),
			PublicKey:   bip340Key.MarshalHex(),
			BabylonAddr: keyPair.Address,
		},
	)

	// write the updated config into the config file
	cfg.BabylonConfig.Key = keyName
	cfg.BabylonConfig.KeyringBackend = keyBackend
	fileParser := flags.NewParser(cfg, flags.Default)

	return flags.NewIniParser(fileParser).WriteFile(covcfg.ConfigFile(homePath), flags.IniIncludeComments|flags.IniIncludeDefaults)
}

var showKeyCommand = cli.Command{
	Name:      "show-key",
	ShortName: "sk",
	Usage:     "Show a Covenant account in the keyring.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  chainIdFlag,
			Usage: "The chainID of the consumer chain",
			Value: defaultChainID,
		},
		cli.StringFlag{
			Name:     keyNameFlag,
			Usage:    "The name of the covenant key",
			Required: true,
		},
		cli.StringFlag{
			Name:  passphraseFlag,
			Usage: "The pass phrase used to decrypt the key",
			Value: defaultPassphrase,
		},
		cli.StringFlag{
			Name:  keyringBackendFlag,
			Usage: "Select keyring's backend",
			Value: defaultKeyringBackend,
		},
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The home directory for the covenant",
			Value: covcfg.DefaultCovenantDir,
		},
	},
	Action: showKey,
}

func showKey(ctx *cli.Context) error {
	homePath := ctx.String(homeFlag)
	chainID := ctx.String(chainIdFlag)
	keyName := ctx.String(keyNameFlag)
	backend := ctx.String(keyringBackendFlag)
	passphrase := ctx.String(passphraseFlag)

	sdkCtx, err := keyring.CreateClientCtx(homePath, chainID)
	if err != nil {
		return err
	}

	krController, err := keyring.NewChainKeyringController(sdkCtx, keyName, backend)
	if err != nil {
		return err
	}

	privKey, err := krController.GetChainPrivKey(passphrase)
	if err != nil {
		return err
	}

	r, err := krController.KeyRecord()
	if err != nil {
		return err
	}

	babylonAddr, err := r.GetAddress()
	if err != nil {
		return err
	}

	_, pk := btcec.PrivKeyFromBytes(privKey.Key)
	bip340Key := types.NewBIP340PubKeyFromBTCPK(pk)
	printRespJSON(
		&covenantKey{
			Name:        ctx.String(keyNameFlag),
			PublicKey:   bip340Key.MarshalHex(),
			BabylonAddr: babylonAddr.String(),
		},
	)
	return nil
}

func printRespJSON(resp interface{}) {
	jsonBytes, err := json.MarshalIndent(resp, "", "    ")
	if err != nil {
		fmt.Println("unable to decode response: ", err)
		return
	}

	fmt.Printf("%s\n", jsonBytes)
}
