// Chat face: message/disclosure rendering, live stream painting, and
// Presentation consumption. Extracted verbatim from shell.html's IIFE.

import { state, sameSession } from './state.js';
import { md, renderMath } from './md.js';
import { mergeReasoningFacts } from './facts.js';

var chat = document.getElementById('chat');
var streamBuf = '';
var reasoningBuf = '';
var progressEl = null, thinkingEl = null, liveEl = null, liveBody = null, liveMeta = null;

function nearBottom() {
  return chat.scrollHeight - chat.scrollTop - chat.clientHeight < 56;
}
// Stick to bottom only when the user is already near it (don't yank mid-read).
function scrollChat(force) {
  if (force || nearBottom()) chat.scrollTop = chat.scrollHeight;
}

function appendAssistant(html) {
  var el = document.createElement('div');
  el.className = 'msg assistant';
  el.innerHTML = html || '';
  chat.appendChild(el); renderMath(el); scrollChat(true); return el;
}
function appendUser(text) {
  var el = document.createElement('div');
  el.className = 'msg user';
  el.textContent = text;
  chat.appendChild(el); scrollChat(true);
}
function appendPre(text, cls) {
  var el = document.createElement('pre');
  el.className = cls || 'message';
  el.style.cssText = 'white-space:pre-wrap;font-family:var(--la-mono);font-size:12px';
  el.textContent = text;
  chat.appendChild(el); scrollChat(true);
}
function clearProgress() { if (progressEl) { progressEl.remove(); progressEl = null; } }
function clearThinking() {
  if (thinkingEl) { thinkingEl.remove(); thinkingEl = null; }
  reasoningBuf = '';
}

// dsh-style disclosure row
function makeDisc(cls, title, meta, bodyText, open) {
  var d = document.createElement('details');
  d.className = 'disc ' + (cls || '');
  if (open) d.open = true;
  var sum = document.createElement('summary');
  var t = document.createElement('span');
  t.className = 's-title';
  t.textContent = title;
  var m = document.createElement('span');
  m.className = 's-meta';
  m.textContent = meta || '';
  sum.appendChild(t); sum.appendChild(m);
  var body = document.createElement('div');
  body.className = 'disc-body';
  body.textContent = bodyText || '';
  d.appendChild(sum); d.appendChild(body);
  return d;
}

// Reasoning channel → Thinking disclosure (live).
function setThinkingText(t) {
  if (!thinkingEl) {
    thinkingEl = makeDisc('think live', 'Thinking', '', '', true);
    chat.appendChild(thinkingEl);
  }
  var body = thinkingEl.querySelector('.disc-body');
  var meta = thinkingEl.querySelector('.s-meta');
  if (body) body.textContent = t;
  if (meta) meta.textContent = t.length > 60 ? t.slice(0, 60) + '…' : t;
  scrollChat();
}
function freezeThinking() {
  if (thinkingEl) {
    thinkingEl.classList.remove('live');
    thinkingEl.open = false;
    thinkingEl = null;
  }
  reasoningBuf = '';
}

// Content stream → live Answer draft. Settle may replace it.
function ensureLive() {
  if (thinkingEl) freezeThinking();
  if (!liveEl) {
    liveEl = makeDisc('think live', 'Answer', '', '', true);
    chat.appendChild(liveEl);
    liveBody = liveEl.querySelector('.disc-body');
    liveMeta = liveEl.querySelector('.s-meta');
  }
  return liveEl;
}
function setLiveText(t) {
  ensureLive();
  if (liveBody) liveBody.textContent = t;
  if (liveMeta) liveMeta.textContent = (t.length > 60 ? t.slice(0, 60) + '…' : t);
  scrollChat();
}
function removeLive() {
  if (liveEl) { liveEl.remove(); liveEl = null; liveBody = null; liveMeta = null; }
  streamBuf = '';
}
// Keep stream as collapsed Think (unless empty).
function freezeLive() {
  if (liveEl && streamBuf) {
    liveEl.classList.remove('live');
    liveEl.open = false;
    liveEl = null; liveBody = null; liveMeta = null;
    streamBuf = '';
    return;
  }
  removeLive();
  clearThinking();
}

function clearTurnUI() {
  clearProgress(); clearThinking();
  if (liveEl) freezeLive();
  else removeLive();
}

// Live stream deltas (SSE topic=stream), routed from events.js.
function onStreamDelta(channel, delta) {
  if (channel === 'reasoning') {
    reasoningBuf += delta || '';
    setThinkingText(reasoningBuf);
  } else {
    streamBuf += delta || '';
    setLiveText(streamBuf);
  }
}

// Turn start: reset live artifacts exactly as sendOrStop did inline.
function beginTurn() {
  streamBuf = '';
  if (liveEl) { liveEl.remove(); liveEl = null; }
  clearThinking(); clearProgress();
}

