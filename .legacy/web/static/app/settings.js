// Settings overlay: per-plugin config forms (Web Medium only).
// Each Plugin that implements config.schema/get is listed; the form is built
// from that Plugin's own schema — plugins "render" their settings via their
// config face (and may override with a custom <name>-settings element).

function el(tag, attrs, children) {
  const n = document.createElement(tag);
  if (attrs) Object.keys(attrs).forEach(k => {
    if (k === 'text') n.textContent = attrs[k];
    else if (k === 'html') n.innerHTML = attrs[k];
    else if (k === 'class') n.className = attrs[k];
    else if (k === 'style') n.setAttribute('style', attrs[k]);
    else n.setAttribute(k, attrs[k]);
  });
  (children || []).forEach(c => n && n.appendChild(c));
  return n;
}

const CSS = `
.st-backdrop{position:absolute;inset:0;background:rgba(0,0,0,.45);display:flex;align-items:center;justify-content:center;padding:24px}
.st-modal{background:var(--la-panel);border:1px solid var(--la-line);border-radius:12px;width:min(920px,100%);max-height:min(85vh,720px);display:flex;flex-direction:column;box-shadow:0 12px 40px rgba(0,0,0,.45);overflow:hidden}
.st-head{display:flex;align-items:center;justify-content:space-between;padding:12px 16px;border-bottom:1px solid var(--la-line);font-size:13px;font-weight:600}
.st-head button{background:transparent;border:1px solid var(--la-line);color:var(--la-dim);border-radius:6px;padding:4px 10px;cursor:pointer;font-size:12px}
.st-head button:hover{color:var(--la-ink);border-color:var(--la-accent)}
.st-body{flex:1;min-height:0;display:flex}
.st-list{width:220px;flex-shrink:0;border-right:1px solid var(--la-line);overflow-y:auto;padding:8px;background:var(--la-panel2)}
.st-item{padding:8px 10px;border-radius:8px;font-size:12px;color:var(--la-dim);cursor:pointer;margin-bottom:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.st-item:hover{background:#1a1f2a;color:var(--la-ink)}
.st-item.active{background:#1a1f2a;color:var(--la-ink);border:1px solid var(--la-line)}
.st-item .caps{display:block;font-size:10px;color:var(--la-accent);font-family:var(--la-mono);margin-top:2px}
.st-main{flex:1;min-width:0;overflow-y:auto;padding:16px 18px}
.st-main h3{margin:0 0 4px;font-size:14px;color:var(--la-ink)}
.st-main .desc{font-size:12px;color:var(--la-dim);margin:0 0 14px}
.st-field{margin-bottom:12px}
.st-field label{display:block;font-size:11px;color:var(--la-dim);margin-bottom:4px;font-family:var(--la-mono)}
.st-field label .sec{color:var(--la-accent);margin-left:6px}
.st-field input,.st-field select{width:100%;max-width:420px;background:var(--la-bg);border:1px solid var(--la-line);color:var(--la-ink);border-radius:6px;padding:7px 10px;font-size:13px;font-family:var(--la-mono)}
.st-field input:focus{outline:none;border-color:var(--la-accent)}
.st-field .hint{font-size:11px;color:var(--la-dim);margin-top:3px}
.st-actions{display:flex;gap:8px;margin-top:16px;align-items:center}
.st-actions button.primary{background:var(--la-accent);color:#0b1020;border:0;border-radius:8px;padding:7px 14px;font-size:12px;font-weight:600;cursor:pointer}
.st-actions button.ghost{background:transparent;border:1px solid var(--la-line);color:var(--la-dim);border-radius:8px;padding:7px 12px;font-size:12px;cursor:pointer}
.st-msg{font-size:12px;margin-left:8px}
.st-msg.ok{color:var(--la-ok)}
.st-msg.err{color:var(--la-err)}
.st-empty{color:var(--la-dim);font-size:12px;padding:24px 8px}
`;

async function callPlugin(plugin, cap, method, payload) {
  const res = await fetch('/api/call', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ plugin: plugin, cap: cap, method: method, payload: payload || {} })
  });
  return res.json();
}

function unwrap(b) {
  if (!b) return null;
  if (b.ok === false) return null;
  const r = b.result;
  if (r == null) return null;
  if (typeof r === 'string') {
    try { return JSON.parse(r); } catch (e) { return null; }
  }
  return r;
}

