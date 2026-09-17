/* agent plugin UI Entry (ADR-0011): mode switcher + Settings face.
 *
 * agent-mode-panel — toolbar chip: current Agent Scheme, click to switch.
 * agent-settings   — Settings overlay override (config.face from this plugin).
 *
 * Independent rendering: Shadow DOM + --la-* tokens; talks only through
 * LiteAgent / Host routes. Shell never hardcodes scheme UI.
 */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'agent';

const LABELS = {
  chat: '极简',
  tool_calling: '标准',
  coding: '编码'
};

function labelOf(id) {
  return LABELS[id] || id || '—';
}

async function agentCall(cap, method, payload) {
  // Target this plugin by name (Host /api/call plugin field).
  const res = await fetch('/api/call', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ plugin: NAME, cap: cap, method: method, payload: payload || {} })
  });
  return res.json();
}

function unwrap(b) {
  if (!b || b.ok === false) return null;
  const r = b.result;
  if (r == null) return null;
  if (typeof r === 'string') {
    try { return JSON.parse(r); } catch (e) { return null; }
  }
  return r;
}

async function loadSchemeState() {
  const face = unwrap(await agentCall('config', 'get', {}));
  if (!face) return { current: '', names: [] };
  const names = Object.keys(face.schemes || {});
  return { current: face.defaultScheme || '', names: names };
}

const MODE_CSS = `
:host {
  display:flex; align-items:center; gap:6px;
  font: 12px/1.4 var(--la-sans, system-ui); color: var(--la-ink,#e8eaed);
  padding: 4px 6px;
}
.chip {
  background: transparent;
  border: 1px solid var(--la-line,#2a2f3a);
  color: var(--la-accent,#7aa2f7);
  border-radius: 999px;
  padding: 2px 10px;
  font-size: 11px;
  cursor: pointer;
  font-family: var(--la-mono,monospace);
}
.chip:hover { background:#1a2438; border-color: var(--la-accent); }
.menu {
  position: fixed; top: 40px; left: 8px; z-index: 200;
  min-width: 160px;
  background: var(--la-panel,#161a22);
  border: 1px solid var(--la-line,#2a2f3a);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0,0,0,.4);
  overflow: hidden;
  display: none;
}
.menu.open { display:block; }
.menu button {
  display:block; width:100%; text-align:left;
  background:transparent; border:0; color:var(--la-ink);
  padding:8px 12px; font-size:12px; cursor:pointer;
}
.menu button:hover { background:#1a2438; }
.menu button.on { color: var(--la-accent); }
.hint {
  padding:6px 12px; font-size:10px; color:var(--la-dim);
  border-top:1px solid var(--la-line); font-family:var(--la-mono);
}
`;

class AgentModePanel extends HTMLElement {
  constructor() {
    super();
    this._current = '';
    this._names = [];
    this._open = false;
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    style.textContent = MODE_CSS;
    root.appendChild(style);
    const wrap = document.createElement('div');
    wrap.style.position = 'relative';
    const chip = document.createElement('button');
    chip.type = 'button';
    chip.className = 'chip';
    chip.textContent = 'scheme…';
    chip.title = 'Agent Scheme';
    const menu = document.createElement('div');
    menu.className = 'menu';
    wrap.appendChild(chip);
    wrap.appendChild(menu);
    root.appendChild(wrap);
    this._chip = chip;
    this._menu = menu;
    chip.addEventListener('click', (e) => {
      e.stopPropagation();
      this._toggle();
    });
    document.addEventListener('click', this._onDoc = (e) => {
      if (!this.shadowRoot) return;
      if (this._open) this._close();
    });
    this._offs = [];
    this._offs.push(LiteAgent.on('__scheme', () => { this.refresh(); }));
    this._offs.push(LiteAgent.on('__session', () => { this.refresh(); }));
    await this.refresh();
  }
  disconnectedCallback() {
    if (this._onDoc) document.removeEventListener('click', this._onDoc);
    (this._offs || []).forEach(off => off());
    this._offs = [];
  }
  set props(v) {
    this._props = v || {};
  }
  get props() { return this._props || {}; }
  async refresh() {
    try {
      const st = await loadSchemeState();
      this._current = st.current;
      this._names = st.names.length ? st.names : ['chat', 'tool_calling', 'coding'];
      this._renderChip();
      this._renderMenu();
    } catch (e) { /* best-effort */ }
  }
  _renderChip() {
    if (!this._chip) return;
    this._chip.textContent = labelOf(this._current);
    this._chip.title = 'Agent Scheme: ' + (this._current || '?') + '（点击切换）';
  }
  _renderMenu() {
    if (!this._menu) return;
    this._menu.innerHTML = '';
    this._names.forEach((id) => {
      const b = document.createElement('button');
      b.type = 'button';
      // Alias + raw id, matching the welcome chips and Settings (BUG-10).
      b.textContent = labelOf(id) + ' (' + id + ')' + (id === this._current ? ' ·' : '');
      if (id === this._current) b.classList.add('on');
      b.onclick = (e) => {
        e.stopPropagation();
        this._set(id);
      };
      this._menu.appendChild(b);
    });
    const hint = document.createElement('div');
    hint.className = 'hint';
    hint.textContent = 'config.set defaultScheme';
    this._menu.appendChild(hint);
  }
  _toggle() {
    this._open = !this._open;
    if (this._menu) this._menu.classList.toggle('open', this._open);
    if (this._open && this.shadowRoot) {
      // Fixed-position menu escapes the right-column scroll container; place it
      // at the chip, clamped inside the viewport so it never gets clipped.
      const chip = this.shadowRoot.querySelector('.chip');
      if (chip) {
        const r = chip.getBoundingClientRect();
        const mw = 160;
        this._menu.style.top = (r.bottom + 4) + 'px';
        this._menu.style.left = Math.max(8, Math.min(r.left, window.innerWidth - mw - 8)) + 'px';
      }
    }
  }
  _close() {
    this._open = false;
    if (this._menu) this._menu.classList.remove('open');
  }
  async _set(id) {
    this._close();
    this._current = id;
    this._renderChip();
    try {
      await agentCall('config', 'set', { defaultScheme: id });
      LiteAgent.emit('__scheme', { scheme: id });
      LiteAgent.emit('__notice', { text: 'agent scheme → ' + labelOf(id), cls: 'message info' });
      this._renderMenu();
    } catch (e) {
      LiteAgent.emit('__notice', { text: 'scheme set failed: ' + e, cls: 'message error' });
    }
  }
}

