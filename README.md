# Market — PrivateNess services and records

Market brings [NESS Faucet](https://github.com/NESS-Network/faucet) and [NVS Exchange](https://github.com/NESS-Network/nvs-exchange) into one application for market.ness.cx. Faucet and NVS run in one Go process, on one origin, with one session and data directory. This is a fresh-start build; it does not import old-site tokens or unfinished payment requests.

The user-designated development repository is `Jeff-Bouchard/market`, with production intended for the NESS-Network organization. This source package does not transfer repositories, publish code, or deploy a server.

The product leads with earning through node services and agent-delivered work. The implemented shared setup creates receiving access, provides the faucet claim, and registers public records with reviewed browser-signed payment. Service execution, agent deployment and customer sales must connect to real provider mechanisms; the homepage is not evidence that those later stages are already implemented.

## Confirmed requirements

- Hostname market.ness.cx, on an existing Debian- or Arch-based Linux server. Both nodes are reported running and synced: NESS at 127.0.0.1:6660 and Emercoin RPC at 127.0.0.1:6662 on that server.
- Create a NESS wallet and optionally an Emercoin wallet from one BIP39 12-word phrase.
- Save the backup, receive NESS/NCH, then create an EmerNVS record with an NCH payment signed in the browser after explicit review.
- Record eight seconds alongside wallet creation and backup; show actual operation results. Blockchain confirmation has its own timing.
- Preserve NESS's Skycoin lineage and Emercoin's Bitcoin merged-mining lineage.
- No network-fee or burn-factor explanations in visitor screens.

## Run locally

For the local preview, double-click `OPEN-LOCAL.cmd`. It starts the bundled app in the background if needed, then opens http://127.0.0.1:8787 in your usual browser. Running it again opens the existing app. It uses the separate preview configuration and data, with node connections disabled. No terminal needs to stay open. If the executable is missing from a source-only download, build it using the command below first.

Double-click `START-WINDOWS.cmd`, then open http://127.0.0.1:8787. The executable `ness-unified.exe` includes browser assets. The launcher creates config.json from the example only if none exists.

Wallet creation, recovery and NCH payment signing run on the visitor's device. The unencrypted backup contains the NESS collection entry plus the shared recovery phrase and versioned paths. Visitors can restore the phrase here or reopen that backup. Emercoin Core requires the optional WIF key export; it does not directly restore this phrase. Only the specific reviewed EmerNVS payment is supported for outgoing browser transfers. See docs/WALLET-SOURCES.md for derivation, recovery and compatibility limits.

Actual faucet payouts and NVS registrations require operator configuration of funded NESS and Emercoin services. The page reports when either is unconfigured. Configuration presence does not establish node health or liquidity. No live payout or blockchain registration has been tested.

The running local inspection uses `preview.config.json`, has both node URLs blank and stores its state in `preview-data/`. It cannot send blockchain transactions.

## Build and check

`go test ./...`

`go vet ./...`

`go build -buildvcs=false -o ness-unified.exe .`

The bundled browser wallet asset is already built. To rebuild it: `npm ci`, `npm run build:wallet`. Its source and dependency lock are included. `npm run test:wallet` verifies the address codec against a known vector and 100 OpenSSL-generated public keys/hashes, plus backup reopening and tamper rejection. Node/npm are not runtime requirements.

## Still to resolve before deployment

- Configure actual wallet IDs/names, local credentials and service rates. Values inherited in config.example.json are examples from the archive, not new approved prices. deploy/config.market.example.json contains the confirmed hostname and server-local node endpoints.
- NVS currently creates new records. Original NVS record updates/renewals and their ownership authorization need an explicit implementation before feature parity can be claimed.
- Faucet timestamp-only tokens preserve the original protocol's same-second collisions. Fresh-start permission removes migration needs but does not itself decide a new token protocol.
- Native wallet-file import has been checked against canonical code, not a running desktop wallet application.
- Visitor wallet creation is separate from PrivateNessTools user/node identity creation. No new identity authority, WORM authorization, node or encrypted-storage service is claimed by this build.

The archive's earlier README is preserved as docs/ARCHIVE-README.md. Historical test reports and deployment claims there are not current verification results. Original material remains untouched in the sibling original directory.
