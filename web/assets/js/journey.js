'use strict';
(() => {
  const $ = id => document.getElementById(id);
  window.PortalJourney = {
    show() { $('forge-stage').scrollIntoView({behavior: 'auto', block: 'center'}); },
    begin() {
      $('count').textContent = '0';
      $('phase-detail').textContent = 'Your access is created on this device. Save your backup as your claim is prepared.';
      $('progress').value = 0;
    },
    update(remaining) {
      if(remaining <= 0) $('phase-detail').textContent = 'Collecting your claim…';
    },
    confirm() { $('phase-detail').textContent = 'Your claim is ready. Save your backup, then continue to your balance.'; $('progress').value = 8; $('op-claim').textContent='Your claim is ready'; $('op-claim').classList.add('complete'); },
    fail() { $('count').textContent = '—'; $('phase-detail').textContent = 'Read the message below, then try again.'; }
  };
})();
