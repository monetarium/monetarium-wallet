# Release notes

## Unreleased — gRPC API behavior changes

### `WalletService.Accounts` — `total_balance` is now VAR-only (BREAKING)

`AccountsResponse.Account.total_balance` previously aggregated VAR plus every
active SKA coin type into a single int64. That aggregation was incorrect:
VAR atoms (1e8/coin) and SKA atoms (1e18/coin) are not summable, and SKA
totals routinely exceed `int64` range. The field is now VAR atoms only.

A new field `ska_total_balances` (`map<uint32, string>`, field number 9) on
the same `Account` message carries confirmed SKA balances keyed by coin
type, with values as base-10 decimal strings of `big.Int` atoms.
Zero-balance coin types are omitted.

UI and tooling that displayed `total_balance` as the user's wallet total
must now read `ska_total_balances` and render per-coin-type SKA balances
alongside VAR. Clients that ignored SKA are unaffected.

### `TransactionDetails.fee` is 0 for SKA transactions; read `ska_fee` instead (BREAKING)

`TransactionDetails.fee` is an int64 of VAR atoms. SKA fees do not fit
in int64 (1 SKA = 1e18 atoms, so even single-coin fees exceed
`int64.Max`), and they are not VAR atoms. To preserve the legacy field's
semantics for VAR consumers, the field is now reported as `0` for any
SKA transaction. SKA fees are surfaced on the new `ska_fee` field
(decimal-coin string of `big.Int` atoms) on the same message.

Clients that iterated a wallet's transaction list and summed `fee` to
compute total fees paid will now under-report on wallets that hold any
SKA. Switch to: read `fee` for VAR transactions, read `ska_fee` for SKA
transactions, and render per-coin totals separately. Pure VAR wallets
are unaffected.

Companion fields on `TransactionDetails`:

- `previous_ska_amount` (decimal-coin string): per-input previous-output
  amount for SKA inputs. The legacy int64 `previous_amount` is 0 for SKA.
- `ska_amount` (decimal-coin string): per-output amount for SKA outputs.
  The legacy int64 `amount` is 0 for SKA.

These accompany the `ska_fee` change; the same VAR-int64 / SKA-string
split applies.

## Unreleased — JSON-RPC behavior changes

### Emission and burn RPCs now require their own passphrase and terminate any ambient `walletpassphrase` window (BREAKING)

`createauthorizedemission`, `sendtoburn`, `generateemissionkey`, and
`importemissionkey` now require their own `walletPassphrase` argument on every
call. The wallet is unlocked for the duration of the call only and
unconditionally relocked on return — including when the wallet was already
unlocked by a prior `walletpassphrase` invocation. This is the per-call
capability gate: each privileged RPC call presents the passphrase once, the
wallet performs the action, and the wallet relocks before responding.

Operators who scripted these RPCs against a wallet unlocked by a long-lived
`walletpassphrase` window must adjust: the ambient unlock window is
terminated by any of the four RPCs above. To continue performing
unlock-requiring work after one of these calls, re-issue `walletpassphrase`.
This is intentional — the per-call gate ensures privileged operations
cannot be silently chained inside an unlock window the operator forgot
to close.

### `listunspent` and `fundrawtransaction` — amount/fee fields are now decimal strings (BREAKING)

The dual-field "VAR-as-JSON-number, SKA-as-string" shape has been collapsed
to a single decimal-coin string field for both coin types. This unifies the
wire shape for SKA-aware clients and removes the legacy footgun where
VAR-only scripts read `amount: 0.0` for SKA UTXOs and silently lost
visibility of SKA holdings.

- `listunspent` results: `amount` is now a JSON string (decimal coins) for
  both VAR (`"0.001"`) and SKA (`"12345.6789"`). The legacy `skaamount`
  field has been removed.
- `fundrawtransaction` results: `fee` is now a JSON string (decimal coins)
  for both VAR and SKA. The legacy `skafee` field has been removed.

Clients that parsed `listunspent.amount` or `fundrawtransaction.fee` as a
JSON number must now parse them as JSON strings. Decimal precision is
preserved across coin types: SKA's full big.Int range is no longer subject
to float64 rounding.

### `fundrawtransaction` — SKA fee-rate selection (bug fix)

`fundrawtransaction` previously underpaid the per-kB relay fee on
SKA-output transactions when the caller did not supply `opts.feeRate`. The
VAR per-kB rate (typically 100,000 atoms / kB) was wrapped as if it were
SKA atoms and passed as a relay-fee override, underpaying by the
AtomsPerCoin ratio (commonly 1e18 vs 1e8). Resulting transactions were
silently rejected by the node while the wallet returned a misleading
`skaFee`. The override is now suppressed for SKA outputs so that
`RelayFeeForCoinType` selects the per-coin-type default. VAR behavior is
unchanged. Callers that explicitly supply `opts.feeRate` for SKA outputs
continue to receive the existing rejection error.

