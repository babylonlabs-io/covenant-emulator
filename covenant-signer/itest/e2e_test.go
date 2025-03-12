//go:build e2e
// +build e2e

package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	asig "github.com/babylonlabs-io/babylon/crypto/schnorr-adaptor-signature"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/btcstaking"
	staking "github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerapp"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/middlewares"
	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/signerservice/types"
)

func buildDataToSign(t *testing.T, covnenantPublicKey *btcec.PublicKey) signerapp.ParsedSigningRequest {
	stakerPrivKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	finalityProviderKey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	stakingTime := uint16(10000)
	unbondingTime := uint16(1000)
	stakingAmount := btcutil.Amount(100000)
	unbondingFee := btcutil.Amount(1000)
	slashingFee := btcutil.Amount(1000)
	slashingRate := sdkmath.LegacyMustNewDecFromStr("0.1")

	fakeInput := wire.NewTxIn(wire.NewOutPoint(&chainhash.Hash{}, 0), nil, nil)
	stakingInfo, err := btcstaking.BuildStakingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{finalityProviderKey.PubKey()},
		[]*btcec.PublicKey{covnenantPublicKey},
		1,
		stakingTime,
		stakingAmount,
		netParams,
	)
	require.NoError(t, err)

	stakingTx := wire.NewMsgTx(2)
	stakingTx.AddTxIn(fakeInput)
	stakingTx.AddTxOut(stakingInfo.StakingOutput)

	stakingSlashingSpendInfo, err := stakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	stakingUnbondingSpendInfo, err := stakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	stakingSlashingScript := stakingSlashingSpendInfo.RevealedLeaf.Script
	stakingUnbondingScript := stakingUnbondingSpendInfo.RevealedLeaf.Script

	unbondingInfo, err := staking.BuildUnbondingInfo(
		stakerPrivKey.PubKey(),
		[]*btcec.PublicKey{finalityProviderKey.PubKey()},
		[]*btcec.PublicKey{covnenantPublicKey},
		1,
		unbondingTime,
		stakingAmount-unbondingFee,
		netParams,
	)
	require.NoError(t, err)

	unbondingSlashingSpendInfo, err := unbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	unbondingSlashingScript := unbondingSlashingSpendInfo.RevealedLeaf.Script

	stakingTxHash := stakingTx.TxHash()
	stakingOutputIndex := uint32(0)

	unbondingTx := wire.NewMsgTx(2)
	unbondingTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&stakingTxHash, stakingOutputIndex), nil, nil))
	unbondingTx.AddTxOut(unbondingInfo.UnbondingOutput)

	stakingSlashingTx, err := btcstaking.BuildSlashingTxFromStakingTxStrict(
		stakingTx,
		stakingOutputIndex,
		stakingSlashingScript,
		stakerPrivKey.PubKey(),
		unbondingTime,
		int64(slashingFee),
		slashingRate,
		netParams,
	)
	require.NoError(t, err)

	unbondingSlashingTx, err := btcstaking.BuildSlashingTxFromStakingTxStrict(
		unbondingTx,
		0,
		unbondingSlashingScript,
		stakerPrivKey.PubKey(),
		unbondingTime,
		int64(slashingFee),
		slashingRate,
		netParams,
	)
	require.NoError(t, err)

	fpEncKey, err := asig.NewEncryptionKeyFromBTCPK(finalityProviderKey.PubKey())
	require.NoError(t, err)

	return signerapp.ParsedSigningRequest{
		StakingTx:               stakingTx,
		SlashingTx:              stakingSlashingTx,
		UnbondingTx:             unbondingTx,
		SlashUnbondingTx:        unbondingSlashingTx,
		StakingOutputIdx:        stakingOutputIndex,
		SlashingScript:          stakingSlashingScript,
		UnbondingScript:         stakingUnbondingScript,
		UnbondingSlashingScript: unbondingSlashingScript,
		FpEncKeys:               []*asig.EncryptionKey{fpEncKey},
	}
}