const SET_CSS = `
:host { display:block; }
h3 { margin:0 0 4px; font-size:14px; color:var(--la-ink); }
.desc { font-size:12px; color:var(--la-dim); margin:0 0 14px; }
.row { margin-bottom:12px; }
.row label { display:block; font-size:11px; color:var(--la-dim); margin-bottom:4px; font-family:var(--la-mono); }
select, input {
  width:100%; max-width:420px;
  background:var(--la-bg,#0f1115); border:1px solid var(--la-line,#2a2f3a);
  color:var(--la-ink); border-radius:6px; padding:7px 10px; font-size:13px; font-family:var(--la-mono);
}
.hint { font-size:11px; color:var(--la-dim); margin-top:4px; }
table { border-collapse:collapse; margin-top:8px; font-size:11px; font-family:var(--la-mono); color:var(--la-dim); }
td, th { border:1px solid var(--la-line); padding:4px 8px; text-align:left; }
.actions { display:flex; gap:8px; margin-top:14px; }
button.primary {
  background:var(--la-accent,#7aa2f7); color:#0b1020; border:0;
  border-radius:8px; padding:7px 14px; font-size:12px; font-weight:600; cursor:pointer;
}
.msg { font-size:12px; margin-left:8px; }
.msg.ok { color:var(--la-ok,#9ece6a); }
.msg.err { color:var(--la-err,#f7768e); }
`;

