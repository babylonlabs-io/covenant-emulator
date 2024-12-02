# Transfer your setup from a covenant signer to a covenant emulator

## Table of Contents 

1. [Purpose of this guide](#1-purpose-of-this-guide)
2. [Prerequesites](#2-prerequisites)
3. [Install Covenant Emulator Binary](#3-install-covenant-emulator-binary)
4. [Setting up the Covenant Emulator Program](#4-setting-up-the-covenant-emulator-program)
	1. [Initialize directories](#41-initialize-directories)
	2. [Configure the covenant emulator](#42-configure-the-covenant-emulator)
5. [Generate key pairs](#5-generate-key-pairs)
6. [Start the emulator daemon](#6-start-the-emulator-daemon)

## 1. Purpose of this guide

This guide outlines the transition from solely using the covenant signer to an 
integrated setup that includes the covenant emulator.

Previously, the [covenant signer](https://github.com/babylonlabs-io/covenant-signer), 
was limited to signing unbonding signatures.  In this transition we are introducing 
the [covenant emulator](https://github.com/babylonlabs-io/covenant-emulator), which 
retrieves delegations from Babylon chain and signs them by communicating with the 
updated [covenant signer](https://github.com/babylonlabs-io/covenant-emulator/tree/main/covenant-signer). 
This means that the covenant emulator can now generate both unbonding signatures 
unbonding signatures and adaptor signatures.

In this guide, we will cover exporting the key from the Bitcoin node and importing 
it into the new integrated keyring in the covenant signer. 

## 2. Prerequisites

To successfully complete this guide, you will need a running instance of the 
[covenant signer](./covenant-signer) with your keys imported from `bitcoind` 
wallet into the cosmos keyring.

Please follow the [covenant signer setup guide](covenant-signer/README.md) to 
complete the setup of the covenant signer with your keys before proceeding.

## 3. Install covenant emulator binary

Once you have the covenant signer running, you can install the covenant emulator 
binary.

```shell
git clone git@github.com:babylonlabs-io/covenant-emulator.git
cd covenant-emulator
git checkout <tag>
```

Run the following command to build the binaries and
install them to your `$GOPATH/bin` directory:

```shell
make install
```

This command will:
- Build and compile all Go packages
- Install `covd` binary to `$GOPATH/bin`
- Make commands globally accessible from your terminal

If your shell cannot find the installed binary, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 4. Setting up the covenant emulator program

### 4.1. Initialize directories

Next, we initialize the node and home directory. It should generate all of the 
necessary files such as `covd.config`, these files will live in the `<path>` 
that you set for the `--home`with the below command.

```shell
covd init --home <path>
```

After initialization, the home directory will have the following structure:

```shell
$ ls <path>
  ├── covd.conf # Covd-specific configuration file.
  ├── logs      # Covd logs
```

### 4.2. Configure the covenant emulator

As you have already set up the covenant signer, you can now configure the covenant 
emulator to use it.

Use the following parameters to configure the `covd.conf` file.

```
# The interval between each query for pending BTC delegations
QueryInterval = 15s

# The maximum number of delegations that the covd processes each time
DelegationLimit = 100

# Bitcoin network to run on
BitcoinNetwork = simnet

# Babylon specific parameters

# Babylon chain ID
ChainID = bbn-test-5

# Babylon node RPC endpoint
RPCAddr = https://rpc-euphrates.devnet.babylonlabs.io:443

# Babylon node gRPC endpoint
GRPCAddr = https://rpc-euphrates.devnet.babylonlabs.io:443

# Name of the key in the keyring to use for signing transactions
Key = covenant-key

# Type of keyring to use,
# supported backends - (os|file|kwallet|pass|test|memory)
# ref https://docs.cosmos.network/v0.46/run-node/keyring.html#available-backends-for-the-keyring
KeyringBackend = test

[remotesigner]
; URL of the remote signer
URL = http://127.0.0.1:9792

; client when making requests to the remote signer
Timeout = 2s

; if true, covenant will use the remote signer to sign transactions
RemoteSignerEnabled = true
```

Ensure that the covenant signer is running and unlocked before proceeding 
otherwise you will be unable to run the emulator.

## 5. Generate key pairs

The covenant emulator daemon requires the existence of a keyring that signs
signatures and interacts with Babylon. Use the following command to generate the
key:

```bash
$ covd create-key --key-name covenant-key --chain-id chain-test
{
    "name": "covenant-key",
    "public-key": "9bd5baaba3d3fb5a8bcb8c2995c51793e14a1e32f1665cade168f638e3b15538"
}
```

After executing the above command, the key name will be saved in the config file
created in [step](#configuration).
Note that the `public-key` in the output should be used as one of the inputs of
the genesis of the Babylon chain.
Also, this key will be used to pay for the fees due to the daemon submitting 
signatures to Babylon.

## 6. Start the emulator daemon

You can start the covenant emulator daemon using the following command:

```bash
$ covd start
2024-01-05T05:59:09.429615Z	info	Starting Covenant Emulator
2024-01-05T05:59:09.429713Z	info	Covenant Emulator Daemon is fully active!
```

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.

Next you will need to unlock the key and sign transactions. Please refer to the 
[covenant signer setup guide](covenant-signer/README.md#using-the-covenant-signer-for-signing-transactions) 
to unlock the key and sign any transactions that are needed.

