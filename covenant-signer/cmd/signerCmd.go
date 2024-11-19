package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/btcclient"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	m "github.com/babylonlabs-io/covenant-emulator/covenant-signer/observability/metrics"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
)

func init() {
	rootCmd.AddCommand(runSignerCmd)
}

var runSignerCmd = &cobra.Command{
	Use:   "start",
	Short: "starts the signer service",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath, err := cmd.Flags().GetString(configPathKey)
		if err != nil {
			return err
		}
		cfg, err := config.GetConfig(configPath)
		if err != nil {
			return err
		}

		parsedConfig, err := cfg.Parse()

		if err != nil {
			return err
		}

		parsedGlobalParams, err := signerapp.NewVersionedParamsRetriever(globalParamPath)

		if err != nil {
			return err
		}

		fullNodeClient, err := btcclient.NewBtcClient(parsedConfig.BtcNodeConfig)

		if err != nil {
			return err
		}

		chainInfo := signerapp.NewBitcoindChainInfo(fullNodeClient)

		signerClient, err := btcclient.NewBtcClient(parsedConfig.BtcSignerConfig.ToBtcConfig())

		if err != nil {
			return err
		}

		var signer signerapp.ExternalBtcSigner
		if parsedConfig.BtcSignerConfig.SignerType == config.PsbtSigner {
			fmt.Println("using psbt signer")
			signer = signerapp.NewPsbtSigner(signerClient)
		} else if parsedConfig.BtcSignerConfig.SignerType == config.PrivKeySigner {
			fmt.Println("using privkey signer")
			signer = signerapp.NewPrivKeySigner(signerClient)
		}

		app := signerapp.NewSignerApp(
			signer,
			chainInfo,
			parsedGlobalParams,
			parsedConfig.BtcNodeConfig.Network,
		)

		metrics := m.NewCovenantSignerMetrics()

		srv, err := signerservice.New(
			cmd.Context(),
			parsedConfig,
			app,
			metrics,
		)

		if err != nil {
			return err
		}

		metricsAddress := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.Port)

		m.Start(metricsAddress, metrics.Registry)

		// TODO: Add signal handling and gracefull shutdown
		return srv.Start()
	},
}
