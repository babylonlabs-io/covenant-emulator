package keyutils

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
)

const (
	HardenedPostfix         = "h"
	ExpectedDerivationDepth = 5
)

func parseHardened(elem string) (uint32, error) {
	// valid hardened element is at least 2 characters example: 0h
	if len(elem) < 2 {
		return 0, fmt.Errorf("invalid hardened element: %s", elem)
	}

	number := strings.TrimSuffix(elem, HardenedPostfix)

	// if the element is unchanged, it means it did not end with correct suffix
	if number == elem {
		return 0, fmt.Errorf("invalid hardened element")
	}

	parsedNum, err := strconv.ParseUint(number, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid hardened element")
	}

	numAsUint32 := uint32(parsedNum)

	// todo check for overflow
	return hdkeychain.HardenedKeyStart + numAsUint32, nil
}

func parseNormal(elem string) (uint32, error) {
	if len(elem) == 0 {
		return 0, fmt.Errorf("invalid normal element")
	}

	parsedNum, err := strconv.ParseUint(elem, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid normal element")
	}

	return uint32(parsedNum), nil
}

func ParsePath(path string) ([]uint32, error) {
	splitted := strings.Split(path, "/")

	if len(splitted) != ExpectedDerivationDepth {
		return nil, fmt.Errorf("invalid derivation path length")
	}

	h1, err := parseHardened(splitted[0])
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path element at index 0: %w", err)
	}

	h2, err := parseHardened(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path element at index 1: %w", err)
	}

	h3, err := parseHardened(splitted[2])
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path element at index 2: %w", err)
	}

	n4, err := parseNormal(splitted[3])
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path element at index 3: %w", err)
	}

	n5, err := parseNormal(splitted[4])
	if err != nil {
		return nil, fmt.Errorf("invalid derivation path element at index 4: %w", err)
	}

	return []uint32{h1, h2, h3, n4, n5}, nil

}

type DerivationResult struct {
	PrivateKey string
	PublicKey  string
}

// DeriveChildKey derives a child key from a master key using a derivation path
// masterKey is a base58 encoded master key
// childPath is a derivation path in the format "84h/1h/0h/0/0", it must be 5 elements long
func DeriveChildKey(masterKey string, childPath string) (*DerivationResult, error) {
	parsedMasterKey, err := hdkeychain.NewKeyFromString(masterKey)
	if err != nil {
		return nil, fmt.Errorf("invalid master key: %w", err)
	}

	derivationPath, err := ParsePath(childPath)

	if err != nil {
		return nil, fmt.Errorf("invalid derivation path: %w", err)
	}

	keyResult := parsedMasterKey
	for _, elem := range derivationPath {
		keyResult, err = keyResult.Derive(elem)
		if err != nil {
			return nil, fmt.Errorf("failed to derive child key: %w", err)
		}
	}

	privKey, err := keyResult.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	pubKey := privKey.PubKey()

	return &DerivationResult{
		PrivateKey: hex.EncodeToString(privKey.Serialize()),
		PublicKey:  hex.EncodeToString(pubKey.SerializeCompressed()),
	}, nil
}
