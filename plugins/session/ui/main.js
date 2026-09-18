/* session plugin UI Entry (ADR-0011): the Session Log's Web faces.
 *
 * session-rail  — the session list / switcher (Shell region `left`), grouped
 *                 by Workspace path (ADR-0020). Switching is the Current
 *                 Session on the session Capability (ADR-0012); announces
 *                 over __session / __new.
 * session-workspace — composite Panel Component for Shell region `center`
 *                 (ADR-0031). Owns chat | trace geometry only; rail lives in
 *                 region `left`. The Shell hosts generic regions only.
 * session-view  — the primary Session Log view (chat pane inside workspace):
 *                 replays history facts, consumes the Presentation stream
 *                 live, and renders the dsh-style disclosure flow. Rendering
 *                 semantics match the CLI Medium (markdown_text / message_text /
 *                 summary_text / stream). It also owns the new-session face:
 *                 at boot (and on "＋") it shows Workspace picker + Agent
 *                 Scheme + composer instead of loading a Session — a Session
 *                 is only loaded once the user picks one or sends a message.
 * session-trace — the Session Log's trace projection (trace pane inside
 *                 workspace). Owns a type-filter bar inside the component
 *                 itself (display kind = message role, or fact type).
 * session-status— chip in Shell region `bottom` (Workspace + Current Session
 *                 + fact count).
 *
 * Facts come from the session Capability through the star route
 * (LiteAgent.call('session','query')). Turn status and cancel are the
 * medium's own endpoints (/api/session, /api/turn/cancel) — the author SDK
 * does not surface them yet. An adaptive backstop (2s while a turn is live,
 * 8s idle; paused when the tab is hidden) covers missed SSE events
 * (incremental; skips rebuild while text is selected). Shell coordination
 * rides private topics: __session (switches), __new (new-session face),
 * __notice (loader diagnostics).
 */
import { esc, md } from './lib/md.js';
import { mergeReasoningFacts } from './lib/facts.js';
import './settings.js';

// Scheme display vocabulary (kept in sync with plugins/agent/ui/main.js):
// a friendly alias with the raw id beside it, so welcome chips, the mode
// menu and Settings all say the same thing (BUG-10).
const SCHEME_LABELS = { chat: '极简', tool_calling: '标准', coding: '编码' };
function schemeLabel(id) {
  return SCHEME_LABELS[id] || id || '—';
}

// SSE is the live path; this timer only backstops missed events.
// Fast while a turn is running, slower when idle; pause when tab is hidden.
const POLL_MS_ACTIVE = 2000;
const POLL_MS_IDLE = 8000;