func TestGetPublicKey(t *testing.T) {
	tm := StartManager(t, 100, false, "")
	// default passphrase is empty in non encrypted keyring
	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", "")
	require.NoError(t, err)

	pubKey, err := signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, tm.hmacKey)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

}

func TestSigningTransactions(t *testing.T) {
	tm := StartManager(t, 100, false, "")
	// default passphrase is empty in non encrypted keyring
	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", "")
	require.NoError(t, err)

	pubKey, err := signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, tm.hmacKey)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	dataToSign := buildDataToSign(t, pubKey)

	sigs, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
		"",
	)

	require.NoError(t, err)
	require.NotNil(t, sigs)

	err = tm.verifyResponse(sigs, &dataToSign, pubKey)
	require.NoError(t, err)
}

func TestRejectToLargeRequest(t *testing.T) {
	tm := StartManager(t, 100, false, "")
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	tmContentLimit := tm.signerConfig.Server.MaxContentLength
	size := tmContentLimit + 1
	tooLargeTx := datagen.GenRandomByteArray(r, uint64(size))

	req := types.SignTransactionsRequest{
		StakingTxHex:   "",
		UnbondingTxHex: hex.EncodeToString(tooLargeTx),
	}

	marshalled, err := json.Marshal(req)
	require.NoError(t, err)

	route := fmt.Sprintf("%s/v1/sign-transactions", tm.SigningServerUrl())

	httpRequest, err := http.NewRequestWithContext(context.Background(), "POST", route, bytes.NewReader(marshalled))
	require.NoError(t, err)

	// use json
	httpRequest.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 10 * time.Second}
	// send the request
	res, err := client.Do(httpRequest)
	require.NoError(t, err)
	require.NotNil(t, res)
	defer res.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode)
}

func TestSigningTransactionsUsingEncryptedFileKeyRing(t *testing.T) {
	tm := StartManager(t, 100, true, "")

	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "testtest", "")
	require.NoError(t, err)

	pubKey, err := signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, tm.hmacKey)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	dataToSign := buildDataToSign(t, pubKey)

	sigs, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
		"",
	)

	require.NoError(t, err)
	require.NotNil(t, sigs)

	err = tm.verifyResponse(sigs, &dataToSign, pubKey)
	require.NoError(t, err)
}

func TestLockingKeyring(t *testing.T) {
	tm := StartManager(t, 100, true, "")

	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "testtest", "")
	require.NoError(t, err)

	pubKey, err := signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, tm.hmacKey)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	dataToSign := buildDataToSign(t, pubKey)

	// lock the keyring, and clear the private key from memory
	err = signerservice.Lock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "")
	require.NoError(t, err)

	// try to sign a transaction with a locked keyring, it should fail
	sigs, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
		"",
	)

	require.Error(t, err)
	require.Nil(t, sigs)
}

func TestHMACAuthentication(t *testing.T) {
	// Test with valid HMAC key
	testHMACKey := "test-hmac-key-for-authentication"
	tm := StartManager(t, 100, false, testHMACKey)

	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", testHMACKey)
	require.NoError(t, err, "Unlock should succeed with valid HMAC key")

	pubKey, err := signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, testHMACKey)
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	dataToSign := buildDataToSign(t, pubKey)

	sigs, err := signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
		testHMACKey,
	)
	require.NoError(t, err, "Signing should succeed with valid HMAC key")
	require.NotNil(t, sigs)

	_, err = signerservice.RequestCovenantSignaure(
		context.Background(),
		tm.SigningServerUrl(),
		10*time.Second,
		&dataToSign,
		"invalid-hmac-key",
	)
	require.Error(t, err, "Signing should fail with invalid HMAC key")
	require.Contains(t, err.Error(), "401", "Error should be a 401 Unauthorized")

	err = signerservice.Lock(context.Background(), tm.SigningServerUrl(), 10*time.Second, testHMACKey)
	require.NoError(t, err, "Lock should succeed with valid HMAC key")

	err = signerservice.Lock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "invalid-hmac-key")
	require.Error(t, err, "Lock should fail with invalid HMAC key")
	require.Contains(t, err.Error(), "401", "Error should be a 401 Unauthorized")
}

