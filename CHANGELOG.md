<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry should have following format:

* [#PullRequestNumber](PullRequestLink) message

Types of changes (Stanzas):

"Features" for new features.
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"Client Breaking" for breaking CLI commands and REST routes used by end-users.
"API Breaking" for breaking exported APIs used by developers building on SDK.
"State Machine Breaking" for any changes that result in a different AppState
given same genesisState and txList.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)

## Unreleased

## v0.12.0

### Improvements

* [#83](https://github.com/babylonlabs-io/covenant-emulator/pull/83) covenant-signer: remove go.mod
* [#95](https://github.com/babylonlabs-io/covenant-emulator/pull/95) removed local signer option
as the covenant emulator should only connect to a remote signer
* [#96](https://github.com/babylonlabs-io/covenant-emulator/pull/96) add pagination to `queryDelegationsWithStatus`
* [#99](https://github.com/babylonlabs-io/covenant-emulator/pull/99) add more metrics
covenant-signer
* [#100](https://github.com/babylonlabs-io/covenant-emulator/pull/100) bump babylon to v1.0.0-rc.4

## v0.11.3

### Bug fixes

* [#91](https://github.com/babylonlabs-io/covenant-emulator/pull/91) Add verification
if the covenant public key was present in the params of the versioned BTC delegation

## v0.11.2

### Bug fixes

* [#84](https://github.com/babylonlabs-io/covenant-emulator/pull/84) Fix bug with
deduplication of already signed delegations

## v0.11.1

### Bug fixes

* [#71](https://github.com/babylonlabs-io/covenant-emulator/pull/71) Fix delegation
response to delegation conversion

## v0.11.0

### Improvements

* [#68](https://github.com/babylonlabs-io/covenant-emulator/pull/68) Bump babylon to v1.0.0-rc.1
* [#63](https://github.com/babylonlabs-io/covenant-emulator/pull/63) Add babylon
address to keys command output and fix casing in dockerfile
* [#67](https://github.com/babylonlabs-io/covenant-emulator/pull/67) Add keys cmd to covenant-signer

## v0.10.0

### Improvements

* [#43](https://github.com/babylonlabs-io/covenant-emulator/pull/43) Test importing
private key to the cosmos keyring
* [#44](https://github.com/babylonlabs-io/covenant-emulator/pull/44) Command
to derive child private keys from the master key
* [#45](https://github.com/babylonlabs-io/covenant-emulator/pull/45) Add e2e test
with encrypted file keyring usage
* [#47](https://github.com/babylonlabs-io/covenant-emulator/pull/47) Add unlocking keyring
through API
* [#48](https://github.com/babylonlabs-io/covenant-emulator/pull/48) Add covenant
signer version that requires unlocking
* [#49](https://github.com/babylonlabs-io/covenant-emulator/pull/49) Add show-key command
* [#52](https://github.com/babylonlabs-io/covenant-emulator/pull/52) Fix CosmosKeyStoreConfig
* [#57](https://github.com/babylonlabs-io/covenant-emulator/pull/57) Bump babylon to v0.18.0

## v0.9.0

* [#33](https://github.com/babylonlabs-io/covenant-emulator/pull/33) Add remote
signer sub module
* [#31](https://github.com/babylonlabs-io/covenant-emulator/pull/31/) Bump docker workflow
version, fix some Dockerfile issue
* [#36](https://github.com/babylonlabs-io/covenant-emulator/pull/36) Add public key
endpoint to the remote signer
* [#38](https://github.com/babylonlabs-io/covenant-emulator/pull/38) Bump Babylon version
to v0.17.1
* [#37](https://github.com/babylonlabs-io/covenant-emulator/pull/37) Add remote signer
to the covenant emulator
* [#40](https://github.com/babylonlabs-io/covenant-emulator/pull/40) Fix min unbonding time
validation

## v0.8.0

### Bug fixes

* [#30](https://github.com/babylonlabs-io/covenant-emulator/pull/30) Ignore duplicated sig error

### Improvements

* [#22](https://github.com/babylonlabs-io/covenant-emulator/pull/22) Go releaser setup
  and move changelog reminder out
* [#32](https://github.com/babylonlabs-io/covenant-emulator/pull/32) Upgrade Babylon
  to v0.16.0

## v0.7.0

### Improvements

* [#25](https://github.com/babylonlabs-io/covenant-emulator/pull/25) Bump Babylon to v0.15.0

## v0.6.0

### Improvements

* [#24](https://github.com/babylonlabs-io/covenant-emulator/pull/24) Bump Babylon to v0.14.0

## v0.5.0

### Bug fixes

* [#23](https://github.com/babylonlabs-io/covenant-emulator/pull/23) Fix pre-approval flow

### Improvements

* [#20](https://github.com/babylonlabs-io/covenant-emulator/pull/20) Add signing behind
interface.
