package cmd

import (
	"fmt"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/keyutils"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(deriveChildKeyCmd)
}

var deriveChildKeyCmd = &cobra.Command{
	Use:   "derive-child-key [master-key] [derivation-path]",
	Args:  cobra.ExactArgs(2),
	Short: "derives a child key from a master key",
	RunE: func(cmd *cobra.Command, args []string) error {
		masterKey := args[0]
		derivationPath := args[1]

		result, err := keyutils.DeriveChildKey(masterKey, derivationPath)
		if err != nil {
			return err
		}

		fmt.Printf("Derived private key: %s\n", result.PrivateKey)
		fmt.Printf("Derived public key: %s\n", result.PublicKey)

		return nil
	},
}
