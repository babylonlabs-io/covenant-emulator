package e2etest

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/babylonlabs-io/babylon/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"

	"github.com/stretchr/testify/require"
)

type babylonNode struct {
	cmd          *exec.Cmd
	pidFile      string
	dataDir      string
	nodeHome     string
	chainID      string
	slashingAddr string
	covenantPk   *types.BIP340PubKey
}

func newBabylonNode(dataDir, nodeHome string, cmd *exec.Cmd, chainID string, slashingAddr string, covenantPk *types.BIP340PubKey) *babylonNode {
	return &babylonNode{
		dataDir:      dataDir,
		nodeHome:     nodeHome,
		cmd:          cmd,
		chainID:      chainID,
		slashingAddr: slashingAddr,
		covenantPk:   covenantPk,
	}
}

func (n *babylonNode) start() error {
	if err := n.cmd.Start(); err != nil {
		return err
	}

	pid, err := os.Create(filepath.Join(n.dataDir,
		fmt.Sprintf("%s.pid", "config")))
	if err != nil {
		return err
	}

	n.pidFile = pid.Name()
	if _, err = fmt.Fprintf(pid, "%d\n", n.cmd.Process.Pid); err != nil {
		return err
	}

	if err := pid.Close(); err != nil {
		return err
	}

	return nil
}

func (n *babylonNode) stop() (err error) {
	if n.cmd == nil || n.cmd.Process == nil {
		// return if not properly initialized
		// or error starting the process
		return nil
	}

	defer func() {
		err = n.cmd.Wait()
	}()

	if runtime.GOOS == "windows" {
		return n.cmd.Process.Signal(os.Kill)
	}
	return n.cmd.Process.Signal(os.Interrupt)
}

func (n *babylonNode) cleanup() error {
	if n.pidFile != "" {
		if err := os.Remove(n.pidFile); err != nil {
			log.Printf("unable to remove file %s: %v", n.pidFile,
				err)
		}
	}

	dirs := []string{
		n.dataDir,
	}
	var err error
	for _, dir := range dirs {
		if err = os.RemoveAll(dir); err != nil {
			log.Printf("Cannot remove dir %s: %v", dir, err)
		}
	}
	return nil
}

func (n *babylonNode) shutdown() error {
	if err := n.stop(); err != nil {
		return err
	}
	if err := n.cleanup(); err != nil {
		return err
	}
	return nil
}

type BabylonNodeHandler struct {
	babylonNode *babylonNode
}

func NewBabylonNodeHandler(t *testing.T, covenantPk *types.BIP340PubKey) *BabylonNodeHandler {
	testDir, err := baseDir("zBabylonTest")
	require.NoError(t, err)
	defer func() {
		if err != nil {
			err := os.RemoveAll(testDir)
			require.NoError(t, err)
		}
	}()

	nodeHome := filepath.Join(testDir, "node0", "babylond")

	slashingAddr := "SZtRT4BySL3o4efdGLh3k7Kny8GAnsBrSW"
	decodedAddr, err := btcutil.DecodeAddress(slashingAddr, &chaincfg.SimNetParams)
	require.NoError(t, err)
	pkScript, err := txscript.PayToAddrScript(decodedAddr)
	require.NoError(t, err)

	initTestnetCmd := exec.Command(
		"babylond",
		"testnet",
		"--v=1",
		fmt.Sprintf("--output-dir=%s", testDir),
		"--starting-ip-address=192.168.10.2",
		"--keyring-backend=test",
		"--chain-id=chain-test",
		"--additional-sender-account",
		"--min-staking-time-blocks=100",
		"--min-staking-amount-sat=10000",
		// default checkpoint finalization timeout is 20, so we set min unbonding time
		// to be 1 block more
		"--unbonding-time=21",
		fmt.Sprintf("--slashing-pk-script=%s", hex.EncodeToString(pkScript)),
		fmt.Sprintf("--covenant-pks=%s", covenantPk.MarshalHex()),
		"--covenant-quorum=1",
	)

	var stderr bytes.Buffer
	initTestnetCmd.Stderr = &stderr

	err = initTestnetCmd.Run()
	if err != nil {
		fmt.Printf("init testnet failed: %s \n", stderr.String())
	}
	require.NoError(t, err)

	f, err := os.Create(filepath.Join(testDir, "babylon.log"))
	require.NoError(t, err)

	startCmd := exec.Command(
		"babylond",
		"start",
		fmt.Sprintf("--home=%s", nodeHome),
		"--log_level=debug",
	)

	startCmd.Stdout = f

	return &BabylonNodeHandler{
		babylonNode: newBabylonNode(testDir, nodeHome, startCmd, chainID, slashingAddr, covenantPk),
	}
}

func (w *BabylonNodeHandler) Start() error {
	if err := w.babylonNode.start(); err != nil {
		// try to cleanup after start error, but return original error
		_ = w.babylonNode.cleanup()
		return err
	}
	return nil
}

func (w *BabylonNodeHandler) Stop() error {
	if err := w.babylonNode.shutdown(); err != nil {
		return err
	}

	return nil
}

func (w *BabylonNodeHandler) GetNodeDataDir() string {
	dir := filepath.Join(w.babylonNode.dataDir, "node0", "babylond")
	return dir
}

func (w *BabylonNodeHandler) GetCovenantPk() *types.BIP340PubKey {
	return w.babylonNode.covenantPk
}
