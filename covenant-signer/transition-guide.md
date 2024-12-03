# Covenant Signer

The Covenant Signer is a daemon program in the Covenant Emulator toolset
that is responsible for securely managing the private key of the
covenant committee member and producing the necessary cryptographic
signatures.

It prioritizes security through isolation, ensuring that private key handling
is confined to an instance with minimal connectivity and simpler application 
logic compared to the Covenant Emulator daemon.

> ⚡ Note: This program is a separate implementation from the
[covenant signer](https://github.com/babylonlabs-io/covenant-signer/blob/main/README.md)
program used for phase-1. All covenant committee members
are required to transition their keys to this program to participate
in phase-2.

This document is intended for covenant committee members that
are transitioning their phase-1 set up to the phase-2 one.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Installation](#2-installation)
3. [Transitioning your covenant key from phase-1 setup](#3-transitioning-your-covenant-key-from-phase-1-setup)
4. [Operation](#4-operation)
    1. [Configuration](#41-configuration)
    2. [Starting the daemon](#42-starting-the-daemon)
    3. [Unlocking the key](#43-unlocking-the-key)
    4. [Testing the setup](#44-testing-the-setup)

## 1. Prerequisites

This guide requires that:

1. you have a Bitcoin node setup for the Bitcoin
network you intend to operate your covenant signer on and
2. you have access to the the private Bitcoin key you
set up your covenant with.

For a refresher on setting up the Bitcoin node, refer to the 
[deployment guide of your phase-1 covenant signer setup](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup).

<!-- TODO: Add a link to the deployment guide instructions when above link is archived -->

## 2. Installation

If you haven't already, download [Golang 1.23](https://go.dev/dl).

Once installed run: 

```shell
go version
```

If you have not yet cloned the repository, run:

```shell
git clone git@github.com:babylonlabs-io/covenant-emulator.git
cd covenant-emulator
git checkout v0.3.0
```
<!-- TODO: check the version of the tag after babylon release -->

> ⚡ Note: Replace the checkout tag with the version you want to install.

Run the following command to build the binaries and
install them to your `$GOPATH/bin` directory:

```shell
cd covenant-signer
make install
```

This command will:
- Build and compile all Go packages
- Install `covenant-signer` binary to `$GOPATH/bin`
- Make commands globally accessible from your terminal

If your shell cannot find the installed binary, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 3. Transitioning your covenant key from phase-1 setup

At this stage, you should already have access to the Bitcoin node. 
If you need a refresher on setting up `bitcoind`, refer to the setup guide. 
Once you have node access, you can proceed with the next steps.

To start off, connect to your Bitcoin node machine and load the wallet that 
contains your covenant key.

```shell
bitcoin-cli loadwallet "covenant-wallet"
```

Parameters:
- `loadwallet`: (string, required) The wallet directory or .dat file. In our 
  example we have named it `covenant-wallet`.

The above input will output the following response:

{
  "name": "covenant-wallet"
}

To verify the key was successfully imported, you can retrieve the key information 
using the command below. Make note of the HD key path information - you'll need 
it later when deriving the covenant private key from the master key. 

```shell
bitcoin-cli getaddressinfo bcrt1qazasawj3ard0ffwj04zpxlw2pt9cp7kwmnqyvk
```

Parameters:
- `getaddressinfo`: (string, required) The Bitcoin address for which to get 
  information on.

This should generate output information on your address.

```json
{
  "address": "bcrt1qazasawj3ard0ffwj04zpxlw2pt9cp7kwmnqyvk",
  "scriptPubKey": "0014e8bb0eba51e8daf4a5d27d44137dca0acb80face",
  "ismine": true,
  "solvable": true,
  "desc": "wpkh([5e174bde/84h/1h/0h/0/0]023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1)#ye9usklr",
  "parent_desc": "wpkh([5e174bde/84h/1h/0h]tpubDDeLN74J6FLfbwXGzwrqQ8ZCG9e4c9uVLP5TjLxLwZVNewFwZ5qB14mu7Fa1g1MStVvUAwXDZHkBzjjNpiRCq9JoA8yxDW9hh7xyqGkhter/0/*)#59fjcx8s",
  "iswatchonly": false,
  "isscript": false,
  "iswitness": true,
  "witness_version": 0,
  "witness_program": "e8bb0eba51e8daf4a5d27d44137dca0acb80face",
  "pubkey": "023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1",
  "ischange": false,
  "timestamp": 1732624709,
  "hdkeypath": "m/84h/1h/0h/0/0",
  "hdseedid": "0000000000000000000000000000000000000000",
  "hdmasterfingerprint": "5e174bde",
  "labels": [
    ""
  ]
}
```

As mentioned above, the most important field to focus on is `hdkeypath` that 
contains the derivation path of our key. In the example it is `84h/1h/0h/0/0` 
(the initial `m/` can be ignored).

Next, we need to retrieve the **base58-encoded master private key** from the 
wallet. This is the key we will use to derive the covenant private key. From 
here we will be able to import this directly into the Cosmos keyring.

To do this, we need to list all descriptors in the wallet ensuring that private 
keys are included in the output. We do this as we need to collect the descriptor 
of the key we want to derive the private key from.

```shell
bitcoin-cli -chain=regtest -rpcuser=user -rpcpassword=pass listdescriptors true | jq -r '.descriptors[] | select(.desc | contains("wpkh(")) | .desc | capture("wpkh\\((?<key>.*?)\\)").key | sub("/\\*$"; "")'
```

The terminal will output your base58-encoded master private key and the 
`hdkeypath`, which should match above. 

```shell
tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq/84h/1h/0h/0
```

The key has been successfully imported. In the next step, we'll retrieve two 
important pieces of information:
1. The **base58-encoded master private key**
  `tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq`
2. The `hdkeypath` which should be similar to what we saved in the 
`getaddressinfo` above.

#### 3.2 Deriving the Covenant Private Key from the Master Key

Next, we'll derive the covenant private key from the master key using 
**BIP32 derivation**. You'll need:

1. The `hdkeypath` we saved from the `getaddressinfo` command
2. Access to the `covenant-signer` directory, which contains the derivation tool

Navigate to the `covenant-signer` directory and run the following command:

```shell
covenant-signer derive-child-key \
  tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq \
  84h/1h/0h/0/0
```

The output will display the derived private and public keys:

```shell
Derived private key: fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06
Derived public key: 023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

You can see that the derived public key matches the public key obtained earlier 
using the `getaddressinfo` command.

We will now use the derived private key from above and import it into the 
Cosmos keyring. To do this, use the following command:

```shell
babylond keys import-hex cov fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06 --keyring-backend file
```

> ⚡ Note: Use the `file` backend to store the private key in encrypted form on 
disk. When running `import-hex` with the encrypted file backend, you will be 
prompted for a passphrase. This passphrase will be required to unlock the signer 
later.

To confirm that the import was successful, run:

```shell
babylond keys show cov
```

The output will display the details of the imported key:

```shell
    address: bbn1azasawj3ard0ffwj04zpxlw2pt9cp7kwjcdqmc
    name: cov
    pubkey: '{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"Ajp5tUbHnX98X/IGINkUtc9yUGMdEvbiZCftnT+Yxcyx"}'
    type: local

```

Here, the `key` field contains the base64-encoded public key. After decoding, 
this key:

```shell
023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

Matches the public key derived earlier and seen in the outputs of `getaddressinfo` 
and `derive-child-key`.

Congratulations! You have successfully imported your keys from the prior setup 
and verified your setup for the covenant emulator.

## 4. Operation
### 4.1. Configuration

Use the example configuration [file](./example/config.toml) to create your own 
configuration file. Then, replace the placeholder values with your own 
configuration.

```toml
[keystore]
keystore-type = "cosmos"

[keystore.cosmos]
# pointing to the directory where the key is stored, unless specified otherwise
key-directory = "/path/to/keydir"

# the backend to be used for storing the key, in this case file
keyring-backend = "file"

# the name of the key you used when importing the key
key-name = "your-key-name"

# the chain id of the chain to be used
chain-id = "your-chain-id"

[server-config]
# The IP address where the covenant-signer server will listen
host = "127.0.0.1"
# The TCP port number where the covenant-signer server will listen
port = 9791

[metrics]
# The IP address where the Prometheus metrics server will listen
host = "127.0.0.1"
# This port is used to expose metrics that can be scraped by Prometheus
port = 2113
```

Below are brief explanations of the configuration entries:
- `keystore-type`: Type of keystore used, which is "cosmos"
- `key-directory`: Path where keys are stored on the filesystem.
- `keyring-backend`: Backend system for key management, e.g., "file", "os".
- `key-name`: Name of the key used for signing transactions.
- `chain-id`: Identifier of the blockchain network.
- `host` (server-config): IP address where the server listens, typically "127.0.0.1" for local access.
- `port` (server-config): TCP port number for the server.
- `host` (metrics): IP address for the Prometheus metrics server, typically "127.0.0.1".
- `port` (metrics): TCP port number for the Prometheus metrics server.

### 4.2. Starting the daemon

The covenant signer can be run using the following command:

```shell
covenant-signer start --config ./path/to/config.toml
```

The covenant signer must be run in a secure network and only accessible by the 
covenant emulator.

Once the covenant signer is set up and unlocked, you can configure the covenant 
emulator to use it. The URL of the covenant signer, which is configured by the 
covenant members but in this example we use the default value of 
`http://127.0.0.1:9791`, should be specified in the covenant emulator's 
configuration file under the `remotesigner` section.

It's important to note that the key specified in the covenant emulator's 
configuration is not the covenant key itself. Instead, it is a 
key used for sending Cosmos transactions.

### 4.3. Unlocking the key

Before you can sign transactions with the covenant key, you must unlock the 
keyring that stores it. The passphrase here must match the one you used when 
importing the covenant key into the keyring. Once the keyring is unlocked, you 
can use the covenant key to sign transactions.

```shell
curl -X POST http://127.0.0.1:9791/v1/unlock -d '{"passphrase": "<passphrase>"}'
```

You can sign transactions using the following command. However, ensure that both 
the staking and unbonding transactions are provided in hex format.

```shell
curl -X POST http://127.0.0.1:9791/v1/sign-transactions \
  -d '{
        "staking_tx_hex": "020000000001",
        "slashing_tx_hex": "020000000001...",
        "unbonding_tx_hex": "020000000001...",
        "slash_unbonding_tx_hex": "020000000001...",
        "staking_output_idx": 0,
        "slashing_script_hex": "76a914...",
        "unbonding_script_hex": "76a914...",
        "unbonding_slashing_script_hex": "76a914...",
        "fp_enc_keys": [
            "0123456789abcdef..."
        ]
      }'
```

### 4.4. Testing the setup

This will generate a signature for the provided transactions and return it in JSON 
format.

```json
{
    "staking_tx_signature": "<hex_encoded_signature>",
    "unbonding_tx_signature": "<hex_encoded_signature>"
}
```

These signatures can then be used to verify that the transactions were signed by 
the covenant key.

Congratulations! You have successfully setup the covenant signer and are now able 
to sign transactions with the covenant key.