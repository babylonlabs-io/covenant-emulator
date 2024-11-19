package btcclient

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/babylonlabs-io/covenant-emulator/covenant-signer/config"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"

	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txauthor"
	notifier "github.com/lightningnetwork/lnd/chainntnfs"
)

type TxStatus int

const (
	TxNotFound TxStatus = iota
	TxInMemPool
	TxInChain
)

const txNotFoundErrMsgBitcoind = "No such mempool or blockchain transaction"

func nofitierStateToClientState(state notifier.TxConfStatus) TxStatus {
	switch state {
	case notifier.TxNotFoundIndex:
		return TxNotFound
	case notifier.TxFoundMempool:
		return TxInMemPool
	case notifier.TxFoundIndex:
		return TxInChain
	case notifier.TxNotFoundManually:
		return TxNotFound
	case notifier.TxFoundManually:
		return TxInChain
	default:
		panic(fmt.Sprintf("unknown notifier state: %s", state))
	}
}

type BtcClient struct {
	RpcClient *rpcclient.Client
}

func btcConfigToConnConfig(cfg *config.ParsedBtcConfig) *rpcclient.ConnConfig {
	return &rpcclient.ConnConfig{
		Host:                 cfg.Host,
		User:                 cfg.User,
		Pass:                 cfg.Pass,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}
}

// client from config
func NewBtcClient(cfg *config.ParsedBtcConfig) (*BtcClient, error) {
	rpcClient, err := rpcclient.New(btcConfigToConnConfig(cfg), nil)

	if err != nil {
		return nil, err
	}

	return &BtcClient{RpcClient: rpcClient}, nil
}

func (c *BtcClient) SendTx(tx *wire.MsgTx) (*chainhash.Hash, error) {
	return c.RpcClient.SendRawTransaction(tx, true)
}

// Helpers to easily build transactions
type Utxo struct {
	Amount       btcutil.Amount
	OutPoint     wire.OutPoint
	PkScript     []byte
	RedeemScript []byte
	Address      string
}

type byAmount []Utxo

func (s byAmount) Len() int           { return len(s) }
func (s byAmount) Less(i, j int) bool { return s[i].Amount < s[j].Amount }
func (s byAmount) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func resultsToUtxos(results []btcjson.ListUnspentResult, onlySpendable bool) ([]Utxo, error) {
	var utxos []Utxo
	for _, result := range results {
		if onlySpendable && !result.Spendable {
			// skip unspendable outputs
			continue
		}

		amount, err := btcutil.NewAmount(result.Amount)

		if err != nil {
			return nil, err
		}

		chainhash, err := chainhash.NewHashFromStr(result.TxID)

		if err != nil {
			return nil, err
		}

		outpoint := wire.NewOutPoint(chainhash, result.Vout)

		script, err := hex.DecodeString(result.ScriptPubKey)

		if err != nil {
			return nil, err
		}

		redeemScript, err := hex.DecodeString(result.RedeemScript)

		if err != nil {
			return nil, err
		}

		utxo := Utxo{
			Amount:       amount,
			OutPoint:     *outpoint,
			PkScript:     script,
			RedeemScript: redeemScript,
			Address:      result.Address,
		}
		utxos = append(utxos, utxo)
	}
	return utxos, nil
}

func makeInputSource(utxos []Utxo) txauthor.InputSource {
	currentTotal := btcutil.Amount(0)
	currentInputs := make([]*wire.TxIn, 0, len(utxos))
	currentScripts := make([][]byte, 0, len(utxos))
	currentInputValues := make([]btcutil.Amount, 0, len(utxos))

	return func(target btcutil.Amount) (btcutil.Amount, []*wire.TxIn,
		[]btcutil.Amount, [][]byte, error) {

		for currentTotal < target && len(utxos) != 0 {
			nextCredit := &utxos[0]
			utxos = utxos[1:]
			nextInput := wire.NewTxIn(&nextCredit.OutPoint, nil, nil)
			currentTotal += nextCredit.Amount
			currentInputs = append(currentInputs, nextInput)
			currentScripts = append(currentScripts, nextCredit.PkScript)
			currentInputValues = append(currentInputValues, nextCredit.Amount)
		}
		return currentTotal, currentInputs, currentInputValues, currentScripts, nil
	}
}

