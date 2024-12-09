
# Covenant Emulator Installation

## Prerequisites

This project requires Go version `1.23` or later.
Install Go by following the instructions on
the [official Go installation guide](https://golang.org/doc/install).

### Download the code

To get started, clone the repository to your local machine from Github:

```bash
$ git clone git@github.com:babylonlabs-io/covenant-emulator.git
```

You can choose a specific version from
the [official releases page](https://github.com/babylonlabs-io/covenant-emulator/releases):

```bash
$ cd covenant-emulator # cd into the project directory
$ git checkout <release-tag>
```

## Build and install the binary

At the top-level directory of the project

```bash
$ make install 
```

The above command will build and install the covenant-emulator daemon (`covd`)
binary to `$GOPATH/bin`:

If your shell cannot find the installed binaries, make sure `$GOPATH/bin` is in
the `$PATH` of your shell. Usually, these commands will do the job

```bash
export PATH=$HOME/go/bin:$PATH
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.profile
```

To build without installing,

```bash
$ make build
```

The above command will put the built binaries in a build directory with the
following structure:

```bash
$ ls build
    └── covd
```

Another common issue with compiling is that some of the dependencies have
components written in C. If a C toolchain is absent, the Go compiler will throw
errors. (Most likely it will complain about undefined names/types.) Make sure a
C toolchain (for example, GCC or Clang) is available. On Ubuntu, this can be
installed by running

```bash
sudo apt install build-essential
```

## Setting up a covenant emulator

### Configuration

The `covd init` command initializes a home directory for the
finality provider daemon.
This directory is created in the default home location or in a
location specified by the `--home` flag.
If the home directory already exists, add `--force` to override the directory if
needed.

```bash
$ covd init --home /path/to/covd/home/
```

After initialization, the home directory will have the following structure

```bash
$ ls /path/to/covd/home/
  ├── covd.conf # Covd-specific configuration file.
  ├── logs      # Covd logs
```

If the `--home` flag is not specified, then the default home directory
will be used. For different operating systems, those are:

- **MacOS** `~/Users/<username>/Library/Application Support/Covd`
- **Linux** `~/.Covd`
- **Windows** `C:\Users\<username>\AppData\Local\Covd`

Below are some important parameters of the `covd.conf` file.

**Note**:
The configuration below requires to point to the path where this keyring is
stored `KeyDirectory`. This `Key` field stores the key name used for interacting
with the Babylon chain and will be specified along with the `KeyringBackend`
field in the next [step](#generate-key-pairs). So we can ignore the setting of
the two fields in this step.

```bash
# The interval between each query for pending BTC delegations
QueryInterval = 15s

# The maximum number of delegations that the covd processes each time
DelegationLimit = 100

# Bitcoin network to run on
BitcoinNetwork = simnet

# Babylon specific parameters

# Babylon chain ID
ChainID = chain-test

# Babylon node RPC endpoint
RPCAddr = http://127.0.0.1:26657

# Babylon node gRPC endpoint
GRPCAddr = https://127.0.0.1:9090

# Name of the key in the keyring to use for signing transactions
Key = <covenant-emulator-key-name>

# Type of keyring to use,
# supported backends - (os|file|kwallet|pass|test|memory)
# ref https://docs.cosmos.network/v0.46/run-node/keyring.html#available-backends-for-the-keyring
KeyringBackend = test

# Directory where keys will be retrieved from and stored
KeyDirectory = /path/to/covd/home
```

To see the complete list of configuration options, check the `covd.conf` file.

## Generate key pairs

The covenant emulator daemon requires the existence of a keyring that signs
signatures and interacts with Babylon. Use the following command to generate the
key:

```bash
$ covd create-key --key-name covenant-key --chain-id chain-test
{
    "name": "cov-key",
    "public-key": "9bd5baaba3d3fb5a8bcb8c2995c51793e14a1e32f1665cade168f638e3b15538"
}
```

After executing the above command, the key name will be saved in the config file
created in [step](#configuration).
Note that the `public-key` in the output should be used as one of the inputs of
the genesis of the Babylon chain.
Also, this key will be used to pay for the fees due to the daemon submitting 
signatures to Babylon.

## Start the daemon

You can start the covenant emulator daemon using the following command:

```bash
$ covd start
2024-01-05T05:59:09.429615Z	info	Starting Covenant Emulator
2024-01-05T05:59:09.429713Z	info	Covenant Emulator Daemon is fully active!
```

All the available CLI options can be viewed using the `--help` flag. These
options can also be set in the configuration file.