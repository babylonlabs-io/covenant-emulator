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
