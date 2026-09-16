/* llm-openai UI Entry: Settings face for this plugin only. */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'llm-openai';

const CSS = `
:host { display:block; }
h3 { margin:0 0 4px; font-size:14px; color:var(--la-ink); }
.desc { font-size:12px; color:var(--la-dim); margin:0 0 14px; }
.field { margin-bottom:12px; }
.field label { display:block; font-size:11px; color:var(--la-dim); margin-bottom:4px; font-family:var(--la-mono); }
.field label .sec { color:var(--la-accent); margin-left:6px; }
.field input {
  width:100%; max-width:420px;
  background:var(--la-bg,#0f1115); border:1px solid var(--la-line,#2a2f3a);
  color:var(--la-ink); border-radius:6px; padding:7px 10px; font-size:13px; font-family:var(--la-mono);
}
.field .hint { font-size:11px; color:var(--la-dim); margin-top:3px; }
.actions { display:flex; gap:8px; margin-top:16px; align-items:center; }
.actions button.primary {
  background:var(--la-accent,#7aa2f7); color:#0b1020; border:0;
  border-radius:8px; padding:7px 14px; font-size:12px; font-weight:600; cursor:pointer;
}
.msg { font-size:12px; margin-left:8px; }
.msg.ok { color:var(--la-ok,#9ece6a); }
.msg.err { color:var(--la-err,#f7768e); }
`;

async function callCap(cap, method, payload) {
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

class LlmOpenAISettings extends HTMLElement {
  constructor() {
    super();
    this._inputs = {};
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
    this._body = document.createElement('div');
    root.appendChild(this._body);
    await this.reload();
  }
  async reload() {
    const face = unwrap(await callCap('config', 'get', {})) || {};
    const fields = face.fields || [];
    const values = {};
    fields.forEach(f => { values[f.name] = f.value; });
    this.render(fields, values);
  }
  render(fields, values) {
    if (!this._body) return;
    this._body.innerHTML = '';
    this._inputs = {};
    const h = document.createElement('h3');
    h.textContent = 'llm-openai';
    this._body.appendChild(h);
    const d = document.createElement('p');
    d.className = 'desc';
    d.textContent = 'OpenAI-compatible LLM provider（DeepSeek 等）。密钥与 model 热更新到下一跳。';
    this._body.appendChild(d);
    fields.forEach(f => {
      const wrap = document.createElement('div');
      wrap.className = 'field';
      const lab = document.createElement('label');
      lab.appendChild(document.createTextNode(f.name));
      if (f.secret) {
        const sec = document.createElement('span');
        sec.className = 'sec';
        sec.textContent = 'secret';
        lab.appendChild(sec);
      }
      wrap.appendChild(lab);
      const input = document.createElement('input');
      input.type = f.secret ? 'password' : (f.type === 'integer' ? 'number' : 'text');
      const cur = values[f.name];
      input.value = cur === undefined || cur === null ? (f.default !== undefined ? String(f.default) : '') : String(cur);
      if (f.secret) {
        input.placeholder = cur ? String(cur) : 'unchanged if empty';
        if (cur) input.value = '';
      }
      wrap.appendChild(input);
      if (f.description) {
        const hint = document.createElement('div');
        hint.className = 'hint';
        hint.textContent = f.description;
        wrap.appendChild(hint);
      }
      this._body.appendChild(wrap);
      this._inputs[f.name] = { input: input, field: f };
    });
    const actions = document.createElement('div');
    actions.className = 'actions';
    const save = document.createElement('button');
    save.type = 'button';
    save.className = 'primary';
    save.textContent = 'Save';
    const msg = document.createElement('span');
    msg.className = 'msg';
    save.onclick = async () => {
      const payload = {};
      Object.keys(this._inputs).forEach(k => {
        const it = this._inputs[k];
        if (it.field.secret && !it.input.value) return;
        if (it.field.type === 'integer') {
          const n = parseInt(it.input.value, 10);
          if (!isNaN(n)) payload[k] = n;
          return;
        }
        payload[k] = it.input.value;
      });
      if (!Object.keys(payload).length) {
        msg.className = 'msg err';
        msg.textContent = 'nothing to save';
        return;
      }
      save.disabled = true;
      msg.className = 'msg';
      msg.textContent = 'saving…';
      const b = await callCap('config', 'set', payload);
      save.disabled = false;
      if (b && b.ok === false) {
        msg.className = 'msg err';
        msg.textContent = b.error || 'failed';
      } else {
        msg.className = 'msg ok';
        msg.textContent = 'saved';
      }
    };
    actions.appendChild(save);
    actions.appendChild(msg);
    this._body.appendChild(actions);
  }
}

if (!customElements.get('llm-openai-settings')) customElements.define('llm-openai-settings', LlmOpenAISettings);

/* ---- llm-openai-status: bottom status bar chip (model + model time) ----
 * Reads llm.stats: process-local counters for requests, total/last model time
 * and tokens. Refreshes on Turn end and on a slow timer.
 */
const STATUS_CSS = `
:host { display:flex; align-items:center; gap:8px; font:11px/1.6 var(--la-mono,monospace); color:var(--la-dim,#9aa0a6); }
.k { color:var(--la-dim,#9aa0a6); opacity:.75; }
.v { color:var(--la-ink,#e8eaed); }
.v.accent { color:var(--la-accent,#7aa2f7); }
`;

function fmtMs(ms) {
  const n = Number(ms) || 0;
  if (n < 1000) return n + 'ms';
  if (n < 60000) return (n / 1000).toFixed(1) + 's';
  return Math.floor(n / 60000) + 'm' + Math.round((n % 60000) / 1000) + 's';
}

class LlmOpenAiStatus extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._offs = [];
    this._timer = null;
    this._st = null;
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
    this._offs.push(LiteAgent.on('status', st => {
      const s = (st && st.status) || '';
      if (s === 'idle' || String(s).indexOf('error:') === 0) this.refresh();
    }));
    this.refresh();
    this._timer = setInterval(() => { if (!document.hidden) this.refresh(); }, 15000);
  }
  disconnectedCallback() {
    this._offs.forEach(off => off());
    this._offs = [];
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    this._root = null;
  }
  async refresh() {
    if (!this._root) return;
    try {
      const sid = window.__liteSessionId || '';
      const res = await fetch('/api/call', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plugin: NAME, cap: 'llm', method: 'stats', payload: { sessionId: sid } })
      });
      const b = await res.json();
      let r = b && b.ok !== false ? b.result : null;
      if (typeof r === 'string') { try { r = JSON.parse(r); } catch (e) { r = null; } }
      if (r) { this._st = r; this.render(); }
    } catch (e) { /* chip is best-effort */ }
  }
  render() {
    if (!this._body) return;
    const st = this._st || {};
    const sess = st.session || null;
    this._body.innerHTML = '';
    const add = (k, v, cls) => {
      const kk = document.createElement('span');
      kk.className = 'k';
      kk.textContent = k;
      const vv = document.createElement('span');
      vv.className = 'v' + (cls ? ' ' + cls : '');
      vv.textContent = v;
      vv.title = k + ': ' + v;
      this._body.appendChild(kk);
      this._body.appendChild(vv);
    };
    add('model', st.model || '—', 'accent');
    add('model-time', fmtMs(sess ? sess.totalMs : st.totalMs));
    add('calls', String((sess ? sess.requests : st.requests) || 0));
    if (st.tokens) add('tokens', String(sess && sess.tokens != null ? sess.tokens : st.tokens));
  }
}

if (!customElements.get('llm-openai-status')) customElements.define('llm-openai-status', LlmOpenAiStatus);
