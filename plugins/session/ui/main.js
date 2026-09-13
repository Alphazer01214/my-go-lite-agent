/* session plugin UI Entry (ADR-0011): the Session Log's Web faces.
 *
 * session-trace — the Session Log's trace projection (main page trace column
 *                 and the /trace debug page). Facts come from the session
 *                 Capability through the star route
 *                 (LiteAgent.call('session','query')), never from a
 *                 dedicated endpoint; live updates arrive over the session
 *                 SSE topic.
 * session-rail  — the session list / switcher (sidebar). Switching is medium
 *                 state (POST /api/session/select); the component announces
 *                 the switch over __session and the shell reloads the chat.
 */
const POLL_MS = 2000;

let currentSid;

function esc(s) {
  return String(s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
}

function summary(f) {
  const t = f.type || '';
  const role = f.role || '';
  if (t === 'message') return (role || 'msg') + ' · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'reasoning') return 'thinking · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'tool_call') {
    const names = ((f.meta && f.meta.tool_calls) || []).map(tc => tc.name).join(',');
    return 'call ' + (names || '?');
  }
  if (t === 'tool_result') return 'result · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'request_header') return 'llm request';
  if (t === 'step_start') return 'step ' + ((f.meta && f.meta.step) || '');
  if (t === 'step_end') return 'step end · ' + ((f.meta && f.meta.reason) || '');
  if (t === 'turn_start') return 'turn start';
  if (t === 'turn_end') return 'turn end · ' + ((f.meta && f.meta.reason) || '');
  return t;
}

class SessionTrace extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._offSession = null;
    this._offFacts = null;
    this._timer = null;
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
    const list = document.createElement('div');
    list.className = 'list';
    root.appendChild(list);
    // The medium owns the current session; learn it, then re-render on switches.
    this._offSession = LiteAgent.onSessionChange(() => this.refresh());
    this._offFacts = LiteAgent.on('session', f => this.onFact(f));
    await this.refresh();
    if (!this._timer) this._timer = setInterval(() => this.refresh(), POLL_MS);
  }
  disconnectedCallback() {
    if (this._offSession) this._offSession();
    if (this._offFacts) this._offFacts();
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    this._root = null;
  }
  onFact(f) {
    // Live append only when the fact belongs to the session being viewed.
    if (!this._root) return;
    const sid = f && f.sessionId !== undefined ? String(f.sessionId) : '';
    if (String(currentSid || '') !== sid) return;
    this.appendFact(f);
  }
  async refresh() {
    if (!this._root) return;
    try {
      // Current session is medium state (GET /api/session), then facts come
      // from the session Capability via the star route.
      const st = await fetch('/api/session').then(r => r.json());
      currentSid = st.sessionId !== undefined && st.sessionId !== null ? st.sessionId : (window.__liteSessionId || '');
      const res = await LiteAgent.call('session', 'query', { sessionId: currentSid, afterSeq: 0, limit: 0 });
      if (!res || res.ok === false) throw new Error(res && res.error || 'session.query failed');
      const facts = mergeReasoning((res.result && res.result.facts) || []);
      this.render(facts);
    } catch (e) {
      if (this._root) this.renderError(e);
    }
  }
  render(facts) {
    if (!this._root) return;
    const list = this._root.querySelector('.list');
    list.innerHTML = '';
    facts.forEach(f => this.appendFact(f));
    list.scrollTop = list.scrollHeight;
  }
  appendFact(f) {
    if (!f || !f.type) return;
    const list = this._root.querySelector('.list');
    const t = f.type;
    const role = f.role || '';
    const cls = t === 'message' && role ? role : t;
    const row = document.createElement('div');
    row.className = 'trace-row ' + cls;
    const kind = t === 'message' ? (role || 'msg') : t;
    const sid = f.sessionId !== undefined && f.sessionId !== null ? String(f.sessionId) : '';
    const short = sid ? (sid.length > 8 ? sid.slice(0, 8) : sid) : '·';
    row.innerHTML = '<span class="t-kind">' + esc(kind) + '</span><span class="t-sum">' + esc(summary(f)) + '</span>'
      + '<span class="t-seq">' + esc(short) + ' #' + (f.seq || '') + '</span>';
    const det = document.createElement('div');
    det.className = 'trace-detail';
    let detail = f.content || '';
    if (f.sessionId !== undefined) detail = 'sessionId: ' + (f.sessionId || '(default)') + '\n\n' + detail;
    if (f.meta) detail += (detail ? '\n\n' : '') + JSON.stringify(f.meta, null, 2);
    det.textContent = detail || '(empty)';
    row.onclick = () => { row.classList.toggle('open'); det.classList.toggle('open'); };
    list.appendChild(row); list.appendChild(det);
    list.scrollTop = list.scrollHeight;
  }
  renderError(e) {
    const list = this._root.querySelector('.list');
    const el = document.createElement('div');
    el.className = 'message error';
    el.textContent = 'trace unavailable: ' + e;
    list.appendChild(el);
  }
}

// Consecutive type=reasoning facts (legacy fragmented flushes) become one row.
function mergeReasoning(facts) {
  const out = [];
  for (let i = 0; i < facts.length; i++) {
    const f = facts[i];
    if (!f || f.type !== 'reasoning') { out.push(f); continue; }
    const parts = [];
    let meta = f.meta, seq = f.seq, sid = f.sessionId;
    while (i < facts.length && facts[i] && facts[i].type === 'reasoning') {
      if (facts[i].content) parts.push(facts[i].content);
      if (facts[i].meta) meta = facts[i].meta;
      if (facts[i].seq !== undefined) seq = facts[i].seq;
      if (facts[i].sessionId !== undefined) sid = facts[i].sessionId;
      i++;
    }
    i--;
    out.push({ type: 'reasoning', role: f.role || 'assistant', content: parts.join(''), meta: meta, seq: seq, sessionId: sid });
  }
  return out;
}

