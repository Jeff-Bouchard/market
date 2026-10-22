# Privateness — Go edition

Go backend. Plain HTML, CSS and browser JavaScript. No PHP, React, npm, Node, Composer, SQLite driver, CDN, or external Go modules. The existing local QR implementation and its license are included.

The NVS value is **prefilled, editable and submitted** as:

```
NS=seed1.emercoin.com,seed2.emercoin.com
```

The API uses that same default when `value` is omitted. An explicitly empty value is rejected; user edits are preserved. This sets the requested record value; it does not verify that those hosts are authoritative for a particular domain.

## Your base camp

Choose a callsign and one of three visual styles: Voyager, Architect, or Alchemist. All three starter abilities are available immediately: forge time tokens, create NVS records, and exchange NCH. Styles change the emblem and colors, not the reward rate or wallet permissions. The cosmetic profile is saved only in this browser; it is not a blockchain identity or account. Tokens, wallet addresses and keys are never stored in the profile.

The ten-second forge has four visual stages, orbiting rings, gathering particles and a token reveal after server confirmation. Switch on **Calm motion** for a still version; the initial setting follows your device’s reduced-motion preference, and you can explicitly choose full animation. Leaving the page or changing service pauses the visible countdown. The same server-enforced release check still applies. Cash-out and paid services need the operator’s configured wallets and RPC services.

## Windows quick start

Extract the ZIP, open the `faucet-nvs-go` folder, and double-click `START-WINDOWS.cmd`. Open **http://127.0.0.1:8787**. The supplied Windows x64 executable needs no build tools. Stop it with Ctrl+C before closing the window.

The portal defaults to **8787**. Ports **8080** and **8000** are reserved for your IPFS and Skywire services and are rejected as portal listeners. If upgrading an existing installation, change `listen` in `config.json` to `127.0.0.1:8787`; keep the existing data directory and keys.

The first start creates `config.json`, `data/signing.seed`, and `data/state.jsonl`. The interface and real signed-token countdown work immediately. Every start replaces the prior claim and requires a fresh ten-second server wait, even after reload or a previous release. Released claims cannot be fetched again. Copy your token before leaving; saved tokens remain manually redeemable. Browser/history restoration clears tokens, and all website/API responses prohibit caching. Wallet payouts and record registration require your RPC configuration; they never simulate success.

## Build from source

Go 1.24 or later is the only build requirement:

```sh
go test ./...
go vet ./...
go build -buildvcs=false -o faucet-nvs .
```

Copy `config.example.json` to `config.json`, then run the executable. Windows builds use `go build -buildvcs=false -o faucet-nvs.exe .`. All web assets are embedded in the binary. There is no dependency installation or frontend build step.

## Connect your services

Edit `config.json` while the app is stopped. `config.example.json` now includes an ignored `_help` object explaining every setting, units, examples, and empty values. You can copy `_help` into an existing configuration without changing its actual settings. Do not overwrite your existing wallet credentials or data path. Restart the faucet after edits:

| Setting | Purpose |
| --- | --- |
| `ness_url` | NESS node HTTP API base URL; default `http://127.0.0.1:6660` |
| `ness_wallet`, `ness_password` | Funded payout/address-generation wallet and password |
| `ness_api_user`, `ness_api_password` | Optional node HTTP Basic Auth, separate from the wallet password |
| `emc_url` | Emercoin JSON-RPC URL, e.g. `http://127.0.0.1:6662/` |
| `emc_user`, `emc_password` | Emercoin RPC credentials |
| `emc_wallet` | Loaded Emercoin wallet name; selects `emc25-kaisen.dat` |
| `exchange_url` | Legacy WORM recovery only; new exchanges run directly |
| `*_per_100_days` | NVS payment price for each started 100-day block |
| `coins_per_second`, `hours_per_second` | Faucet reward rates |
| `timezone` | Token signing timezone; UTC by default |

The NCH exchange runs directly in this Go app at **10,000 NCH = 1 NESS**. Enter whole NCH and a NESS destination. Send exactly the quoted NCH plus 0.000001 NESS to the unique deposit address, then check payment after confirmation. Your sending wallet pays its network fee separately. The quoted NESS is the net payout; no additional 5% is deducted. One year has 8,760 hours (8,784 in a leap year); 10,000 is the chosen exchange rate.

Unknown payout results and mismatched deposits stay in operator review and are never broadcast again automatically. Submitted means broadcast accepted, not confirmed. Do not reuse deposit addresses. Pending quotes reserve NESS liquidity and deposit inputs; cancel unused requests before sending funds to release the reservation. Late deposits require operator reconciliation. Use a dedicated wallet and do not spend it from another app. Old WORM slots retain their previous service flow for recovery only.

Emercoin wallet RPC calls use `/wallet/<escaped-name>`, fixing RPC -19 when multiple wallets are loaded. Set `emc_wallet` to the exact loaded name: `emc25-kaisen.dat` here. An empty string selects the unnamed wallet; null auto-selects only a single loaded wallet. Keep a pre-existing wallet path in `emc_url` consistent with this setting. Unlock the wallet for transactions and keep enough EMC for registration fees.

To upgrade: stop the existing faucet, back up its folder, replace the executable with the one from this ZIP, and restart using your existing config and data directory. Preserve your signing key and payment journal. This package contains no credentials or live wallet data.

EMC is offered when configured. NESS/NCH payment options are also generated when the NESS node and wallet are configured. Confirmed NESS balances use uint64 droplets (1 NESS = 1,000,000 droplets), verified against both NESS and current Skycoin source. Amount settings preserve decimal text and use exact integer calculations. NCH payments require at least one droplet alongside the required hours. Payment checks enforce the full duration-adjusted price.

