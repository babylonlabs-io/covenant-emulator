# Covenant Signer

The Covenant Signer is a daemon program in the Covenant Emulator toolset
that is responsible for securely managing the private key of the
covenant committee member and producing the necessary cryptographic
signatures.
It prioritizes security through isolation,
ensuring that private key handling is confined to an instance with
minimal connectivity and simpler application logic compared to the
Covenant Emulator daemon.

If you participated in phase-1 as a covenant signer, read this
[document](docs/transition-from-phase1.md) in order to set up
your phase-2 covenant signer.