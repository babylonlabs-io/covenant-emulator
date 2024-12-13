package cmd

import (
	"os"

	"github.com/babylonlabs-io/babylon/app/params"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/spf13/cobra"
)

func PersistClientCtx(ctx client.Context) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		encCfg := params.DefaultEncodingConfig()
		std.RegisterInterfaces(encCfg.InterfaceRegistry)
		bstypes.RegisterInterfaces(encCfg.InterfaceRegistry)

		ctx = ctx.
			WithCodec(encCfg.Codec).
			WithInterfaceRegistry(encCfg.InterfaceRegistry).
			WithTxConfig(encCfg.TxConfig).
			WithLegacyAmino(encCfg.Amino).
			WithInput(os.Stdin)

		// set the default command outputs
		cmd.SetOut(cmd.OutOrStdout())
		cmd.SetErr(cmd.ErrOrStderr())

		if err := client.SetCmdClientContextHandler(ctx, cmd); err != nil {
			return err
		}

		ctx = client.GetClientContextFromCmd(cmd)

		// updates the ctx in the cmd in case something was modified bt the config
		return client.SetCmdClientContext(cmd, ctx)
	}
}
