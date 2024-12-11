# Covenant Emulator Setup

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

This guide outlines the transition from solely using the phase-1 covenant signer
to the phase-2 covenant emulator full setup.

The [phase-1 covenant signer](https://github.com/babylonlabs-io/covenant-signer), 
was limited to signing unbonding signatures.  Phase-2 requires additional 
functionality that is covered by the 
[covenant emulator](https://github.com/babylonlabs-io/covenant-emulator), which 
retrieves delegations from Babylon chain and signs them by communicating with the 
a new [covenant signer daemon](https://github.com/babylonlabs-io/covenant-emulator/tree/main/covenant-signer), specifically focused on phase-2 functionality.

In this guide, we will cover exporting the key from the Bitcoin node and importing 
it into the new integrated keyring in the covenant signer. 

## 2. Prerequisites

To successfully complete this guide, you will need:

1. A running instance of the [covenant signer](../covenant-signer) 
  with the url that you configured it to. Please follow the 
  [covenant signer setup guide](covenant-signer/README.md) to 
  complete the setup of the covenant signer with your keys before proceeding.
2. A connection to a Babylon node. To run your own node, please refer to the 
  [Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/sam/bbn-test-5/bbn-test-5/babylon-node/README.md).

## 3. Install covenant emulator binary

If you haven't already, download [Golang 1.23](https://go.dev/dl).

Once installed run: 

```shell
go version
```

If you have not yet cloned the repository, run:

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
that you set for the `--home` with the below command.

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
# ref https://docs.cosmos.network/v0.50/run-node/keyring.html#available-backends-for-the-keyring
KeyringBackend = test

[remotesigner]
; URL of the remote signer
URL = http://127.0.0.1:9792

; client when making requests to the remote signer
Timeout = 2s

; if true, covenant will use the remote signer to sign transactions
RemoteSignerEnabled = true
```

Below are brief explanations of the configuration entries:

- `QueryInterval` - How often to check for new BTC delegations that need processing
- `DelegationLimit` - Maximum number of delegations to process in a single batch
- `BitcoinNetwork` - Which Bitcoin network to connect to (mainnet, testnet, simnet, etc.)
- `ChainID` - Unique identifier of the Babylon blockchain network
- `RPCAddr` - HTTP endpoint for connecting to a Babylon node
- `GRPCAddr` - gRPC endpoint for connecting to a Babylon node
- `Key` - Name of the key in the keyring used for transaction signing
- `KeyringBackend` - Storage backend for the keyring (os, file, kwallet, pass, test, memory)
- `URL` - Endpoint where the remote signing service is running
- `Timeout` - Maximum time to wait for remote signer responses
- `RemoteSignerEnabled` - Whether to use the remote signing service

Ensure that the covenant signer is running and unlocked before proceeding 
otherwise you will be unable to run the emulator.

## 5. Generate key pairs

The covenant emulator daemon requires the existence of a Babylon keyring that 
signs signatures and interacts with Babylon. Use the following command to generate 
the key:

```bash
covd create-key --key-name <name> --chain-id <chain-id> --keyring-backend <backend>
{
    "name": "covenant-key",
    "public-key-hex": "6dd4c9415a4091b74f45fdce71f5b8eebe743e5990b547009ff1dce8393d5df2",
    "babylon-address": "bbn1gw5ns0kmcjj8y0edu5h4nkkw4aq263eyx2ynnp"
}
```

Parameters:
- `--key-name`: Name for the key in the keyring
- `--chain-id`: ID of the Babylon chain (e.g., bbn-test-5)
- `--keyring-backend`: Backend for key storage, we will use `file` as it is the 
  most secure option.

After executing the above command, the key name will be saved in the config file
created in the last [step](#42-configure-the-covenant-emulator).

**Note:** that the `public-key` in the output should be used as one of the inputs of
the genesis of the Babylon chain.

Also, this key will be used to pay for the fees due to the daemon submitting 
signatures to Babylon.

To check your balance, View your account on the 
[Babylon Explorer](https://babylon-testnet.l2scan.co) by searching for your 
address.


## 6. Start the emulator daemon

You can start the covenant emulator daemon using the following command:

```bash
covd start
2024-01-05T05:59:09.429615Z	info	Starting Covenant Emulator
2024-01-05T05:59:09.429713Z	info	Covenant Emulator Daemon is fully active!
```

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.