## Verified NESS node settings

The default NESS HTTP API is **6660**; **6006** is the P2P port. The bundled `node/ness-fiber.toml` is an unchanged snapshot from the NESS repository. Current Skycoin supports `daemon --fiber-config node/ness-fiber.toml`; use a dedicated NESS data directory. Enable the `READ,WALLET,TXN` API sets, not seed-export APIs. The source settings are in [NESS fiber.toml](https://github.com/ness-network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/fiber.toml).

NESS burn factor **20** sets the minimum network burn rate to **1/20 = 5%**, with upward rounding to whole NCH. The node calculates the required fee from the transaction’s total consumed coin-hours; it is not an arbitrary user-entered amount. This follows [Skycoin’s RequiredFee implementation](https://github.com/skycoin/skycoin/blob/a5d2f6d2bb395ea0ef5b32264e7d5895833ed668/src/util/fee/fee.go). The portal checks the returned fee and exact recipient amounts before broadcasting.

Before reserving a payout token or allocating NESS payment addresses, the server verifies the node ticker, wallet API, burn factor, precision, blockchain key when exposed, and NESS genesis data. A stock Skycoin node without NESS fiber settings is rejected. Node Basic Auth and enabled/disabled CSRF modes follow the upstream client contract. Read-only preflight failures leave tokens unspent.

Configured rewards are NET recipient amounts; do not subtract 5% again from the displayed NCH quote. The source wallet must fund the recipient amount plus the node-calculated fee.

## Persistence and deployment

Use one running instance per data directory. An exclusive lock prevents two writers. If a crash leaves `server.lock`, first confirm the previous process has stopped, then remove only that lock file. Back up the entire data directory and configuration while stopped. The append-only journal is synced before a payout or registration can be sent; it grows over time and must be monitored. A truncated/corrupt journal prevents startup instead of silently forgetting payments.

Failed or uncertain external writes stay in `review` and are not automatically retried. Reconcile them with the wallet/node before taking further action. Session cookies last 24 hours; request recovery requires the original cookie. Save request references for operator support.

For public service, place the app behind an HTTPS reverse proxy, keep the Go listener and wallet APIs private, set `secure_cookies` to true and `public_origin` to your exact HTTPS origin (no trailing slash). Apply deployment-level rate limits. Go serves the frontend and API together on one origin.

This is a standalone Go application with a refreshed interface, **not a native Lovable platform project**. It follows the requested no-React/no-npm stack.

## Migration boundaries

This is a new Go implementation of the four portal flows, not an in-place PHP database upgrade. Existing SQLite databases, signing keys, sessions, saved slots, and legacy record-editing pages are not imported automatically. Keep the old deployment and its data available for outstanding tokens/requests; do not discard it when testing this package. Existing registered records can be managed through the owning Emercoin wallet.

The original timestamp + Ed25519 signature protocol and ten-second wait are retained, but a fresh installation generates its own signing key. Original tokens require the original key, timezone and redeemed-token history; copying only a key is insufficient. Tokens issued in the same second remain identical, as in the source protocol, and only one redemption succeeds.

Automated tests use fake wallet/RPC servers only. No live funds or blockchain records were used to validate this package. Source licenses are retained as `LICENSE-faucet` and `LICENSE-exchange`.

## Cash-out reports different wallet and network policies

The observed local health response had `user_verify_transaction` at **10 / 3 decimals**, while `unconfirmed_verify_transaction` was **20 / 6 decimals**. The first controls wallet-created transactions; the second controls admission of network transactions. Both must report **20 / 6** for this portal. The wallet setting at 10 means 10% minimum burn, not the requested 5%. Three decimals cannot represent the default one-droplet reward.

Set these variables in the **NESS node's startup environment**, then restart the node with its existing arguments and data directory:

```bash
export USER_BURN_FACTOR=20
export USER_MAX_DECIMALS=6
```

PowerShell equivalent:

```powershell
$env:USER_BURN_FACTOR='20'
$env:USER_MAX_DECIMALS='6'
```

Run the node command in that same terminal. The `node/with-ness-policy.sh` and `node/WITH-NESS-POLICY.cmd` wrappers can prefix an existing node command. They do not stop or restart a running node. Setting variables on the faucet process cannot alter a separately running node. Check `/api/v1/health` afterward: both policy objects must show `burn_factor: 20` and `max_decimals: 6`. The NESS network settings in the bundled fiber snapshot remain unchanged.

In a GOPATH build, `github.com/skycoin/skycoin/src/params` may resolve to a separate Skycoin checkout despite launching `cmd/privateness` from the NESS directory. That explains why editing only the NESS copy of `src/params/params.go` can leave runtime wallet settings unchanged. Node environment overrides are supported by both codebases.


## Guided interface update

The service menu has three choices: Get a reward, Exchange hours, Register a name. Reward creation and cash-out are successive steps of one flow. The explorer profile is optional, with no duplicate service cards. Payment screens show one currency at a time and explain when to wait, check, or contact the operator. Submitted transactions are never described as confirmed.

The fee is displayed as **5% (burn factor 20; 1/20 = 5%)**, rounded upward to whole NCH. Port numbers and connection settings are unchanged. Connection ports are not shown in the visitor interface. Error responses include a stable status code, plain-language explanation, next action, and a support reference for logged operation failures. Technical details are expandable. The original error is retained in the operator console with the support reference.

Form progress and accessible status announcements follow [W3C form guidance](https://www.w3.org/WAI/tutorials/forms/multi-page/) and [notification guidance](https://www.w3.org/WAI/tutorials/forms/notifications/).
