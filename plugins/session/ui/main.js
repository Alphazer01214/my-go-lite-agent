/* session plugin UI Entry (ADR-0011): the Session Log's Web faces.
 *
 * session-view  — the primary Session Log view (main chat surface): replays
 *                 history facts, consumes the Presentation stream live, and
 *                 renders the dsh-style disclosure flow. Rendering semantics
 *                 match the CLI Medium (markdown_text / message_text /
 *                 summary_text / stream).
 * session-rail  — the session list / switcher (sidebar). Switching is the
 *                 Current Session on the session Capability (ADR-0012); the
 *                 component announces the switch over __session so the view
 *                 reloads.
 * session-trace — the Session Log's trace projection (main page trace column
 *                 and the /trace debug page).
 *
 * Facts come from the session Capability through the star route
 * (LiteAgent.call('session','query')), never from dedicated endpoints.
 * Shell coordination rides private topics: __session (switches),
 * __notice (loader diagnostics → view).
 */
import { esc, md, renderMath } from '/app/md.js';
import { mergeReasoningFacts } from '/app/facts.js';

const POLL_MS = 2000;

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
  if (t === 'turn_end') {
    const reason = (f.meta && f.meta.reason) || '';
    const err = (f.meta && f.meta.error) || '';
    return err ? ('turn end · ' + reason + ' · ' + String(err).slice(0, 60)) : ('turn end · ' + reason);
  }
  if (t === 'llm_usage') {
    const u = (f.meta && f.meta.usage) || {};
    const tok = u.total_tokens != null ? u.total_tokens : (u.totalTokens != null ? u.totalTokens : '');
    return tok !== '' ? ('llm usage · tokens=' + tok) : 'llm usage';
  }
  if (t === 'context_summary') return 'context summary · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 60);
  return t;
}

class SessionTrace extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._viewSid = '';
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
    if (String(this._viewSid || '') !== sid) return;
    this.appendFact(f);
  }
  async refresh() {
    if (!this._root) return;
    try {
      // Current Session is a session Capability fact (ADR-0012).
      const cur = await LiteAgent.call('session', 'current', {});
      const curId = (cur && cur.ok !== false && cur.result && cur.result.sessionId) || '';
      this._viewSid = curId !== '' ? curId : (window.__liteSessionId || '');
      const res = await LiteAgent.call('session', 'query', { sessionId: this._viewSid, afterSeq: 0, limit: 0 });
      if (!res || res.ok === false) throw new Error(res && res.error || 'session.query failed');
      const facts = mergeReasoningFacts((res.result && res.result.facts) || []);
      this.render(facts);
    } catch (e) {
      if (this._root) this.renderError(e);
    }
  }
  render(facts) {
    if (!this._root) return;
    const list = this._root.querySelector('.list');
    // Preserve expanded rows across the 2s rebuild (otherwise open state collapses).
    const open = new Set();
    list.querySelectorAll('.trace-row.open').forEach(r => open.add(String(r.dataset.seq)));
    const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 48;
    list.innerHTML = '';
    facts.forEach(f => this.appendFact(f, open));
    if (nearBottom) list.scrollTop = list.scrollHeight;
  }
  appendFact(f, openSet) {
    if (!f || !f.type) return;
    const list = this._root.querySelector('.list');
    const t = f.type;
    const role = f.role || '';
    const cls = t === 'message' && role ? role : t;
    const row = document.createElement('div');
    row.className = 'trace-row ' + cls;
    row.dataset.seq = String(f.seq != null ? f.seq : '');
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
    if (openSet && openSet.has(row.dataset.seq)) {
      row.classList.add('open');
      det.classList.add('open');
    }
    list.appendChild(row); list.appendChild(det);
    // Live append during a run may stick to bottom; full render uses nearBottom in render().
    if (!openSet) list.scrollTop = list.scrollHeight;
  }
  renderError(e) {
    const list = this._root.querySelector('.list');
    const el = document.createElement('div');
    el.className = 'message error';
    el.textContent = 'trace unavailable: ' + e;
    list.appendChild(el);
  }
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
      // Current Session lives on the session Capability (ADR-0012).
      const curRes = await LiteAgent.call('session', 'current', {});
      const listRes = await LiteAgent.call('session', 'list', {});
      const cur = (curRes && curRes.ok !== false && curRes.result && curRes.result.sessionId) || window.__liteSessionId || '';
      const list = (listRes && listRes.ok !== false && listRes.result && listRes.result.sessions) || [];
      this.render(cur || '', list);
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
    const b = await LiteAgent.call('session', 'select', { sessionId: id });
    if (!b || b.ok === false) return;
    // Announce the switch; the session-view reloads its history for this session.
    window.__liteSessionId = id;
    LiteAgent.emit('__session', id);
    this.loadSessions();
  }
  async newSession() {
    // create mints a fresh id and selects it as Current Session (ADR-0012).
    const b = await LiteAgent.call('session', 'create', {});
    if (!b || b.ok === false) return;
    const id = (b.result && b.result.sessionId) || '';
    window.__liteSessionId = id;
    LiteAgent.emit('__session', id);
    this.loadSessions();
  }
}

