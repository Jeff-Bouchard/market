// LOCAL TEST ONLY. Synthetic balances and records, no chain or funds.
import http from 'node:http';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
const f=JSON.parse(readFileSync(new URL('../testdata/browser-payment-native.json',import.meta.url),'utf8').replace(/^\uFEFF/,''));
const destination=f.template.transaction.outputs[0].address;
const emc='EKH9ShrFW6hcuCmsiuCFAtQ9da8U5QUmGZ';
let submitted=false, record=null;
http.createServer(async(req,res)=>{
 let body='';for await(const chunk of req)body+=chunk;
 const data=body&&req.headers['content-type']?.includes('json')?JSON.parse(body):{};
 const url=new URL(req.url,'http://127.0.0.1');
 const send=value=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(value));};
 if(url.pathname==='/emc' || url.pathname.startsWith('/emc/wallet/')) {
  let result;
  if(data.method==='listwallets')result=[''];
  else if(data.method==='getwalletinfo')result={private_keys_enabled:true};
  else if(data.method==='getnewaddress')result=emc;
  else if(data.method==='getreceivedbyaddress')result='0';
  else if(data.method==='name_show'){
   if(!record){send({result:null,error:{code:-4,message:'failed to read from name DB'}});return;}
   result=record;
  } else if(data.method==='name_new'){
   record={name:data.params[0],value:data.params[1],address:data.params[3]||emc,txid:'ab'.repeat(32),expires_in:17500};result=record.txid;
  } else {res.statusCode=500;send({error:{code:-32601,message:'Unexpected fixture method'}});return;}
  send({result,error:null});return;
 }
 switch(url.pathname){
 case '/api/v1/health':send({wallet_api_enabled:true,blockchain_pubkey:'02933015bd2fa1e0a885c05fb08eb7c647bf8c3188ed5120b51d0d09ccaf525036',fiber:{ticker:'NESS',coin_hours_ticker:'NCH'},user_verify_transaction:{burn_factor:20,max_decimals:6},unconfirmed_verify_transaction:{burn_factor:20,max_decimals:6}});break;
 case '/api/v1/blocks':send({blocks:[{header:{seq:0,timestamp:1650046010},body:{txns:[{outputs:[{dst:'24GJTLPMoz61sV4J4qg1n14x5qqDwXqyJJy',coins:'165000000.000000'}]}]}}]});break;
 case '/api/v1/csrf':send({csrf_token:'local-test'});break;
 case '/api/v1/wallet/newAddress':send({addresses:[destination]});break;
 case '/api/v2/transaction':
  if(data.addresses?.[0]!==f.address){res.statusCode=400;send({error:'Public test wallet required'});break;}
  send({data:f.template});break;
 case '/api/v1/injectTransaction':submitted=true;send(createHash('sha256').update(Buffer.from(data.rawtx,'hex')).digest('hex'));break;
 case '/api/v1/balance':send({confirmed:{coins:submitted?1:0,hours:submitted?1:0},predicted:{coins:submitted?1:0,hours:submitted?1:0}});break;
 default:res.statusCode=404;send({error:'Not in local test fixture'});
 }
}).listen(18887,'127.0.0.1',()=>console.log('Synthetic RPC test fixture on loopback:18887; no real funds.'));
