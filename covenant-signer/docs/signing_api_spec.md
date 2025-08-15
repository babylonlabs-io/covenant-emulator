# Covenant Signer API specification

Covenant signer must support following APIs to correctly work with covenant emulator.

## Endpoints

### `POST /v1/sign-transactions`

This endpoint is used to request signatures for a set of transactions.

#### Request Body

The request body is a JSON object with the following structure:

```json
{
  "staking_tx_hex": "string",
  "slashing_tx_hex": "string",
  "unbonding_tx_hex": "string",
  "slash_unbonding_tx_hex": "string",
  "staking_output_idx": "number",
  "slashing_script_hex": "string",
  "unbonding_script_hex": "string",
  "unbonding_slashing_script_hex": "string",
  "fp_enc_keys": ["string"],
  "stake_expansion": {
    "previous_active_stake_tx_hex": "string",
    "other_funding_output_hex": "string",
    "previous_staking_output_idx": "number",
    "previous_active_stake_unbonding_script_hex": "string"
  }
}
```

#### `SignTransactionsRequest` description

| Field | Type | Description |
| --- | --- | --- |
| `staking_tx_hex` | `string` | The hexadecimal representation of the staking transaction. |
| `slashing_tx_hex` | `string` | The hexadecimal representation of the slashing transaction. |
| `unbonding_tx_hex` | `string` | The hexadecimal representation of the unbonding transaction. |
| `slash_unbonding_tx_hex` | `string` | The hexadecimal representation of the slash unbonding transaction. |
| `staking_output_idx` | `number` | The index of the staking output in the staking transaction. |
| `slashing_script_hex` | `string` | The hexadecimal representation of the slashing script. |
| `unbonding_script_hex` | `string` | The hexadecimal representation of the unbonding script. |
| `unbonding_slashing_script_hex` | `string` | The hexadecimal representation of the unbonding slashing script. |
| `fp_enc_keys` | `[]string` | An array of hexadecimal representations of the finality provider encryption keys. |
| `stake_expansion` | `StakeExpansionRequest` | An optional object containing information about the stake expansion. |

#### `StakeExpansionResponse` description

| Field | Type | Description |
| --- | --- | --- |
| `previous_active_stake_tx_hex` | `string` | The hexadecimal representation of the previous active stake transaction. |
| `other_funding_output_hex` | `string` | The hexadecimal representation of the other funding output. |
| `previous_staking_output_idx` | `number` | The index of the previous staking output in the previous active stake transaction. |
| `previous_active_stake_unbonding_script_hex` | `string` | The hexadecimal representation of the unbonding script of the previous active stake transaction. |

#### Response Body

The response body is a JSON object with the following structure:

```json
{
  "slashing_transactions_signatures": ["string"],
  "unbonding_transaction_signature": "string",
  "slash_unbonding_transactions_signatures": ["string"],
  "stake_expansion_transaction_signature": "string"
}
```

#### `SignTransactionsResponse` description

| Field | Type | Description |
| --- | --- | --- |
| `slashing_transactions_signatures` | `[]string` | An array of hexadecimal representations of the adaptor signatures for the slashing transactions. |
| `unbonding_transaction_signature` | `string` | The hexadecimal representation of the signature for the unbonding transaction. |
| `slash_unbonding_transactions_signatures` | `[]string` | An array of hexadecimal representations of the adaptor signatures for the slash unbonding transactions. |
| `stake_expansion_transaction_signature` | `string` | The hexadecimal representation of the signature for the stake expansion transaction. field will only be filled if `stake_expansion` object in request was not nil|

### `GET /v1/public-key`

This endpoint is used to retrieve the public key of the signer.

#### Response Body

```json
{
  "public_key": "string"
}
```

#### Response body description

| Field | Type | Description |
| --- | --- | --- |
| `public_key` | `string` | A hexadecimal representation of covenant signer 33byte public key. |