if (!customElements.get('session-rail')) customElements.define('session-rail', SessionRail);

/* ---- session-view: the primary Session Log view (main chat surface) ---- */

const VIEW_CSS = `
  :host { display:flex; flex-direction:column; min-height:0; height:100%; overflow:hidden;
          font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed); }
  .flow { flex:1; min-height:0; overflow-y:auto; overflow-x:hidden; padding:16px 20px 8px; }
  .msg { margin:0 0 14px; max-width:52rem; }
  .msg.user { color:var(--la-ink,#e8eaed); background:var(--la-panel2,#12161f); border-radius:10px; padding:10px 12px; }
  .msg.user:before { content:"you"; display:block; color:var(--la-dim,#9aa0a6); font-size:11px; margin-bottom:4px; }
  .msg.assistant { line-height:1.55; }
  .msg.assistant pre { background:#0b0e14; border:1px solid var(--la-line,#2a2f3a); border-radius:8px; padding:10px; overflow:auto; font-family:var(--la-mono,monospace); font-size:13px; }
  .msg.assistant code { font-family:var(--la-mono,monospace); font-size:0.92em; background:#1a1f2a; padding:1px 4px; border-radius:4px; }
  .msg.assistant pre code { background:none; padding:0; }
  .msg.assistant h1,.msg.assistant h2,.msg.assistant h3 { margin:0.6em 0 0.35em; line-height:1.25; }
  .msg.assistant ul { margin:0.3em 0; padding-left:1.2em; }
  .msg.assistant blockquote { border-left:3px solid var(--la-line,#2a2f3a); margin:0.4em 0; padding:0.1em 0 0.1em 0.8em; color:var(--la-dim,#9aa0a6); }
  /* dsh-style disclosure rows: everything except user + final assistant is folded */
  .disc { margin:0 0 8px; border:1px solid var(--la-line,#2a2f3a); border-radius:8px; background:var(--la-panel2,#12161f); overflow:hidden; }
  .disc>summary { list-style:none; cursor:pointer; padding:7px 10px; font-size:12px; color:var(--la-dim,#9aa0a6);
    display:flex; gap:8px; align-items:center; user-select:none; }
  .disc>summary::-webkit-details-marker { display:none; }
  .disc>summary:before { content:"▸"; font-size:10px; color:var(--la-dim,#9aa0a6); }
  .disc[open]>summary:before { content:"▾"; }
  .disc>summary .s-title { color:var(--la-ink,#e8eaed); font-weight:600; }
  .disc>summary .s-meta { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-family:var(--la-mono,monospace); font-size:11px; }
  .disc>.disc-body { padding:0 10px 10px 26px; font-size:12px; color:var(--la-dim,#9aa0a6); white-space:pre-wrap; font-family:var(--la-mono,monospace); }
  .disc.think>summary:before { content:"▸"; }
  .disc.tool>summary .s-title { color:var(--la-accent,#7aa2f7); }
  .disc.live { border-color:#334; }
  .disc.live>summary:before { content:"◉"; color:var(--la-accent,#7aa2f7); }
  .message { font-size:12px; margin:0 0 8px; color:var(--la-dim,#9aa0a6); }
  .message.error { color:var(--la-err,#f7768e); }
  .message.warn { color:#e0af68; }
  .progress { font-size:12px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); margin-bottom:8px; }
  .composer { display:flex; gap:8px; padding:12px 16px; border-top:1px solid var(--la-line,#2a2f3a);
    background:var(--la-panel,#161a22); flex-shrink:0; align-items:flex-end; }
  .input { flex:1; background:var(--la-bg,#0f1115); border:1px solid var(--la-line,#2a2f3a); color:var(--la-ink,#e8eaed);
    border-radius:10px; padding:10px 12px; font:inherit; resize:none; min-height:44px; max-height:160px; }
  .btn-send { background:var(--la-accent,#7aa2f7); color:#0b1020; border:0; border-radius:10px;
    padding:0 18px; height:44px; font-weight:700; cursor:pointer; font-size:13px; min-width:72px; }
  .btn-send.running { background:var(--la-stop,#f7768e); color:#fff; }
  .usage-bar {
    flex-shrink:0; padding:4px 16px; font-size:11px; color:var(--la-dim,#9aa0a6);
    font-family:var(--la-mono,monospace); border-top:1px solid var(--la-line,#2a2f3a);
    background:var(--la-panel,#161a22); display:flex; gap:10px; align-items:center;
    min-height:22px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis;
  }
  .usage-bar .u-label { color:var(--la-dim,#9aa0a6); opacity:.75; }
  .usage-bar .u-val { color:var(--la-ink,#e8eaed); }
  .usage-bar .u-src { color:var(--la-accent,#7aa2f7); }
`;

