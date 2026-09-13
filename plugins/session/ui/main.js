/* session plugin UI Entry (ADR-0011): the Session Log's trace view.
 *
 * session-trace renders the current Session's facts — the same projection
 * the Shell's builtin trace column and /trace page used to render. Facts
 * come from the session Capability through the star route
 * (LiteAgent.call('session','query')), never from a dedicated endpoint;
 * live updates arrive over the session SSE topic.
 *
 * Mounted on the main page's trace column and the /trace debug page.
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
