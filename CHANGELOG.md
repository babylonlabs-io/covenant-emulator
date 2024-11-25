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
