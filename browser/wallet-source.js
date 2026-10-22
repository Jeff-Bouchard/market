// NESS uses the Skycoin address codec and collection-wallet JSON format.
// See docs/WALLET-SOURCES.md. This module performs no network or storage I/O.
import { secp256k1 } from '@noble/curves/secp256k1.js';
import { sha256 } from '@noble/hashes/sha2.js';
import { ripemd160 } from '@noble/hashes/legacy.js';
import { bytesToHex, hexToBytes } from '@noble/hashes/utils.js';
import { generateMnemonic, validateMnemonic, mnemonicToSeedSync } from '@scure/bip39';
import { wordlist } from '@scure/bip39/wordlists/english.js';
import { HDKey } from '@scure/bip32';

export const NESS_PATH = "m/44'/8000'/0'/0/0";
// Emercoin Core's external branch, applied to this app's BIP39 seed.
// Core does not import BIP39 phrases: export the resulting WIF for Core.
export const EMC_PATH = "m/0'/0'/0'";

const alphabet = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
function base58(bytes) {
  let n = BigInt('0x' + bytesToHex(bytes)), result = '';
  while (n) { result = alphabet[Number(n % 58n)] + result; n /= 58n; }
  for (const byte of bytes) { if (byte) break; result = '1' + result; }
  return result;
}
export function addressFromPublicKey(publicKey) {
  const body = new Uint8Array(21);
  body.set(ripemd160(sha256(sha256(publicKey))));
  const bytes = new Uint8Array(25);
  bytes.set(body); bytes.set(sha256(body).subarray(0, 4), 21);
  return base58(bytes);
}
export function validAddress(address) {
  if (typeof address !== 'string' || !/^[1-9A-HJ-NP-Za-km-z]{26,35}$/.test(address)) return false;
  let n = 0n;
  for (const char of address) n = n * 58n + BigInt(alphabet.indexOf(char));
  let hex = n.toString(16); if (hex.length % 2) hex = '0' + hex;
  const decoded = hexToBytes(hex);
  const leading = address.match(/^1*/)[0].length;
  const bytes = new Uint8Array(leading + decoded.length); bytes.set(decoded, leading);
  if (bytes.length !== 25 || bytes[20] !== 0) return false;
  const checksum = sha256(bytes.subarray(0, 21)).subarray(0, 4);
  return checksum.every((v, i) => v === bytes[21 + i]) && base58(bytes) === address;
}
export function walletFromSecret(secret, created = Date.now()) {
  const secretBytes = typeof secret === 'string' ? hexToBytes(secret) : secret;
  const publicBytes = secp256k1.getPublicKey(secretBytes);
  const address = addressFromPublicKey(publicBytes);
  return {
    meta: { filename: `ness-${address}.wlt`, label: 'My NESS wallet', type: 'collection',
      version: '0.4', coin: 'skycoin', encrypted: 'false', tm: String(Math.floor(created / 1000)) },
    entries: [{ address, public_key: bytesToHex(publicBytes), secret_key: bytesToHex(secretBytes) }]
  };
}
export function newWallet() {
  return walletFromPhrase(generateMnemonic(wordlist, 128));
}
export function walletFromPhrase(phrase) {
  phrase = phrase.normalize('NFKD').trim().toLowerCase().split(/\s+/).join(' ');
  if (phrase.split(' ').length !== 12 || !validateMnemonic(phrase, wordlist)) throw new Error('Enter the 12 recovery words in their original order.');
  const seed = mnemonicToSeedSync(phrase), root = HDKey.fromMasterSeed(seed);
  const child = root.derive(NESS_PATH), secret = child.privateKey;
  try {
    const wallet = walletFromSecret(secret);
    wallet.recovery = {format: 'privateness-market-v1', phrase, ness_path: NESS_PATH, emc_path: EMC_PATH};
    return wallet;
  } finally { seed.fill(0); secret.fill(0); child.wipePrivateData(); root.wipePrivateData(); }
}
function base58Check(body) {
  const bytes = new Uint8Array(body.length + 4);
  bytes.set(body); bytes.set(sha256(sha256(body)).subarray(0, 4), body.length);
  return base58(bytes);
}
export function emercoinWallet(wallet) {
  if (!wallet.recovery) throw new Error('Restore your 12 words or create a new wallet to add Emercoin.');
  const seed = mnemonicToSeedSync(wallet.recovery.phrase), root = HDKey.fromMasterSeed(seed);
  const child = root.derive(EMC_PATH), secret = child.privateKey;
  try {
    const publicKey = secp256k1.getPublicKey(secret);
    return {address: base58Check(Uint8Array.of(33, ...ripemd160(sha256(publicKey)))),
      wif: base58Check(Uint8Array.of(128, ...secret, 1)), path: EMC_PATH};
  } finally { seed.fill(0); secret.fill(0); child.wipePrivateData(); root.wipePrivateData(); }
}
export function readWallet(text) {
  if (typeof text !== 'string' || text.length > 65536) throw new Error('Choose a wallet file saved by this page.');
  const wallet = JSON.parse(text);
  if (wallet?.meta?.type !== 'collection' || wallet.meta.encrypted !== 'false' ||
      wallet.meta.coin !== 'skycoin' || wallet.entries?.length !== 1) {
    throw new Error('Choose the wallet file saved by this page, or use a receiving address from your wallet app.');
  }
  const entry = wallet.entries[0];
  if (!/^[a-f0-9]{64}$/.test(entry.secret_key)) throw new Error('This wallet file is incomplete.');
  const verified = walletFromSecret(entry.secret_key);
  if (entry.address !== verified.entries[0].address || entry.public_key !== verified.entries[0].public_key) {
    throw new Error('The wallet file does not match its receiving address.');
  }
  if (wallet.recovery) {
    if (wallet.recovery.format !== 'privateness-market-v1' || wallet.recovery.ness_path !== NESS_PATH || wallet.recovery.emc_path !== EMC_PATH) throw new Error('Unrecognized wallet recovery format.');
    const restored = walletFromPhrase(wallet.recovery.phrase);
    if (restored.entries[0].address !== entry.address) throw new Error('The recovery words do not match this wallet.');
    return restored;
  }
  return verified;
}

