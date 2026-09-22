/* context-manager UI Entry (ADR-0011): one chip of the Host bottom status bar.
 *
 * context-manager-status — Model Context occupancy for the Current Session:
 * tokens (provider usage when reported, else a char estimate), message count
 * and the Context Window filled ratio. Reads only the context Capability over
 * the star route; the Session Log stays the single truth source.
 */
const NAME = new URL(import.meta.url).searchParams.get('plugin') || 'context-manager';

const CSS = `
:host { display:flex; align-items:center; gap:8px; font:11px/1.6 var(--la-mono,monospace); color:var(--la-dim,#9aa0a6); }
.k { color:var(--la-dim,#9aa0a6); opacity:.75; }
.v { color:var(--la-ink,#e8eaed); }
.v.accent { color:var(--la-accent,#7aa2f7); }
.v.warn { color:#e0af68; }
.v.src { color:var(--la-ok,#9ece6a); }
`;

function shortNum(n) {
  const v = Number(n) || 0;
  if (v < 1000) return String(v);
  if (v < 1000000) return (v / 1000).toFixed(1) + 'k';
  return (v / 1000000).toFixed(2) + 'M';
}

class ContextManagerStatus extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._offs = [];
    this._timer = null;
    this._u = null;
    this._window = 0;
  }
  connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
    this._body = document.createElement('div');
    this._body.style.display = 'flex';
    this._body.style.gap = '8px';
    this._body.style.alignItems = 'center';
    root.appendChild(this._body);
    this._offs.push(LiteAgent.on('session', f => {
      if (f && f.type === 'llm_usage') this.refresh();
    }));
    this._offs.push(LiteAgent.on('status', st => {
      const s = (st && st.status) || '';
      if (s === 'idle' || String(s).indexOf('error:') === 0) this.refresh();
    }));
    this._offs.push(LiteAgent.on('__new', () => { this._u = null; this._window = 0; this.render(); }));
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
      const b = await LiteAgent.callCap('context-manager', 'context', 'usage', { sessionId: sid });
      if (!b || b.ok === false) return;
      const r = b.result || {};
      this._u = r.usage || null;
      this._window = r.contextWindow || 0;
      this.render();
    } catch (e) { /* chip is best-effort */ }
  }
  render() {
    if (!this._body) return;
    const u = this._u;
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
    if (!u) {
      add('context', '—', 'warn');
      return;
    }
    const tok = u.estimatedTokens != null ? u.estimatedTokens : 0;
    const pct = this._window > 0 ? Math.round((tok / this._window) * 100) : 0;
    add('tokens', shortNum(tok), u.source === 'provider' ? 'src' : '');
    if (this._window > 0) add('/window', shortNum(this._window) + (pct ? ' (' + pct + '%)' : ''), pct >= 80 ? 'warn' : '');
    if (u.messageCount != null) add('msgs', String(u.messageCount));
    add('src', u.source || 'chars');
  }
}

if (!customElements.get('context-manager-status')) customElements.define('context-manager-status', ContextManagerStatus);
