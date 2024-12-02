# Covenant Signer

## Table of Contents
1. [Purpose of this guide](#purpose-of-this-guide)
2. [Prerequisites](#prerequisites)
3. [Install covenant emulator](#install-covenant-emulator)
4. [Export the key from the Bitcoin node](#export-the-key-from-the-bitcoin-node)
5. [Import the key into the cosmos keyring](#import-the-key-into-the-cosmos-keyring)
6. [Create the configuration file](#create-the-configuration-file)
7. [Running the Covenant Signer](#running-the-covenant-signer)
8. [Using the covenant signer for signing transactions](#using-the-covenant-signer-for-signing-transactions)

## 1. Purpose of this guide

This guide outlines the setup and configuration of the 
[covenant signer](https://github.com/babylonlabs-io/covenant-emulator/tree/main/covenant-signer).
in conjunction with the [covenant emulator](https://github.com/babylonlabs-io/covenant-emulator).

## 2. Prerequisites

To successfully complete this guide, ensure the following setup and resources 
are in place:

1. Setup of bitcoin node in order to retrieve keys. For more information, refer to 
   [Bitcoin Node Setup](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup).
2. Ensure you have access to the private Bitcoin key, also referred to as the 
   covenant emulator key, which will be used for signing operations.

## 3. Install covenant signer binary

If you havent already, download [Golang 1.21](https://go.dev/dl) 

Using the go version 1.21. Once installed run: 

```shell
go version
```

Subsequently clone the covenant [repository](https://github.com/babylonlabs-io/covenant-emulator).

```shell
git clone git@github.com:babylonlabs-io/covenant-emulator.git
cd covenant-emulator
git checkout <tag>
```

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

## 4. Export the key from the Bitcoin node

At this stage, you should already have access to the Bitcoin node. 
If you need a refresher on setting up `bitcoind`, refer to the [setup guide](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup). 
Once you have node access, you can proceed with the next steps.

Load wallet with your covenant key.

```shell
bitcoind bitcoin-cli loadwallet "covenant-wallet"
```

If we want to then confirm that this was successful, we can then retrieve 
information about our address to the corresponding key that was returned. 

```shell
bitcoind bitcoin-cli getaddressinfo bcrt1qazasawj3ard0ffwj04zpxlw2pt9cp7kwmnqyvk
```

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

The most important field to focus on is `hdkeypath` that contains derivation path 
of our key. In the example it is `84h/1h/0h/0/0` (the intilal `m/` can be ignored).

Next, list all descriptors in the wallet, ensuring that private keys are included 
in the output:

```shell
docker exec -it bitcoind bitcoin-cli -chain=regtest -rpcuser=user -rpcpassword=pass listdescriptors true
```

The terminal should produce output similar to the following:

```json
{
  "wallet_name": "covenant-wallet",
  "descriptors": [
    {
      "desc": "wpkh(tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq/84h/1h/0h/0/*)#sachkrde",
      "timestamp": 1732624709,
      "active": true,
      "internal": false,
      "range": [
        0,
        1000
      ],
      "next": 1,
      "next_index": 1
    }
    ...
  ]
}

```

The most important field to note is the `desc` value:

```json
"desc": "wpkh(tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq/84h/1h/0h/0/*)#sachkrde"
```

Here, you can see the string starting with `tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq`is the **base58-encoded master private key** of the covenant wallet. This key is critical for signing operations and should be securely stored.


#### Deriving the Covenant Private Key from the Master Key

You can derive the covenant private key from the master key by performing a **BIP32 derivation**. The `covenant-signer`repository includes a command to accomplish this:

```shell
covenant-signer derive-child-key \ tprv8ZgxMBicQKsPe9aCeUQgMEMy2YMZ6PHnn2iCuG12y5E8oYhYNEvUqUkNy6sJ7ViBmFUMicikHSK2LBUNPx5do5EDJBjG7puwd6azci2wEdq \ 84h/1h/0h/0/0
```

The output will display the derived private and public keys:

```shell
Derived private key: fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06
Derived public key: 023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

As seen, the **Derived Public Key**:

```
023a79b546c79d7f7c5ff20620d914b5cf7250631d12f6e26427ed9d3f98c5ccb1
```

Matches the public key obtained earlier using the `getaddressinfo` command.

#### Importing the Derived Private Key into the Cosmos Keyring

The derived private key can now be imported into the Cosmos keyring. Use the 
following command:

```shell
babylond keys import-hex cov fe1c56c494c730f13739c0655bf06e615409870200047fc65cdf781837cf7f06
```

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

## 6. Create the configuration file

Use the example configuration [file](./example/config.toml) to create your own 
configuration file. Then, replace the placeholder values with your own configuration.

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
host = "127.0.0.1"
port = 9791

[metrics]
host = "127.0.0.1"
port = 2113
```

## 7. Running the Covenant Signer

The covenant signer can be run using the following command:

```shell
covenant-signer start --config ./path/to/config.toml
```

the covenant signer must be run in a secure network and only accessible by the 
covenant emulator.

Once the covenant signer is set up and unlocked, you can configure the covenant 
emulator to use it. The URL of the covenant signer, which is http://127.0.0.1:9791, 
should be specified in the covenant emulator's configuration file under the 
remotesigner section.

It's important to note that the key specified in the covenant emulator's 
configuration is not the covenant key itself. Instead, it is a 
key used for sending Cosmos transactions.

## 8. Using the covenant signer for signing transactions

To enable signing transactions with the covenant key, you need to unlock it first.

```shell
curl -X POST http://127.0.0.1:9791/v1/unlock -d '{"passphrase": "<passphrase>"}'
```

Now that the key is unlocked you can add its url to the covenant emulator's 
configuration file. See [here](./docs/transfer-setup.md#42-configure-the-covenant-emulator) 
for more information.

You can sign transactions using the following command. However, ensure that both 
the staking and unbonding transactions are provided in hex format:

```shell
curl -X POST http://127.0.0.1:9791/v1/sign-transactions \
  -d '{
    "staking_tx_hex": "",
    "unbonding_tx_hex": ""
  }'
```

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