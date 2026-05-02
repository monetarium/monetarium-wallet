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

## Unreleased — JSON-RPC API surface

### `signrawtransaction` — `complete` semantics fix (BREAKING — bug fix)

`wallet.Wallet.SignTransaction` now returns `(SignatureError, bool, error)`.
The new `bool` is `complete`: `true` only when every input was fully signed
and the script engine validated, `false` when any input is partially signed
(e.g. an m-of-n P2SH multisig with fewer than `m` signatures available in
the wallet's keyring). The boolean is surfaced as the `complete` field of
`signrawtransaction`.

This corrects a long-standing upstream bug: previously, partial multisig
inputs whose pkScript came from the wallet's own UTXO set (rather than the
caller-supplied `additionalPrevScripts` map) were classified as a hard
signing error rather than a partial-multisig underflow. Combined with the
absence of an explicit `complete` flag, downstream callers received a
`SignatureError` for what is in fact a valid in-progress transaction.
Workflows that queued a "completed" partial transaction for broadcast must
re-check the `complete` flag — and the absence of `signErrors` — before
broadcasting.

Callers using the gRPC `WalletService.SignTransaction` are unaffected: the
gRPC response has never carried completeness, so the new boolean is
discarded at the gRPC boundary.

### `version` RPC — canonical keys added

The response now includes canonical keys `monw` and `monwjsonrpcapi`
alongside the legacy keys `dcrwallet` and `dcrwalletjsonrpcapi`. The legacy
keys remain populated with the same values for one deprecation cycle.
Tooling should switch to the canonical keys.

### HTTP Basic-auth realm renamed

The Basic-auth realm advertised by the JSON-RPC server changed from
`dcrwallet RPC` to `monw RPC`. Browser-based clients with cached
credentials will be re-prompted on first connection after upgrade. Headless
clients that hard-code the realm in scripted authentication flows must
update.

## Unreleased — Database upgrade

### Wallet DB version 31 → 32

This release introduces wallet database version 32 (`multisigCoinTypeVersion`).
The upgrade extends every persisted P2SH multisig output record with a
1-byte `CoinType` field plus a length-prefixed SKA amount, enabling
SKA-denominated multisig outputs without ambiguity. Pre-upgrade records
(VAR-only, fixed 135-byte length) are backfilled in-place as
`{CoinType: VAR, SKAAmount: 0}`.

The upgrade is automatic on first launch and runs in a single walletdb
write transaction. **Take a wallet backup before upgrading.** Downgrade to
a pre-32 binary is not supported — older binaries will refuse to open the
upgraded database.

A 136-byte length is reserved between v1 and v2 records and is explicitly
rejected on read. This is a forward-compat reservation and is not produced
by any current code path.

## Unreleased — Cryptography

### Emission-key backup format v1 → v2 (BREAKING)

The encrypted-blob format produced by `generateemissionkey` and consumed by
`importemissionkey` is now v2:

- KDF: scrypt(`N=2^15`, `r=8`, `p=1`) with a per-blob random 16-byte salt
  (was: unsalted `sha256(passphrase)`).
- Cipher: AES-256-GCM with a per-blob random 12-byte nonce.
- Wire format: `aes256gcm:v2:<salt_hex>:<N>:<r>:<p>:<nonce_hex>:<ct_hex>`.

v1 blobs (`aes256gcm:<iv_hex>:<ct_hex>`) are explicitly rejected with an
actionable error. **Operators with v1 backups must re-export from the
canonical wallet DB via `generateemissionkey` before this release becomes
load-bearing in their disaster-recovery procedure.** The plaintext key
material is unchanged — only the at-rest envelope is rotated.

The decode path also caps `N <= 2^20`, `r <= 16`, and `p <= 16` to bound
CPU and memory cost on a malicious blob. Passphrase byte slices are zeroed
after key derivation.

### Imported emission keys

`importemissionkey` now requires exactly 32 bytes and rejects the all-zero
scalar; previously, shorter inputs were silently accepted and the secp256k1
library would coerce them, masking truncation bugs in callers.

## Unreleased — HD-wallet derivation

### SLIP-0044 cointype re-registration: m/44'/42' → m/44'/9508' (BREAKING)

Monetarium has been assigned its own SLIP-0044 cointype, `9508`
(registered via [satoshilabs/slips#2013](https://github.com/satoshilabs/slips/pull/2013)),
replacing the inherited Decred slot `42`. Existing wallets created against
prior releases derive all keys at `m/44'/42'/...`. New wallets created on
this release derive at `m/44'/9508'/...`.

#### Migration

Existing wallets must be migrated. Two new wallet-level methods are
exposed:

- `NeedsReregisteredSLIP0044Migration(ctx)` reports whether the wallet's
  active SLIP-0044 keys are derived under a cointype other than the
  chain's current `SLIP0044CoinType`. Detection does not require the
  wallet to be unlocked.
- `MigrateToReregisteredSLIP0044(ctx, seed)` performs the migration. The
  seed must be supplied — wallet DB version 4 stopped persisting it, and
  BIP-32 does not allow deriving sibling cointype keys from an existing
  cointype key. The wallet must be unlocked. The supplied seed is verified
  against the stored xpub before any state mutation; an `errors.Invalid`
  is returned on mismatch (UI layers may surface this as "wrong seed
  phrase").

The migration is idempotent: re-invoking on an already-migrated wallet, a
fresh wallet at the new path, or a chain where legacy and SLIP-0044
cointypes coincide (e.g. testnets using the universal slot) is a no-op.

#### Funds at the historical derivation path

The pre-migration cointype keys (`m/44'/42'/...`) are preserved under
separate database entries (`ctpriv-slip0044-old` / `ctpub-slip0044-old`)
so that UTXOs at the old derivation path remain spendable via sweep
flows. The migration does not move funds — operators should sweep
historical balances to the new derivation path before publishing the new
wallet xpub to external services.

#### Concurrency precondition

`MigrateToReregisteredSLIP0044` updates on-disk derivations and
in-memory address buffers in two phases (DB transaction commits, then
`addressBuffersMu` is taken to swap the cached buffers). Callers must
serialize the call against all other wallet operations on the same
`*Wallet` — concurrent readers may otherwise briefly observe an
inconsistent state where the DB has new derivations and the in-memory
buffer still holds the old branch xpubs. UI layers should gate this
behind a "wallet is upgrading" state.
