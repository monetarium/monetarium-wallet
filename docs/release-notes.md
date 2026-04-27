# Release notes

## Unreleased — JSON-RPC behavior changes

### `redeemmultisigouts` (BREAKING)

`redeemmultisigouts` now caps the per-call result count at 256 to prevent
authenticated callers from stalling the RPC server with an unbounded address
list. When the on-chain unspent multisig output count exceeds the cap, the
response includes `truncated: true`; callers must paginate by spending the
returned redemptions and calling again.

`number: 0` (and a missing `number`) are now treated as "use the default cap
(256)" rather than "return zero results". Callers that previously relied on
the zero value to receive an empty result must instead avoid calling the
method.

### `sendtomultisig` (BREAKING — bug fix)

- `amount` is now strictly validated. Negative amounts, zero, empty strings,
  whitespace-only strings, and SKA fractional precision exceeding the
  configured atom precision are rejected up front. Previously, `amount: "0"`
  would drain the source account into a zero-value multisig output.
- `fromaccount` is now honored (it was previously documented as "Unused"
  while the handler ignored it). Empty string is treated as `"default"` to
  preserve backward compatibility for clients that relied on the old
  behavior; any other value must name an existing account.
