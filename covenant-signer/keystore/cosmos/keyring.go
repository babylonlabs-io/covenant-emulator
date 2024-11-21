package cosmos

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

func CreateKeyring(keyringDir string, chainId string, backend string, input *strings.Reader) (keyring.Keyring, error) {
	ctx, err := CreateClientCtx(keyringDir, chainId)
	if err != nil {
		return nil, err
	}

	if backend == "" {
		return nil, fmt.Errorf("the keyring backend should not be empty")
	}

	kr, err := keyring.New(
		ctx.ChainID,
		backend,
		ctx.KeyringDir,
		input,
		ctx.Codec,
		ctx.KeyringOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create keyring: %w", err)
	}

	return kr, nil
}

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
		WithCodec(MakeCodec()).
		WithKeyringDir(keyringDir), nil
}

// CreateCovenantKey creates a new key inside the keyring
func CreateCovenantKey(keyringDir, chainID, keyName, backend, passphrase, hdPath string) (*ChainKeyInfo, error) {
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
