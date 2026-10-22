# Recovery verification — 2026-10-10

Confirmed scope: one backend and fresh state; market.ness.cx; dark simplified screens; eight seconds alongside real operations; shared BIP39 12-word recovery for NESS and optional Emercoin; browser signing after record/amount confirmation; Skycoin lineage; no reward or network-fee marketing copy. Existing Linux server (Debian/Arch family); both nodes reported running and synced at server-local ports 6660 (NESS) and 6662 (Emercoin).

## Passed

- Full Go suite and go vet. Mock services and temporary journals only.
- Browser-wallet tests: known address vector, 100 independent OpenSSL curve/hash checks, phrase recovery matching native NESS derivation, Emercoin address/WIF encoding, backup reopening, invalid phrase and tampered backup rejection.
- Browser signatures: native NESS transaction decoding, Verify(), and VerifyInputSignatures() accepted the browser-signed public fixture. Test harness source is preserved under docs/canonical-wallet/marketcheck.go.txt; fixtures are under testdata/. These are public test keys, never funding wallets.
- Signing refuses altered amount, destination, input ownership, transaction hash and pre-signed templates.
- Backend contract tests reject cross-session access and transaction mutation; journal before injection; submit at most once even after uncertain response; preserve state through restart.
- EmerNVS confirmation requires the matching record content, owner, transaction and a live expiry. Confirmation does not issue a new registration.
- Browser UI: create both wallets locally, download backup, acknowledge saving, automatically record eight seconds, continue to the faucet quote with the selected address, populate the Emercoin owner. No console errors. No payout was enabled in the unconnected preview.

## Evidence and limits

PrivateNessTools subset matches are in sibling source-verification.json. Exact source revisions, derivation and restoration details are in docs/WALLET-SOURCES.md. The production template has the user-supplied hostname and server-local node URLs. The operator's running nodes have not been contacted or changed.

The attempted temporary mock-server launch for a complete browser payment/registration test was rejected by automatic approval review, without a specific reason. That browser end-to-end test is not claimed. Native-signature checks and isolated backend integration tests passed. Live transfers, registrations, desktop wallet import, deployment, and target-host Linux execution remain untested. Original NVS updates/renewals remain unfinished. The inherited timestamp-only faucet protocol still has same-second collisions.

Local preview configuration keeps both blockchain URLs empty. Empty test wallets and preview state remain outside distribution archives. Production rates and service wallet selection require actual operator configuration; archived example amounts are not newly approved rates.

## Audience revision — 2026-10-21

The main flow now uses access, backup, starting balance and name registration. Addresses, claim codes, wallet-app exports and infrastructure background are in optional details. The exact final payment remains visible before confirmation. Record control is selected by default, and the setup option explains service custody if disabled. Backup loss/access consequences and public-record persistence remain explicit.

The working nameserver entry `NS=seed1.emercoin.com,seed2.emercoin.com` is retained at the user's explicit direction. Record names, duration and exchange amount have valid examples and point-of-use tips. Receiving/owner addresses and claim codes use the visitor's real generated values; recovery help directs the visitor to their own private backup instead of suggesting public example keys.

Validation for this revision: Go tests and vet pass; all five browser-wallet tests pass; changed JavaScript parses; HTML labels, help targets, unique IDs and JavaScript element references pass. Windows and Linux amd64/arm64 builds pass. The running preview API reports eight seconds, the exact default nameservers, and both node integrations disabled.

Visual/browser interaction verification is not claimed for this revision: browser inspection of the existing error-page tab was blocked by the browser tool's URL policy. The preview is running at `http://127.0.0.1:8787/#faucet` for user review. No live node transaction or deployment occurred.

Related user clarification: the data poisoner targets advertisers/advertising profiling, not AI training. The unified site does not currently implement that separate feature; its purpose is preserved in the persistent PrivateNess reference context.

Tone refinement later on 2026-10-21 replaces “Make a name yours” with “Secure records. Under your control.” The flow consistently refers to records serving domains, identities and services, with direct setup/registration instructions. Working defaults and tips remain. No new protocol implementation, identity generator, or quantitative security-equivalence claim is introduced. Go tests and vet plus JavaScript syntax checks were repeated for this text revision; browser visual verification remains outstanding.

Further user steering makes provider income the main outcome and selects both node services and agent-delivered work for launch. The homepage now leads with “Build services. Get paid.”, introduces both routes, and identifies the current working flow as shared access/registration setup. The agent-factory launch/sales connection is not present in this build; its actual repository or app has been requested. The income framing is not evidence of completed jobs, customers, installed nodes, or operational agent deployment.
