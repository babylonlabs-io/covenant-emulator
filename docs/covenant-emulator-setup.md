# Covenant Emulator Setup

This document outlines the setup of the covenant-emulator
daemon program.

## Table of Contents

1. [Prerequesites](#1-prerequisites)
2. [Boot Order and Signer Dependency](#2-boot-order-and-signer-dependency)
3. [Install Covenant Emulator Binary](#3-install-covenant-emulator-binary)
4. [Setting up the Covenant Emulator Program](#4-setting-up-the-covenant-emulator-program)
	1. [Initialize directories](#41-initialize-directories)
	2. [Configure the covenant emulator](#42-configure-the-covenant-emulator)
5. [Generate key pairs](#5-generate-key-pairs)
6. [Start the emulator daemon](#6-start-the-emulator-daemon)

## 1. Prerequisites

To successfully complete this guide, you will need:

1. A running instance of the [covenant signer](../covenant-signer)
  with the url that you configured it to. Please follow the
  [covenant signer setup guide](./covenant-signer-setup.md) to
  complete the setup of the covenant signer with your keys before proceeding.
  Note that the phase-2 covenant-signer program is a different one than the one
  used doing phase-1
2. A connection to a Babylon node. To run your own node, please refer to the
  [Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/main/bbn-test-5/bbn-test-5/babylon-node/README.md).

## 2. Boot Order and Signer Dependency

The Covenant Emulator requires the Covenant Signer to be running and
unlocked before it can start. This is a critical dependency that is enforced 
through a health check during the emulator's startup process.

### Required Boot Sequence

1. Start the Covenant Signer:
   ```shell
   # Start the signer service
   covenant-signer start
   ```

2. Unlock the Covenant Signer:
   ```shell
   # Unlock the signer (if required)
   covenant-signer unlock
   ```

3. Verify the signer is running and accessible:
   ```shell
   # Test the signer's health
   curl -X GET http://<signer-url>/health
   ```

4. Start the Covenant Emulator:
   ```shell
   covd start --home <path>
   ```

### Health Check Behavior

The emulator performs an automatic health check during startup by attempting 
to fetch the signer's public key. If this check fails, the emulator will:
- Log an error message indicating the signer is not accessible
- Provide guidance to ensure the signer is running and unlocked
- Exit with a non-zero status code

### Troubleshooting

If the emulator fails to start with a signer-related error:
1. Verify the signer is running and accessible at the configured URL
2. Check if the signer is locked and needs to be unlocked
3. Verify the HMAC key in the emulator's configuration matches the signer's configuration
4. Check the signer's logs for any errors or issues

## 3. Install covenant emulator binary

If you haven't already, download [Golang 1.23](https://go.dev/dl).

Once installed, run:

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

Next, initialize the node and home directory by generating all of the
necessary files such as `covd.conf`. These files will live in the `<path>`
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

Use the following parameters to configure the `covd.conf` file.

```
# The interval between each query for pending BTC delegations
QueryInterval = 15s

# The maximum number of delegations that the covd processes each time
DelegationLimit = 100

# Bitcoin network to run on
BitcoinNetwork = signet

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

; HMAC key for authenticating requests to the remote signer
; Leave empty to disable HMAC authentication (not recommended for production)
HMACKey = ""
```

Below are brief explanations of the configuration entries:

- `QueryInterval` - How often to check for new BTC delegations that need processing
- `DelegationLimit` - Maximum number of delegations to process in a single batch
- `BitcoinNetwork` - Which Bitcoin network to connect to (mainnet, testnet, signet, etc.)
- `ChainID` - Unique identifier of the Babylon blockchain network
- `RPCAddr` - HTTP endpoint for connecting to a Babylon node
- `GRPCAddr` - gRPC endpoint for connecting to a Babylon node
- `Key` - Name of the key in the keyring used for transaction signing
- `KeyringBackend` - Storage backend for the keyring (os, file, kwallet, pass, test, memory)

> **IMPORTANT LIMITATION**: The covenant emulator only supports the `test` 
keyring backend. This limitation exists because the emulator requires automated
signing capabilities and the test backend is the only one that supports this
feature without manual passphrase entry. Other keyring backends 
(os, file, kwallet, pass, memory) will not work and will
cause the emulator to fail at startup.

- `URL` - Endpoint where the remote signing service is running
- `Timeout` - Maximum time to wait for remote signer responses
- `HMACKey` - Secret key for HMAC authentication with the covenant-signer

### 4.3. Configure HMAC Authentication (Recommended for Production)

HMAC (Hash-based Message Authentication Code) authentication provides an additional layer of security for the communication between the covenant-emulator and covenant-signer by ensuring that only authenticated requests are processed.

#### Setting up HMAC Authentication

1. **Generate a strong random key**:
   ```bash
   # Generate a 32-byte random key and encode it as hex
   openssl rand -hex 32
   ```

2. **Configure the covenant-emulator**:
   Add the HMAC key to your `covd.conf` file in the `[remotesigner]` section:
   ```ini
   [remotesigner]
   URL = http://127.0.0.1:9792
   Timeout = 2s
   HMACKey = "your-generated-random-key"
   ```

3. **Configure the covenant-signer**:
   The same HMAC key must be configured in your covenant-signer's configuration. Refer to the covenant-signer setup guide to set the matching key.


### Verification

When HMAC authentication is properly configured, all requests between the covenant-emulator and covenant-signer will include an authentication header. If there's a key mismatch or other authentication issues, you'll see error messages in the logs.


## 5. Generate key pairs

The covenant emulator daemon requires the existence of a Babylon keyring that
signs signatures and interacts with Babylon. Use the following command to generate
the key:

```bash
covd create-key --key-name <name> --chain-id <chain-id> --keyring-backend <backend>
{
    "name": "babylon-tx-key",
    "public-key-hex": "6dd4c9415a4091b74f45fdce71f5b8eebe743e5990b547009ff1dce8393d5df2",
    "babylon-address": "bbn1gw5ns0kmcjj8y0edu5h4nkkw4aq263eyx2ynnp"
}
```

Parameters:
- `--key-name`: Name for the key in the keyring
- `--chain-id`: ID of the Babylon chain (e.g., bbn-test-5)
- `--keyring-backend`: Backend for key storage, we will use `test`
  for this guide.

After executing the above command, the key name will be saved in the config file
created in the last [step](#42-configure-the-covenant-emulator).

**⚡ Note:** that the `public-key` in the output should be used as one of the
inputs of the genesis of the Babylon chain.

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