func buildTxFromOutputs(
	utxos []Utxo,
	outputs []*wire.TxOut,
	feeRatePerKb btcutil.Amount,
	changeScript []byte) (*wire.MsgTx, error) {

	if len(utxos) == 0 {
		return nil, fmt.Errorf("there must be at least 1 usable UTXO to build transaction")
	}

	if len(outputs) == 0 {
		return nil, fmt.Errorf("there must be at least 1 output in transaction")
	}

	ch := txauthor.ChangeSource{
		NewScript: func() ([]byte, error) {
			return changeScript, nil
		},
		ScriptSize: len(changeScript),
	}

	inputSource := makeInputSource(utxos)

	authoredTx, err := txauthor.NewUnsignedTransaction(
		outputs,
		feeRatePerKb,
		inputSource,
		&ch,
	)

	if err != nil {
		return nil, err
	}

	return authoredTx.Tx, nil
}

func (w *BtcClient) UnlockWallet(timoutSec int64, passphrase string) error {
	return w.RpcClient.WalletPassphrase(passphrase, timoutSec)
}

func (w *BtcClient) DumpPrivateKey(address btcutil.Address) (*btcec.PrivateKey, error) {
	privKey, err := w.RpcClient.DumpPrivKey(address)

	if err != nil {
		return nil, err
	}

	return privKey.PrivKey, nil
}

func (w *BtcClient) CreateTransaction(
	outputs []*wire.TxOut,
	feeRatePerKb btcutil.Amount,
	changeAddres btcutil.Address) (*wire.MsgTx, error) {

	utxoResults, err := w.RpcClient.ListUnspent()

	if err != nil {
		return nil, err
	}

	utxos, err := resultsToUtxos(utxoResults, true)

	if err != nil {
		return nil, err
	}

	// sort utxos by amount from highest to lowest, this is effectively strategy of using
	// largest inputs first
	sort.Sort(sort.Reverse(byAmount(utxos)))

	changeScript, err := txscript.PayToAddrScript(changeAddres)

	if err != nil {
		return nil, err
	}

	tx, err := buildTxFromOutputs(utxos, outputs, feeRatePerKb, changeScript)

	if err != nil {
		return nil, err
	}

	return tx, err
}

func (w *BtcClient) CreateAndSignTx(
	outputs []*wire.TxOut,
	feeRatePerKb btcutil.Amount,
	changeAddress btcutil.Address,
) (*wire.MsgTx, error) {
	tx, err := w.CreateTransaction(outputs, feeRatePerKb, changeAddress)

	if err != nil {
		return nil, err
	}

	fundedTx, signed, err := w.SignRawTransaction(tx)

	if err != nil {
		return nil, err
	}

	if !signed {
		// TODO: Investigate this case a bit more thoroughly, to check if we can recover
		// somehow
		return nil, fmt.Errorf("not all transactions inputs could be signed")
	}

	return fundedTx, nil
}

func (w *BtcClient) SignRawTransaction(tx *wire.MsgTx) (*wire.MsgTx, bool, error) {
	return w.RpcClient.SignRawTransactionWithWallet(tx)
}

func (w *BtcClient) ListOutputs(onlySpendable bool) ([]Utxo, error) {
	utxoResults, err := w.RpcClient.ListUnspent()

	if err != nil {
		return nil, err
	}

	utxos, err := resultsToUtxos(utxoResults, onlySpendable)

	if err != nil {
		return nil, err
	}

	return utxos, nil
}

func (w *BtcClient) TxDetails(txHash *chainhash.Hash, pkScript []byte) (*notifier.TxConfirmation, TxStatus, error) {
	req, err := notifier.NewConfRequest(txHash, pkScript)

	if err != nil {
		return nil, TxNotFound, err
	}

	res, state, err := notifier.ConfDetailsFromTxIndex(w.RpcClient, req, txNotFoundErrMsgBitcoind)

	if err != nil {
		return nil, TxNotFound, err
	}

	return res, nofitierStateToClientState(state), nil
}

func (w *BtcClient) SignPsbt(packet *psbt.Packet) (*psbt.Packet, error) {
	psbtEncoded, err := packet.B64Encode()

	if err != nil {
		return nil, err
	}

	sign := true
	result, err := w.RpcClient.WalletProcessPsbt(
		psbtEncoded,
		&sign,
		// TODO: Hacky way of forcing bitcoind to use sighash DEFAULT
		"DEFAULT",
		nil,
	)

	if err != nil {
		return nil, err
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(result.Psbt)

	if err != nil {
		return nil, err
	}

	decoded, err := psbt.NewFromRawBytes(bytes.NewReader(decodedBytes), false)

	if err != nil {
		return nil, err
	}

	return decoded, nil
}

func (w *BtcClient) BestBlockHeight() (uint32, error) {
	count, err := w.RpcClient.GetBlockCount()

	if err != nil {
		return 0, err
	}
	//#nosec G115 -- safe conversion, nubmer of blocks is always positive and less than math.MaxUint32
	return uint32(count), nil
}
