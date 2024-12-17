package cmd

import (
	"github.com/cosmos/cosmos-sdk/client/keys"
)

func init() {
	rootCmd.AddCommand(keys.Commands())
}
