package utils

import (
	"bytes"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/wire"
)

func NewBTCTxFromBytes(txBytes []byte) (*wire.MsgTx, error) {
	var msgTx wire.MsgTx
	rbuf := bytes.NewReader(txBytes)
	if err := msgTx.Deserialize(rbuf); err != nil {
		return nil, err
	}

	return &msgTx, nil
}

func NewBTCTxFromHex(txHex string) (*wire.MsgTx, []byte, error) {
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, nil, err
	}

	parsed, err := NewBTCTxFromBytes(txBytes)

	if err != nil {
		return nil, nil, err
	}

	return parsed, txBytes, nil
}

func SerializeBTCTx(tx *wire.MsgTx) ([]byte, error) {
	var txBuf bytes.Buffer
	if err := tx.Serialize(&txBuf); err != nil {
		return nil, err
	}
	return txBuf.Bytes(), nil
}

func SerializeBTCTxToHex(tx *wire.MsgTx) (string, error) {
	bytes, err := SerializeBTCTx(tx)

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil

}

func PubKeyFromHex(hexString string) (*btcec.PublicKey, error) {
	bytes, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, err
	}

	key, err := schnorr.ParsePubKey(bytes)

	if err != nil {
		return nil, err
	}

	return key, nil
}

func SchnorSignatureFromHex(hexString string) (*schnorr.Signature, error) {
	bytes, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, err
	}

	sig, err := schnorr.ParseSignature(bytes)

	if err != nil {
		return nil, err
	}

	return sig, nil
}
