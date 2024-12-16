package cmd

import (
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"
)

var (
	// Used for flags.
	configPath    string
	configPathKey = "config"

	//   C:\Users\<username>\AppData\Local\signer on Windows
	//   ~/.signer on Linux
	//   ~/Library/Application Support/signer on MacOS
	defaultConfigDir  = btcutil.AppDataDir("signer", false)
	defaultConfigPath = filepath.Join(defaultConfigDir, "config.toml")

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

	cmd.PersistentFlags().String(flags.FlagHome, defaultConfigDir, "The application home directory")
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
		defaultConfigPath,
		"path to the configuration file",
	)
}