async function loadPluginConfigFace(name) {
  const schemaRes = await callPlugin(name, 'config', 'schema', {});
  const getRes = await callPlugin(name, 'config', 'get', {});
  const schema = unwrap(schemaRes);
  const current = unwrap(getRes);
  if (!schema || !Array.isArray(schema.fields) || !schema.fields.length) return null;
  const values = {};
  if (current && Array.isArray(current.fields)) {
    current.fields.forEach(f => { if (f && f.name !== undefined) values[f.name] = f.value; });
  }
  return { fields: schema.fields, values: values };
}

function renderForm(host, plugin, face, onSaved) {
  host.innerHTML = '';
  const customTag = plugin + '-settings';
  // Prefer the plugin's own Settings face (independent render, ADR-0011).
  // The UI Entry may still be loading — wait briefly for definition.
  if (customElements.get(customTag)) {
    mountCustomSettings(host, customTag, plugin, face);
    return;
  }
  let settled = false;
  const timer = setTimeout(function () {
    if (settled) return;
    settled = true;
    renderSchemaForm(host, plugin, face, onSaved);
  }, 400);
  customElements.whenDefined(customTag).then(function () {
    if (settled) return;
    settled = true;
    clearTimeout(timer);
    mountCustomSettings(host, customTag, plugin, face);
  });
}

function mountCustomSettings(host, customTag, plugin, face) {
  host.innerHTML = '';
  const node = document.createElement(customTag);
  node.plugin = plugin;
  node.configFace = face;
  host.appendChild(node);
}

function renderSchemaForm(host, plugin, face, onSaved) {
  host.innerHTML = '';
  host.appendChild(el('h3', { text: plugin }));
  host.appendChild(el('p', { class: 'desc', text: 'Settings provided by this plugin (config.schema / config.get).' }));
  const form = el('div');
  const inputs = {};
  face.fields.forEach(f => {
    const name = f.name;
    const type = f.type || 'string';
    const secret = !!f.secret;
    const wrap = el('div', { class: 'st-field' });
    const lab = el('label');
    lab.appendChild(document.createTextNode(name));
    if (secret) lab.appendChild(el('span', { class: 'sec', text: 'secret' }));
    wrap.appendChild(lab);
    const input = el('input');
    input.type = secret ? 'password' : (type === 'integer' ? 'number' : 'text');
    const cur = face.values[name];
    input.value = cur === undefined || cur === null ? (f.default !== undefined ? String(f.default) : '') : String(cur);
    if (secret) {
      input.placeholder = cur ? String(cur) : 'unchanged if empty';
      if (cur) input.value = '';
    }
    wrap.appendChild(input);
    if (f.description) wrap.appendChild(el('div', { class: 'hint', text: f.description }));
    form.appendChild(wrap);
    inputs[name] = { input: input, field: f, secret: secret, original: cur };
  });
  const actions = el('div', { class: 'st-actions' });
  const btnSave = el('button', { type: 'button', class: 'primary', text: 'Save' });
  const btnReload = el('button', { type: 'button', class: 'ghost', text: 'Reload' });
  const msg = el('span', { class: 'st-msg' });
  actions.appendChild(btnSave);
  actions.appendChild(btnReload);
  actions.appendChild(msg);
  form.appendChild(actions);
  host.appendChild(form);

  btnSave.onclick = async () => {
    const payload = {};
    Object.keys(inputs).forEach(k => {
      const it = inputs[k];
      if (it.secret && !it.input.value) return;
      let v = it.input.value;
      if (it.field.type === 'integer') {
        const n = parseInt(v, 10);
        if (!isNaN(n)) payload[k] = n;
        return;
      }
      payload[k] = v;
    });
    if (!Object.keys(payload).length) {
      msg.className = 'st-msg err';
      msg.textContent = 'nothing to save';
      return;
    }
    btnSave.disabled = true;
    msg.className = 'st-msg';
    msg.textContent = 'saving…';
    try {
      const b = await callPlugin(plugin, 'config', 'set', payload);
      if (b && b.ok === false) {
        msg.className = 'st-msg err';
        msg.textContent = b.error || 'save failed';
      } else {
        msg.className = 'st-msg ok';
        msg.textContent = 'saved';
        if (onSaved) onSaved();
      }
    } catch (e) {
      msg.className = 'st-msg err';
      msg.textContent = String(e && e.message || e);
    }
    btnSave.disabled = false;
  };
  btnReload.onclick = async () => {
    msg.className = 'st-msg';
    msg.textContent = 'reloading…';
    try {
      await callPlugin(plugin, 'config', 'reload', {});
      const face2 = await loadPluginConfigFace(plugin);
      if (face2) renderForm(host, plugin, face2, onSaved);
      msg.className = 'st-msg ok';
    } catch (e) {
      msg.className = 'st-msg err';
      msg.textContent = String(e && e.message || e);
    }
  };
}

let keyHandler = null;