### `createauthorizedemission` — `force` split into `forcewindow` / `forcenonce` (BREAKING — unreleased flag)

The single `force` boolean previously bypassed two independent safety
guards (out-of-window block height AND non-default emission nonce). It has
been replaced by two independent flags:

- `forcewindow` (boolean, default false): bypasses only the out-of-window
  height check.
- `forcenonce` (boolean, default false): bypasses only the
  non-default-nonce check.

Each guard must be opted out of explicitly. The legacy `force` field has
been removed; this flag was unreleased, so no external migration is
expected. Audit log entries emitted on each bypass now include the
wallet's local synced tip so the at-sign-time wallet state is captured
even when `cmd.height` overrides it.

Default behavior also changed: out-of-window heights and non-default
nonces are now rejected with `ErrRPCInvalidParameter`. The previous
behavior was warn-and-proceed (the node would have rejected the
authorization at validation time anyway, so the new defaults surface the
failure earlier). Cold-signing pipelines that pre-stage authorizations
slightly before the emission window opens must now pass `forcewindow=true`
explicitly; the same applies to non-default nonces with `forcenonce=true`.

`createauthorizedemission` also now requires the wallet master passphrase
on every call as the `passphrase` field. The handler unlocks the wallet
for the duration of the call only and re-locks on return; the ambient
`walletpassphrase` unlock window is not consulted. Calls without
`passphrase` are rejected with `ErrRPCInvalidParameter "walletpassphrase
is required"`. See `docs/emission-ops.md` for the full per-call
authorization model.

### `generateemissionkey` / `importemissionkey` — `walletpassphrase` field added (BREAKING — unreleased)

Both commands now require the wallet master passphrase as a separate
`walletpassphrase` field. The existing `passphrase` field's meaning
narrowed from "wallet master passphrase" (per the prior helpdesc) to
"backup-blob encryption passphrase" only. Calls without
`walletpassphrase` are rejected with
`ErrRPCInvalidParameter "walletpassphrase is required"`.

The Go constructors changed correspondingly:

- `NewGenerateEmissionKeyCmd(keyName, passphrase)` →
  `NewGenerateEmissionKeyCmd(keyName, walletPassphrase, passphrase)`
- `NewImportEmissionKeyCmd(coinType, keyName, privateKey, passphrase)` →
  `NewImportEmissionKeyCmd(coinType, keyName, privateKey, walletPassphrase, passphrase)`

Tooling that imports `monetarium-wallet/rpc/jsonrpc/types` must
recompile against the new signatures. Handwritten JSON-RPC clients that
previously sent `passphrase=<wallet master passphrase>` (per the old
helpdesc) must instead send `walletpassphrase=<master>` and use
`passphrase` only for the backup-blob encryption secret (or omit it
where not applicable). The helpdesc text in
`internal/rpchelp/helpdescs_en_US.go` is now unambiguous about which
passphrase is which.

This change is part of moving emission RPCs from an ambient unlock-
window model to a per-call capability gate. See `docs/emission-ops.md`
for the full authorization model and operator guidance.

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

### `sendtoaddress` — optional `subtractfeefromamount` parameter (additive)

`sendtoaddress` now accepts an optional 6th positional parameter
`subtractfeefromamount` (boolean, default `false`). When `true`, the
recipient output absorbs the transaction fee instead of the change output:
the recipient receives `amount − fee` and the change is `inputs − amount`
(rather than `inputs − amount − fee`). This matches Bitcoin Core's
`subtractfeefromamount` semantics on `sendtoaddress`.

Both VAR and SKA coin types are supported. The fee absorption is rejected
(no transaction broadcast, RPC returns an error) if the post-subtraction
recipient amount is at or below the dust threshold for the recipient's
script — VAR uses the standard dust check against the configured relay
fee; SKA uses `MinSKADustAmount` (30 atoms).

Existing callers do not need to change — omitting the parameter or
passing `false` preserves the previous behavior.

### `signrawtransaction` — optional `skaValueIn` field on `RawTxInput` (additive)

Each `RawTxInput` entry in the `inputs` argument now accepts an optional
`skaValueIn` decimal-coin string (e.g. `"1.234567890123456789"`) that
asserts the SKA atom value of the prevout being spent. The wallet uses it
to populate `wire.TxIn.SKAValueIn` before signing.