// Session switch / new chat: wipe the painted conversation.
function resetChatUI() {
  chat.innerHTML = '';
  streamBuf = '';
  if (liveEl) { liveEl.remove(); liveEl = null; }
  clearThinking(); clearProgress();
}

function onPresentation(raw) {
  clearProgress();
  var data = raw && raw.kind !== undefined ? raw : (raw && raw.data) || raw || {};
  var kind = data.kind || data.Kind;
  var text = data.text !== undefined ? data.text : data.Text;
  var sid = data.sessionId !== undefined ? data.sessionId : data.SessionID;
  // Host Loop always tags sessionId ("" = default). Ignore foreign Session paints.
  if (sid !== undefined && sid !== null && !sameSession(sid)) return;
  if (kind === 'markdown_text') {
    // Final assistant body is always expanded. Drop live stream if it was the same answer.
    var settled = (text || '').trim();
    if (streamBuf && settled && streamBuf.replace(/\s+/g, ' ').indexOf(settled.slice(0, 40).replace(/\s+/g, ' ')) >= 0) {
      removeLive(); // stream was the answer — avoid double print
    } else {
      freezeLive(); // keep as Think disclosure
    }
    freezeThinking();
    appendAssistant(md(text || ''));
    return;
  }
  if (kind === 'message_text') {
    var level = data.level || data.Level || '';
    // Non-final status: fold as dim one-liner disclosure (dsh-like).
    freezeLive();
    var row = makeDisc('', 'status', text || '', '', false);
    if (level === 'error') row.style.borderColor = 'var(--la-err)';
    chat.appendChild(row); scrollChat();
    return;
  }
  if (kind === 'summary_text') {
    freezeLive();
    var pairsArr = data.pairs || data.Pairs || [];
    var title = data.title !== undefined ? data.title : data.Title;
    var detail = data.detail !== undefined ? data.detail : data.Detail;
    var meta = pairsArr.map(function (p) {
      var k = p.key !== undefined ? p.key : p.Key;
      var v = p.value !== undefined ? p.value : p.Value;
      return k + '=' + String(v).slice(0, 40);
    }).join('  ');
    var body = pairsArr.map(function (p) {
      var k = p.key !== undefined ? p.key : p.Key;
      var v = p.value !== undefined ? p.value : p.Value;
      return k + ': ' + v;
    }).join('\n');
    if (detail) body += (body ? '\n\n' : '') + detail;
    var card = makeDisc('tool', '⏺ ' + (title || 'tool'), meta, body, false);
    chat.appendChild(card); scrollChat();
  }
}

function renderHistoryFact(f) {
  if (!f || !f.type) return;
  var t = f.type;
  if (t === 'message') {
    if (f.role === 'user' && f.content) appendUser(f.content);
    else if (f.role === 'assistant' && f.content) appendAssistant(md(f.content));
    return;
  }
  if (t === 'reasoning') {
    var body = f.content || '';
    if (!body) return;
    var step = (f.meta && f.meta.step) || '';
    var row = makeDisc('think', 'Thinking', step ? 'step ' + step : '', body, false);
    chat.appendChild(row);
    return;
  }
  if (t === 'tool_call') {
    var names = ((f.meta && f.meta.tool_calls) || []).map(function (tc) { return tc.name; }).join(',');
    var card = makeDisc('tool', '⏺ ' + (names || 'tool'), '', f.content || names || '', false);
    chat.appendChild(card);
    return;
  }
  if (t === 'tool_result') {
    var res = makeDisc('tool', '⏺ result', '', f.content || '', false);
    chat.appendChild(res);
  }
}

function loadHistory() {
  return fetch('/api/history').then(function (r) { return r.json(); }).then(function (b) {
    chat.innerHTML = '';
    // Prefer Session Log facts so Thinking/tools survive refresh & session switch.
    var facts = mergeReasoningFacts(b.facts || []);
    if (facts.length) {
      facts.forEach(renderHistoryFact);
    } else {
      (b.messages || []).forEach(function (m) {
        if (m.role === 'user') appendUser(m.content);
        else if (m.role === 'assistant') appendAssistant(md(m.content));
      });
    }
    state.historyReady = true;
    scrollChat(true);
  }).catch(function () { state.historyReady = true; });
}

// Mid-run switch/refresh: resume live Thinking from rebuilt facts.
function maybeResumeLiveThinking(isRunning) {
  if (!isRunning || thinkingEl || liveEl) return;
  var thinks = chat.querySelectorAll('details.think:not(.live) .disc-body');
  if (thinks.length) {
    reasoningBuf = thinks[thinks.length - 1].textContent || '';
  }
}

export {
  scrollChat, appendAssistant, appendUser, appendPre,
  clearProgress, clearThinking, clearTurnUI, beginTurn, resetChatUI,
  onPresentation, onStreamDelta, renderHistoryFact, loadHistory, maybeResumeLiveThinking,
};