function decode58(value) {
  let n = 0n;
  for (const c of value) { const v = alphabet.indexOf(c); if (v < 0) throw new Error('Invalid address.'); n = n * 58n + BigInt(v); }
  let hex = n.toString(16); if (hex.length % 2) hex = '0' + hex;
  return Uint8Array.of(...new Uint8Array(value.match(/^1*/)[0].length), ...hexToBytes(hex));
}
const equal = (a,b) => a.length === b.length && a.every((v,i) => v === b[i]);
function units(value) {
  if (typeof value !== 'string' || !/^\d+(\.\d{1,6})?$/.test(value)) throw new Error('Invalid payment amount.');
  const [whole, fraction = ''] = value.split('.');
  return BigInt(whole) * 1000000n + BigInt(fraction.padEnd(6,'0'));
}
function integer(value) {
  if (typeof value !== 'string' || !/^\d+$/.test(value) || BigInt(value) > 18446744073709551615n) throw new Error('Invalid coin hours.');
  return BigInt(value);
}
// Validate the complete wire transaction against the visitor's reviewed request
// before any signature is made. No caller-supplied arbitrary signing operation.
export function signNVSPayment(wallet, template, expected) {
  const entry = wallet.entries[0], own = entry.address;
  if (!validAddress(expected.address) || expected.address === own) throw new Error('Invalid payment destination.');
  const hours = integer(expected.hours), bytes = hexToBytes(template.encoded_transaction);
  if (bytes.length < 49 || bytes.length > 7500) throw new Error('Invalid transaction size.');
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (view.getUint32(0,true) !== bytes.length || bytes[4] !== 0) throw new Error('Invalid transaction header.');
  const count = view.getUint32(37,true), innerOffset = 41 + count * 65;
  if (count < 1 || count > 64 || innerOffset + 4 > bytes.length || bytes.slice(41,innerOffset).some(v => v !== 0)) throw new Error('Expected an unsigned payment.');
  if (!equal(bytes.slice(5,37),sha256(bytes.slice(innerOffset)))) throw new Error('Invalid payment hash.');
  let pos = innerOffset;
  const ni = view.getUint32(pos,true); pos += 4;
  if (ni !== count || template.transaction.inputs.length !== ni || pos + ni*32 + 4 > bytes.length) throw new Error('Invalid payment inputs.');
  let inCoins=0n, inHours=0n; const seen = new Set(), inputs = [];
  for (let i=0;i<ni;i++) {
    const hash=bytes.slice(pos,pos+32); pos+=32;
    const input=template.transaction.inputs[i], id=bytesToHex(hash);
    if (seen.has(id) || id !== input.uxid || input.address !== own) throw new Error('Payment input does not belong to this wallet.');
    seen.add(id); inputs.push(hash); inCoins+=units(input.coins); inHours+=integer(input.calculated_hours);
  }
  const no=view.getUint32(pos,true); pos+=4;
  if (no<1 || no>2 || pos+no*37!==bytes.length) throw new Error('Unexpected payment outputs.');
  let outCoins=0n,outHours=0n,paidCoins=0n,paidHours=0n;
  for(let i=0;i<no;i++) {
    const raw=bytes.slice(pos,pos+21), coins=view.getBigUint64(pos+21,true), h=view.getBigUint64(pos+29,true); pos+=37;
    if(coins===0n) throw new Error('Invalid empty payment output.');
    const matches=address=>{const decoded=decode58(address);return raw[0]===decoded[20] && equal(raw.slice(1),decoded.slice(0,20));};
    if(matches(expected.address)){paidCoins+=coins;paidHours+=h;}
    else if(!matches(own)) throw new Error('Payment would send change to another wallet.');
    outCoins+=coins;outHours+=h;
  }
  if(paidCoins!==1n || paidHours!==hours || inCoins!==outCoins || inHours-outHours!==(inHours+19n)/20n || inHours===0n) throw new Error('Payment does not match the reviewed amount.');
  const key=hexToBytes(entry.secret_key);
  try {
    for(let i=0;i<ni;i++) {
      const hash=sha256(Uint8Array.of(...bytes.slice(5,37),...inputs[i]));
      const sig=secp256k1.sign(hash,key,{prehash:false,lowS:true,format:'recovered'});
      bytes.set(sig.subarray(1),41+i*65);bytes[41+i*65+64]=sig[0];
    }
    return {rawtx:bytesToHex(bytes),txid:bytesToHex(sha256(bytes))};
  } finally {key.fill(0);}
}
