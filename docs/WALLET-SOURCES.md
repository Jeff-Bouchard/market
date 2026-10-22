# Wallet recovery and local signing

One cryptographically random BIP39 English 12-word phrase (128-bit entropy with checksum) restores both wallets in this application. No optional BIP39 passphrase is used. The phrase and keys remain in page memory and the visitor's explicit private backup. Reloading requires recovery or reopening the file.

## Versioned derivation

Format `privateness-market-v1`:

- NESS: BIP39 seed -> `m/44'/8000'/0'/0/0`, compressed secp256k1, native Skycoin address codec. This follows NESS's default BIP44 coin type 8000, not a newly assigned NESS SLIP44 number. The site manages only the first receiving address, returning change to it.
- Emercoin: the same BIP39 seed -> `m/0'/0'/0'`, matching Core's external child-path layout, then compressed secp256k1, HASH160, mainnet address version 33 and double-SHA256 Base58Check. The site applies this path to a BIP39 seed; Core generates a 256-bit seed instead. This is an explicit application recovery contract, not a claim that Core imports BIP39 or that Emercoin has a registered BIP44 path.
- Emercoin Core recovery: export the resulting compressed WIF (version 128) and import that key into the visitor's own Core wallet. This recovers the single record-owning address. The key is never sent to the service wallet. Desktop import has not been exercised here.
- The NESS `.wlt` backup contains the native unencrypted collection entry plus a `recovery` extension with the phrase, version and paths. Keep the original backup: other wallet software may discard unknown extension fields when resaving. Old collection files without the extension remain usable for NESS but cannot produce a phrase retroactively.

## Canonical sources

NESS revision `9dd55e9ee8fec7de9e195cce7d8c98fb922ae648`:

- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/cipher/bip44/bip44.go
- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/wallet/bip44wallet/account.go
- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/cipher/address.go
- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/wallet/collection/decoder.go
- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/coin/transactions.go
- https://github.com/NESS-Network/ness/blob/9dd55e9ee8fec7de9e195cce7d8c98fb922ae648/src/api/spend.go

Emercoin revision `5e32f396be2da1ef0aa2dc6d48bd1fd65aee383c`:

- https://github.com/emercoin/emercoin/blob/5e32f396be2da1ef0aa2dc6d48bd1fd65aee383c/src/wallet/wallet.cpp (`DeriveNewChildKey`)
- https://github.com/emercoin/emercoin/blob/5e32f396be2da1ef0aa2dc6d48bd1fd65aee383c/src/chainparams.cpp (address/WIF prefixes)
- https://github.com/emercoin/emercoin/blob/5e32f396be2da1ef0aa2dc6d48bd1fd65aee383c/src/namecoin.cpp (`name_show`, confirmed name database)

## PrivateNess foundation

Coin wallets are distinct from PrivateNessTools Ed25519/X25519 user/node identities and WORM authority. Existing identity, node and storage mechanisms remain the baseline; this page creates no identity authority or permission grant. PrivateNessTools at `4f0bfc4dd3e7ea33625af9d2b606845181198650` was inspected first. Its `generate_bip39_seed` selects words from a wordlist without constructing the BIP39 checksum. This wallet therefore uses a checksum-valid implementation matching NESS native wallet requirements; it does not replace the identity mechanisms.

## Signing boundary

The backend requests an unsigned native NESS transaction for the session's immutable record, exact NCH price and one droplet, supplying only the visitor's public address. The browser checks the encoded transaction's length/type/hash, input IDs and addresses, exact destination/amount, coin conservation, native Coin Hour calculation, and change returning to the same wallet. The explicit confirmation action signs `SHA256(inner_hash || input_id)` with low-S recoverable secp256k1. The service accepts only the prepared transaction with signatures filled in. A durable submission record precedes broadcasting; unknown results stay in review and repeated submissions do not broadcast again.

Confirmed NCH triggers native Emercoin `name_new` from the funded service wallet, assigning the optional visitor EMC address as owner. `name_show` must match name, value, owner and transaction before the UI reports confirmation. Node-reported balances and input metadata are trusted; this browser is not a light client. No EMC transaction signing or record update/renewal interface is implemented yet.

Dependencies: `@noble/curves` 2.4.0, `@noble/hashes` 2.4.0, `@scure/bip32` 2.2.0 and `@scure/bip39` 2.2.0, pinned with integrity in package-lock.json. Served locally without a CDN. Node/npm are build/test tools only. Use `npm ci`, `npm run build:wallet`, and `npm run test:wallet`.