function paint(root) {
  let style = root.querySelector('#st-style');
  if (!style) {
    style = el('style', { id: 'st-style', text: CSS });
    document.head.appendChild(style);
  }
  let selected = null;
  const body = el('div', { class: 'st-body' });
  const list = el('div', { class: 'st-list' });
  const main = el('div', { class: 'st-main' });
  body.appendChild(list);
  body.appendChild(main);
  main.appendChild(el('div', { class: 'st-empty', text: 'Loading plugins…' }));

  root.innerHTML = '';
  if (keyHandler) document.removeEventListener('keydown', keyHandler);
  keyHandler = null;
  const backdrop = el('div', { class: 'st-backdrop' });
  const modal = el('div', { class: 'st-modal' });
  const head = el('div', { class: 'st-head' });
  head.appendChild(el('span', { text: 'Settings · plugins' }));
  const btnClose = el('button', { type: 'button', text: 'Close' });
  const close = () => { root.innerHTML = ''; if (keyHandler) document.removeEventListener('keydown', keyHandler); keyHandler = null; };
  keyHandler = (e) => { if (e.key === 'Escape') close(); };
  document.addEventListener('keydown', keyHandler);
  btnClose.onclick = close;
  head.appendChild(btnClose);
  modal.appendChild(head);
  modal.appendChild(body);
  backdrop.appendChild(modal);
  backdrop.addEventListener('click', (e) => { if (e.target === backdrop) close(); });
  root.appendChild(backdrop);

  fetch('/api/plugins').then(r => r.json()).then(async (data) => {
    const plugins = (data.plugins || []).filter(p => !p.state || p.state === 'mounted');
    const faces = {};
    // List first (fast). Config faces load only for the selected plugin —
    // probing every plugin's config.schema/get is a Frame round-trip each.
    const names = plugins.map(p => p.name).sort();
    const meta = {};
    plugins.forEach(p => { meta[p.name] = p; });

    async function ensureFace(n) {
      if (faces[n]) return faces[n];
      try {
        const face = await loadPluginConfigFace(n);
        if (face) faces[n] = { face: face, meta: meta[n] };
      } catch (e) { /* no config face */ }
      return faces[n];
    }

    function paintList(configurable) {
      list.innerHTML = '';
      if (!names.length) {
        list.appendChild(el('div', { class: 'st-empty', text: 'No plugins mounted.' }));
        return;
      }
      names.forEach(n => {
        const has = configurable == null ? true : configurable.has(n);
        const item = el('div', { class: 'st-item' + (n === selected ? ' active' : '') });
        item.textContent = n;
        const caps = ((meta[n] && meta[n].provides) || []).join(', ');
        if (caps) item.appendChild(el('span', { class: 'caps', text: caps }));
        if (has === false) item.style.opacity = '0.45';
        item.onclick = async () => {
          selected = n;
          paintList(configurable);
          main.innerHTML = '';
          main.appendChild(el('div', { class: 'st-empty', text: 'Loading settings…' }));
          const got = await ensureFace(n);
          paintMain();
        };
        list.appendChild(item);
      });
    }
    function paintMain() {
      if (!selected || !faces[selected]) {
        main.innerHTML = '';
        main.appendChild(el('div', { class: 'st-empty', text: 'This plugin has no config.schema face.' }));
        return;
      }
      renderForm(main, selected, faces[selected].face, async () => {
        try {
          const face = await loadPluginConfigFace(selected);
          if (face) faces[selected].face = face;
        } catch (e) { /* keep */ }
      });
    }
    if (!selected && names.length) selected = names[0];
    paintList(null);
    // Lazy: only the initially selected plugin's face is fetched.
    await ensureFace(selected);
    // Mark plugins that actually expose config (probe remaining in background).
    const configurable = new Set();
    if (faces[selected]) configurable.add(selected);
    paintList(null);
    paintMain();
    names.forEach(async (n) => {
      if (n === selected) return;
      const got = await ensureFace(n);
      if (got) {
        configurable.add(n);
        // soft-refresh list opacity without stealing selection
        if (!document.querySelector('.st-backdrop')) return;
        paintList(configurable);
      }
    });
  }).catch(e => {
    main.innerHTML = '';
    main.appendChild(el('div', { class: 'st-empty', text: 'Failed to load plugins: ' + e }));
  });
}

export function openSettingsPanel() {
  const root = document.getElementById('main-overlay');
  if (!root) return;
  if (root.querySelector('.st-backdrop')) {
    root.innerHTML = '';
    if (keyHandler) document.removeEventListener('keydown', keyHandler);
    keyHandler = null;
    return;
  }
  paint(root);
}
