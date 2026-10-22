'use strict';
(() => {
  const $ = id => document.getElementById(id);
  let wallet = null, emc = null, saved = false, busy = false;
  const keys = import('./wallet-keys.js');
  const message = text => { $('wallet-status').textContent = text; };
  function operation(id, text) { $(id).textContent = text; $(id).classList.add('complete'); }
  async function addEmercoin() {
    if (!wallet?.recovery) throw new Error('Set up your access here or restore your 12 recovery words first.');
    emc = (await keys).emercoinWallet(wallet);
    UISetOwner(emc.address);
    $('emc-wallet-address').textContent = 'Emercoin: ' + emc.address;
    $('emc-wallet-address').hidden = $('emc-wallet-export').hidden = $('emc-wallet-help').hidden = false;
    operation('op-emc', 'Your record access is ready');
    $('nvs-wallet-status').textContent = 'You will control this record using your saved access.';
    $('nvs-create-emc').hidden = true;
  }
  function UISetOwner(address) { window.PrivatenessUI.setValue($('nvs-owner'), address); }
  function download(text, filename) {
    const url = URL.createObjectURL(new Blob([text], {type:'application/json'}));
    const link = document.createElement('a'); link.href=url; link.download=filename;
    document.body.append(link); link.click(); link.remove(); setTimeout(()=>URL.revokeObjectURL(url),30000);
  }
  function selectAddress(address) {
    window.PrivatenessUI.setValue($('cash-address'), address);
    $('cash-address').dispatchEvent(new Event('input', {bubbles: true}));
    $('wallet-selected').textContent = address;
    $('wallet-selected-row').hidden = false;
    $('faucet-claim').hidden = false;
    window.dispatchEvent(new Event('ness-wallet-ready'));
  }
  function showWallet() {
    $('wallet-create').hidden = true;
    $('wallet-options').hidden = true;
    $('wallet-heading').textContent = saved ? 'Your access is ready.' : 'Keep your access safe.';
    $('wallet-intro').textContent = saved ? 'Your backup lets you return on this device or another one.' : 'Save your backup before you continue.';
    $('wallet-backup').hidden = false;
    $('wallet-address').textContent = wallet.entries[0].address;
    $('wallet-phrase-box').hidden = !wallet.recovery;
    $('wallet-phrase').textContent = wallet.recovery?.phrase || '';
    $('wallet-phrase-details').open = false;
    operation('op-wallet', saved ? 'Your access is saved' : 'Your access is created · save your backup');
    $('wallet-save').textContent = saved ? 'Download backup again' : 'Save my backup';
    if (saved) selectAddress(wallet.entries[0].address);
  }
  $('wallet-create').addEventListener('click', async () => {
    if (busy || wallet) return;
    busy = true; $('wallet-create').disabled = true;
    $('faucet-claim').hidden = false;
    window.dispatchEvent(new Event('ness-wallet-creating'));
    message('Creating your access on this device…');
    try {
      if (!window.isSecureContext || !crypto?.getRandomValues) throw new Error('Open the secure HTTPS version of this site to get started.');
      wallet = (await keys).newWallet(); saved = false;
      showWallet();
      if ($('wallet-add-emc').checked) await addEmercoin();
      message('Ready. Save your backup and keep your recovery words private.');
      $('wallet-save').focus();
    } catch (error) { message(error.message || 'We couldn’t set up your access. Please try again.'); }
    finally { busy = false; $('wallet-create').disabled = false; }
  });
  $('wallet-save').addEventListener('click', () => {
    if (!wallet) return;
    download(JSON.stringify(wallet,null,2)+'\n',wallet.meta.filename);
    // A download request does not prove that a file was saved. Ask for the
    // visitor's acknowledgement before selecting this new receiving address.
    $('wallet-saved').hidden = false;
    message('Check that your backup has downloaded before continuing. Keep it private: it gives access to your balance.');
    $('wallet-saved').focus();
  });
  $('wallet-saved').addEventListener('click', () => {
    if (!wallet) return;
    saved = true; $('wallet-saved').hidden = true; showWallet();
    message('Backup saved. Continue below to collect your balance.');
    if ($('cash-token').value.trim()) location.hash = 'cashout';
    else ($('use-token').disabled ? $('wallet-status') : $('use-token')).focus();
  });
  $('wallet-use-existing').addEventListener('click', async () => {
    const address = $('wallet-existing-address').value.trim();
    if (!(await keys).validAddress(address)) {
      $('wallet-existing-address').setCustomValidity('Enter a valid NESS receiving address.');
      $('wallet-existing-address').reportValidity(); return;
    }
    selectAddress(address); message('Your receiving address is selected.');
    if ($('cash-token').value.trim()) location.hash = 'cashout';
    else $('start').focus();
  });
  $('wallet-existing-address').addEventListener('input', () => $('wallet-existing-address').setCustomValidity(''));
  $('wallet-restore').addEventListener('click', async () => {
    if (busy || (wallet && !saved)) { message('Save your current backup before restoring another one.'); return; }
    busy=true;
    try {
      wallet=(await keys).walletFromPhrase($('wallet-recovery').value); saved=true;
      $('wallet-recovery').value=''; showWallet(); await addEmercoin();
      message('Your balance and record access are restored on this device.');
    } catch(error) {message(error.message);} finally {busy=false;}
  });
  $('nvs-create-emc').addEventListener('click',async()=>{
    try {await addEmercoin();} catch(error) {$('nvs-wallet-status').textContent=error.message;window.NessReceivingWallet.focus();}
  });
  $('nvs-owner').addEventListener('input',()=>{
    const owner=$('nvs-owner').value.trim();
    $('nvs-wallet-status').textContent=owner ? (owner===emc?.address ? 'You will control this record using your saved access.' : 'The owner of the address you entered will control this record.') : 'The service will hold this record. Set up your own control if you want to hold it yourself.';
    $('nvs-create-emc').hidden = Boolean(owner && owner===emc?.address);
  });
  $('wallet-add-emc').addEventListener('change',async()=>{
    if(wallet && $('wallet-add-emc').checked) {try{await addEmercoin();}catch(error){message(error.message);}}
  });
  $('emc-wallet-export').addEventListener('click',()=>{
    if(emc) download(JSON.stringify({format:'emercoin-private-key-export',address:emc.address,wif:emc.wif,derivation:emc.path,note:'Private key for importprivkey in Emercoin Core. Keep private.'},null,2)+'\n','emercoin-'+emc.address+'.json');
  });
  $('wallet-open').addEventListener('change', async event => {
    const file = event.target.files[0]; if (!file || busy) return;
    if(wallet && !saved){message('Save your current backup before opening another one.');event.target.value='';return;}
    busy = true;
    try {
      if (file.size > 65536) throw new Error('Choose the backup file saved by this page.');
      wallet = (await keys).readWallet(await file.text()); saved = true;
      emc=null; UISetOwner('');
      $('emc-wallet-address').hidden=$('emc-wallet-export').hidden=$('emc-wallet-help').hidden=true;
      $('op-emc').textContent='Set up control of your records';$('op-emc').classList.remove('complete');
      showWallet(); if(wallet.recovery) await addEmercoin(); message('Your saved access is restored.');
      if ($('cash-token').value.trim()) location.hash = 'cashout';
    } catch (error) { message(error instanceof SyntaxError ? 'This is not a backup file saved by this page.' : error.message); }
    finally { busy = false; event.target.value = ''; }
  });
  $('cash-create-wallet').addEventListener('click', () => {
    location.hash = 'faucet';
    $('wallet-setup').scrollIntoView({block: 'start'});
    (wallet ? $('wallet-save') : $('wallet-create')).focus();
  });
  window.NessReceivingWallet = Object.freeze({
    signingAddress: () => saved && wallet && wallet.entries[0].address === $('cash-address').value.trim() ? wallet.entries[0].address : null,
    signPayment: async (template, expected) => {
      if (!saved || !wallet || wallet.entries[0].address !== $('cash-address').value.trim()) throw new Error('Restore your wallet before signing.');
      return (await keys).signNVSPayment(wallet,template,expected);
    },
    ready: () => Boolean($('cash-address').value.trim()) &&
      (!wallet || $('cash-address').value.trim() !== wallet.entries[0].address || saved),
    focus: () => { location.hash = 'faucet'; $('wallet-setup').scrollIntoView({block: 'start'}); (wallet ? $('wallet-save') : $('wallet-create')).focus(); }
  });
  window.addEventListener('pagehide', () => { wallet = null; emc=null; saved = false; $('wallet-phrase').textContent=''; $('wallet-recovery').value=''; });
})();
