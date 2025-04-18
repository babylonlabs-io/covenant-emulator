package main

import (
	"fmt"
	"path/filepath"

	covcfg "github.com/babylonlabs-io/covenant-emulator/config"
	"github.com/babylonlabs-io/covenant-emulator/log"
	"github.com/babylonlabs-io/covenant-emulator/remotesigner"
	"github.com/babylonlabs-io/covenant-emulator/util"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/lightningnetwork/lnd/signal"
	"github.com/urfave/cli"

	"github.com/babylonlabs-io/covenant-emulator/clientcontroller"
	"github.com/babylonlabs-io/covenant-emulator/covenant"
	covsrv "github.com/babylonlabs-io/covenant-emulator/covenant/service"
)

var startCommand = cli.Command{
	Name:        "start",
	Usage:       "Start the Covenant Emulator Daemon",
	Description: "Start the Covenant Emulator Daemon. Note that the Covenant key pair should be created beforehand",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  homeFlag,
			Usage: "The path to the covenant home directory",
			Value: covcfg.DefaultCovenantDir,
		},
	},
	Action: start,
}

func start(ctx *cli.Context) error {
	homePath, err := filepath.Abs(ctx.String(homeFlag))
	if err != nil {
		return err
	}
	homePath = util.CleanAndExpandPath(homePath)

	cfg, err := covcfg.LoadConfig(homePath)
	if err != nil {
		return fmt.Errorf("failed to load config at %s: %w", homePath, err)
	}

	if cfg.BabylonConfig.KeyringBackend != keyring.BackendTest {
		return fmt.Errorf("the keyring backend in config must be `test` for automatic signing, got %s. Other keyring backends are not supported as they require manual passphrase entry", cfg.BabylonConfig.KeyringBackend)
	}

	logger, err := log.NewRootLoggerWithFile(covcfg.LogFile(homePath), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to load the logger: %w", err)
	}

	bbnClient, err := clientcontroller.NewBabylonController(cfg.BabylonConfig, &cfg.BTCNetParams, logger)
	if err != nil {
		return fmt.Errorf("failed to create rpc client for the consumer chain: %w", err)
	}

	signer, err := newRemoteSignerFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create remote signer from config: %w", err)
	}

	// Perform health check on the remote signer
	logger.Info("Performing health check on remote signer...")
	if _, err := signer.PubKey(); err != nil {
		return fmt.Errorf("remote signer health check failed - ensure the signer is running and unlocked: %w", err)
	}
	logger.Info("Remote signer health check passed")

	ce, err := covenant.NewCovenantEmulator(cfg, bbnClient, logger, signer)
	if err != nil {
		return fmt.Errorf("failed to start the covenant emulator: %w", err)
	}

	// Hook interceptor for os signals.
	shutdownInterceptor, err := signal.Intercept()
	if err != nil {
		return err
	}

	srv := covsrv.NewCovenantServer(logger, ce, shutdownInterceptor)
	if err != nil {
		return fmt.Errorf("failed to create covenant server: %w", err)
	}

	return srv.RunUntilShutdown()
}

func newRemoteSignerFromConfig(cfg *covcfg.Config) (covenant.Signer, error) {
	return remotesigner.NewRemoteSigner(cfg.RemoteSigner), nil
}
