# Covenant Signer Setup

> **⚡ Note:** This document is intended for covenant committee members that
> are setting up a phase-2 stack based on an existing phase-1 stack.

The Covenant Signer is a daemon program in the Covenant Emulator toolset
that is responsible for securely managing the private key of the
covenant committee member and producing the necessary cryptographic
signatures.

It prioritizes security through isolation, ensuring that private key handling
is confined to an instance with minimal connectivity and simpler application
logic.

> **⚡ Note:** This program is a separate implementation from the
> [covenant signer](https://github.com/babylonlabs-io/covenant-signer/)
> program used for phase-1. All covenant committee members
> are required to transition their keys to this program to participate
> in phase-2.

Previously, private keys were stored in the Bitcoin wallet using PSBT (Partially
Signed Bitcoin Transactions) for signing operations. The new design uses a
dedicated Covenant Signer that acts as a remote signing service, storing private
keys in an encrypted Cosmos SDK keyring. This approach not only improves security
through isolation but also enables the creation of both Schnorr signatures and
Schnorr adaptor signatures required for covenant operations.

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Shell Configuration](#2-shell-configuration)
3. [Installation](#3-installation)
4. [Transitioning your covenant key from phase-1 setup](#4-transitioning-your-covenant-key-from-phase-1-setup)
5. [Operation](#5-operation)
    1. [Configuration](#51-configuration)
    2. [Starting the daemon](#52-starting-the-daemon)
    3. [Unlocking the key](#53-unlocking-the-key)

## 1. Prerequisites

This guide requires that:

1. You have a Bitcoin node setup to load your wallet and retrieve
  your master private key.
2. You have access to the private Bitcoin key you set up your covenant with.
3. A connection to a Babylon node. To run your own node, please refer to the
  [Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/main/bbn-test-5/babylon-node/README.md).

For a refresher on setting up the Bitcoin node, refer to the
[deployment guide of your phase-1 covenant signer setup](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup).

<!-- TODO: Add a link to the deployment guide instructions when above link is archived -->

## 2. Shell Configuration

For security when entering sensitive commands, configure your shell to ignore
commands that start with a space:

For Bash users, please update if you are using a different shell.

```shell
# Add to your ~/.bashrc:
export HISTCONTROL=ignorespace

# Then either restart your shell or run:
source ~/.bashrc
```
Please ensure that any commands that you wish to be hidden from your shell
history start with a space.

## 3. Installation

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

> ⚡ Note: Replace the checkout tag with the version you want to install.

Enter the covenant-signer directory, and run the following
command to build the `covenant-signer` binary
and install it to your `$GOPATH/bin` directory:

```shell
cd covenant-signer
make install
```

This command will:
- Build and compile all Go packages
- Install `covenant-signer` binary to `$GOPATH/bin`
- Make it globally accessible from your terminal

If your shell cannot find the installed binary, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Use the following command to add it to your profile
depending on your shell.

```shell
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

## 4. Transitioning your covenant key from phase-1 setup

After installing the necessary binaries, we are ready
to transition our covenant private key from the `bitcoind` wallet
into a Cosmos keyring. This is necessary as the `bitcoind` wallet
does not have support for important covenant signer operations,
such as the generation of adaptor signatures.

To complete this process, you are going to need to have access
to the machine that holds your `bitcoind` wallet and
know the Bitcoin address associated with your covenant's public key.
If you need a refresher on the functionalities supported by your
`bitcoind` wallet or how you previously set it up, you can refer
to the relevant
[phase-1 guide](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup).

In the following, we'll go through all the necessary steps
to transition your wallet.

#### Step 1: Load wallet

We start off by loading the wallet holding the covenant keys,
using the `loadwallet` command. It receives as an argument
the wallet directory or `.dat` file. In the below example,
we are loading the wallet named `covenant-wallet`.

```shell
 bitcoin-cli loadwallet "covenant-wallet"
{
  "name": "covenant-wallet"
}
```

#### Step 2: Extract the covenant address' `hdkeypath`

Next, we are going to retrieve the `hdkeypath` of the Bitcoin address
associated with our covenant key.

We do this through the usage of the `getaddresssinfo` command
which takes your covenant Bitcoin address as a parameter. As mentioned above,
you will need access to the Bitcoin key you set up your covenant with.

```shell
 bitcoin-cli -datadir=./1/ getaddressinfo <address> | \
jq '.hdkeypath | sub("^m/"; "") | sub("/[^/]+$"; "")'
```

In the above command, we use the `jq` utility to extract only the relevant
`hdkeypath` information, which in this example is `84h/1h/0h/0/0`
(the initial `m/` can be ignored).

Make note of the `hdkeypath` information - you'll need it later when
deriving the covenant private key from the master key for verification purposes.

#### Step 3: Retrieve the master private key

In this step,
we are going to retrieve the **base58-encoded master private key** from the
Bitcoin wallet. This key will be used to derive the covenant private key,
which can then be imported directly into the Cosmos keyring.

The command below will list all descriptors in the wallet with private keys.
This will provide you with the descriptor needed to derive the private key.

Since Bitcoin wallets typically contain multiple descriptors
(usually 6 by default), we use `jq` to find the specific descriptor that
matches our previously saved `hdkeypath` in this example `(84h/1h/0h/0)`
and extract the master private key from it.

So, before you run this command you will need to replace the `<hdkeypath>` below
with the one you retrieved in step 2.

```shell
 bitcoin-cli listdescriptors true | jq -r '
  .descriptors[] |
  select(.desc | contains("<hdkeypath>")) |
  .desc
'
```

The output will be:

```shell
wpkh(tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq/84h/1h/0h/0/*)#sachkrde
}
```

As you can see above there is a concatenated string of your private key and
part of your `hdkeypath`. To extract the private key:

1. Remove everything outside the parentheses `wpkh(` and `)`
2. Remove the `hdkeypath` after the private key
(everything after and including `/`)

You'll be left with just the **base58-encoded master private key**, similar to
below:

```
tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq
```
Now you have your **base58-encoded master private key**.
You can now pass the above information to the `covenant-signer` binary to
derive the covenant private key from the master key using **BIP32 derivation**.

Use the following command to derive the covenant private key:

```shell
 covenant-signer derive-child-key \
    tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq \
    84h/1h/0h/0/0
```

The output will be:

```shell
derived_private_key: fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06
derived_public_key: 023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

Parameters:
- `<master-private-key>`: The base58-encoded master private key from your
Bitcoin wallet (first parameter)
- `<derivation-path>`: The HD derivation path that specifies how to derive
the child key (second parameter)

To verify, you can execute the following:

```shell
 bitcoin-cli getaddressinfo <address> | jq .publickey
```

If the public key matches the `derived_public_key`s output from the
`derive-child-key` command, the verification is successful.

#### Step 4: Import the private key into a Cosmos Keyring

Next, we are going to import the derived private key into the Cosmos keyring.

```shell
 covenant-signer keys import-hex cov fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06 \
    --keyring-backend file \
    --keyring-dir /path/to/your/keyring/directory
```

This command:
- Uses `import-hex` to import the raw private key
- Names the key `cov` in the keyring
- Uses the secure `file` backend which encrypts the key on disk
- Will prompt you for a passphrase to encrypt the key

Note that the passphrase you set here will be needed later on
to unlock the keyring.

> **⚡ Note:** While both `os` and `file` backends are supported, the authors
> of the docs have more thoroughly tested the `file` backend across
> different environments.
> The `file` backend stores the private key in encrypted form
> on disk. When running `import-hex` with the `file` backend, you will be
> prompted for a passphrase. This passphrase will be required to unlock the
> signer later.

To confirm that the import was successful, run:

```shell
 covenant-signer keys show cov
```

The output will display the details of the imported key:

```shell
  - address: bbn1azasawj3ard0ffwj04zpxlw2pt9cp7kwjcdqmc
    name: cov
    pubkey: '{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"Ajp5tUbHnX98X/IGINkUtc9yUGMdEvbiZCftnT+Yxcyx"}'
    type: local
```

Congratulations! You have successfully imported your key.

## 5. Operation
### 5.1. Configuration

Next, we can return to the covenant signer directory
and create your own configuration file. Use the
following command to dump the configuration template:

```shell
 covenant-signer dump-cfg --config <path-to-config-file>
```

This will create a configuration file, from the example configuration,
in the specified path.

Replace the placeholder values with your own
configuration. This configuration can be placed directly in the
`covenant-signer` directory.

```toml
[keystore]
# Type of keystore to use for managing private keys. Currently only
# "cosmos" is supported, which uses the Cosmos SDK keyring system for
# secure key storage.
keystore-type = "cosmos"

[keystore.cosmos]
# pointing to the directory where the key is stored, unless specified otherwise
key-directory = "/path/to/keydir"

# the backend to be used for storing the key, in this case `file`
keyring-backend = "file"

# the key name you specified when importing your covenant key
key-name = "your-key-name"

# the chain id of the chain the covenant will connect to
chain-id = "network-chain-id"

[server-config]
# The IP address where the covenant-signer server will listen
host = "127.0.0.1"
# The TCP port number where the covenant-signer server will listen
port = 9791

[metrics]
# The IP address where the Prometheus metrics server will listen
host = "127.0.0.1"
# The TCP port number where the Prometheus metrics server will listen
port = 2113
```

Below are brief explanations of the configuration entries:

- `keystore-type`: Type of keystore used. Should be set to `"cosmos"`
- `key-directory`: Path where keys are stored. Do not include the keyring
  backend type in the path (e.g., use `/path/to/keys` not
  `/path/to/keys/keyring-file`).
- `keyring-backend`: Backend system for key management, e.g., "file", "os".
- `key-name`: Name of the key used for signing transactions.
- `chain-id`: The Chain ID of the Babylon network you connect to.
- `host` (server-config): IP address where the server listens, typically "127.0.0.1" for local access.
- `port` (server-config): TCP port number for the server.
- `host` (metrics): IP address for the Prometheus metrics server, typically "127.0.0.1".
- `port` (metrics): TCP port number for the Prometheus metrics server.

### 5.2. Starting the daemon

We will then run the following command to start the daemon:

```shell
 covenant-signer start --config ./path/to/config.toml
```

The covenant signer must be run in a secure network and only accessible by the
covenant emulator.

Once the covenant signer is set up and unlocked, you can configure the covenant
emulator to use it. The URL of the covenant signer is configurable (`remotesigner` section)
but in this example we use the default value of
`http://127.0.0.1:9791`.

### 5.3. Unlocking the key

Before you can sign transactions with the covenant key, you must unlock the
keyring that stores it. This happens through a `POST` request
on the `v1/unlock` endpoint with a payload containing
the covenant keyring passphrase.

```shell
 curl -X POST http://127.0.0.1:9791/v1/unlock -d '{"passphrase": "<passphrase>"}'
```

> ⚡ Note: Even if you provide the passphrase in the curl command to unlock the
> keyring, the CLI configuration for starting the service will still prompt you
> to enter the passphrase interactively.

You can sign transactions by invoking the `v1/sign-transactions` endpoint,
which expects staking and unbonding transactions in hex format.

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

<!-- TODO: add in a real staking tx to test -->

The above command will generate a signature for the provided
transactions and return it in JSON format.

```json
{
    "staking_tx_signature": "<hex_encoded_signature>",
    "unbonding_tx_signature": "<hex_encoded_signature>"
}
```

These signatures can then be used to verify that the transactions were signed by
the covenant key.

Congratulations! You have successfully set up the covenant signer and are now able
to sign transactions with the covenant key.
