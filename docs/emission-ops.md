# SKA emission key operations

This document is for operators running an SKA emission flow against a Monetarium wallet (`monetarium-wallet`).

## Authorization model

The SKA emission RPCs — `generateemissionkey`, `importemissionkey`, and `createauthorizedemission` — use a **per-call capability gate**: every emission RPC takes the wallet master passphrase as a `walletpassphrase` parameter, the handler unlocks the wallet for the duration of the call only, and re-locks on return (including on panic). The ambient `walletpassphrase`/`walletlock` window used by other private-key-using RPCs is **not** consulted by these handlers.

This means:

- Calling an emission RPC without `walletpassphrase` is rejected with `ErrRPCInvalidParameter "walletpassphrase is required"`. There is no fallback to an ambient unlock.
- The master passphrase is resident in the wallet process only for the duration of the single RPC, not for an arbitrary unlock window. This is a strict reduction in exposure compared to the ambient model.
- Each emission RPC is one privileged operation; there is no concept of "batching" several emission calls under one unlock.

### `walletpassphrase` vs `passphrase`

`generateemissionkey` and `importemissionkey` accept two separate passphrase fields. They are not interchangeable:

- `walletpassphrase` — Wallet master passphrase. Unlocks the wallet so the new emission key can be encrypted with the manager's crypto key and persisted. **Required.**
- `passphrase` — Backup-blob encryption passphrase. Encrypts the returned key backup (or, for `importemissionkey`, decrypts an `aes256gcm:`-prefixed import blob). Minimum 12 characters where present. **Conditionally required** (see helpdesc).

`createauthorizedemission` only needs the wallet master passphrase, which it accepts as `passphrase` on that command (the backup-blob concept does not apply at signing time). The helpdesc text in `internal/rpchelp/helpdescs_en_US.go` is the canonical reference for each field's semantics.

## Recommended pattern

Treat each emission RPC as a privileged single-shot operation, passing the wallet master passphrase directly on the call:

```sh
monetarium-wallet rpc generateemissionkey "<keyname>" "<wallet-master-passphrase>" "<backup-blob-passphrase>"

monetarium-wallet rpc createauthorizedemission "<coin-type>" "<address>" "<height>" "<wallet-master-passphrase>"
```

There is no need to call `walletpassphrase` beforehand or `walletlock` afterwards for these RPCs. The handler unlocks and re-locks internally.

If your previous tooling wrapped emission RPCs in `walletpassphrase`/`walletlock` brackets, remove the brackets — leaving them in place will not break correctness, but it widens the exposure window unnecessarily.

## `forcewindow` and `forcenonce`

`createauthorizedemission` rejects out-of-window heights and non-default nonces by default. The previous behavior was warn-and-proceed; the node would have rejected the resulting authorization at validation time, so the new defaults surface the failure earlier.

Two **independent** flags let operators override these checks where the workflow demands it:

- `forcewindow=true` — bypass the "current height is outside the emission window for this coin type" check. Use this only when pre-staging an authorization through a cold-signing pipeline that runs ahead of the window opening; the node will accept the signed authorization if and only if it lands inside the window at submit time.
- `forcenonce=true` — bypass the "nonce ≠ 1" check. Use this only when re-authorizing a coin type whose previous authorization is not the next-expected nonce (this is rare; emissions are one-shot per coin type).

The flags are independent: setting one does not imply the other. The previous combined `force` flag has been removed.

## What not to do

- Do **not** rely on an ambient `walletpassphrase` unlock for emission RPCs — it is not consulted. Pass the master passphrase as `walletpassphrase` directly on each call.
- Do **not** set `forcewindow=true` "by default" in operator scripts. The check is there to surface mistakes before they become wasted signing rounds.
- Do **not** reuse the master passphrase as the backup-blob passphrase. They are independent secrets; mixing them defeats the backup-blob encryption purpose.
- Do **not** assume binding to `127.0.0.1` is sufficient — anything reachable on the host (including local browser pages, see the websocket origin policy) can issue privileged RPCs if it learns the passphrase. Treat the master passphrase as you would treat an HSM credential.
- Do **not** embed secrets, account numbers, ticket IDs, or other PII in the emission `keyName` argument. Successful retrievals are recorded in the audit log verbatim alongside the public key; treat `keyName` as a routing label, not a free-form note.

## Where to look in the code

- `internal/rpc/jsonrpc/methods.go` — `createAuthorizedEmission`, `generateEmissionKey`, `importEmissionKey` handlers; the `withWalletUnlocked` helper that implements the per-call gate.
- `internal/rpchelp/helpdescs_en_US.go` — canonical descriptions of every emission RPC parameter, including the `walletpassphrase` vs `passphrase` distinction.
- `rpc/jsonrpc/types/methods.go` — `GenerateEmissionKeyCmd`, `ImportEmissionKeyCmd`, `CreateAuthorizedEmissionCmd` field definitions and constructors.
- `internal/rpc/jsonrpc/server.go` — websocket origin policy (rejects cross-origin browser upgrades; non-browser clients without an `Origin` header are allowed).
