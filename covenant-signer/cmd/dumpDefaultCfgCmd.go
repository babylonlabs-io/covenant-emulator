package cmd

import (
	"fmt"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(dumpCfgCmd)
}

var dumpCfgCmd = &cobra.Command{
	Use:   "dump-cfg",
	Short: "dumps default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cmd.Flags().GetString(configPathKey)
		if err != nil {
			return err
		}

		err = config.WriteConfigToFile(path, config.DefaultConfig())

		if err != nil {
			return err
		}

		fmt.Printf("Default configuration file dumped to: %s \n", path)
		return nil
	},
}