function fmtTs(ts) {
  const n = Number(ts);
  if (!n || !isFinite(n)) return '';
  const d = new Date(n);
  const pad = (x) => String(x).padStart(2, '0');
  return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

// Trace display kind: message facts filter by role, everything else by type.
function factKind(f) {
  const t = (f && f.type) || '';
  if (t === 'message') return (f && f.role) || 'msg';
  return t;
}

// Preferred chip order; unknown kinds append in first-seen order.
const TRACE_KIND_ORDER = [
  'user', 'assistant', 'system', 'msg',
  'reasoning',
  'tool_call', 'tool_result',
  'request_header',
  'step_start', 'step_end',
  'turn_start', 'turn_end',
  'llm_usage',
  'context_summary',
  'todo'
];

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
    this._lastSeq = -1;
    this._lastCount = -1;
    this._booted = false;
    // The Shell opens on the new-session face; the trace column stays empty
    // until a Session is actually entered (__session).
    this._active = false;
    // Type filter: null = show all; Set of display kinds = show only those.
    this._filter = null;
  }
  async connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    this._root = root;
    const style = document.createElement('style');
    style.textContent = CSS;
    root.appendChild(style);
    const bar = document.createElement('div');
    bar.className = 'toolbar';
    bar.hidden = true;
    root.appendChild(bar);
    const list = document.createElement('div');
    list.className = 'list';
    root.appendChild(list);
    this.renderIdle();
    // Announcement-driven (not onSessionChange): at boot nothing is "current".
    this._offSession = LiteAgent.on('__session', () => { this._active = true; this.refresh(true); });
    this._offNew = LiteAgent.on('__new', () => {
      this._active = false;
      this._viewSid = '';
      this._booted = false;
      this._filter = null;
      this.renderIdle();
    });
    this._offFacts = LiteAgent.on('session', f => this.onFact(f));
    // SSE is the live path; this timer only backstops missed events.
    this._busy = false;
    this._offsStatus = [];
    this._offsStatus.push(LiteAgent.on('status', st => {
      const s = (st && st.status) || '';
      this._busy = (s && s !== 'idle' && String(s).indexOf('error:') !== 0);
    }));
    this._offsStatus.push(LiteAgent.on('stream', () => { this._busy = true; }));
    this._offsStatus.push(LiteAgent.on('presentation', () => { this._busy = true; }));
    this._armPoll();
  }
  _armPoll() {
    if (this._timer) { clearTimeout(this._timer); this._timer = null; }
    if (!this._root) return;
    const delay = this._busy ? POLL_MS_ACTIVE : POLL_MS_IDLE;
    this._timer = setTimeout(() => {
      this._timer = null;
      if (!this._root) return;
      if (!document.hidden) this.refresh(false);
      this._armPoll();
    }, delay);
  }
  disconnectedCallback() {
    if (this._offSession) this._offSession();
    if (this._offNew) this._offNew();
    if (this._offFacts) this._offFacts();
    if (this._offsStatus) { this._offsStatus.forEach(off => off()); this._offsStatus = []; }
    if (this._timer) { clearTimeout(this._timer); this._timer = null; }
    this._root = null;
  }
  renderIdle() {
    if (!this._root) return;
    const bar = this._root.querySelector('.toolbar');
    if (bar) { bar.innerHTML = ''; bar.hidden = true; }
    const list = this._root.querySelector('.list');
    if (list) list.innerHTML = '<div class="message">未进入会话 — 选择或新建一个会话后显示事实流。</div>';
  }
  _onFilterChip(kind) {
    if (!kind) {
      this._filter = null;
    } else if (!this._filter) {
      this._filter = new Set([kind]);
    } else if (this._filter.has(kind)) {
      this._filter.delete(kind);
      if (this._filter.size === 0) this._filter = null;
    } else {
      this._filter.add(kind);
    }
    this._applyFilter();
  }
  _kindMatches(kind) {
    return !this._filter || this._filter.has(kind);
  }
  _applyFilter() {
    if (!this._root) return;
    const list = this._root.querySelector('.list');
    if (!list) return;
    const rows = list.querySelectorAll('.trace-row');
    rows.forEach(row => {
      const show = this._kindMatches(row.dataset.kind || '');
      row.classList.toggle('filtered-out', !show);
      const det = row.nextElementSibling;
      if (det && det.classList.contains('trace-detail')) {
        det.classList.toggle('filtered-out', !show);
      }
    });
    this._ensureFilterEmpty();
    this._syncFilterBar();
  }
  _ensureFilterEmpty() {
    if (!this._root) return;
    const list = this._root.querySelector('.list');
    if (!list) return;
    const empty = list.querySelector('.filter-empty');
    const rows = list.querySelectorAll('.trace-row');
    let visible = 0;
    rows.forEach(r => { if (!r.classList.contains('filtered-out')) visible++; });
    if (rows.length && visible === 0) {
      if (!empty) {
        const el = document.createElement('div');
        el.className = 'message filter-empty';
        el.textContent = '无匹配事实 — 当前类型过滤下没有可见行。';
        list.appendChild(el);
      }
    } else if (empty) {
      empty.remove();
    }
  }
  _syncFilterBar() {
    if (!this._root) return;
    const bar = this._root.querySelector('.toolbar');
    const list = this._root.querySelector('.list');
    if (!bar || !list) return;
    const rows = list.querySelectorAll('.trace-row');
    if (!this._active || (!rows.length && !this._filter)) {
      bar.innerHTML = '';
      bar.hidden = true;
      return;
    }
    const present = new Set();
    rows.forEach(r => { if (r.dataset.kind) present.add(r.dataset.kind); });
    if (this._filter) this._filter.forEach(k => present.add(k));
    const kinds = TRACE_KIND_ORDER.filter(k => present.has(k));
    present.forEach(k => { if (kinds.indexOf(k) === -1) kinds.push(k); });
    let visible = 0;
    rows.forEach(r => { if (!r.classList.contains('filtered-out')) visible++; });
    bar.hidden = false;
    bar.innerHTML = '';
    const all = document.createElement('button');
    all.type = 'button';
    all.className = 'chip' + (!this._filter ? ' on' : '');
    all.textContent = '全部';
    all.title = '显示全部类型';
    all.onclick = () => this._onFilterChip('');
    bar.appendChild(all);
    kinds.forEach(k => {
      const on = !!(this._filter && this._filter.has(k));
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'chip' + (on ? ' on' : '');
      b.dataset.kind = k;
      b.textContent = k;
      b.title = this._filter
        ? (on ? '取消过滤 ' + k : '加入过滤 ' + k)
        : ('只看 ' + k);
      b.onclick = () => this._onFilterChip(k);
      bar.appendChild(b);
    });
    const cnt = document.createElement('span');
    cnt.className = 'count';
    cnt.textContent = this._filter ? (visible + '/' + rows.length) : String(rows.length);
    cnt.title = this._filter ? '可见 / 全部' : '事实总数';
    bar.appendChild(cnt);
  }
  // Shell slots are generic (ADR-0030/0031): do not assume slot-specific host CSS.
  // Walk light DOM and cross shadow hosts to find the real scroll container.
  _scrollParent() {
    let n = this;
    while (n) {
      if (n !== this && n.clientHeight > 0 && n.scrollHeight > n.clientHeight) {
        const oy = getComputedStyle(n).overflowY;
        if (oy === 'auto' || oy === 'scroll' || oy === 'overlay') return n;
      }
      let next = n.parentElement;
      if (!next) {
        const rn = n.getRootNode && n.getRootNode();
        next = (rn && rn.host) || null;
      }
      if (next === n) break;
      n = next;
    }
    return (this._root && this._root.querySelector('.list')) || null;
  }
  _isNearBottom() {
    const sc = this._scrollParent();
    if (!sc) return true;
    return sc.scrollHeight - sc.scrollTop - sc.clientHeight < 48;
  }
  _scrollToBottom() {
    const sc = this._scrollParent();
    if (sc) sc.scrollTop = sc.scrollHeight;
  }
  hasTextSelection() {
    const sel = this._root && this._root.getSelection ? this._root.getSelection() : window.getSelection();
    if (!sel || sel.isCollapsed || sel.rangeCount === 0) return false;
    const node = sel.anchorNode;
    return !!(node && this._root && this._root.contains(node));
  }
  onFact(f) {
    // Live append only when the fact belongs to the session being viewed.
    if (!this._root) return;
    const sid = f && f.sessionId !== undefined ? String(f.sessionId) : '';
    if (String(this._viewSid || '') !== sid) return;
    if (f && f.seq != null) {
      const n = Number(f.seq);
      if (n > this._lastSeq) this._lastSeq = n;
    }
    this.appendFact(f);
  }
  async refresh(force) {
    if (!this._root || !this._active) return;
    // Backstop only: skip while the tab is hidden (SSE still fills on focus).
    if (!force && document.hidden) return;
    // Never tear down the DOM while the user is selecting text in this panel.
    if (!force && this.hasTextSelection()) return;
    try {
      // Prefer the shell's live Current Session; only poll the capability on
      // force/first boot. Idle polls then cost one incremental query, not two
      // full round-trips (session.current + full session.query).
      const shellSid = window.__liteSessionId || '';
      let sid = this._viewSid;
      if (shellSid && String(shellSid) !== String(sid || '')) {
        sid = shellSid;
        force = true;
      } else if (force || !sid) {
        const cur = await LiteAgent.call('session', 'current', {});
        const curId = (cur && cur.ok !== false && cur.result && cur.result.sessionId) || '';
        sid = curId !== '' ? curId : shellSid;
      }
      const prevSid = this._viewSid;
      const switched = sid !== '' && String(prevSid || '') !== String(sid);
      if (switched) force = true;
      this._viewSid = sid;
      // Incremental backstop: pull only facts after lastSeq unless rebuilding.
      const afterSeq = (!force && this._booted) ? this._lastSeq : 0;
      const res = await LiteAgent.call('session', 'query', {
        sessionId: this._viewSid,
        afterSeq: afterSeq,
        limit: 0
      });
      if (!res || res.ok === false) throw new Error(res && res.error || 'session.query failed');
      const facts = mergeReasoningFacts((res.result && res.result.facts) || []);
      // Nothing new → leave the DOM (and selection) alone.
      if (!force && this._booted && !switched && facts.length === 0) {
        return;
      }
      if (force || switched || !this._booted) {
        this.render(facts);
        this._lastCount = facts.length;
      } else {
        // Append only facts the live path may have missed.
        const fresh = facts.filter(f => Number(f.seq || 0) > this._lastSeq);
        fresh.forEach(f => this.appendFact(f));
        this._lastCount += fresh.length;
      }
      const lastSeq = facts.length ? Number(facts[facts.length - 1].seq || 0) : -1;
      this._lastSeq = Math.max(this._lastSeq, lastSeq);
      this._booted = true;
    } catch (e) {
      if (this._root) this.renderError(e);
    }
  }
  render(facts) {
    if (!this._root) return;
    const list = this._root.querySelector('.list');
    // Preserve expanded rows across rebuilds (otherwise open state collapses).
    const open = new Set();
    list.querySelectorAll('.trace-row.open').forEach(r => open.add(String(r.dataset.seq)));
    const nearBottom = this._isNearBottom();
    list.innerHTML = '';
    this._lastSeq = -1;
    facts.forEach(f => this.appendFact(f, open));
    if (facts.length) this._lastSeq = Number(facts[facts.length - 1].seq || 0);
    this._applyFilter();
    if (nearBottom) this._scrollToBottom();
  }
  appendFact(f, openSet) {
    if (!f || !f.type) return;
    const list = this._root.querySelector('.list');
    const t = f.type;
    const role = f.role || '';
    const cls = t === 'message' && role ? role : t;
    const kind = factKind(f);
    const row = document.createElement('div');
    row.className = 'trace-row ' + cls;
    row.dataset.seq = String(f.seq != null ? f.seq : '');
    row.dataset.kind = kind;
    const sid = f.sessionId !== undefined && f.sessionId !== null ? String(f.sessionId) : '';
    const short = sid ? (sid.length > 8 ? sid.slice(0, 8) : sid) : '·';
    const ts = f.ts ? fmtTs(f.ts) : '';
    row.innerHTML = '<span class="t-kind">' + esc(kind) + '</span><span class="t-sum">' + esc(summary(f)) + '</span>'
      + (ts ? '<span class="t-ts">' + esc(ts) + '</span>' : '')
      + '<span class="t-seq">' + esc(short) + ' #' + (f.seq || '') + '</span>';
    const det = document.createElement('div');
    det.className = 'trace-detail';
    let detail = f.content || '';
    if (f.ts) detail = 'ts: ' + new Date(Number(f.ts)).toISOString() + '\n\n' + detail;
    if (f.sessionId !== undefined) detail = 'sessionId: ' + (f.sessionId || '(default)') + '\n\n' + detail;
    if (f.meta) detail += (detail ? '\n\n' : '') + JSON.stringify(f.meta, null, 2);
    det.textContent = detail || '(empty)';
    row.onclick = () => { row.classList.toggle('open'); det.classList.toggle('open'); };
    if (openSet && openSet.has(row.dataset.seq)) {
      row.classList.add('open');
      det.classList.add('open');
    }
    if (!this._kindMatches(kind)) {
      row.classList.add('filtered-out');
      det.classList.add('filtered-out');
    }
    list.appendChild(row); list.appendChild(det);
    // Live append may stick to the shell slot bottom; full render uses nearBottom in render().
    if (!openSet) {
      this._scrollToBottom();
      this._ensureFilterEmpty();
      this._syncFilterBar();
    }
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
  /* Independent Panel Component (ADR-0010/0030): no Shell slot CSS assumed.
     Filter bar sticks in the generic scrolling trace slot; list is content-height. */
  :host { display:block; font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed); }
  .toolbar {
    position:sticky; top:0; z-index:2;
    display:flex; flex-wrap:wrap; gap:4px; align-items:center;
    padding:6px 12px; margin:-6px 0 0;
    background:var(--la-panel,#161a22);
    border-bottom:1px solid var(--la-line,#2a2f3a);
  }
  .toolbar[hidden] { display:none !important; }
  .toolbar .chip {
    background:transparent; border:1px solid var(--la-line,#2a2f3a); color:var(--la-dim,#9aa0a6);
    border-radius:999px; padding:2px 8px; font-size:10px; cursor:pointer;
    font-family:var(--la-mono,monospace); line-height:1.4;
  }
  .toolbar .chip:hover { color:var(--la-ink,#e8eaed); border-color:var(--la-accent,#7aa2f7); }
  .toolbar .chip.on {
    background:rgba(122,162,247,.16); border-color:var(--la-accent,#7aa2f7); color:var(--la-accent,#7aa2f7);
  }
  .toolbar .count {
    margin-left:auto; font-size:10px; color:var(--la-dim,#9aa0a6);
    font-family:var(--la-mono,monospace);
  }
  .list { min-height:0; }
  .trace-row { padding:6px 12px; font-size:12px; cursor:pointer; display:flex; gap:8px; align-items:baseline; line-height:1.3; }
  .trace-row:hover { background:#1a1f2a; }
  .trace-row .t-kind { flex:0 0 72px; font-size:10px; text-transform:uppercase; letter-spacing:.04em; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .trace-row .t-sum { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; color:var(--la-ink,#e8eaed); }
  .trace-row .t-ts { flex:0 0 auto; font-size:10px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .trace-row .t-seq { flex:0 0 auto; font-size:10px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .trace-row.user .t-kind { color:#7aa2f7; }
  .trace-row.assistant .t-kind { color:var(--la-ok,#9ece6a); }
  .trace-row.system .t-kind { color:#bb9af7; }
  .trace-row.tool_call .t-kind { color:#e0af68; }
  .trace-row.tool_result .t-kind { color:var(--la-err,#f7768e); }
  .trace-row.reasoning .t-kind { color:#bb9af7; }
  .trace-row.open { background:#1a1f2a; }
  .trace-row.filtered-out, .trace-detail.filtered-out { display:none !important; }
  .trace-detail { display:none; padding:4px 12px 10px 92px; font-size:11px; color:var(--la-dim,#9aa0a6); white-space:pre-wrap; font-family:var(--la-mono,monospace); border-bottom:1px solid var(--la-line,#2a2f3a); }
  .trace-detail.open { display:block; }
  .message { padding:8px 12px; color:var(--la-dim,#9aa0a6); }
  .message.error { color:var(--la-err,#f7768e); padding:8px 12px; }
  .filter-empty { color:var(--la-dim,#9aa0a6); }
`;

if (!customElements.get('session-trace')) customElements.define('session-trace', SessionTrace);

/* ---- session-rail: the session list / switcher (workspace left pane) ---- */

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
  .sess-item.child { padding-left:22px; }
  .sess-item .badge {
    display:inline-block; margin-right:6px; font-size:10px; color:var(--la-accent,#7aa2f7);
    font-family:var(--la-mono,monospace);
  }
  .ws-head {
    margin:10px 4px 4px; font-size:10px; letter-spacing:.04em;
    text-transform:uppercase; color:var(--la-dim,#9aa0a6);
    font-family:var(--la-mono,monospace);
    white-space:nowrap; overflow:hidden; text-overflow:ellipsis;
  }
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
    // Announcement-driven highlighting plus reload: a freshly minted Session
    // (__session) or a switch must appear/highlight in the rail without a page
    // reload — the medium emits no turn-level status in the L0 Medium, so the
    // rail cannot lean on idle like the CLI face did.
    this._offs.push(LiteAgent.on('__session', id => { this.markActive(id); this.loadSessions(); }));
    this._offs.push(LiteAgent.on('__new', () => { this.markActive(''); this.loadSessions(); }));
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
      const listRes = await LiteAgent.call('session', 'list', {});
      const list = (listRes && listRes.ok !== false && listRes.result && listRes.result.sessions) || [];
      // The Shell opens on the new-session face, so no Session is marked active
      // until the user switches to one (see markActive on __session).
      this.render(this._current, list);
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
    // Workspace path (ADR-0020) is the ONLY grouping basis: sessions of one
    // project stay together, regardless of Subagent parentage. Subagent rows
    // keep their ↳ badge, but the tree nesting is dropped on purpose.
    const groups = new Map();
    list.forEach(s => {
      const ws = (s.workspace || '').trim() || '';
      if (!groups.has(ws)) groups.set(ws, []);
      groups.get(ws).push(s);
    });
    const keys = Array.from(groups.keys()).sort((a, b) => {
      if (a === '') return 1;
      if (b === '') return -1;
      return a.localeCompare(b);
    });
    keys.forEach((ws) => {
      const head = document.createElement('div');
      head.className = 'ws-head';
      head.title = ws || '(no workspace)';
      const name = ws ? ws.split(/[\\/]/).filter(Boolean).pop() || ws : '未绑定工作区';
      const members = groups.get(ws);
      head.textContent = '▸ ' + name + '  (' + members.length + ')';
      el.appendChild(head);
      members
        .slice()
        .sort((a, b) => (a.id || '').localeCompare(b.id || ''))
        .forEach(s => {
          const id = s.id || '';
          const title = s.title || id;
          const item = document.createElement('div');
          item.className = 'sess-item' + (id === this._current ? ' active' : '');
          if (window.__liteApprovalPending && window.__liteApprovalPending[id]) {
            const pb = document.createElement('span');
            pb.className = 'appr-badge';
            pb.textContent = '待确认';
            pb.title = '该会话有工具等待确认';
            item.appendChild(pb);
          }
          if (s.origin === 'subagent') {
            const badge = document.createElement('span');
            badge.className = 'badge';
            badge.textContent = '↳';
            item.appendChild(badge);
          }
          item.appendChild(document.createTextNode(title));
          item.title = id + (s.parentSession ? '\nparent: ' + s.parentSession : '') + (ws ? '\nws: ' + ws : '');
          item.onclick = () => this.select(id);
          el.appendChild(item);
        });
    });
  }
  markActive(id) {
    this._current = id || '';
    if (!this._root) return;
    this._root.querySelectorAll('.sess-item').forEach(el => {
      el.classList.toggle('active', el.title.split('\n')[0] === this._current);
    });
  }
  async select(id) {
    const b = await LiteAgent.call('session', 'select', { sessionId: id });
    if (!b || b.ok === false) return;
    // Announce the switch; the session-view reloads its history for this session.
    window.__liteSessionId = id;
    LiteAgent.emit('__session', id);
    this.markActive(id);
  }
  async newSession() {
    // No prompt here: the Session View owns the new-session face (Workspace
    // picker + Agent Scheme + composer) and mints the Session on first send.
    this.markActive('');
    LiteAgent.emit('__new', {});
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
    cursor:pointer; user-select:none;
  }
  .usage-bar:hover { color:var(--la-ink,#e8eaed); }
  .usage-bar .u-label { color:var(--la-dim,#9aa0a6); opacity:.75; }
  .usage-bar .u-val { color:var(--la-ink,#e8eaed); }
  .usage-bar .u-src { color:var(--la-accent,#7aa2f7); }
  .usage-bar .u-hint { margin-left:auto; font-size:10px; opacity:.6; }
  .ctx-panel {
    flex-shrink:0; max-height:28vh; overflow-y:auto; border-top:1px solid var(--la-line,#2a2f3a);
    background:var(--la-panel2,#12161f); padding:8px 12px; font-size:11px;
    font-family:var(--la-mono,monospace); display:none;
  }
  .ctx-panel.open { display:block; }
  .ctx-panel .ctx-head { color:var(--la-dim,#9aa0a6); margin-bottom:6px; }
  .ctx-msg { margin:0 0 6px; padding:6px 8px; border-radius:6px; background:var(--la-bg,#0f1115);
    border:1px solid var(--la-line,#2a2f3a); white-space:pre-wrap; word-break:break-word; }
  .ctx-msg .role { color:var(--la-accent,#7aa2f7); font-weight:600; margin-right:6px; }
  .ctx-msg.tool { border-color:#e0af6855; }
  .ctx-msg.system .role { color:#bb9af7; }
  .family-bar {
    flex-shrink:0; display:flex; gap:8px; align-items:center; flex-wrap:wrap;
    padding:6px 16px; font-size:11px; color:var(--la-dim,#9aa0a6);
    border-bottom:1px solid var(--la-line,#2a2f3a); background:var(--la-panel2,#12161f);
    font-family:var(--la-mono,monospace);
  }
  .family-bar:empty { display:none; }
  .family-bar button {
    background:transparent; border:1px solid var(--la-line,#2a2f3a); color:var(--la-accent,#7aa2f7);
    border-radius:6px; padding:3px 8px; font:inherit; cursor:pointer;
  }
  .family-bar button:hover { background:#1a1f2a; }
  .family-bar .f-label { opacity:.8; }
  /* New-session face: Workspace picker + Agent Scheme + composer. */
  .welcome { max-width:52rem; margin:0 auto; padding:28px 4px 8px; }
  .welcome h1 { font-size:20px; margin:0 0 6px; color:var(--la-ink,#e8eaed); }
  .welcome .sub { color:var(--la-dim,#9aa0a6); font-size:12px; margin:0 0 20px; line-height:1.55; }
  .w-row { margin-bottom:16px; }
  .w-label { font-size:11px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); margin-bottom:6px; }
  .w-value { font-size:13px; font-family:var(--la-mono,monospace); color:var(--la-ink,#e8eaed); word-break:break-all; margin-bottom:8px; }
  .w-value.empty { color:var(--la-dim,#9aa0a6); font-style:italic; }
  .w-actions { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
  .w-btn { background:transparent; border:1px solid var(--la-line,#2a2f3a); color:var(--la-accent,#7aa2f7);
    border-radius:8px; padding:6px 12px; font-size:12px; cursor:pointer; }
  .w-btn:hover { background:#1a2438; border-color:var(--la-accent,#7aa2f7); }
  .w-input { flex:1; min-width:220px; background:var(--la-bg,#0f1115); border:1px solid var(--la-line,#2a2f3a);
    color:var(--la-ink,#e8eaed); border-radius:8px; padding:7px 10px; font-size:12px; font-family:var(--la-mono,monospace); }
  .w-chips { display:flex; gap:8px; flex-wrap:wrap; }
  .w-chip { background:transparent; border:1px solid var(--la-line,#2a2f3a); color:var(--la-dim,#9aa0a6);
    border-radius:999px; padding:5px 12px; font-size:12px; cursor:pointer; }
  .w-chip:hover { color:var(--la-ink,#e8eaed); border-color:var(--la-accent,#7aa2f7); }
  .w-chip.on { color:#0b1020; background:var(--la-accent,#7aa2f7); border-color:var(--la-accent,#7aa2f7); font-weight:600; }
  .w-hint { font-size:11px; color:var(--la-dim,#9aa0a6); margin-top:8px; line-height:1.55; }
  .w-cand { display:flex; flex-direction:column; gap:4px; margin-top:8px; }
  /* Permission mode (ADR-0033) + in-chat tool approval (no browser modal). */
  .perm-bar { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:8px; }
  .perm-bar select { background:var(--la-bg,#0f1115); border:1px solid var(--la-line,#2a2f3a);
    color:var(--la-ink,#e8eaed); border-radius:6px; padding:3px 8px; font-size:11px;
    font-family:var(--la-mono,monospace); }
  .appr-card { border:1px solid var(--la-accent,#7aa2f7); border-radius:10px; padding:10px 12px;
    margin:8px 0; background:var(--la-panel,#161a22); max-width:40rem; }
  .appr-card.done-allow { border-color:var(--la-ok,#9ece6a); }
  .appr-card.done-deny { border-color:var(--la-err,#f7768e); }
  .appr-card .appr-title { font-size:12px; font-weight:600; color:var(--la-ink,#e8eaed);
    font-family:var(--la-mono,monospace); margin-bottom:6px; }
  .appr-card .appr-meta { font-size:11px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace);
    margin-bottom:6px; word-break:break-all; }
  .appr-card pre { margin:0 0 8px; padding:8px; background:var(--la-bg,#0f1115); border-radius:6px;
    font-size:11px; max-height:120px; overflow:auto; white-space:pre-wrap; word-break:break-all;
    color:var(--la-ink,#e8eaed); }
  .appr-actions { display:flex; gap:8px; align-items:center; }
  .appr-actions button { border-radius:6px; padding:5px 12px; font-size:12px; cursor:pointer;
    border:1px solid var(--la-line,#2a2f3a); background:transparent; color:var(--la-ink,#e8eaed); }
  .appr-actions .ok { border-color:var(--la-ok,#9ece6a); color:var(--la-ok,#9ece6a); }
  .appr-actions .no { border-color:var(--la-err,#f7768e); color:var(--la-err,#f7768e); }
  .appr-actions .st { font-size:11px; color:var(--la-dim,#9aa0a6); font-family:var(--la-mono,monospace); }
  .appr-badge { display:inline-block; margin-left:6px; padding:1px 6px; border-radius:999px;
    font-size:9px; border:1px solid #e0af68; color:#e0af68; font-family:var(--la-mono,monospace); }
`;

class SessionView extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._sid = '';
    // mode: 'welcome' (new-session face) | 'chat' (a Session is loaded).
    this._mode = 'welcome';
    // Workspace chosen on the new-session face (Session metadata, ADR-0020).
    this._ws = '';
    this._wsCandidates = [];
    this._schemes = [];
    this._scheme = '';
    // Session Permission Mode (ADR-0033): default workspace_write.
    this._permMode = 'workspace_write';
    // Live tool-approval cards keyed by Host approval id.
    this._approvals = new Map();
    // Pending approvals for other sessions (rail badge).
    this._pendingOther = new Map();
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
    // Parent/child Session bar: enter a Subagent child, or return to the parent.
    const familyBar = document.createElement('div');
    familyBar.className = 'family-bar';
    root.appendChild(familyBar);
    this._familyBar = familyBar;
    const flow = document.createElement('div');
    flow.className = 'flow';
    root.appendChild(flow);
    // Per-session Context Usage strip (CONTEXT.md Context Usage).
    const usageBar = document.createElement('div');
    usageBar.className = 'usage-bar';
    usageBar.title = 'Show Model Context (messages sent to the LLM)';
    usageBar.innerHTML = '<span class="u-label">context</span><span class="u-val">—</span><span class="u-hint">show ▾</span>';
    root.appendChild(usageBar);
    this._usageEl = usageBar.querySelector('.u-val');
    this._usageBar = usageBar;
    const ctxPanel = document.createElement('div');
    ctxPanel.className = 'ctx-panel';
    root.appendChild(ctxPanel);
    this._ctxPanel = ctxPanel;
    usageBar.onclick = () => this.toggleModelContext();
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
    this._offs.push(LiteAgent.on('tool_approval', d => this.onToolApproval(d)));
    // Announcement-driven, never onSessionChange: that fires immediately with
    // the Host's Current Session and would load history at boot (the Shell opens
    // on the new-session face by design).
    this._offs.push(LiteAgent.on('__session', sid => this.onSessionChange(sid)));
    this._offs.push(LiteAgent.on('__new', () => this.showWelcome()));
    this.showWelcome();
  }
  async onToolApproval(d) {
    if (!d || !d.id) return;
    // Other Session: rail badge only — never auto-approve (ADR-0029 first-wins).
    if (this._sid && d.sessionId && !this.isCurrent(d.sessionId)) {
      const sid = d.sessionId || 'default';
      this._pendingOther.set(sid, (this._pendingOther.get(sid) || 0) + 1);
      if (!window.__liteApprovalPending) window.__liteApprovalPending = {};
      window.__liteApprovalPending[sid] = (window.__liteApprovalPending[sid] || 0) + 1;
      try { LiteAgent.emit('__approval_pending', { sessionId: sid, tool: d.tool }); } catch (e) {}
      return;
    }
    // Welcome face / no current session yet: still show the card in this view.
    this.queueOr(() => this.renderApprovalCard(d));
  }

  renderApprovalCard(d) {
    const flow = this._flow();
    if (!flow) return;
    const card = document.createElement('div');
    card.className = 'appr-card';
    card.dataset.apprId = d.id;
    const title = document.createElement('div');
    title.className = 'appr-title';
    title.textContent = '工具确认 · ' + (d.tool || '(unknown tool)');
    const meta = document.createElement('div');
    meta.className = 'appr-meta';
    meta.textContent = 'session=' + (d.sessionId || 'default') +
      (d.workspace ? '  ws=' + d.workspace : '') +
      '  id=' + d.id;
    const pre = document.createElement('pre');
    let argsText = '';
    try {
      argsText = typeof d.arguments === 'string' ? d.arguments : JSON.stringify(d.arguments || {}, null, 2);
    } catch (e) { argsText = String(d.arguments || ''); }
    pre.textContent = (argsText || '{}').slice(0, 2000);
    const actions = document.createElement('div');
    actions.className = 'appr-actions';
    const st = document.createElement('span');
    st.className = 'st';
    const deadline = Date.now() + 20000;
    const tick = () => {
      const left = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
      if (card.classList.contains('done-allow') || card.classList.contains('done-deny')) return;
      if (left <= 0) {
        st.textContent = '超时 · Host 将拒绝';
        return;
      }
      st.textContent = left + 's 内确认（超时按拒绝）';
    };
    tick();
    const timer = setInterval(tick, 1000);
    const btnOk = document.createElement('button');
    btnOk.type = 'button';
    btnOk.className = 'ok';
    btnOk.textContent = '允许';
    const btnNo = document.createElement('button');
    btnNo.type = 'button';
    btnNo.className = 'no';
    btnNo.textContent = '拒绝';
    const reply = async (approved) => {
      clearInterval(timer);
      btnOk.disabled = true;
      btnNo.disabled = true;
      card.classList.add(approved ? 'done-allow' : 'done-deny');
      st.textContent = approved ? '已允许' : '已拒绝';
      this.appendPre((approved ? 'allowed ' : 'denied ') + (d.tool || ''), approved ? 'message' : 'message error');
      try {
        const res = await fetch('/api/tool-approval', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: d.id, approved: approved })
        });
        const body = await res.json().catch(() => ({}));
        if (body && body.ok === false) {
          st.textContent = 'Host 已决/超时 · ' + (body.error || '');
          card.classList.add('done-deny');
        }
      } catch (e) {
        st.textContent = '应答失败 · Host 超时将拒绝';
        card.classList.add('done-deny');
      }
    };
    btnOk.onclick = () => reply(true);
    btnNo.onclick = () => reply(false);
    actions.appendChild(btnOk);
    actions.appendChild(btnNo);
    actions.appendChild(st);
    card.appendChild(title);
    card.appendChild(meta);
    card.appendChild(pre);
    card.appendChild(actions);
    flow.appendChild(card);
    flow.scrollTop = flow.scrollHeight;
  }

  paintRailPending() {
    // Rail component listens to __approval_pending; nothing to paint in view shadow.
    if (this._pendingOther && this._pendingOther.size && this._familyBar) {
      // Keep family bar for session tree; pending is rail-owned.
    }
  }
  disconnectedCallback() {
    this._offs.forEach(off => off());
    this._offs = [];
    this._root = null;
    this._input = null;
    this._btnSend = null;
    this._usageEl = null;
    this._usageBar = null;
    this._ctxPanel = null;
    this._familyBar = null;
  }
  isCurrent(sid) {
    // Empty/missing ids all mean the default Session (always non-empty).
    const norm = (v) => (v === undefined || v === null || String(v) === '') ? 'default' : String(v);
    return norm(sid) === norm(this._sid);
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
      await this.refreshFamilyBar();
    } catch (e) {
      this._ready = true;
      this.appendPre('history unavailable: ' + e, 'message error');
    }
  }
  // Family bar: "← parent" when this is a Subagent child; child chips when this parent spawned Subagents.
  async refreshFamilyBar() {
    if (!this._familyBar) return;
    this._familyBar.innerHTML = '';
    // perm bar is re-attached after family content is rebuilt
    try {
      const res = await LiteAgent.call('session', 'list', {});
      const list = (res && res.ok !== false && res.result && res.result.sessions) || [];
      const me = list.find(s => (s.id || '') === (this._sid || ''));
      if (me && me.parentSession) {
        const label = document.createElement('span');
        label.className = 'f-label';
        label.textContent = me.origin === 'subagent' ? 'subagent of' : 'parent';
        this._familyBar.appendChild(label);
        const back = document.createElement('button');
        back.type = 'button';
        back.textContent = '← ' + me.parentSession;
        back.title = 'Open parent session ' + me.parentSession;
        back.onclick = () => this.enterSession(me.parentSession);
        this._familyBar.appendChild(back);
      }
      const kids = list.filter(s => s.parentSession && s.parentSession === (this._sid || ''));
      if (kids.length) {
        const label = document.createElement('span');
        label.className = 'f-label';
        label.textContent = kids.length === 1 ? '1 subagent' : (kids.length + ' subagents');
        this._familyBar.appendChild(label);
        kids.forEach(s => {
          const btn = document.createElement('button');
          btn.type = 'button';
          btn.textContent = '↳ ' + ((s.title || s.id || '').slice(0, 28));
          btn.title = 'Open subagent session ' + s.id;
          btn.onclick = () => this.enterSession(s.id);
          this._familyBar.appendChild(btn);
        });
      }
    } catch (e) { /* family bar is best-effort */ }
    if (this._mode === 'chat') this.renderPermBar();
  }
  async enterSession(id) {
    if (!id) return;
    const b = await LiteAgent.call('session', 'select', { sessionId: id });
    if (!b || b.ok === false) return;
    window.__liteSessionId = id;
    LiteAgent.emit('__session', id);
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
      const res = await LiteAgent.callCap('context-manager', 'context', 'usage', { sessionId: this._sid || '' });
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
    if (f.type === 'turn_end') {
      this.refreshUsage();
      if (this._ctxPanel && this._ctxPanel.classList.contains('open')) this.loadModelContext();
      // A Turn may have spawned Subagent children — refresh the family bar.
      this.refreshFamilyBar();
    }
  }
  async toggleModelContext() {
    if (!this._ctxPanel) return;
    if (this._ctxPanel.classList.contains('open')) {
      this._ctxPanel.classList.remove('open');
      this._ctxPanel.innerHTML = '';
      return;
    }
    this._ctxPanel.classList.add('open');
    await this.loadModelContext();
  }
  async loadModelContext() {
    if (!this._ctxPanel) return;
    this._ctxPanel.innerHTML = '<div class="ctx-head">Model Context…</div>';
    try {
      // listContext n=0 → full prepare snapshot (messages that entered the LLM).
      const res = await LiteAgent.callCap('context-manager', 'context', 'listContext', { sessionId: this._sid || '', n: 0 });
      if (!res || res.ok === false) throw new Error((res && res.error) || 'listContext failed');
      const msgs = (res.result && res.result.messages) || [];
      const total = (res.result && res.result.total) || msgs.length;
      this._ctxPanel.innerHTML = '';
      const head = document.createElement('div');
      head.className = 'ctx-head';
      head.textContent = 'Model Context · ' + total + ' messages · last prepare';
      this._ctxPanel.appendChild(head);
      if (!msgs.length) {
        const empty = document.createElement('div');
        empty.className = 'ctx-msg';
        empty.textContent = 'No prepare snapshot yet — send a message first.';
        this._ctxPanel.appendChild(empty);
        return;
      }
      msgs.forEach(m => {
        const row = document.createElement('div');
        row.className = 'ctx-msg' + (m.role === 'system' ? ' system' : '') + (m.role === 'tool' ? ' tool' : '');
        const role = document.createElement('span');
        role.className = 'role';
        role.textContent = m.role || '?';
        const body = document.createElement('span');
        let text = m.content || '';
        if (!text && m.tool_calls && m.tool_calls.length) {
          text = m.tool_calls.map(tc => '→ ' + (tc.name || '?')).join(', ');
        }
        if (!text) text = '(empty)';
        body.textContent = text;
        row.appendChild(role);
        row.appendChild(body);
        this._ctxPanel.appendChild(row);
      });
    } catch (e) {
      this._ctxPanel.innerHTML = '';
      const err = document.createElement('div');
      err.className = 'ctx-msg';
      err.textContent = 'Model Context unavailable: ' + e;
      this._ctxPanel.appendChild(err);
    }
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
      const parts = [];
      if (step) parts.push('step ' + step);
      if (f.ts) parts.push(fmtTs(f.ts));
      const row = this.makeDisc('think', 'Thinking', parts.join(' · '), body, false);
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
    if (this._thinkingEl || this._liveEl) return;
    const thinks = this._root.querySelectorAll('details.think:not(.live) .disc-body');
    if (thinks.length) this._reasoningBuf = thinks[thinks.length - 1].textContent || '';
  }
  sendOrStop() {
    if (!this._input) return;
    if (this._running) {
      LiteAgent.callCap('agent', 'loop', 'cancel', { sessionId: this._sid })
        .then(() => { this._running = false; this.setSendState(false); })
        .catch(() => { this._running = false; this.setSendState(false); });
      return;
    }
    const text = this._input.value.trim();
    if (!text) return;
    this._input.value = '';
    // Slash commands are Host/session-scoped and work without a Session.
    if (text.charAt(0) === '/') {
      if (this._mode !== 'welcome') this.appendUser(text);
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
    // New-session face: the first send mints the Session (with the chosen
    // Workspace) instead of pressing an idle Session into service.
    if (this._mode === 'welcome') {
      this.startSessionAndSend(text);
      return;
    }
    this.appendUser(text);
    this.beginTurn();
    this._running = true;
    this.setSendState(true);
    LiteAgent.callCap('agent', 'loop', 'turn', { input: text, sessionId: this._sid, allowSubagent: true }).then(b => {
      this._running = false;
      this.setSendState(false);
      if (b && b.ok === false) {
        this.appendPre(b.error || 'send failed', 'message error');
      }
    }).catch(() => { this._running = false; this.setSendState(false); });
  }
  // ---- new-session face (Workspace + Agent Scheme + composer) ----

  showWelcome() {
    this._mode = 'welcome';
    this._sid = '';
    this._hostDefaultWs = '';
    this._running = false;
    this._ready = true; // nothing to replay: no history gate
    this._queue = [];
    this.setSendState(false);
    this.setUsageBar(null);
    if (this._familyBar) this._familyBar.innerHTML = '';
    if (this._mode === 'welcome') this._permMode = this._permMode || 'workspace_write';
    if (this._usageBar) this._usageBar.style.display = 'none';
    if (this._ctxPanel) { this._ctxPanel.classList.remove('open'); this._ctxPanel.innerHTML = ''; }
    this.renderWelcome();
    this.loadSchemeState();
    this.loadHostDefaultWs();
    if (this._input) this._input.focus();
  }
  renderWelcome() {
    const flow = this._flow();
    if (!flow) return;
    flow.innerHTML = '';
    const card = document.createElement('div');
    card.className = 'welcome';
    const h1 = document.createElement('h1');
    h1.textContent = '新建会话';
    const sub = document.createElement('p');
    sub.className = 'sub';
    sub.textContent = '工作区可留空；agent 模式决定这一轮会话拉起哪些插件与工具。也可以在左侧选择已有会话。';
    card.appendChild(h1);
    card.appendChild(sub);

    // Workspace row (ADR-0020): the Folder-Picker cannot yield an absolute path
    // in the L0 Medium (the name→path resolver was a removed Host face), so the
    // row is a plain path input; leaving it empty uses the Host default.
    const wsRow = document.createElement('div');
    wsRow.className = 'w-row';
    wsRow.appendChild(this._wLabel('工作区 Workspace'));
    const val = document.createElement('div');
    val.className = 'w-value' + (this._ws ? '' : ' empty');
    // When empty, be honest about what the Host default (ADR-0020) resolves to.
    val.textContent = this._ws || (this._hostDefaultWs
      ? '（将使用 Host 默认工作区：' + this._hostDefaultWs + '）'
      : '（留空 — 使用 Host 默认工作区）');
    wsRow.appendChild(val);
    const actions = document.createElement('div');
    actions.className = 'w-actions';
    const input = document.createElement('input');
    input.className = 'w-input';
    input.placeholder = '填写此会话的绝对路径（可留空用 Host 默认）';
    input.value = this._ws || '';
    input.onchange = () => { this._ws = input.value.trim(); this.renderWelcome(); };
    const btnClear = document.createElement('button');
    btnClear.type = 'button';
    btnClear.className = 'w-btn';
    btnClear.textContent = '清空';
    btnClear.onclick = () => { this._ws = ''; this.renderWelcome(); };
    actions.appendChild(input);
    actions.appendChild(btnClear);
    wsRow.appendChild(actions);
    card.appendChild(wsRow);

    // Agent Scheme row.
    const schRow = document.createElement('div');
    schRow.className = 'w-row';
    schRow.appendChild(this._wLabel('agent 模式 Agent Scheme'));
    const chips = document.createElement('div');
    chips.className = 'w-chips';
    if (!this._schemes.length) {
      const none = document.createElement('span');
      none.className = 'w-hint';
      none.textContent = '未取到 agent scheme（agent 插件未挂载？）';
      chips.appendChild(none);
    }
    this._schemes.forEach(s => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'w-chip' + (s === this._scheme ? ' on' : '');
      b.textContent = schemeLabel(s) + ' (' + s + ')';
      b.title = 'config.set defaultScheme=' + s + '（下一 Turn 生效）';
      b.onclick = () => this.applyScheme(s);
      chips.appendChild(b);
    });
    schRow.appendChild(chips);
    const hint = document.createElement('div');
    hint.className = 'w-hint';
    hint.textContent = '进入会话前会经 Host ensurePlugins 拉起该模式声明的插件。';
    schRow.appendChild(hint);
    card.appendChild(schRow);

    // Permission mode chips (ADR-0033).
    const permRow = document.createElement('div');
    permRow.className = 'w-row';
    permRow.appendChild(this._wLabel('会话权限 Permission Mode'));
    const pchips = document.createElement('div');
    pchips.className = 'w-chips';
    const modes = [
      { id: 'read_only', label: '只读 read_only' },
      { id: 'workspace_write', label: '工作区可写 workspace_write' },
      { id: 'full_access', label: '完全访问 full_access' }
    ];
    modes.forEach(m => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'w-chip' + (m.id === this._permMode ? ' on' : '');
      b.textContent = m.label;
      b.title = '创建会话时写入 Session 元数据；可随时在会话头修改';
      b.onclick = () => { this._permMode = m.id; this.renderWelcome(); };
      pchips.appendChild(b);
    });
    permRow.appendChild(pchips);
    const phint = document.createElement('div');
    phint.className = 'w-hint';
    phint.textContent = 'read_only：未命中规则的写/shell 拒绝 · workspace_write：工作区内写放行、shell 需确认 · full_access：按 severityPolicy。显式 permissions.json 规则可放宽模式。';
    permRow.appendChild(phint);
    card.appendChild(permRow);

    flow.appendChild(card);
  }

  renderPermBar() {
    if (!this._familyBar) return null;
    let bar = this._familyBar.querySelector('.perm-bar');
    if (!bar) {
      bar = document.createElement('div');
      bar.className = 'perm-bar';
      this._familyBar.appendChild(bar);
    }
    bar.innerHTML = '';
    const lab = document.createElement('span');
    lab.className = 'f-label';
    lab.textContent = '权限';
    const sel = document.createElement('select');
    [['read_only', '只读'], ['workspace_write', '工作区可写'], ['full_access', '完全访问']].forEach(([v, t]) => {
      const o = document.createElement('option');
      o.value = v; o.textContent = t + ' (' + v + ')';
      if (v === this._permMode) o.selected = true;
      sel.appendChild(o);
    });
    sel.onchange = () => this.applyPermissionMode(sel.value);
    bar.appendChild(lab);
    bar.appendChild(sel);
    return bar;
  }

  async applyPermissionMode(mode) {
    if (!this._sid) {
      this._permMode = mode;
      if (this._mode === 'welcome') this.renderWelcome();
      return;
    }
    if (mode === this._permMode) return;
    this._permMode = mode;
    try {
      const b = await LiteAgent.call('session', 'setPermissionMode', {
        sessionId: this._sid, permissionMode: mode
      });
      if (!b || b.ok === false) throw new Error((b && b.error) || 'set failed');
      LiteAgent.emit('__notice', { text: 'permissionMode → ' + mode, cls: 'message' });
      this.renderPermBar();
    } catch (e) {
      LiteAgent.emit('__notice', { text: 'permissionMode 设置失败: ' + e, cls: 'message error' });
    }
  }

  async loadPermissionMode() {
    if (!this._sid) return;
    try {
      const b = await LiteAgent.call('session', 'info', { sessionId: this._sid });
      const r = b && b.ok !== false ? b.result : null;
      this._permMode = (r && r.permissionMode) || 'workspace_write';
    } catch (e) { this._permMode = 'workspace_write'; }
    if (this._mode === 'chat') this.renderPermBar();
  }
  _wLabel(text) {
    const l = document.createElement('div');
    l.className = 'w-label';
    l.textContent = text;
    return l;
  }
  async loadHostDefaultWs() {
    // The Host's default Workspace (ADR-0020) is the current Session's at boot;
    // surface the resolved path so "留空" is honest about what will be used.
    try {
      const b = await LiteAgent.call('session', 'info', {});
      this._hostDefaultWs = (b && b.ok !== false && b.result && b.result.workspace) || '';
    } catch (e) { this._hostDefaultWs = ''; }
    if (this._mode === 'welcome') this.renderWelcome();
  }
  async loadSchemeState() {
    try {
      const res = await fetch('/api/call', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plugin: 'agent', cap: 'config', method: 'get', payload: {} })
      });
      const b = await res.json();
      let face = b && b.ok !== false ? b.result : null;
      if (typeof face === 'string') { try { face = JSON.parse(face); } catch (e) { face = null; } }
      if (!face) return;
      this._scheme = face.defaultScheme || '';
      this._schemes = Object.keys(face.schemes || {}).sort();
      if (this._mode === 'welcome') this.renderWelcome();
    } catch (e) { /* scheme row is best-effort */ }
  }
  async applyScheme(name) {
    if (!name || name === this._scheme) return;
    this._scheme = name;
    if (this._mode === 'welcome') this.renderWelcome();
    try {
      const res = await fetch('/api/call', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plugin: 'agent', cap: 'config', method: 'set', payload: { defaultScheme: name } })
      });
      const b = await res.json();
      if (b && b.ok === false) throw new Error(b.error || 'set failed');
      LiteAgent.emit('__scheme', { scheme: name });
      LiteAgent.emit('__notice', { text: 'agent scheme → ' + name, cls: 'message' });
    } catch (e) {
      LiteAgent.emit('__notice', { text: 'scheme set failed: ' + e, cls: 'message error' });
    }
  }
  async startSessionAndSend(text) {
    this._running = true;
    this.setSendState(true);
    try {
      const ws = (this._ws || '').trim();
      const b = await LiteAgent.call('session', 'create', {
        workspace: ws,
        origin: 'web',
        permissionMode: this._permMode || 'workspace_write'
      });
      if (!b || b.ok === false) throw new Error((b && b.error) || 'session.create failed');
      const id = (b.result && b.result.sessionId) || '';
      if (!id) throw new Error('session.create returned no id');
      // From here on this view is a normal Session view.
      this._mode = 'chat';
      this._sid = id;
      window.__liteSessionId = id;
      if (this._usageBar) this._usageBar.style.display = '';
      if (b.result && b.result.permissionMode) this._permMode = b.result.permissionMode;
      this.renderPermBar();
      LiteAgent.emit('__session', id); // rail highlight + trace follow
      await this.reload();
      this.loadPermissionMode();
      this.appendUser(text);
      this.beginTurn();
      const sent = await LiteAgent.callCap('agent', 'loop', 'turn', { input: text, sessionId: id, allowSubagent: true });
      if (sent && sent.ok === false) throw new Error(sent.error || 'send failed');
      this._running = false;
      this.setSendState(false);
    } catch (e) {
      this._running = false;
      this.setSendState(false);
      if (this._mode === 'welcome') this.renderWelcome();
      this.appendPre(String(e && e.message || e), 'message error');
    }
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
    if (String(this._sid) === String(sid) && this._mode === 'chat') return;
    this._mode = 'chat';
    this._sid = sid;
    if (this._usageBar) this._usageBar.style.display = '';
    this._running = false;
    this.setSendState(false);
    this.setUsageBar(null);
    this._approvals = new Map();
    this.renderPermBar();
    this.loadPermissionMode();
    this.reload();
  }
  onStreamEvent(d) {
    // Streams without sessionId (older LLM plugins / host) are treated as the
    // current view — dropping them is what made live tokens invisible.
    if (d && d.sessionId !== undefined && d.sessionId !== null && d.sessionId !== '') {
      if (!this.isCurrent(d.sessionId)) return;
    }
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
        // Final assistant body = turn boundary: the usage strip refreshes here
        // (no turn-level status event is guaranteed in the L0 Medium, BUG-06).
        this.refreshUsage();
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
    this._root.querySelector('.flow').appendChild(el); this.scroll(true); return el;
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

/* ---- session-status: the Host bottom status bar's session chip ---- */

const STATUS_CSS = `
  :host { display:flex; align-items:center; gap:8px; font:11px/1.6 var(--la-mono,monospace); color:var(--la-dim,#9aa0a6); }
  .k { color:var(--la-dim,#9aa0a6); opacity:.75; }
  .v { color:var(--la-ink,#e8eaed); }
  .v.accent { color:var(--la-accent,#7aa2f7); }
  .v.warn { color:#e0af68; }
`;

class SessionStatus extends HTMLElement {
  constructor() {
    super();
    this._root = null;
    this._offs = [];
    this._timer = null;
    this._state = { ws: '', sid: '', seq: 0, facts: 0, active: false };
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
    this._offs.push(LiteAgent.on('__session', id => { this._state.active = true; this._state.sid = id || ''; this.refresh(); }));
    this._offs.push(LiteAgent.on('__new', () => { this._state.active = false; this._state.sid = ''; this._state.ws = ''; this._state.seq = 0; this.render(); }));
    this._offs.push(LiteAgent.on('session', () => { this._state.seq++; this.render(); }));
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
    if (!this._state.active) {
      // Welcome face: show the Host default workspace as a preview.
      try {
        const b = await LiteAgent.call('session', 'info', {});
        const ws = (b && b.ok !== false && b.result && b.result.workspace) || '';
        this._state.ws = ws;
      } catch (e) { /* best-effort */ }
      this.render();
      return;
    }
    try {
      const res = await LiteAgent.call('session', 'list', {});
      const list = (res && res.ok !== false && res.result && res.result.sessions) || [];
      const me = list.find(s => (s.id || '') === (this._state.sid || ''));
      if (me) {
        this._state.ws = me.workspace || '';
        this._state.seq = me.seq || 0;
        if (me.title) this._state.title = me.title;
      }
    } catch (e) { /* best-effort */ }
    this.render();
  }
  shortPath(p) {
    if (!p) return '';
    const parts = String(p).split(/[\\/]/).filter(Boolean);
    if (parts.length <= 2) return p;
    return '…/' + parts.slice(-2).join('/');
  }
  render() {
    if (!this._body) return;
    const s = this._state;
    this._body.innerHTML = '';
    const add = (k, v, cls) => {
      const kk = document.createElement('span');
      kk.className = 'k';
      kk.textContent = k;
      const vv = document.createElement('span');
      vv.className = 'v' + (cls ? ' ' + cls : '');
      vv.textContent = v;
      vv.title = v;
      this._body.appendChild(kk);
      this._body.appendChild(vv);
    };
    add('ws', s.ws ? this.shortPath(s.ws) : '—', s.ws ? '' : 'warn');
    if (s.active) {
      add('session', s.sid ? (s.sid.length > 12 ? s.sid.slice(0, 12) : s.sid) : '—', 'accent');
      add('facts', String(s.seq || 0));
    } else {
      add('session', '未开始（新建会话）', 'warn');
    }
  }
}

if (!customElements.get('session-status')) customElements.define('session-status', SessionStatus);

/* ---- session-workspace: chat | trace composite (Shell region center) ----
 * ADR-0031: Shell hosts top/bottom + left|center|right. Rail is a separate
 * mount on `left`; this component owns only the center chat|trace split. */

const WORKSPACE_CSS = `
  :host {
    display:flex; flex-direction:column; height:100%; min-height:0; overflow:hidden;
    font:12px/1.5 var(--la-sans, system-ui); color:var(--la-ink,#e8eaed);
    background:var(--la-bg,#0f1115);
  }
  .ws {
    flex:1; min-height:0; display:grid;
    grid-template-columns:minmax(0,1fr) minmax(260px,0.85fr);
  }
  .col { min-width:0; min-height:0; display:flex; flex-direction:column; overflow:hidden; }
  .col.chat { border-right:1px solid var(--la-line,#2a2f3a); }
  .col.trace { background:var(--la-bg,#0f1115); }
  .col > .pane-head {
    flex:0 0 auto; padding:10px 12px; border-bottom:1px solid var(--la-line,#2a2f3a);
    background:var(--la-panel,#161a22); font-size:12px; color:var(--la-dim,#9aa0a6);
  }
  .col > .pane-body { flex:1; min-height:0; display:flex; flex-direction:column; overflow:hidden; }
  .col > .pane-body > * { flex:1; min-height:0; }
  /* Trace pane scrolls in the component host container so the type-filter
     bar can stick without any Shell slot specialization (ADR-0031). */
  .col.trace > .pane-body { display:block; overflow-y:auto; overflow-x:hidden; }
  .col.trace > .pane-body > session-trace { display:block; min-height:100%; }
`;

class SessionWorkspace extends HTMLElement {
  connectedCallback() {
    const root = this.attachShadow({ mode: 'open' });
    const style = document.createElement('style');
    style.textContent = WORKSPACE_CSS;
    root.appendChild(style);
    const ws = document.createElement('div');
    ws.className = 'ws';
    ws.innerHTML =
      '<div class="col chat">' +
      '<div class="pane-head">Chat</div>' +
      '<div class="pane-body"><session-view></session-view></div>' +
      '</div>' +
      '<div class="col trace">' +
      '<div class="pane-head">Session trace</div>' +
      '<div class="pane-body"><session-trace></session-trace></div>' +
      '</div>';
    root.appendChild(ws);
  }
}

if (!customElements.get('session-workspace')) customElements.define('session-workspace', SessionWorkspace);
