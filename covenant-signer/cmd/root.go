package cmd

import (
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/spf13/cobra"
)

var (
	// Used for flags.
	configPath    string
	configPathKey = "config"

	rootCmd = &cobra.Command{
		Use:   "covenant-signer",
		Short: "remote signing serivce to perform covenant duties",
	}

	//   C:\Users\<username>\AppData\Local\signer on Windows
	//   ~/.signer on Linux
	//   ~/Library/Application Support/signer on MacOS
	dafaultConfigDir  = btcutil.AppDataDir("signer", false)
	dafaultConfigPath = filepath.Join(dafaultConfigDir, "config.toml")
)

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
