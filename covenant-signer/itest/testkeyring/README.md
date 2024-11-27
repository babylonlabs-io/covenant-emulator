Folder for testing encrypted file keyring.

Contains one key with name `test` and passphrase `testtest`.

Key was created by calling `btcec.NewPrivateKey()`, encoded to hex and importing
to cosmos keyring by using :
```
./build/babylond keys --keyring-backend=file --keyring-dir=babylon/testkeyring import-hex test d9d4221f4f1c98d6ce4b9390038f3b08b2c47156fd29bb417d03bf6118923f3f
```