class AgentSettings extends HTMLElement {
  constructor() {
    super();
    this._face = null;
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    style.textContent = SET_CSS;
    root.appendChild(style);
    this._body = document.createElement('div');
    root.appendChild(this._body);
    await this.reload();
  }
  async reload() {
    const face = unwrap(await agentCall('config', 'get', {}));
    this._face = face || { defaultScheme: '', schemes: {} };
    this.render();
  }
  render() {
    if (!this._body) return;
    const face = this._face || { defaultScheme: '', schemes: {} };
    this._body.innerHTML = '';
    const h = document.createElement('h3');
    h.textContent = 'agent · Agent Scheme';
    this._body.appendChild(h);
    const d = document.createElement('p');
    d.className = 'desc';
    d.textContent = 'Loop 策略：dependsPlugins 经 ensurePlugins 拉起；allowedTools 决定模型可见 tool（无 readOnly 门禁）。';
    this._body.appendChild(d);

    const row = document.createElement('div');
    row.className = 'row';
    const lab = document.createElement('label');
    lab.textContent = 'defaultScheme';
    row.appendChild(lab);
    const sel = document.createElement('select');
    const names = Object.keys(face.schemes || {});
    names.forEach((n) => {
      const o = document.createElement('option');
      o.value = n;
      o.textContent = labelOf(n) + ' (' + n + ')';
      if (n === face.defaultScheme) o.selected = true;
      sel.appendChild(o);
    });
    row.appendChild(sel);
    const hint = document.createElement('div');
    hint.className = 'hint';
    hint.textContent = '热切换：下一 Turn 生效；scheme 名写入 Session Log。';
    row.appendChild(hint);
    this._body.appendChild(row);

    const table = document.createElement('table');
    const thead = document.createElement('tr');
    ['scheme', 'dependsPlugins', 'allowedTools', 'maxSteps'].forEach((t) => {
      const th = document.createElement('th');
      th.textContent = t;
      thead.appendChild(th);
    });
    table.appendChild(thead);
    names.forEach((n) => {
      const sc = (face.schemes || {})[n] || {};
      const tr = document.createElement('tr');
      const c0 = document.createElement('td');
      c0.textContent = labelOf(n);
      tr.appendChild(c0);
      const c1 = document.createElement('td');
      c1.textContent = (sc.dependsPlugins && sc.dependsPlugins.join(', ')) || '—';
      tr.appendChild(c1);
      const c2 = document.createElement('td');
      if (sc.allowedTools === undefined || sc.allowedTools === null) c2.textContent = '(不过滤)';
      else if (Array.isArray(sc.allowedTools) && sc.allowedTools.length === 0) c2.textContent = '(无外部 tool)';
      else c2.textContent = (sc.allowedTools || []).join(', ');
      tr.appendChild(c2);
      const c3 = document.createElement('td');
      c3.textContent = sc.maxSteps != null ? String(sc.maxSteps) : 'default';
      tr.appendChild(c3);
      table.appendChild(tr);
    });
    this._body.appendChild(table);

    const actions = document.createElement('div');
    actions.className = 'actions';
    const save = document.createElement('button');
    save.type = 'button';
    save.className = 'primary';
    save.textContent = 'Save';
    const msg = document.createElement('span');
    msg.className = 'msg';
    save.onclick = async () => {
      msg.className = 'msg';
      msg.textContent = 'saving…';
      const b = await agentCall('config', 'set', { defaultScheme: sel.value });
      if (b && b.ok === false) {
        msg.className = 'msg err';
        msg.textContent = b.error || 'failed';
      } else {
        msg.className = 'msg ok';
        msg.textContent = 'saved';
        LiteAgent.emit('__scheme', { scheme: sel.value });
        await this.reload();
      }
    };
    actions.appendChild(save);
    actions.appendChild(msg);
    this._body.appendChild(actions);
  }
}

if (!customElements.get('agent-mode-panel')) customElements.define('agent-mode-panel', AgentModePanel);
if (!customElements.get('agent-settings')) customElements.define('agent-settings', AgentSettings);

/* ---- agent-status: bottom status bar chip (active Agent Scheme) ---- */

const STATUS_CSS = `
:host { display:flex; align-items:center; gap:8px; font:11px/1.6 var(--la-mono,monospace); color:var(--la-dim,#9aa0a6); }
.k { color:var(--la-dim,#9aa0a6); opacity:.75; }
.v { color:var(--la-ink,#e8eaed); }
.v.accent { color:#c678dd; }
`;

class AgentStatus extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._timer = null;
    this._name = '';
  }
  connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = STATUS_CSS;
    root.appendChild(style);
    this._body = document.createElement('div');
    this._body.style.display = 'flex';
    this._body.style.gap = '8px';
    this._body.style.alignItems = 'center';
    root.appendChild(this._body);
    this.refresh();
    this._offs = [];
    this._offs.push(LiteAgent.on('__scheme', () => { this.refresh(); }));
    this._offs.push(LiteAgent.on('__session', () => { this.refresh(); }));
    this._offs.push(LiteAgent.on('status', () => {
      // ensurePlugins during a turn can change the mounted set; nudge the chip.
      if (!document.hidden) this.refresh();
    }));
    this._timer = setInterval(() => { if (!document.hidden) this.refresh(); }, 15000);
  }
  disconnectedCallback() {
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    (this._offs || []).forEach(off => off());
    this._offs = [];
    this._root = null;
  }
  async refresh() {
    if (!this._root) return;
    try {
      const st = await loadSchemeState();
      if (st.current !== this._name || !this._body.childNodes.length) {
        this._name = st.current;
        this.render(st);
      }
    } catch (e) { /* chip is best-effort */ }
  }
  render(st) {
    if (!this._body) return;
    this._body.innerHTML = '';
    const add = (k, v, cls) => {
      const kk = document.createElement('span');
      kk.className = 'k';
      kk.textContent = k;
      const vv = document.createElement('span');
      vv.className = 'v' + (cls ? ' ' + cls : '');
      vv.textContent = v;
      vv.title = 'agent scheme: ' + (st.current || '?');
      this._body.appendChild(kk);
      this._body.appendChild(vv);
    };
    add('agent', labelOf(st.current), 'accent');
    if (st.names && st.names.length) add('schemes', String(st.names.length));
  }
}

if (!customElements.get('agent-status')) customElements.define('agent-status', AgentStatus);
