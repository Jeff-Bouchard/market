// Public fixture only, no live wallet or funds.
import { readFileSync, writeFileSync } from 'node:fs';
import { walletFromPhrase, signNVSPayment } from './wallet-source.js';
const f=JSON.parse(readFileSync(new URL('../testdata/browser-payment-native.json',import.meta.url),'utf8').replace(/^\uFEFF/,''));
const signed=signNVSPayment(walletFromPhrase(f.phrase),f.template,{address:f.template.transaction.outputs[0].address,hours:'1'});
writeFileSync(new URL('../testdata/browser-payment-signed.json',import.meta.url),JSON.stringify(signed)+'\n');
