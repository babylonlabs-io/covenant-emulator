# Transfer your setup from a covenant signer to a covenant emulator

## Table of Contents 

1. [Purpose of this guide](#1-purpose-of-this-guide)
2. [Prerequesites](#2-prerequisites)
3. [Install Covenant Emulator Binary](#3-install-covenant-emulator-binary)
4. [Setting up the Covenant Emulator Program](#4-setting-up-the-covenant-emulator-program)
	1. [Initialize directories](#41-initialize-directories)
	2. [Configure the covenant emulator](#42-configure-the-covenant-emulator)
5. [Importing your keys from the prior setup](#5-importing-your-keys-from-the-prior-setup)
6. [Verifying Your Setup](#6-verifying-your-setup)

## 1. Purpose of this guide

This guide outlines the transition from solely using the covenant signer to an integrated setup that includes the covenant emulator.

Previously, the [covenant signer](https://github.com/babylonlabs-io/covenant-signer), was limited to signing unbonding signatures.  With this 
transition we are introducing the [covenant emulator](https://github.com/babylonlabs-io/covenant-emulator), which retrieves delegations from Babylon chain
 and signs them by communicating with the updated [covenant signer](https://github.com/babylonlabs-io/covenant-emulator/tree/main/covenant-signer). This means that the 
 covenant emulator can now generate both unbonding signatures and adaptor signatures.

In this guide, we will cover exporting the key from the Bitcoin node and importing 
it into the new integrated keyring in the covenant signer. 

## 2. Prerequisites

To successfully complete this guide, ensure the following setup and resources 
are in place:

1. Setup of bitcoin node in order to retrieve keys. For more information, refer to 
   [Bitcoin Node Setup](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup).
2. Ensure you have access to the private Bitcoin key, also referred to as the 
   covenant emulator key, which will be used for signing operations.
<!-- 3 should we include the babylon node setup? -->

## 3. Install covenant emulator binary 

Download [Golang 1.21](https://go.dev/dl) 

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

To set up the connection parameters for the Babylon chain and other covenant 
emulator settings, configure the `covd.conf` file with the following parameters.

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
```

Now we are ready to import the keys from the prior setup.

## 5. Importing your keys from the prior setup

At this stage, you should already have access to the Bitcoin node. If you need a
refresher on setting up `bitcoind`, refer to the [setup guide](https://github.com/babylonlabs-io/covenant-signer/blob/main/docs/deployment.md#2-bitcoind-setup). 
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

## 6. Verifying your setup

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

Matches the public key derived earlier and seen in the outputs of `getaddressinfo` and `derive-child-key`.

Congratulations! You have successfully imported your keys from the prior setup 
and verified your setup for the covenant emulator.