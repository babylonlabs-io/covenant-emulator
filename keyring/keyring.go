package keyring

import (
	"os"
	"path"

	"github.com/cosmos/cosmos-sdk/client"

	"github.com/babylonlabs-io/covenant-emulator/codec"
	"github.com/babylonlabs-io/covenant-emulator/types"
)

func CreateClientCtx(keyringDir string, chainId string) (client.Context, error) {
	var err error
	var homeDir string

	if keyringDir == "" {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return client.Context{}, err
		}
		keyringDir = path.Join(homeDir, ".covenant-emulator")
	}
	return client.Context{}.
		WithChainID(chainId).
		WithCodec(codec.MakeCodec()).
		WithKeyringDir(keyringDir), nil
}

// CreateCovenantKey creates a new key inside the keyring
func CreateCovenantKey(keyringDir, chainID, keyName, backend, passphrase, hdPath string) (*types.ChainKeyInfo, error) {
	sdkCtx, err := CreateClientCtx(
		keyringDir, chainID,
	)
	if err != nil {
		return nil, err
	}

	krController, err := NewChainKeyringController(
		sdkCtx,
		keyName,
		backend,
	)
	if err != nil {
		return nil, err
	}

	return krController.CreateChainKey(passphrase, hdPath)
}
