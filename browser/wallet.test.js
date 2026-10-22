import test from 'node:test';
import assert from 'node:assert/strict';
import { createECDH, createHash, randomBytes } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { newWallet, walletFromSecret, walletFromPhrase, emercoinWallet, signNVSPayment, readWallet, validAddress } from './wallet-source.js';
const fixture=JSON.parse(readFileSync(new URL('../testdata/browser-payment-native.json',import.meta.url),'utf8').replace(/^\uFEFF/,''));

test('matches known Skycoin-family address and OpenSSL public key', () => {
  const secret = '7e4c9fbbbbb21be96fd496653b81036bab6e599a10557c790d1aa4d29c461ab9';
  const wallet = walletFromSecret(secret);
  assert.equal(wallet.entries[0].address, '2VLXYmAWYnX5MMCfRwAvt46YyEk2X7TvrS9');
  assert.equal(wallet.entries[0].public_key, '03a4e90c34b9f359364b01f2136f1ff850d648d56da2f4b4aed6cbb6fede346831');
});

test('12-word restoration matches native NESS derivation and Emercoin Core address/key encoding',()=>{
  const wallet=walletFromPhrase(fixture.phrase);
  assert.equal(wallet.entries[0].address,fixture.address);
  assert.equal(wallet.recovery.phrase.split(' ').length,12);
  assert.deepEqual(walletFromPhrase(newWallet().recovery.phrase).entries.length,1);
  const emc=emercoinWallet(wallet);
  const alphabet='123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  const decode=text=>{let n=0n;for(const c of text)n=n*58n+BigInt(alphabet.indexOf(c));let h=n.toString(16);return Buffer.from(h.length%2?'0'+h:h,'hex');};
  const hash=(alg,x)=>createHash(alg).update(x).digest();
  const ec=createECDH('secp256k1');ec.setPrivateKey(Buffer.from(fixture.emc_test_secret,'hex'));
  const addr=decode(emc.address),wif=decode(emc.wif);
  assert.equal(addr[0],33);assert.equal(wif[0],128);assert.equal(wif[33],1);
  assert.deepEqual(wif.subarray(1,33),Buffer.from(fixture.emc_test_secret,'hex'));
  assert.deepEqual(addr.subarray(1,21),hash('ripemd160',hash('sha256',ec.getPublicKey(null,'compressed'))));
  for(const b of [addr,wif])assert.deepEqual(b.subarray(-4),hash('sha256',hash('sha256',b.subarray(0,-4))).subarray(0,4));
  assert.throws(()=>walletFromPhrase('abandon '.repeat(12)));
  const altered=structuredClone(wallet);altered.recovery.phrase=newWallet().recovery.phrase;
  assert.throws(()=>readWallet(JSON.stringify(altered)));
});

test('native unsigned payment signs; wrong amount, destination, input owner, hash and signature state are rejected',()=>{
  const wallet=walletFromPhrase(fixture.phrase), expected={address:fixture.template.transaction.outputs[0].address,hours:'1'};
  const signed=signNVSPayment(wallet,fixture.template,expected);
  assert.equal(signed.txid.length,64);
  assert.throws(()=>signNVSPayment(wallet,fixture.template,{...expected,hours:'2'}));
  assert.throws(()=>signNVSPayment(wallet,fixture.template,{...expected,address:fixture.address}));
  const forged=structuredClone(fixture.template);forged.transaction.inputs[0].address=expected.address;
  assert.throws(()=>signNVSPayment(wallet,forged,expected));
  const changed=structuredClone(fixture.template);changed.encoded_transaction=changed.encoded_transaction.slice(0,-2)+'01';
  assert.throws(()=>signNVSPayment(wallet,changed,expected));
  const alreadySigned={...fixture.template,encoded_transaction:signed.rawtx};
  assert.throws(()=>signNVSPayment(wallet,alreadySigned,expected));
});
test('100 keys match independent OpenSSL curve and address hashes', () => {
  const alphabet = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  const hash = (algorithm, bytes) => createHash(algorithm).update(bytes).digest();
  for (let i = 0; i < 100; i++) {
    const key = randomBytes(32), ec = createECDH('secp256k1'); ec.setPrivateKey(key);
    const pub = ec.getPublicKey(null, 'compressed');
    const body = Buffer.concat([hash('ripemd160', hash('sha256', hash('sha256', pub))), Buffer.from([0])]);
    const raw = Buffer.concat([body, hash('sha256', body).subarray(0, 4)]);
    let n = BigInt('0x' + raw.toString('hex')), expected = '';
    while (n) { expected = alphabet[Number(n % 58n)] + expected; n /= 58n; }
    for (const byte of raw) { if (byte) break; expected = '1' + expected; }
    const entry = walletFromSecret(key).entries[0];
    assert.equal(entry.public_key, pub.toString('hex')); assert.equal(entry.address, expected);
    assert.ok(validAddress(expected));
  }
});
test('saved wallet reopens; altered address or key and wrong format are rejected', () => {
  const wallet = newWallet();
  assert.deepEqual(readWallet(JSON.stringify(wallet)).entries, wallet.entries);
  const altered = structuredClone(wallet); altered.entries[0].address = 'wrong';
  assert.throws(() => readWallet(JSON.stringify(altered)));
  altered.entries[0] = {...wallet.entries[0], secret_key: '00'.repeat(32)};
  assert.throws(() => readWallet(JSON.stringify(altered)));
  assert.throws(() => readWallet('{"meta":{"type":"deterministic"}}'));
  assert.equal(validAddress('not-an-address'), false);
  const address = wallet.entries[0].address;
  assert.equal(validAddress(address.slice(0, -1) + (address.endsWith('1') ? '2' : '1')), false);
});
