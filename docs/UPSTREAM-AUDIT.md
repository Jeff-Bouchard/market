# NESS / current Skycoin compatibility audit — 2026-09-14

This audit replaces the original package's unverified PHP-derived wallet assumptions. It covers every node HTTP call made by this portal, address validation, units, transaction creation/validation/broadcast, fee interpretation, node identity, and configuration. It also rechecks the portal's token, persistence, and NVS protections. This is source and automated-test verification; no live wallet payout or NVS registration was performed.

## Exact source revisions

- [NESS master: 9dd55e9ee8fec7de9e195cce7d8c98fb922ae648](https://github.com/ness-network/ness/tree/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648), commit dated 2022-11-18. This is the repository's default branch HEAD retrieved for this audit, not a claim that its implementation is current Skycoin.
- [Skycoin develop: a5d2f6d2bb395ea0ef5b32264e7d5895833ed668](https://github.com/skycoin/skycoin/tree/a5d2f6d2bb395ea0ef5b32264e7d5895833ed668), commit dated 2026-09-06. This is the current default branch HEAD retrieved for this audit. Its own go.mod specifies Go 1.26.1.

NESS network parameters come from the NESS fiber file. Current protocol and API behavior are checked against Skycoin develop, with old NESS API differences noted. Source paths and SHA256 hashes are recorded in `upstream-sources.json`. `node/ness-fiber.toml` is byte-for-byte the pinned NESS file.

## Findings and corrections

| Area | Source evidence | Result in this ZIP |
| --- | --- | --- |
| Node ports | NESS `fiber.toml`, `[node]` | HTTP API default corrected to **6660**. **6006** is P2P. Removed the old API example. The portal itself listens on 8787. |
| Fee | NESS fiber settings; both repos `src/util/fee/fee.go: RequiredFee` | **1/20 = 5%** minimum network burn, with upward rounding to whole NCH. The protocol calculates the fee from consumed transaction coin-hours. No additional 5% deduction from net recipient rewards. |
| Precision | NESS fiber `user_max_decimals`, `unconfirmed_max_decimals`; Skycoin `src/util/droplet/droplet.go` | NESS uses 6 decimal places and 1,000,000 droplets per coin. Configuration and payment amounts now preserve decimal text and calculate with integers; excess precision and overflow fail. |
| Chain identity | NESS genesis/key; both `src/api/health.go`; Skycoin `src/readable/block.go` | Before wallet writes, check NESS/NCH branding, enabled WALLET API, fee/precision, blockchain key if present, and genesis timestamp/address/volume. Old NESS health lacks the key field; genesis checking still applies. |
| Basic Auth | Skycoin `src/api/client.go: SetAuth/applyAuth`, `middleware.go` | Add separate node API credentials; send them on NESS requests only. Wallet password remains a separate request parameter. Redirects remain rejected. |
| CSRF | Both `src/api/csrf.go`; Skycoin `client.go: CSRF` | Enabled mode requires a nonempty token. HTTP 404 means node CSRF is disabled; proceed without that header. Other failures do not silently disable protection. Portal browser CSRF stays enabled independently. |
| New address | Both `src/api/wallet.go: walletNewAddressesHandler` | POST form fields `id`, `num=1`, `password`; response `addresses`. Validate the returned NESS address. |
| Wallet balance | Both `src/api/wallet.go`, `src/readable/wallet.go` | GET `wallet/balance?id=...`; addresses map and confirmed/predicted pairs. Coins/hours decode as **uint64**, never float. |
| Address balance | Same sources | GET `balance?addrs=...`. NESS payments compare droplets against the full duration price. NCH requires whole hours plus at least one droplet. Unconfirmed predicted balances do not count as confirmed payment. |
| Wallet transaction | Skycoin `src/api/client.go`, `src/api/spend.go` | POST JSON to `wallet/transaction`, manual hours, decimal-string coins/hours, wallet/password, explicit source addresses and wallet-owned change address. |
| Created transaction | Skycoin `src/coin/transactions.go`, `transaction_skyencoder.go`, `src/api/spend.go` | Decode wire layout; validate length, type, nonzero signature fields, input counts, inner/outer SHA256, recipient amounts, wallet-owned change, totals, and required fee. JSON description must match encoded bytes. The node validates secp256k1 signatures and actual UTXOs. |
| Broadcast receipt | Both `src/api/transaction.go: injectTransactionHandler` | POST `rawtx` to `injectTransaction`. Successful response is a JSON string transaction hash. Require exact agreement with the validated transaction hash. HTTP 503/uncertain outcomes remain in review and are never rebroadcast automatically. |
| Address encoding | Skycoin `src/cipher/address.go` | Replace character-only validation with NESS key-first, version-0, single-SHA256 checksum validation. EMC uses separate mainnet Base58Check validation; it is not a Skycoin address. |
| NVS payment price | Local review plus node unit contracts | Exact duration-scaled amounts; underpayment rejected. A failed check for one payment currency does not prevent checking the other offered currencies. |
| Empty optional owner | Local browser/form review | Blank EMC owner no longer receives a custom validation error. Actual NS textarea default remains `NS=seed1.emercoin.com,seed2.emercoin.com`. |
| Process lifecycle | Local Go review | Embed timezone data for Windows token signing; wait for graceful HTTP shutdown before releasing the data-directory lock; handle Ctrl+C and SIGTERM. |
| Local HTTP host | Skycoin `src/api/middleware.go: HostCheck` | Reject foreign Host headers on loopback listeners instead of trusting any matching browser Host/Origin pair; configured public origins restrict the host explicitly. |

The default reward rates and NVS service prices are **application policy**, not values defined by Skycoin or NESS fiber.toml. They remain configurable. Chain fee calculations apply to all input hours, including hours returned as change.

## Verification performed

1. Cloned both repositories and recorded their exact default-branch commits.
2. Read each endpoint handler, request/response type, address codec, droplet conversion, fee calculation, and relevant fiber-loading/API-set configuration.
3. Executed current Skycoin tests directly from the pinned checkout with its Go 1.26.1 toolchain and vendored dependencies:

```sh
go test -mod=vendor ./src/util/fee ./src/util/droplet ./src/cipher -run 'Test(RequiredFee|RemainingHours|Address|DecodeBase58Address|FromString|ToString)' -count=1
go test -mod=vendor ./src/api -run '^(TestWalletCreateTransaction|TestInjectTransaction|TestWalletNewAddressesHandler|TestGetBalanceHandler|TestWalletBalanceHandler|TestCSRF|TestCSRFWrapper)$' -count=1
```

Both commands passed. They are targeted upstream unit tests, not the complete Skycoin test suite.

4. The portal's tests use the actual upstream golden balance and encoded/readable transaction fixtures under `testdata/upstream/`. The decoded transaction hash, recipient addresses, amounts, and hours must match those independent fixtures.
5. Added tests for 5% rounding, amounts beyond floating-point exactness, checksum errors, current/legacy health, wrong-chain/key/fee/precision rejection, Basic Auth, CSRF enabled/disabled, address generation form fields, payout JSON, correct/underpaid fees, output tampering, change destination, broadcast hash, successful idempotent retry, and HTTP-503 no-rebroadcast behavior.
6. Re-ran portal tests, Go vet, and the Windows executable build using Go 1.26.1. HTTP smoke checks exercise the homepage and assets, early-release rejection, the actual ten-second wait, signed token release and quote. The initial browser accessibility check confirmed the rendered NVS default and optional owner field. On 2026-09-15 the redesigned interface was also visually inspected in a Chromium browser at desktop and phone sizes; callsign/style persistence, ability navigation, four countdown phases, token release and quote, calm-motion control, and service-switch pause/resume were exercised. No browser JavaScript errors were observed in that run.
7. Attempted a read-only health request to `http://127.0.0.1:6660/api/v1/health`. The connection was refused; no local live node was available for staging. No live wallet calls or transactions were made.

## Remaining boundaries

- A real node with the operator's wallet, sufficient coins/hours, network access, and the intended NESS fiber configuration is still required for live staging. No test here asserts live liquidity, peer reachability, synchronization, or successful on-chain settlement.
- New NCH exchanges execute directly in this Go app at the user-selected 10,000:1 rate. The external WORM contract remains only for recovery of old slots; its service is not audited here.
- Emercoin is a separate codebase. NVS calls remain JSON-RPC calls to the operator's node, and their tests use a simulated service. EMC address prefixes were separately checked in `emercoin/emercoin`'s chainparams source (mainnet P2PKH 33, P2SH 92).
- NCH balance includes accrued coin-hours according to the node's balance API. It is not a ledger of only the hours initially transferred. Each request receives its own generated address; there is no automatic sweep/reuse of payment addresses.
- Persistence is one writer with a durable append-only journal, not a scalable shared database. Operator reconciliation, backups, journal growth monitoring and reverse-proxy rate limits remain deployment responsibilities.
- This does not migrate PHP SQLite state, outstanding old-key tokens, saved old sessions, or the legacy record editor. Preserve the original deployment for outstanding operations.
- New decimal amounts are emitted as JSON strings (numeric configuration inputs still work). Old Go journals with more precision than supported now fail payment checks for review rather than silently rounding. Do not automatically reprice outstanding requests.

## Live configuration diagnosis — 2026-09-15

A later read-only check reached the local node on port 6660. It reported wallet policy 10 / 3 decimals and network policy 20 / 6 decimals. The local NESS run-client.sh uses GO111MODULE=off, and its imports resolve github.com/skycoin/skycoin/src/params from a separate GOPATH checkout. That checkout has defaults 10 / 3, explaining the observed mismatch even though the local NESS params source says 20 / 6. Both NESS run scripts were updated with USER_BURN_FACTOR=20 and USER_MAX_DECIMALS=6. A separate, read-only Go probe of the exact imported package confirmed 20 / 6 with these environment variables. After the operator restarted the node, a live health check confirmed both policy blocks at 20 / 6. The live genesis block also matched the NESS timestamp, address and supply. No wallet transaction was sent for verification.

Every faucet start now creates a fresh claim and a full ten-second deadline. A released claim is not a token retrieval endpoint. No-store headers cover both the interface/assets and APIs, and browser page/history lifecycle handling discards displayed tokens. Durable signing keys and redemption history are retained.


## Direct exchange and Emercoin follow-up, 2026-09-15

Primary Emercoin sources inspected: [wallet RPC](https://github.com/emercoin/emercoin/blob/master/src/wallet/rpcwallet.cpp) and [name RPC](https://github.com/emercoin/emercoin/blob/master/src/namecoin.cpp). Wallet selection uses `/wallet/<URI-encoded-name>`; the unnamed wallet uses `/wallet/`. Root RPC cannot choose between multiple loaded wallets. `getnewaddress` explicitly requests legacy addresses. Watch-only and locked wallets are rejected before allocating NVS payment addresses. Minting-only unlock may still be rejected by `name_new`; that request stays in review. Missing-name errors are distinguished from disk/decoding failures. Expired/deleted names can be registered again subject to node validation.

Direct quotes use integer arithmetic and six NESS decimals. NCH amounts are JSON strings to preserve browser precision. Payment requires exactly one droplet and the quoted confirmed NCH at a unique address. Unconfirmed and insufficient deposits cannot trigger payouts; mismatches require review. Pending deposits and quote liquidity are excluded from unrelated payouts. Confirmed and predicted balances both constrain liquidity. Cancellation requires no confirmed or predicted deposit. Cancelled addresses remain excluded from spending; late deposits require reconciliation.

Payout intent is persisted before signing/broadcast. An uncertain result is never automatically retried, including after restart. Submitted is not a confirmation. Tests use mock services; they do not prove live block inclusion or constitute a production security/load certification. Original browser sessions are required for self-service; operators must reconcile requests with lost sessions. Use a dedicated funded wallet and rate-limit public access so anonymous quotes cannot tie up liquidity indefinitely.