Why this exists: the V13 wire format carries `SKAValueIn` through
deserialize/sign/serialize, so SKA transactions built by this wallet
continue to round-trip correctly through `signrawtransaction` without any
caller change. The new field defends against a third-party tool that
builds `wire.MsgTx` from primitives (only `previousOutPoint`, `valueIn`,
and `signatureScript` — the upstream `dcrwallet` shape) without the V13
extension fields. Without `skaValueIn`, the wallet would happily sign an
SKA transaction with `SKAValueIn=nil` and the node would reject the
broadcast with a fraud-proof error.

Population priority per SKA input:

1. The caller-supplied `skaValueIn` field on `RawTxInput`, if present.
2. The wallet's own UTXO set (when the wallet owns the prevout).
3. Otherwise, refusal with `ErrRPCInvalidParameter`.

VAR transactions ignore the field. Existing callers do not need to
change. The field is `omitempty`, so legacy clients continue to send
`signrawtransaction` requests with the same JSON wire shape.

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

### Emission-key backup format v1 → v2 → v3 (BREAKING)

The encrypted-blob format produced by `generateemissionkey` and consumed by
`importemissionkey` is now v3:

- KDF: scrypt(`N=2^15`, `r=8`, `p=1`) with a per-blob random 16-byte salt.
- Cipher: AES-256-GCM with a per-blob random 12-byte nonce.
- **New in v3:** the scrypt parameters (`salt`, `N`, `r`, `p`) are bound to
  the ciphertext as AES-GCM additional-authenticated-data, so any in-transit
  tampering with the KDF parameters trips authentication on decrypt.
- Wire format: `aes256gcm:v3:<salt_hex>:<N>:<r>:<p>:<nonce_hex>:<ct_hex>`.

v1 blobs (`aes256gcm:<iv_hex>:<ct_hex>`) and v2 blobs
(`aes256gcm:v2:<salt_hex>:<N>:<r>:<p>:<nonce_hex>:<ct_hex>` — KDF params not
authenticated) are explicitly rejected with an actionable error. **Operators
with v1 or v2 backups must re-export from the canonical wallet DB via
`generateemissionkey` before this release becomes load-bearing in their
disaster-recovery procedure.** The plaintext key material is unchanged — only
the at-rest envelope is rotated.

The decode path also caps `N <= 2^20`, `r <= 16`, and `p <= 16` to bound
CPU and memory cost on a malicious blob. Decrypted plaintext is now
required to be exactly 32 bytes and the all-zero scalar is rejected, so a
malformed v3 blob whose AAD-authenticated plaintext has the wrong shape is
caught upfront instead of silently coerced by `secp256k1.PrivKeyFromBytes`.
Passphrase byte slices are zeroed after key derivation.

### Imported emission keys

`importemissionkey` now requires exactly 32 bytes and rejects the all-zero
scalar; previously, shorter inputs were silently accepted and the secp256k1
library would coerce them, masking truncation bugs in callers.

## Unreleased — HD-wallet derivation

### SLIP-0044 cointype: m/44'/9508'/... (BREAKING for pre-release wallets)

