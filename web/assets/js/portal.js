'use strict';
(() => {
  const $ = id => document.getElementById(id);
  const UI = window.PrivatenessUI;
  const faucetApi = '/api/faucet';
  const exchangeApi = document.body.dataset.exchangeApi;
  const csrf = new Map();
  let faucetConfigured = false;
  fetch(faucetApi, {credentials: 'same-origin', cache: 'no-store'})
    .then(async response => {
      if (!response.ok) throw new Error('Service unavailable');
      const session = await response.json();
      csrf.set(faucetApi, session.csrf);
      faucetConfigured = session.faucet_configured === true;
      const unavailable = [];
      if (!faucetConfigured) unavailable.push('Preview: you can set up and save your access. Receiving a balance is not connected yet.');
      if (!session.nvs_configured) unavailable.push('Record registration is not available yet.');
      $('service-availability').textContent = unavailable.join(' ');
      $('service-availability').hidden = unavailable.length === 0;
    })
    .catch(() => {
      $('service-availability').hidden = false;
      $('service-availability').textContent = 'The service could not be reached. You can still set up and save your access.';
    });
  let claimId = '', token = '', running = false, remaining = 8000, lastTick = 0, claimEpoch = 0;
  function status(id, message, error = false) {
    const box = $(id); box.replaceChildren(); box.classList.toggle('error', error);
    if (!(message instanceof Error)) { UI.text(box, message); return; }
    const info = message.detail;
    const title = document.createElement('strong'); title.textContent = info?.title || 'We could not complete that step'; box.append(title);
    const explanation = document.createElement('p'); explanation.textContent = info?.message || message.message; box.append(explanation);
    const next = document.createElement('p'); next.textContent = info?.next_step || 'Keep your claim code or request reference. If you were sending a payment, ask the operator to check its result before retrying.'; box.append(next);
    const details = document.createElement('details'), summary = document.createElement('summary'), text = document.createElement('p');
    summary.textContent = 'Details to share with the operator';
    text.textContent = [info?.code || 'CONNECTION_INTERRUPTED', message.httpStatus ? `HTTP ${message.httpStatus}` : '', message.reference ? `Reference: ${message.reference}` : ''].filter(Boolean).join(' · ');
    details.append(summary,text); box.append(details);
  }
  function responseError(result, response) {
    const error = new Error(result.error || 'The service could not complete this step.');
    error.detail = result.status_detail; error.httpStatus = response.status; error.reference = result.reference; return error;
  }
  async function api(url, data) {
    if (!csrf.has(url)) {
      const response = await fetch(url, {credentials: 'same-origin', cache: 'no-store'});
      const session = await response.json();
      if (!response.ok || !session.csrf) throw responseError(session, response);
      csrf.set(url, session.csrf);
    }
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 90000);
    let response;
    try {
      response = await fetch(url, {method: 'POST', credentials: 'same-origin', cache: 'no-store', signal: controller.signal,
        headers: {'Content-Type': 'application/json', 'X-CSRF-Token': csrf.get(url)}, body: JSON.stringify(data)});
    } catch (error) {
      throw new Error((data.action === 'withdraw' || data.process === true) ? 'The payment result is unknown. Check with the operator before retrying.' : 'Connection interrupted. Please try again.');
    } finally { clearTimeout(timeout); }
    const result = await response.json();
    if (response.status === 403) csrf.delete(url);
    if (response.status === 425) return result;
    if (!response.ok) throw responseError(result, response);
    return result;
  }
  function navigate() {
    const name = location.hash.slice(1) || 'faucet';
    const active = ['faucet', 'cashout', 'exchange', 'nvs'].includes(name) ? name : 'faucet';
    document.querySelectorAll('.panel').forEach(panel => { panel.hidden = panel.id !== active; });
    document.querySelectorAll('[data-panel]').forEach(link => {
      if (link.dataset.panel === (active === 'cashout' ? 'faucet' : active)) link.setAttribute('aria-current', 'page');
      else link.removeAttribute('aria-current');
    });
    if (active === 'cashout') {
      const ready = window.NessReceivingWallet.ready();
      $('cash-details').open = !$('cash-token').value.trim() || !ready;
      $('cash-create-wallet').hidden = ready;
      $('cash-destination').textContent = ready ? 'Your selected access will receive this balance.' : 'Restore your access or choose a receiving address below.';
    }
    lastTick = performance.now();
  }
  window.addEventListener('hashchange', () => {
    navigate();
    const heading = document.querySelector('.panel:not([hidden]) h1');
    if (heading) { heading.tabIndex = -1; heading.focus(); }
  });
  document.addEventListener('visibilitychange', () => { lastTick = performance.now(); });
  navigate();
  async function start(creating = false) {
    if (running) return;
    if (!creating && !window.NessReceivingWallet.ready()) { window.NessReceivingWallet.focus(); return; }
    running = true;
    const epoch = ++claimEpoch;
    $('cash-token').value = '';
    clearQuote();
    $('start').disabled = true;
    $('count-label').textContent = 'seconds';
    window.PortalJourney.begin();
    $('new-claim').hidden = true;
    $('copy').disabled = $('use-token').disabled = true;
    $('new-token').disabled = true;
    $('new-token').value = token = '';
    try {
      const result = await api(faucetApi, {action: 'start'});
      if (epoch !== claimEpoch) return;
      claimId = result.claim_id;
      remaining = Math.max(8000, result.remaining_ms);
      lastTick = performance.now();
      status('claim-status', 'Your claim is being prepared. Save your private backup above.');
      $('start').hidden = true;
      if (!creating) window.PortalJourney.show();
      tick(epoch);
    } catch (error) { if (epoch === claimEpoch) claimError(error); }
  }
  function claimError(error) {
    running = false;
    window.PortalJourney.fail();
    $('start').hidden = false;
    $('start').disabled = false;
    $('start').textContent = 'Retry claim';
    status('claim-status', error, true);
  }
  async function tick(epoch) {
    if (!running || epoch !== claimEpoch) return;
    const now = performance.now();
    if (!document.hidden && !$('faucet').hidden) remaining = Math.max(0, remaining - (now - lastTick));
    lastTick = now;
    $('count').textContent = String(Math.floor((8000 - remaining) / 1000));
    $('progress').value = 8 - remaining / 1000;
    window.PortalJourney.update(remaining);
    if (remaining > 0) { setTimeout(() => tick(epoch), 100); return; }
    status('claim-status', 'Preparing your claim…');
    try {
      const result = await api(faucetApi, {action: 'release', claim_id: claimId});
      if (epoch !== claimEpoch) return;
      if (!result.token) {
        remaining = Math.max(100, result.remaining_ms);
        lastTick = performance.now();
        setTimeout(() => tick(epoch), 100);
        return;
      }
      token = result.token;
      // The real token is offered only after server release.

      $('cash-token').placeholder = 'Paste your saved claim code';
      $('new-token').disabled = false;
      $('new-token').value = token;
      $('copy').disabled = $('use-token').disabled = false;
      $('new-claim').hidden = false;
      window.PortalJourney.confirm();
      $('count').textContent = '✓';
      $('count-label').textContent = 'ready to use';
      status('claim-status', 'Ready. Continue to see your starting balance.');
      running = false;
    } catch (error) { if (epoch === claimEpoch) claimError(error); }
  }
  function discardBrowserTokens() {
    claimEpoch++;
    running = false;
    claimId = token = '';
    $('new-token').value = $('cash-token').value = '';
    $('new-token').disabled = true;
    $('copy').disabled = $('use-token').disabled = true;
    clearQuote();
  }
  // Do not restore a released token or a completed timer from browser history.
  window.addEventListener('pagehide', discardBrowserTokens);
  window.addEventListener('pageshow', event => {
    if (event.persisted) { discardBrowserTokens(); location.reload(); }
  });
  discardBrowserTokens();
  window.addEventListener('ness-wallet-creating', () => start(true));
  $('start').addEventListener('click', () => start());
  $('new-claim').addEventListener('click', () => start());
  $('copy').addEventListener('click', async () => {
    if (!token) return;
    try {
      if (!navigator.clipboard) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(token);
      status('claim-status', 'Copied. Keep the claim code somewhere private.');
    } catch (_) {
      $('new-token').focus(); $('new-token').select();
      status('claim-status', 'Claim code selected. Use your browser’s Copy command.');
    }
  });
  $('use-token').addEventListener('click', () => {
    if (!token) return;
    if (!window.NessReceivingWallet.ready()) { window.NessReceivingWallet.focus(); return; }
    UI.setValue($('cash-token'), token);
    clearQuote();
    location.hash = 'cashout';
    $('quote').click();
  });
  function clearQuote() { $('withdraw').disabled = true; $('quote-result').hidden = true; $('cashout').querySelectorAll('.steps li').forEach((item,index) => { if(index===1)item.setAttribute('aria-current','step');else item.removeAttribute('aria-current'); }); }
  $('cash-token').addEventListener('input', clearQuote);
  $('cash-address').addEventListener('input', clearQuote);
  $('quote').addEventListener('click', async () => {
    if (!$('cash-token').reportValidity()) return;
    $('quote').disabled = true;
    $('withdraw').disabled = true;
    const checkedToken = $('cash-token').value.trim();
    status('cash-status', 'Checking your claim…');
    try {
      const result = await api(faucetApi, {action: 'quote', token: checkedToken});
      if ($('cash-token').value.trim() !== checkedToken) return;
      const amount=document.createElement('strong'); amount.textContent=`${result.quote.nch} hours to use`;
      const explanation=document.createElement('p'); explanation.className='small'; explanation.textContent='Use hours to register records. You’ll see the cost before confirming.';
      const details=document.createElement('details'), label=document.createElement('summary'), exact=document.createElement('p');
      label.textContent='Exact balance'; exact.textContent=`${result.quote.ness} NESS + ${result.quote.nch} NCH`;details.append(label,exact);
      $('quote-result').replaceChildren(amount,explanation,details);
      $('quote-result').hidden = false;
      $('withdraw').disabled = !faucetConfigured || !window.NessReceivingWallet.ready();
      status('cash-status', faucetConfigured ? 'Choose Receive my balance to collect. You can check the receiving details above.' : 'Receiving is not connected in this preview. Your balance has not been sent.');
      $('cashout').querySelectorAll('.steps li').forEach((item,index) => { if(index===2)item.setAttribute('aria-current','step');else item.removeAttribute('aria-current'); });
    } catch (error) { status('cash-status', error, true); }
    finally { $('quote').disabled = false; }
  });
  $('cashout-form').addEventListener('submit', async event => {
    event.preventDefault();
    if ($('withdraw').disabled || !window.NessReceivingWallet.ready() || !UI.validate(event.target)) return;
    $('withdraw').disabled = true;
    $('quote').disabled = true;
    const address = $('cash-address').value.trim();
    status('cash-status', 'Sending your starting balance…');
    try {
      const result = await api(faucetApi, {action: 'withdraw', token: $('cash-token').value.trim(), address});
      if (result.redemption.state !== 'submitted') throw new Error('Payment needs operator review.');
      status('cash-status', `Your balance is on its way. You can prepare your record while it is confirmed. Reference: ${result.redemption.txid}`);
      $('op-receive').textContent='Balance sent · confirmation pending';
      $('continue-nvs').hidden=false;
      if (!$('pay-address').value || $('pay-address').value === $('pay-address').dataset.example) UI.setValue($('pay-address'), address);
    } catch (error) { status('cash-status', error, true); }
    finally { $('quote').disabled = false; }
  });
  const watchedNVS=new Set();
  function watchNVS(slotId) {
    if(watchedNVS.has(slotId))return;watchedNVS.add(slotId);
    let attempts=0, previous='';
    const check=async()=>{
      if(document.hidden || $('nvs').hidden){watchedNVS.delete(slotId);return;}
      try {
        const result=await api(exchangeApi,{action:'status',kind:'nvs',slot_id:slotId,process:true});
        const state=[result.status,result.browser_payment_state,result.txid].join(':');
        if(state!==previous){renderSlot('nvs',result);previous=state;}
        if(result.status==='done' || result.status==='review' || ++attempts>=120){watchedNVS.delete(slotId);return;}
        setTimeout(check,5000);
      }catch(error){watchedNVS.delete(slotId);status('nvs-status',error,true);}
    };
    setTimeout(check,5000);
  }
  function renderBrowserPayment(box, request) {
    const payment=request.payments.NCH;
    const summary=document.createElement('p');
    summary.textContent=`Payment: ${payment.min_sum} NCH + 0.000001 NESS from your wallet.`; box.append(summary);
    if(request.browser_payment_state) {
      const state=document.createElement('p');
      state.textContent=request.browser_payment_state==='submitted'
        ? `Your signed payment was submitted. Check its confirmation below. Reference: ${request.browser_payment_tx}`
        : `The payment result needs checking. Do not send another payment. Reference: ${request.browser_payment_tx}`;
      box.append(state); return;
    }
    const review=document.createElement('button'); review.textContent='Review and use my balance';
    const reviewBox=document.createElement('div');box.append(review,reviewBox);
    review.addEventListener('click',async()=>{
      const source=window.NessReceivingWallet.signingAddress();
      if(!source){status('nvs-status','Restore your 12 recovery words or open your backup to use your balance here.');window.NessReceivingWallet.focus();return;}
      review.disabled=true;reviewBox.replaceChildren();
      status('nvs-status','Checking your available balance…');
      try {
        const prepared=await api(exchangeApi,{action:'prepare_payment',kind:'nvs',slot_id:request.slot_id,address:source});
        if(prepared.browser_payment_state){renderSlot('nvs',prepared);return;}
        if(['slot_id','name','value','days','owner'].some(k=>(prepared[k]??'')!==(request[k]??'')) ||
          prepared.payments?.NCH?.addr!==payment.addr || String(prepared.payments?.NCH?.min_sum)!==String(payment.min_sum) || prepared.browser_source!==source) throw new Error('The request changed. Reload and review it before signing.');
        status('nvs-status','Your balance is available. Review the details and confirm below.');
        $('op-receive').textContent='Your balance is ready to use';$('op-receive').classList.add('complete');
        const detail=document.createElement('p');detail.textContent=`Register ${request.name} for ${request.days} days. Pay ${payment.min_sum} NCH + 0.000001 NESS.`;
        const confirm=document.createElement('button');confirm.textContent=`Confirm and pay ${payment.min_sum} NCH + 0.000001 NESS`;
        const cancel=document.createElement('button');cancel.className='secondary';cancel.textContent='Go back';
        reviewBox.append(detail,confirm,cancel);
        cancel.addEventListener('click',()=>{reviewBox.replaceChildren();review.disabled=false;});
        confirm.addEventListener('click',async()=>{
          confirm.disabled=true;cancel.disabled=true;
          let signed;
          try {
            signed=await window.NessReceivingWallet.signPayment(prepared.browser_template,{address:payment.addr,hours:String(payment.min_sum)});
            status('nvs-status',`Sending your approved payment. Reference: ${signed.txid}`);
            const result=await api(exchangeApi,{action:'submit_payment',kind:'nvs',slot_id:request.slot_id,rawtx:signed.rawtx});
            renderSlot('nvs',result);
            if(result.browser_payment_state==='submitted') {status('nvs-status','Payment sent. Checking its confirmation before registering your record…');watchNVS(request.slot_id);}
          } catch(error) {
            status('nvs-status',signed ? `Submission result is unknown. Keep transaction reference ${signed.txid}. Check this request before doing anything else.` : error,true);
            if(!signed){cancel.disabled=false;}
          }
        });
      } catch(error) {status('nvs-status',error,true);review.disabled=false;}
    });
  }
  function renderSlot(kind, result) {
    status(kind+'-status','');
    const box = $(kind + '-slot'); box.replaceChildren(); box.hidden = false;
    const direct = result.exchange_rate === 10000;
    const waiting = ['generated','updated'].includes(result.status);
    const info = result.status_detail || {code: result.status, title: 'Your request', message: 'Your request has been saved.', next_step: 'Keep the reference below and check its status before paying again.'};
    const heading = document.createElement('h2'); heading.textContent = info.title; box.append(heading);
    const card = document.createElement('div'); card.className = 'status-card'; card.setAttribute('role','status');
    for (const [text, cls] of [[info.message,''],[info.next_step,'next-step']]) { const p = document.createElement('p'); p.textContent = text; p.className = cls; card.append(p); } box.append(card);
    const addText = text => { const p = document.createElement('p'); p.textContent = text; box.append(p); };
    if (direct) addText(`Your quote: ${result.exchange_nch} NCH → ${result.exchange_ness} NESS. The rate is 10,000:1.`);
    if (result.name) addText(`Your record: ${result.name}`);
    if (kind === 'nvs') {
      addText(`Public information: ${result.value}`);
      addText(`Duration: ${result.days} days`);
      addText(result.owner ? 'The owner address below will control this record.' : 'The service will hold this record.');
      if (result.owner) {const details=document.createElement('details'), summary=document.createElement('summary'), owner=document.createElement('p'); summary.textContent='Owner address';owner.textContent=result.owner;details.append(summary,owner);box.append(details);}
    }
    if (waiting && direct) {
      addText(`Step 2 · In your NESS wallet, send exactly ${result.exchange_nch} NCH and 0.000001 NESS together to this deposit address. Use this address for this one request only.`);
      box.append(UI.wallet(result.gen_address));
      addText('Step 3 · Wait until your wallet shows the deposit as confirmed. Then choose Check my payment below. The service will submit your quoted NESS payout.');
    } else if (waiting && kind === 'nvs' && result.payments?.NCH) {
      renderBrowserPayment(box, result);
    } else if (waiting) {
      addText('Step 2 · Choose ONE currency. Pay only the amount and address shown for that choice.');
      const label=document.createElement('label'); label.textContent='Pay with';label.htmlFor=kind+'-currency';
      const choice=document.createElement('select');choice.id=kind+'-currency';
      const payment=document.createElement('div');payment.className='payment';
      for(const currency of Object.keys(result.payments || {})){const option=document.createElement('option');option.value=currency;option.textContent=currency==='NCH'?'Coin hours (NCH)':currency==='EMC'?'Emercoin (EMC)':'NESS';choice.append(option);}
      const showPayment=()=>{payment.replaceChildren();const currency=choice.value,info=result.payments?.[currency];if(!info)return;const amount=document.createElement('p');amount.textContent=`Send ${info.min_sum} ${currency}${currency==='NCH'?' together with 0.000001 NESS':''} to:`;payment.append(amount,UI.wallet(info.addr));};
      choice.addEventListener('change',showPayment);box.append(label,choice,payment);showPayment();
      addText('Step 3 · Wait for confirmation in your wallet, then choose Check my payment. Do not send another payment while waiting.');
    }
    if(kind==='nvs' && ['payed','done'].includes(result.status)) {
      $('op-record').textContent=result.status==='done'?'Your record is registered':'Record submitted · confirmation pending'; $('op-record').classList.toggle('complete',result.status==='done');
    }
    const details=document.createElement('details');details.className='support-details';const summary=document.createElement('summary');summary.textContent='Receipt and status details';details.append(summary);
    const list=document.createElement('dl');
    for(const [key,value] of Object.entries({'Request reference':result.slot_id,'Status code':info.code,'Payout destination':result.pay_address,'Transaction reference':result.txid,'Service explanation':result.error})){if(!value)continue;const term=document.createElement('dt'),valueEl=document.createElement('dd');term.textContent=key;valueEl.textContent=String(value);list.append(term,valueEl);}details.append(list);box.append(details);
    const actions=document.createElement('div');actions.className='slot-actions';
    if(waiting || result.status==='activated' || (kind==='nvs' && result.status==='payed')) {
      const refresh=document.createElement('button');refresh.textContent=result.status==='payed'?'Check my record':'Check my payment';
      refresh.addEventListener('click',async()=>{refresh.disabled=true;refresh.textContent='Checking the network…';try{renderSlot(kind,await api(exchangeApi,{action:'status',kind,slot_id:result.slot_id,process:true}));}catch(error){status(kind+'-status',error,true);refresh.disabled=false;refresh.textContent='Check my payment';}});actions.append(refresh);
    }
    if(direct && waiting){const cancel=document.createElement('button');cancel.className='secondary';cancel.textContent='Cancel · I have not sent a deposit';cancel.addEventListener('click',async()=>{cancel.disabled=true;try{renderSlot(kind,await api(exchangeApi,{action:'cancel',kind,slot_id:result.slot_id}));}catch(error){status(kind+'-status',error,true);cancel.disabled=false;}});actions.append(cancel);}
    if(['cancelled','submitted','payed','done'].includes(result.status)){const another=document.createElement('button');another.className='secondary';another.textContent=kind==='exchange'?'Start a different exchange':'Register another record';another.addEventListener('click',()=>{box.hidden=true;$(kind+'-form').hidden=false;status(kind+'-status','Start a new request below. Keep the receipt from your previous request.');$(kind).querySelectorAll('.steps li').forEach((item,index)=>{if(index===0)item.setAttribute('aria-current','step');else item.removeAttribute('aria-current');});$(kind+'-form').querySelector('input').focus();});actions.append(another);}
    box.append(actions);$(kind+'-form').hidden=true;heading.tabIndex=-1;if(!$(kind).hidden)heading.focus();
    // Status errors stay visible even when the completed request hides its form.
    box.before($(kind+'-status'));
    $(kind).querySelectorAll('.steps li').forEach((item,index)=>{if(index===(waiting?1:2))item.setAttribute('aria-current','step');else item.removeAttribute('aria-current');});
    try { sessionStorage.setItem('privateness-' + kind + '-slot', result.slot_id); } catch (_) {}
  }
  $('exchange-hours').addEventListener('input', () => {
    const amount = $('exchange-hours').value;
    if (!/^[1-9][0-9]*$/.test(amount)) { $('exchange-quote').textContent = 'Enter a positive whole NCH amount.'; return; }
    const droplets = BigInt(amount) * 100n;
    $('exchange-quote').textContent = `You receive ${droplets / 1000000n}.${String(droplets % 1000000n).padStart(6, '0')} NESS.`;
  });
  for (const kind of ['exchange', 'nvs']) {
    $(kind + '-form').addEventListener('submit', async event => {
      event.preventDefault();
      if (!UI.validate(event.target)) return;
      const button = event.target.querySelector('button[type=submit]'); button.disabled = true;
      const data = Object.fromEntries(new FormData(event.target));
      status(kind + '-status', 'Creating your request…');
      try { renderSlot(kind, await api(exchangeApi, {action: 'create', kind, ...data})); status(kind + '-status', kind==='nvs'?'Review your record and payment below.':'Request saved. Follow the payment instructions below.'); }
      catch (error) { status(kind + '-status', error, true); }
      finally { button.disabled = false; }
    });
    let saved;
    try { saved = sessionStorage.getItem('privateness-' + kind + '-slot'); } catch (_) {}
    if (saved) api(exchangeApi, {action: 'status', kind, slot_id: saved, process: false}).then(result => renderSlot(kind, result)).catch(error => status(kind + '-status', error, true));
  }
})();