class SessionView extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._sid = '';
    this._running = false;
    this._streamBuf = '';
    this._reasoningBuf = '';
    this._progressEl = null;
    this._thinkingEl = null;
    this._liveEl = null;
    this._liveBody = null;
    this._liveMeta = null;
    this._ready = false;
    this._queue = [];
    this._offs = [];
  }
  connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = VIEW_CSS;
    root.appendChild(style);
    const flow = document.createElement('div');
    flow.className = 'flow';
    root.appendChild(flow);
    // Per-session Context Usage strip (CONTEXT.md Context Usage).
    const usageBar = document.createElement('div');
    usageBar.className = 'usage-bar';
    usageBar.innerHTML = '<span class="u-label">context</span><span class="u-val">—</span>';
    root.appendChild(usageBar);
    this._usageEl = usageBar.querySelector('.u-val');
    this._usageBar = usageBar;
    // Composer (ADR-0011 ticket 08): the input belongs to the view, not the layout.
    const composer = document.createElement('div');
    composer.className = 'composer';
    const input = document.createElement('textarea');
    input.className = 'input';
    input.rows = 2;
    input.placeholder = 'Message…  / for commands';
    const btnSend = document.createElement('button');
    btnSend.className = 'btn-send';
    btnSend.title = 'Send';
    btnSend.textContent = 'Send';
    composer.appendChild(input); composer.appendChild(btnSend);
    root.appendChild(composer);
    this._input = input;
    this._btnSend = btnSend;
    btnSend.onclick = () => this.sendOrStop();
    input.addEventListener('keydown', e => {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.sendOrStop(); }
    });
    this._offs.push(LiteAgent.on('presentation', d => this.onPresentation(d)));
    this._offs.push(LiteAgent.on('stream', d => this.onStreamEvent(d)));
    this._offs.push(LiteAgent.on('status', d => this.onStatusEvent(d)));
    this._offs.push(LiteAgent.on('session', d => this.onSessionFact(d)));
    this._offs.push(LiteAgent.on('__notice', d => this.onNotice(d)));
    this._offs.push(LiteAgent.onSessionChange(sid => this.onSessionChange(sid)));
    this.reload();
  }
  disconnectedCallback() {
    this._offs.forEach(off => off());
    this._offs = [];
    this._root = null;
    this._input = null;
    this._btnSend = null;
    this._usageEl = null;
    this._usageBar = null;
  }
  isCurrent(sid) {
    // Missing sessionId means the default Session (""), not "any session".
    if (sid === undefined || sid === null) return !this._sid;
    return String(sid) === String(this._sid || '');
  }
  // Presentation/stream events that fire before history lands are queued and
  // replayed — the old shell gate (history-first, then SSE) becomes local.
  queueOr(fn) {
    if (this._ready) fn();
    else this._queue.push(fn);
  }
  async reload() {
    this._ready = false;
    this._queue = [];
    if (this._root) this._root.querySelector('.flow').innerHTML = '';
    try {
      // Prefer Current Session from the capability; fall back to local id.
      const cur = await LiteAgent.call('session', 'current', {});
      if (cur && cur.ok !== false && cur.result && cur.result.sessionId) {
        this._sid = cur.result.sessionId;
      }
      const res = await LiteAgent.call('session', 'query', { sessionId: this._sid, afterSeq: 0, limit: 0 });
      if (!res || res.ok === false) throw new Error(res && res.error || 'session.query failed');
      // Prefer Session Log facts so Thinking/tools survive refresh & session switch.
      const facts = mergeReasoningFacts((res.result && res.result.facts) || []);
      if (this._root) {
        const flow = this._root.querySelector('.flow');
        flow.innerHTML = '';
        let lastUsageFact = null;
        facts.forEach(f => {
          this.renderHistoryFact(f);
          if (f.type === 'llm_usage') lastUsageFact = f;
        });
        this.scroll(true);
        if (lastUsageFact) this.applyUsageFromFact(lastUsageFact);
      }
      this._ready = true;
      this.maybeResumeLiveThinking();
      this.refreshUsage();
    } catch (e) {
      this._ready = true;
      this.appendPre('history unavailable: ' + e, 'message error');
    }
  }
  formatUsage(u) {
    if (!u) return '—';
    const tok = u.estimatedTokens != null ? u.estimatedTokens : '';
    const chars = u.chars != null ? u.chars : '';
    const n = u.messageCount != null ? u.messageCount : '';
    const src = u.source || 'chars';
    const parts = [];
    if (tok !== '') parts.push('≈' + tok + ' tok');
    if (chars !== '') parts.push(chars + ' chars');
    if (n !== '') parts.push(n + ' msgs');
    if (!parts.length) return '—';
    return parts.join(' · ') + ' · ' + src;
  }
  setUsageBar(u) {
    if (!this._usageEl) return;
    this._usageEl.textContent = this.formatUsage(u);
    this._usageEl.className = 'u-val' + (u && u.source === 'provider' ? ' u-src' : '');
  }
  async refreshUsage() {
    if (!this._root) return;
    try {
      const res = await LiteAgent.call('context', 'usage', { sessionId: this._sid || '' });
      const u = (res && res.ok !== false && res.result && res.result.usage) || null;
      this.setUsageBar(u);
    } catch (e) { /* usage strip is best-effort */ }
  }
  applyUsageFromFact(f) {
    const mu = f && f.meta && f.meta.usage;
    if (!mu || !this._root) return;
    const tok = mu.total_tokens != null ? mu.total_tokens : mu.totalTokens;
    this.setUsageBar({
      estimatedTokens: tok != null ? tok : null,
      source: tok != null ? 'provider' : 'chars',
    });
  }
  onSessionFact(f) {
    if (!f || !f.type) return;
    if (!this.isCurrent(f.sessionId)) return;
    if (f.type === 'llm_usage') this.applyUsageFromFact(f);
    if (f.type === 'turn_end') this.refreshUsage();
  }
  renderHistoryFact(f) {
    if (!f || !f.type) return;
    const t = f.type;
    if (t === 'message') {
      if (f.role === 'user' && f.content) this.appendUser(f.content);
      else if (f.role === 'assistant' && f.content) this.appendAssistant(md(f.content));
      return;
    }
    if (t === 'reasoning') {
      const body = f.content || '';
      if (!body) return;
      const step = (f.meta && f.meta.step) || '';
      const row = this.makeDisc('think', 'Thinking', step ? 'step ' + step : '', body, false);
      this._root.querySelector('.flow').appendChild(row);
      return;
    }
    if (t === 'tool_call') {
      const names = ((f.meta && f.meta.tool_calls) || []).map(tc => tc.name).join(',');
      const card = this.makeDisc('tool', '⏺ ' + (names || 'tool'), '', f.content || names || '', false);
      this._root.querySelector('.flow').appendChild(card);
      return;
    }
    if (t === 'tool_result') {
      const res = this.makeDisc('tool', '⏺ result', '', f.content || '', false);
      this._root.querySelector('.flow').appendChild(res);
    }
  }
  // Mid-run refresh: resume live Thinking from rebuilt facts.
  maybeResumeLiveThinking() {
    // Host turn status is still medium-level (running/idle); facts rebuild from Log.
    fetch('/api/session').then(r => r.json()).then(b => {
      if (b.status !== 'running' || this._thinkingEl || this._liveEl) return;
      const thinks = this._root.querySelectorAll('details.think:not(.live) .disc-body');
      if (thinks.length) this._reasoningBuf = thinks[thinks.length - 1].textContent || '';
    }).catch(() => { });
  }
  sendOrStop() {
    if (!this._input) return;
    if (this._running) {
      fetch('/api/turn/cancel', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sessionId: this._sid }) })
        .then(() => { this._running = false; this.setSendState(false); });
      return;
    }
    const text = this._input.value.trim();
    if (!text) return;
    this._input.value = '';
    if (text.charAt(0) === '/') {
      this.appendUser(text);
      LiteAgent.runCommand(text).then(body => {
        if (body.error) this.appendPre(body.error, 'message error');
        else if (body.output) this.appendPre(body.output, 'message');
        // /refresh rescans manifests server-side; module identities can't be
        // swapped in place — the Session Log is the truth, so reload (ADR-0010).
        const cmd = text.replace(/^\//, '').split(/\s+/)[0];
        if (cmd === 'refresh') location.reload();
      });
      return;
    }
    this.appendUser(text);
    this.beginTurn();
    this._running = true;
    this.setSendState(true);
    LiteAgent.sendMessage(text, this._sid).then(b => {
      if (b && b.ok === false) {
        this._running = false;
        this.setSendState(false);
        this.appendPre(b.error || 'send failed', 'message error');
      }
    }).catch(() => { this._running = false; this.setSendState(false); });
  }
  setSendState(running) {
    if (!this._btnSend) return;
    this._btnSend.classList.toggle('running', running);
    this._btnSend.textContent = running ? 'Stop' : 'Send';
    this._btnSend.title = running ? 'Stop generation' : 'Send';
  }
  onNotice(d) {
    this.queueOr(() => this.appendPre((d && d.text) || '', (d && d.cls) || 'message'));
  }
  onSessionChange(sid) {
    sid = sid || '';
    if (String(this._sid) === String(sid)) return;
    this._sid = sid;
    this._running = false;
    this.setSendState(false);
    this.setUsageBar(null);
    this.reload();
  }
  onStreamEvent(d) {
    if (!this.isCurrent(d.sessionId)) return;
    this.queueOr(() => {
      if (d.channel === 'reasoning') {
        this._reasoningBuf += d.delta || '';
        this.setThinkingText(this._reasoningBuf);
      } else {
        this._streamBuf += d.delta || '';
        this.setLiveText(this._streamBuf);
      }
    });
  }
  onStatusEvent(d) {
    const st = d.status || '';
    const sid = d.sessionId !== undefined ? d.sessionId : '';
    this.queueOr(() => {
      if (st === 'running') {
        if (this.isCurrent(sid)) { this._running = true; this.setSendState(true); }
        return;
      }
      if (String(st).indexOf('error:') === 0) {
        if (this.isCurrent(sid)) {
          this._running = false;
          this.setSendState(false);
          this.clearTurnUI();
          this.appendPre(st.slice('error:'.length).trim() || 'turn failed', 'message error');
          this.refreshUsage();
        }
        return;
      }
      if (st === 'idle' || st === 'cancelling') {
        if (this.isCurrent(sid)) {
          this._running = false;
          this.setSendState(false);
          this.clearTurnUI();
          this.refreshUsage();
        }
      }
    });
  }
  onPresentation(raw) {
    this.queueOr(() => {
      this.clearProgress();
      const data = raw && raw.kind !== undefined ? raw : (raw && raw.data) || raw || {};
      const kind = data.kind || data.Kind;
      const text = data.text !== undefined ? data.text : data.Text;
      const sid = data.sessionId !== undefined ? data.sessionId : data.SessionID;
      // Host Loop always tags sessionId ("" = default). Ignore foreign Session paints.
      if (sid !== undefined && sid !== null && !this.isCurrent(sid)) return;
      if (kind === 'markdown_text') {
        // Final assistant body is always expanded. Drop live stream if it was the same answer.
        const settled = (text || '').trim();
        if (this._streamBuf && settled && this._streamBuf.replace(/\s+/g, ' ').indexOf(settled.slice(0, 40).replace(/\s+/g, ' ')) >= 0) {
          this.removeLive(); // stream was the answer — avoid double print
        } else {
          this.freezeLive(); // keep as Think disclosure
        }
        this.freezeThinking();
        this.appendAssistant(md(text || ''));
        return;
      }
      if (kind === 'message_text') {
        const level = data.level || data.Level || '';
        // Non-final status: fold as dim one-liner disclosure (dsh-like).
        this.freezeLive();
        const row = this.makeDisc('', 'status', text || '', '', false);
        if (level === 'error') row.style.borderColor = 'var(--la-err,#f7768e)';
        this._root.querySelector('.flow').appendChild(row); this.scroll();
        return;
      }
      if (kind === 'summary_text') {
        this.freezeLive();
        const pairsArr = data.pairs || data.Pairs || [];
        const title = data.title !== undefined ? data.title : data.Title;
        const detail = data.detail !== undefined ? data.detail : data.Detail;
        const meta = pairsArr.map(p => {
          const k = p.key !== undefined ? p.key : p.Key;
          const v = p.value !== undefined ? p.value : p.Value;
          return k + '=' + String(v).slice(0, 40);
        }).join('  ');
        let body = pairsArr.map(p => {
          const k = p.key !== undefined ? p.key : p.Key;
          const v = p.value !== undefined ? p.value : p.Value;
          return k + ': ' + v;
        }).join('\n');
        if (detail) body += (body ? '\n\n' : '') + detail;
        const card = this.makeDisc('tool', '⏺ ' + (title || 'tool'), meta, body, false);
        this._root.querySelector('.flow').appendChild(card); this.scroll();
      }
    });
  }
  nearBottom() {
    const flow = this._flow();
    if (!flow) return true;
    return flow.scrollHeight - flow.scrollTop - flow.clientHeight < 56;
  }
  // Stick to bottom only when the user is already near it (don't yank mid-read).
  scroll(force) {
    const flow = this._flow();
    if (!flow) return;
    if (force || this.nearBottom()) flow.scrollTop = flow.scrollHeight;
  }
  _flow() {
    return this._root && this._root.querySelector('.flow');
  }
  appendAssistant(html) {
    const el = document.createElement('div');
    el.className = 'msg assistant';
    el.innerHTML = html || '';
    this._root.querySelector('.flow').appendChild(el); renderMath(el); this.scroll(true); return el;
  }
  appendUser(text) {
    const el = document.createElement('div');
    el.className = 'msg user';
    el.textContent = text;
    this._root.querySelector('.flow').appendChild(el); this.scroll(true);
  }
  appendPre(text, cls) {
    const el = document.createElement('pre');
    el.className = cls || 'message';
    el.style.cssText = 'white-space:pre-wrap;font-family:var(--la-mono,monospace);font-size:12px';
    el.textContent = text;
    this._root.querySelector('.flow').appendChild(el); this.scroll(true);
  }
  clearProgress() { if (this._progressEl) { this._progressEl.remove(); this._progressEl = null; } }
  clearThinking() {
    if (this._thinkingEl) { this._thinkingEl.remove(); this._thinkingEl = null; }
    this._reasoningBuf = '';
  }
  makeDisc(cls, title, meta, bodyText, open) {
    const d = document.createElement('details');
    d.className = 'disc ' + (cls || '');
    if (open) d.open = true;
    const sum = document.createElement('summary');
    const t = document.createElement('span');
    t.className = 's-title';
    t.textContent = title;
    const m = document.createElement('span');
    m.className = 's-meta';
    m.textContent = meta || '';
    sum.appendChild(t); sum.appendChild(m);
    const body = document.createElement('div');
    body.className = 'disc-body';
    body.textContent = bodyText || '';
    d.appendChild(sum); d.appendChild(body);
    return d;
  }
  setThinkingText(t) {
    if (!this._thinkingEl) {
      this._thinkingEl = this.makeDisc('think live', 'Thinking', '', '', true);
      this._root.querySelector('.flow').appendChild(this._thinkingEl);
    }
    const body = this._thinkingEl.querySelector('.disc-body');
    const meta = this._thinkingEl.querySelector('.s-meta');
    if (body) body.textContent = t;
    if (meta) meta.textContent = t.length > 60 ? t.slice(0, 60) + '…' : t;
    this.scroll();
  }
  freezeThinking() {
    if (this._thinkingEl) {
      this._thinkingEl.classList.remove('live');
      this._thinkingEl.open = false;
      this._thinkingEl = null;
    }
    this._reasoningBuf = '';
  }
  ensureLive() {
    if (this._thinkingEl) this.freezeThinking();
    if (!this._liveEl) {
      this._liveEl = this.makeDisc('think live', 'Answer', '', '', true);
      this._root.querySelector('.flow').appendChild(this._liveEl);
      this._liveBody = this._liveEl.querySelector('.disc-body');
      this._liveMeta = this._liveEl.querySelector('.s-meta');
    }
    return this._liveEl;
  }
  setLiveText(t) {
    this.ensureLive();
    if (this._liveBody) this._liveBody.textContent = t;
    if (this._liveMeta) this._liveMeta.textContent = (t.length > 60 ? t.slice(0, 60) + '…' : t);
    this.scroll();
  }
  removeLive() {
    if (this._liveEl) { this._liveEl.remove(); this._liveEl = null; this._liveBody = null; this._liveMeta = null; }
    this._streamBuf = '';
  }
  freezeLive() {
    if (this._liveEl && this._streamBuf) {
      this._liveEl.classList.remove('live');
      this._liveEl.open = false;
      this._liveEl = null; this._liveBody = null; this._liveMeta = null;
      this._streamBuf = '';
      return;
    }
    this.removeLive();
    this.clearThinking();
  }
  clearTurnUI() {
    this.clearProgress(); this.clearThinking();
    if (this._liveEl) this.freezeLive();
    else this.removeLive();
  }
  beginTurn() {
    this._streamBuf = '';
    if (this._liveEl) { this._liveEl.remove(); this._liveEl = null; }
    this.clearThinking(); this.clearProgress();
  }
}

if (!customElements.get('session-view')) customElements.define('session-view', SessionView);