Monetarium has been assigned its own SLIP-0044 cointype, `9508`
(registered via [satoshilabs/slips#2013](https://github.com/satoshilabs/slips/pull/2013)),
replacing the inherited Decred slot `42`. New wallets created on this
release derive all keys at `m/44'/9508'/...`.

This release does **not** ship an in-place migration for wallets created
against a prior pre-release at the inherited Decred slot, and no such
migration is planned. The production network is being restarted from
genesis, so no production state exists at the legacy path: every prod
wallet is derived under cointype `9508` from the start. Pre-release
simnet/testnet wallets at `m/44'/42'/...` should be treated as throwaway
state and recreated against this release.

The previously-prototyped `MigrateToReregisteredSLIP0044` /
`NeedsReregisteredSLIP0044Migration` helpers (introduced in an internal
build and never released) have been removed along with the
`coinTypeOldSLIP0044{Pub,Priv}KeyName` side bucket. Operators of
internal pre-restart testnets must move funds out before the restart;
there is no in-wallet sweep path.

## Unreleased — Notifications

### Transaction summaries surface every wallet-owned output (bug fix)

`TransactionNotifications` summaries previously walked credits and outputs
in lockstep and silently dropped any wallet-owned output after the first
non-credit gap (e.g. a wallet that owned outputs 0 and 3 of a 4-output
transaction would receive a summary listing only output 0). The summary
output loop now iterates `details.Credits` directly and looks up each
`MsgTx.TxOut[credit.Index]`, so all wallet-relevant outputs surface.

Downstream consumers (UIs, accounting tools) that depended on the previous
shape — for instance, by treating the summary's output list as a contiguous
prefix — must adjust. The on-disk transaction record is unchanged.

## Unreleased — Cryptography (continued)

### Argon2id KDF — `Time` parameter raised from 1 to 3

The wallet's encryption-key Argon2id KDF now uses `Time=3` (OWASP minimum)
for newly created wallets. Memory and parallelism parameters are unchanged
(256 MiB, capped 256 threads). The change increases brute-force resistance
roughly threefold at the cost of an equivalent unlock-time increase on
modern hardware (still sub-second on commodity CPUs).

Existing wallets retain the parameters stored in their encrypted-key blob —
the KDF parameters are read from disk per-blob, so the bump is forward-only
and does not invalidate any existing passphrase. Operators rotating their
passphrase after upgrade will get the new parameters automatically.

## Unreleased — Startup validation

### `wallet.Open` rejects non-power-of-10 `SKACoinConfig.AtomsPerCoin`

On startup, `wallet.Open` now validates every entry in
`chainParams.SKACoins` and refuses to open if any coin's `AtomsPerCoin`
is not `10^k` for some `k >= 0`. This guards against silent fund
truncation in the decimal-amount UI/RPC layer: `coinsToAtomsBig` and
the user-facing "decimal coin amount" path infer decimal places by
counting digits, which only matches the true scale when `AtomsPerCoin`
is exactly a power of 10. A non-pow10 scale would silently misround
both display and parse paths.

The default Monetarium chain parameters (mainnet, testnet, simnet) are
unaffected — all bundled SKA coins use pow-10 scales (`1e8`, etc.).

Operators running custom chains or sidechains: ensure each
`SKACoinConfig.AtomsPerCoin` is a power of 10 (typical values: `1e8`,
`1e10`, `1e18`). A wallet started against a chain that violates this
invariant will fail to open with
`SKA coin <id>: AtomsPerCoin <N> is not a power of 10`. There is no
auto-migration; either republish chain params with a pow-10 scale (a
consensus break) or sweep funds with the prior wallet binary first.

## Unreleased — Multisig

### `PrepareRedeemMultiSigOutTxOutput` — VAR redemption now enforces dust threshold (bug fix)

The VAR multisig redemption path previously accepted any post-fee output
amount above zero, while the SKA redemption path enforced
`cointype.MinSKADustAmount`. The two paths are now symmetric: VAR
redemptions whose post-fee amount falls below the relay-fee dust
threshold (`txrules.IsDustAmount`) are rejected up front with
`errors.Policy`, instead of being constructed and then rejected by the
mempool's standardness rules. No protocol or wire change.

## Unreleased — Wallet bookkeeping

### `addCredit` rejects SKA outputs with non-zero VAR `Value` (defense-in-depth)

The wallet's transaction-store `addCredit` path now rejects an SKA output
whose VAR `Value` field is non-zero (in addition to the existing rejection
of nil / non-positive `SKAValue`). The dual-coin wire protocol uses `Value`
for VAR atoms and `SKAValue` for SKA atoms; an output that carries both is
a wire-protocol violation. The validator already enforces this on
acceptance; this is a redundant local check so a future validator
regression cannot silently corrupt the UTXO bookkeeping.

## Unreleased — Wallet Go API

### `Wallet.SendOutputs` — new `subtractFeeFromAmountIdx int` parameter (BREAKING)

`Wallet.SendOutputs` and `txauthor.NewUnsignedTransaction` both gained a
new required `subtractFeeFromAmountIdx int` parameter at the end of their
signatures. External Go consumers that call these functions directly will
need to update their call sites — pass `-1` to preserve the previous
behavior (inputs cover outputs + fee, fee paid out of change), or pass a
valid output index to enable Bitcoin Core's `subtractfeefromamount`
semantics on that recipient (see the `sendtoaddress` JSON-RPC entry above
for the user-facing behavior).

In-tree callers (`createtx.go`, `methods.go`, the txauthor and wallet
test suites) have been updated; only out-of-tree Go importers are
affected.

No backwards-compatibility shim is provided: this network is being
restarted from genesis as a controlled rollout, so external Go API
stability for the pre-release wallet module is not a release-gate
constraint.

### `txauthor.NewUnsignedTransaction` no longer mutates the caller's `outputs` slice

When `subtractFeeFromAmountIdx >= 0`, the function previously decremented
`outputs[idx].Value` (or reassigned `outputs[idx].SKAValue`) directly on
the caller's `*wire.TxOut`. It now allocates a fresh `*wire.TxOut` with
the post-fee value and substitutes it into a *local* copy of the outputs
slice; the caller's slice header, slice elements, and the underlying
`*wire.TxOut` objects are all left untouched.

Callers that build a single `outputs` slice and pass it to multiple
`NewUnsignedTransaction` invocations (for retry-on-error or for batched
sends) will no longer see the recipient amount silently shrink across
calls. Callers that observed the old in-place mutation as their source of
truth for the post-fee recipient amount should read it from the returned
`AuthoredTx.Tx.TxOut[idx]` instead.