const CSS = `
  :host { display:block; font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed); }
  .list { height:100%; overflow-y:auto; min-height:0; }
  .trace-row { padding:6px 12px; font-size:12px; cursor:pointer; display:flex; gap:8px; align-items:baseline; line-height:1.3; }
  .trace-row:hover { background:#1a1f2a; }
  .trace-row .t-kind { flex:0 0 72px; font-size:10px; text-transform:uppercase; letter-spacing:.04em; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .trace-row .t-sum { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--la-ink,#e8eaed); }
  .trace-row .t-seq { flex:0 0 auto; font-size:10px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .trace-row.user .t-kind { color:#7aa2f7; }
  .trace-row.assistant .t-kind { color:var(--la-ok,#9ece6a); }
  .trace-row.system .t-kind { color:#bb9af7; }
  .trace-row.tool_call .t-kind { color:#e0af68; }
  .trace-row.tool_result .t-kind { color:var(--la-err,#f7768e); }
  .trace-row.reasoning .t-kind { color:#bb9af7; }
  .trace-row.open { background:#1a1f2a; }
  .trace-detail { display:none; padding:4px 12px 10px 92px; font-size:11px; color:var(--la-dim,#9aa0a6); white-space:pre-wrap; font-family:var(--la-mono,monospace); border-bottom:1px solid var(--la-line,#2a2f3a); }
  .trace-detail.open { display:block; }
  .message.error { color:var(--la-err,#f7768e); padding:8px 12px; }
`;

if (!customElements.get('session-trace')) customElements.define('session-trace', SessionTrace);

/* ---- session-rail: the session list / switcher (sidebar) ---- */

const RAIL_CSS = `
  :host { display:flex; flex-direction:column; height:100%; min-height:0; font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed); }
  .head { padding:12px; border-bottom:1px solid var(--la-line,#2a2f3a); display:flex; gap:8px; align-items:center; flex-shrink:0; }
  .brand { font-size:13px; font-weight:600; flex:1; }
  .btn-new { background:var(--la-accent,#7aa2f7); color:#0b1020; border:0; border-radius:8px; padding:6px 10px; font-size:12px; font-weight:600; cursor:pointer; }
  .list { flex:1; overflow-y:auto; padding:8px; min-height:0; }
  .sess-item {
    padding:8px 10px; border-radius:8px; font-size:12px; color:var(--la-dim,#9aa0a6);
    cursor:pointer; margin-bottom:4px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;
  }
  .sess-item:hover { background:#1a1f2a; color:var(--la-ink,#e8eaed); }
  .sess-item.active { background:#1a1f2a; color:var(--la-ink,#e8eaed); border:1px solid var(--la-line,#2a2f3a); }
`;

class SessionRail extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._current = '';
    this._offs = [];
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = RAIL_CSS;
    root.appendChild(style);
    const head = document.createElement('div');
    head.className = 'head';
    const brand = document.createElement('span');
    brand.className = 'brand';
    brand.textContent = 'lite agent';
    const btnNew = document.createElement('button');
    btnNew.className = 'btn-new';
    btnNew.title = 'New chat';
    btnNew.textContent = '＋';
    btnNew.onclick = () => this.newSession();
    head.appendChild(brand); head.appendChild(btnNew);
    root.appendChild(head);
    const list = document.createElement('div');
    list.className = 'list';
    root.appendChild(list);
    this._offs.push(LiteAgent.on('status', st => {
      const s = (st && st.status) || '';
      if (s === 'idle' || String(s).indexOf('error:') === 0) this.loadSessions();
    }));
    this._offs.push(LiteAgent.onSessionChange(id => this.markActive(id)));
    await this.loadSessions();
  }
  disconnectedCallback() {
    this._offs.forEach(off => off());
    this._offs = [];
    this._root = null;
  }
  async loadSessions() {
    if (!this._root) return;
    try {
      const b = await fetch('/api/sessions').then(r => r.json());
      this.render(b.current || '', b.sessions || []);
    } catch (e) { /* rail is best-effort */ }
  }
  render(cur, list) {
    if (!this._root) return;
    this._current = cur || '';
    const el = this._root.querySelector('.list');
    el.innerHTML = '';
    if (!list.length) {
      const empty = document.createElement('div');
      empty.className = 'sess-item';
      empty.textContent = 'No sessions yet';
      el.appendChild(empty);
      return;
    }
    list.forEach(s => {
      const id = s.id || '';
      const title = s.title || id;
      const item = document.createElement('div');
      item.className = 'sess-item' + (id === this._current ? ' active' : '');
      item.textContent = title;
      item.title = id;
      item.onclick = () => this.select(id);
      el.appendChild(item);
    });
  }
  markActive(id) {
    this._current = id || '';
    if (!this._root) return;
    this._root.querySelectorAll('.sess-item').forEach(el => {
      el.classList.toggle('active', el.title === this._current);
    });
  }
  async select(id) {
    const b = await fetch('/api/session/select', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sessionId: id })
    }).then(r => r.json());
    if (b && b.ok === false) return;
    // Announce the switch; the shell reloads the chat face for this session.
    window.__liteSessionId = id;
    LiteAgent.emit('__session', id);
    this.loadSessions();
  }
  async newSession() {
    const b = await fetch('/api/session/new', { method: 'POST' }).then(r => r.json());
    if (!b || b.ok === false) return;
    window.__liteSessionId = b.sessionId || '';
    LiteAgent.emit('__session', b.sessionId || '');
    this.loadSessions();
  }
}

if (!customElements.get('session-rail')) customElements.define('session-rail', SessionRail);
