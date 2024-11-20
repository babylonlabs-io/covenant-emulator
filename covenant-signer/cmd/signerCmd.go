package cmd

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/spf13/cobra"

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

		privKey, err := btcec.NewPrivateKey()

		if err != nil {
			return err
		}

		// TODO: Implement other approach to store keys
		prk := signerapp.NewHardcodedPrivKeyRetriever(privKey)

		app := signerapp.NewSignerApp(
			prk,
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
