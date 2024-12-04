# Covenant Signer

The Covenant Signer is a daemon program in the Covenant Emulator toolset
that is responsible for securely managing the private key of the
covenant committee member and producing the necessary cryptographic
signatures.

It prioritizes security through isolation, ensuring that private key handling
is confined to an instance with minimal connectivity and simpler application 
logic compared to the Covenant Emulator daemon.

> ⚡ Note: This program is a separate implementation from the
[covenant signer](https://github.com/babylonlabs-io/covenant-signer/)
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

## 1. Prerequisites

This guide requires that:

1. you have a Bitcoin node setup for the Bitcoin
network you intend to operate your covenant signer on and
2. you have access to the private Bitcoin key you
set up your covenant with.
3. A connection to a Babylon node. To run your own node, please refer to the 
[Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/blob/sam/bbn-test-5/bbn-test-5/babylon-node/README.md).

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

## 3. Transitioning your covenant key from phase-1 setup

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
to the relevant phase-1 guide.

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
<!-- you correctly guessed it later i.e there could be many descriptors and in order to find the right one, we need to match the `hdkeypath`
that we received previously.  Each bitcoind wallet will by default have 6 differet descriptors we need to retrieve correct one
 -->

 <!-- RESPONSE FROM KONRAD: this is needed to find the correct hdkeypath for the descriptor -->
Next, we are going to retrieve the `hdkeypath` of the Bitcoin address
associated with our covenant key.
We do this through the usage of the `getaddresssinfo` command
which takes your covenant Bitcoin address as a parameter.
```shell
bitcoin-cli getaddressinfo bcrt1qazasawj3ard0ffwj04zpxlw2pt9cp7kwmnqyvk | jq .hdkeypath
"m/84h/1h/0h/0/0"
```

In the above command, we use the `jq` utility to extract only the relevant `hdkeypath`
information, which in this example is `84h/1h/0h/0/0` (the initial `m/` can be ignored).

Make note of the `hdkeypath` information - you'll need it later when
deriving the covenant private key from the master key for verification purposes.

#### Step 3: Retrieve the master private key

In this step,
we are going to retrieve the **base58-encoded master private key** from the Bitcoin wallet. 
This key will be used to derive the covenant private key, which can then be 
imported directly into the Cosmos keyring.

What we need you to do is replace the `<hdkeypath>` with the one you received
in step 2.

List all descriptors in the wallet with private keys included in the output. 
This will provide the descriptor needed to derive the private key. 

```shell
bitcoin-cli listdescriptors true | jq -r '
  .descriptors[] |
  select(.desc | contains("/84h/1h/0h/0/")) |
  .desc
' descriptors.json

wpkh(tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq/84h/1h/0h/0/*)#sachkrde
```
<!-- TODO: maybe there could be many descriptors
and in order to find the right one, we need to match the `hdkeypath`
that we received previously. If so, this should be explained here
and we can avoid being overly smart by simplifying the above command. -->

<!-- ADDED please see below -->

Since Bitcoin wallets typically contain multiple descriptors 
(usually 6 by default), we need to use `jq` to find the specific descriptor that
 matches our previously saved `hdkeypath` (84h/1h/0h/0/0) and extract the master 
 private key from it.

To extract the private key:
1. Remove everything outside the parentheses `wpkh(` and `)`
2. Remove the derivation path after the private key 
(everything after and including `/`)

You'll be left with just the base58-encoded master private key:

```
tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq
```

The above output contains two key pieces of information
as a concatenated string:
1. The **base58-encoded master private key**
   `tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq`
2. The `hdkeypath` which should be exactly match the one you received
   on step 2.

We are going pass the above pieces to the `covenant-signer` binary
to derive the covenant private key from the master key using **BIP32 derivation**.

<!-- TODO: ask Konrad: given that the descriptor output contains a single string,
why did we decide for the covenant-signer CLI to have two parameters instead of a single string? -->
<!-- RESPONSE FROM KONRAD:
- becouse the descriptor string does not contain full path to the derived kay, 
but part of it i.e it has /84h/1h/0h/0/* so the user still would need to provide 
the path under the * 
- it simplified a bit parsing on program side -->

```shell
covenant-signer derive-child-key \
    tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq \
    84h/1h/0h/0/0
Derived private key: fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06
Derived public key: 023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

The above output displays the derived private and public keys.

<!-- TODO: leftover sentences. It's nice that there's some verification steps though.
Wonder if we can have something in their place -->
<!-- CHANGED: Let me know if this is ok. -->
You can verify your key derivation was successful by checking that the public 
key matches the one shown earlier in both:
- The `getaddressinfo` command output
- The `derive-child-key` command output

This verification ensures you've extracted the correct master private key from the descriptor.

#### Step 4: Import the private key into a Cosmos Keyring

As mentioned above as a prerequesite, you will need access to a Babylon node, or 
one setup on your machine. The reason for this is that we need to access the 
`babylond` binary to import the private key into the Cosmos keyring. Currently 
the `covenant-signer` does not have support for importing keys. If you need a 
guide on how to set up a Babylon node, you can refer to the 
[Babylon Node Setup Guide](https://github.com/babylonlabs-io/networks/bbn-test-5/babylon-node/README.md).


Next, navigate to where you have the babylon node running and import the derived 
private key into the Cosmos keyring using the following command:

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

Congratulations! You have successfully imported your key.

## 4. Operation
### 4.1. Configuration

Next, we can return to the terminal where you have the covenant signer directory 
and create your own configuration file.

Use the example configuration [file](../example/config.toml) to create your own 
configuration file. Then, replace the placeholder values with your own 
configuration. This can be placed directly in the `covenant-signer` directory.

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

We then will run the following command to start the daemon from the 
`covenant-signer` directory:

```shell
covenant-signer start --config ./path/to/config.toml
```

The covenant signer must be run in a secure network and only accessible by the 
covenant emulator.

Once the covenant signer is set up and unlocked, you can configure the covenant 
emulator to use it. The URL of the covenant signer is configurable (`remotesigner` section)
but in this example we use the default value of 
`http://127.0.0.1:9791`.

### 4.3. Unlocking the key

Before you can sign transactions with the covenant key, you must unlock the 
keyring that stores it. This happens through a `POST` request
on the `v1/unlock` endpoint with a payload containing
the covenant keyring passphrase.

```shell
curl -X POST http://127.0.0.1:9791/v1/unlock -d '{"passphrase": "<passphrase>"}'
```

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

<!-- TODO: Some nice additional sections
* Testing the setup: e.g. through a healthcheck endpoint
* Prometheus metrics and logs -->
<!-- RESPONSE: should i put this in an issue  for now as its not a priority?-->