func TestHMACDirectRequest(t *testing.T) {
	testHMACKey := "test-hmac-key-for-direct-request"
	tm := StartManager(t, 100, false, testHMACKey)

	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", testHMACKey)
	require.NoError(t, err)

	body := []byte(`{"passphrase":""}`)
	route := fmt.Sprintf("%s/v1/unlock", tm.SigningServerUrl())

	hmacValue, err := middlewares.GenerateHMAC(testHMACKey, body)
	require.NoError(t, err)

	httpRequest, err := http.NewRequestWithContext(context.Background(), "POST", route, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set(middlewares.HeaderCovenantHMAC, hmacValue)

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "Request with valid HMAC should succeed")

	httpRequest, err = http.NewRequestWithContext(context.Background(), "POST", route, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set(middlewares.HeaderCovenantHMAC, "invalidhmacvalue")

	res, err = client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "Request with invalid HMAC should fail with 401")

	httpRequest, err = http.NewRequestWithContext(context.Background(), "POST", route, bytes.NewReader(body))
	require.NoError(t, err)
	httpRequest.Header.Set("Content-Type", "application/json")

	res, err = client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "Request with missing HMAC should fail with 401")

	pkRoute := fmt.Sprintf("%s/v1/public-key", tm.SigningServerUrl())

	httpRequest, err = http.NewRequestWithContext(context.Background(), "GET", pkRoute, nil)
	require.NoError(t, err)

	res, err = client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode, "Public key request without HMAC should fail with 401")

	emptyBody := []byte{}
	hmacValue, err = middlewares.GenerateHMAC(testHMACKey, emptyBody)
	require.NoError(t, err)

	httpRequest, err = http.NewRequestWithContext(context.Background(), "GET", pkRoute, nil)
	require.NoError(t, err)
	httpRequest.Header.Set(middlewares.HeaderCovenantHMAC, hmacValue)

	res, err = client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "Public key request with valid HMAC should succeed")

	var respData map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&respData)
	require.NoError(t, err, "Should receive valid JSON response")

	dataObj, exists := respData["data"]
	require.True(t, exists, "Response should contain a 'data' object")

	dataMap, ok := dataObj.(map[string]interface{})
	require.True(t, ok, "Data should be an object")

	_, exists = dataMap["public_key_hex"]
	require.True(t, exists, "Response should contain a public key")
}

func TestHMACMismatchedKeys(t *testing.T) {
	// Test scenario where client and server have different HMAC keys
	serverKey := "server-hmac-key"
	clientKey := "different-client-hmac-key"

	tm := StartManager(t, 100, false, serverKey)

	err := signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", clientKey)
	require.Error(t, err, "Unlock should fail with mismatched HMAC keys")
	require.Contains(t, err.Error(), "401", "Error should be a 401 Unauthorized")

	_, err = signerservice.GetPublicKey(context.Background(), tm.SigningServerUrl(), 10*time.Second, clientKey)
	require.Error(t, err, "GetPublicKey should require HMAC")
	require.Contains(t, err.Error(), "401", "Error should be a 401 Unauthorized")

	err = signerservice.Unlock(context.Background(), tm.SigningServerUrl(), 10*time.Second, "", serverKey)
	require.NoError(t, err, "Unlock should succeed with matching HMAC key")

	route := fmt.Sprintf("%s/v1/public-key", tm.SigningServerUrl())
	httpRequest, err := http.NewRequestWithContext(context.Background(), "GET", route, nil)
	require.NoError(t, err)

	hmacValue, err := middlewares.GenerateHMAC(serverKey, []byte{})
	require.NoError(t, err)
	httpRequest.Header.Set(middlewares.HeaderCovenantHMAC, hmacValue)

	client := http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(httpRequest)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "Request with valid HMAC should succeed")
}
