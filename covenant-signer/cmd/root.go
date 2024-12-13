package cmd

import (
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
)

var (
	// Used for flags.
	configPath    string
	configPathKey = "config"

	//   C:\Users\<username>\AppData\Local\signer on Windows
	//   ~/.signer on Linux
	//   ~/Library/Application Support/signer on MacOS
	dafaultConfigDir  = btcutil.AppDataDir("signer", false)
	dafaultConfigPath = filepath.Join(dafaultConfigDir, "config.toml")

	rootCmd = NewRootCmd()
)

// NewRootCmd creates a new root command for fpd. It is called once in the main function.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "covenant-signer",
		Short:             "remote signing service to perform covenant duties",
		SilenceErrors:     false,
		PersistentPreRunE: PersistClientCtx(client.Context{}),
	}

	return cmd
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&configPath,
		configPathKey,
		dafaultConfigPath,
		"path to the configuration file",
	)
}
