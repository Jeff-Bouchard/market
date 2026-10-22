'use strict';
(() => {
  const pattern = /(?<![A-Za-z0-9])[1-9A-HJ-NP-Za-km-z]{26,35}(?![A-Za-z0-9])/g;
  function qr(value) {
    const code = new PrivatenessQR(-1, 0); // Error correction M; automatic size.
    code.addData(value); code.make();
    const count = code.getModuleCount(), quiet = 4, size = count + quiet * 2;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', `0 0 ${size} ${size}`);
    svg.setAttribute('width', '148'); svg.setAttribute('height', '148');
    svg.setAttribute('role', 'img'); svg.setAttribute('aria-label', 'QR code for the adjacent address');
    svg.setAttribute('shape-rendering', 'crispEdges'); svg.classList.add('wallet-qr');
    const background = document.createElementNS(svg.namespaceURI, 'rect');
    background.setAttribute('width', String(size)); background.setAttribute('height', String(size));
    background.setAttribute('fill', '#ffffff'); svg.append(background);
    let path = '';
    for (let row = 0; row < count; row++) for (let col = 0; col < count; col++) {
      if (code.isDark(row, col)) path += `M${col + quiet},${row + quiet}h1v1h-1z`;
    }
    const ink = document.createElementNS(svg.namespaceURI, 'path');
    ink.setAttribute('d', path); ink.setAttribute('fill', '#000000'); svg.append(ink);
    return svg;
  }
  function wallet(address) {
    const value = String(address || '').trim();
    const pair = document.createElement('span'); pair.className = 'wallet-pair';
    // Prepare the QR before exposing any address text.
    const graphic = qr(value);
    const content = document.createElement('span'); content.className = 'wallet-details';
    const text = document.createElement('code'); text.textContent = value;
    const copy = document.createElement('button'); copy.type = 'button'; copy.className = 'wallet-copy secondary'; copy.textContent = 'Copy address';
    copy.addEventListener('click', async () => {
      try { await navigator.clipboard.writeText(value); copy.textContent = 'Copied'; }
      catch (_) {
        const range = document.createRange(); range.selectNodeContents(text);
        const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
        copy.textContent = 'Selected — use Copy';
      }
    });
    content.append(text, copy); pair.append(graphic, content);
    return pair;
  }
  function textWithWallets(target, text) {
    const fragment = document.createDocumentFragment(); let last = 0;
    for (const match of String(text).matchAll(pattern)) {
      fragment.append(document.createTextNode(String(text).slice(last, match.index)), wallet(match[0]));
      last = match.index + match[0].length;
    }
    fragment.append(document.createTextNode(String(text).slice(last)));
    target.replaceChildren(fragment);
  }
  function setValue(field, value) {
    field.value = value; field.classList.remove('example-preview');
    field.dispatchEvent(new Event('input', {bubbles: true}));
  }
  function setupField(field) {
    if (field.dataset.uiReady) return;
    field.dataset.uiReady = 'true';
    const example = field.dataset.example || '';
    if (field.dataset.previewDefault === 'true') field.value = '';
    if (!field.value) field.classList.add('example-preview');
    let hint = null, embedded = null;
    if (field.dataset.walletText) {
      embedded = document.createElement('div'); embedded.className = 'wallet-text-preview';
      field.after(embedded);
    }
    if (field.dataset.wallet) {
      hint = document.createElement('span'); hint.className = 'wallet-field-preview';
      // Put the QR in the same bounded group as the input (including on small screens).
      const group = document.createElement('div'); group.className = 'wallet-input-group';
      field.before(group); group.append(field, hint);
    }
    function refresh() {
      if (embedded) {
        try {
          const values = [...new Set(Array.from(field.value.matchAll(pattern), match => match[0]))];
          embedded.replaceChildren(...values.map(wallet));
          field.classList.add('wallet-text-ready');
        } catch (_) {
          field.classList.remove('wallet-text-ready');
          embedded.textContent = 'QR unavailable. Reload to view this record.';
        }
      }
      if (hint) {
        const value = field.value.trim() || example;
        if (!value) {
          hint.replaceChildren(); hint.hidden = true;
          field.classList.add('wallet-ready'); field.setCustomValidity('');
          return;
        }
        hint.hidden = false;
        try {
          const graphic = qr(value), caption = document.createElement('span');
          caption.className = 'wallet-caption';
          caption.textContent = value === example ? 'Example only · use your own address' : `${field.dataset.wallet} · matches this field`;
          hint.replaceChildren(graphic, caption); field.classList.add('wallet-ready');
        } catch (_) {
          field.classList.remove('wallet-ready');
          hint.textContent = 'QR unavailable. Reload before entering an address.';
        }
        field.setCustomValidity(example && field.value.trim() === example ? 'Replace the example with an address from your own wallet.' : '');
      }
      if (field.hasAttribute('data-format-only')) field.setCustomValidity(field.dataset.formatOnly && field.value === example ? 'Claim a real time token in Faucet, or paste your saved token.' : '');
    }
    function activate() {
      if (!field.value && field.dataset.example && !field.disabled && !field.readOnly) {
        field.value = field.dataset.example; field.classList.remove('example-preview');
        refresh(); field.dispatchEvent(new Event('input', {bubbles:true}));
      } else field.classList.remove('example-preview');
    }
    field.addEventListener('focus', activate);
    field.addEventListener('click', activate);
    field.addEventListener('input', () => { field.classList.toggle('example-preview', !field.value); refresh(); });
    field.addEventListener('change', refresh);
    refresh();
  }
  function validate(form) {
    // Optional, untouched examples stay empty. Required examples need deliberate focus/click.
    return form.reportValidity();
  }
  // Open optional details before native validation tries to focus a hidden field.
  document.addEventListener('invalid', event => {
    for (let parent = event.target.parentElement; parent; parent = parent.parentElement) {
      if (parent.tagName === 'DETAILS') parent.open = true;
    }
  }, true);
  function init(root = document) {
    root.querySelectorAll('[data-example],input[data-wallet]').forEach(setupField);
    root.querySelectorAll('[data-wallet-display]').forEach(node => {
      try { node.replaceWith(wallet(node.dataset.walletDisplay)); }
      catch (_) { node.textContent = 'QR unavailable. Reload to view this address.'; node.removeAttribute('data-wallet-display'); node.hidden = false; }
    });
  }
  window.PrivatenessUI = {wallet, text: textWithWallets, setValue, validate, init, qr};
  init();
})();
