// Shell entry: composer and boot order. The face modules (chat/rail/events)
// and plugin Panel Components hold the rendering; this module conducts the
// chat flow. Session switches converge on the __session event: components
// (e.g. the session rail) set the medium session and emit, the shell
// reloads the chat face.

import { state, setRunning, setSessionId } from './state.js';
import { appendUser, appendPre, clearTurnUI, beginTurn, loadHistory, maybeResumeLiveThinking } from './chat.js';
import { setPageLoader, createLoader } from './loader.js';
import { openSSE } from './events.js';

var input = document.getElementById('input');
var btnSend = document.getElementById('btn-send');
var sessionLabel = document.getElementById('session-label');

// This page's Panel mounts (ADR-0011): sidebar/toolbar/overlay plus the
// trace column slot, filled by plugin components.
const pageLoader = createLoader('main', function panelHost(slot) {
  if (slot === 'sidebar') return document.getElementById('rail');
  if (slot === 'main-overlay') return document.getElementById('main-overlay');
  if (slot === 'trace') return document.getElementById('slot-trace');
  return document.getElementById('slot-toolbar-right');
}, function (msg) { appendPre(msg, 'message error'); });
setPageLoader(pageLoader);

function refreshRunState() {
  fetch('/api/session').then(function (r) { return r.json(); }).then(function (b) {
    if (b.sessionId !== undefined) setSessionId(b.sessionId);
    if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
    setRunning(b.status === 'running', state.currentSessionId);
    // Mid-run switch/refresh: resume live Thinking from rebuilt facts.
    maybeResumeLiveThinking(b.status === 'running');
  }).catch(function () { });
}

LiteAgent.on('__session', function (sid) {
  sid = sid || '';
  state.currentSessionId = sid;
  window.__liteSessionId = sid;
  sessionLabel.textContent = sid || '(default)';
  state.historyReady = false;
  clearTurnUI();
  setRunning(false);
  loadHistory().then(function () {
    state.historyReady = true;
    refreshRunState();
  });
});

function sendOrStop() {
  if (state.running) {
    fetch('/api/turn/cancel', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sessionId: state.currentSessionId }) })
      .then(function () { setRunning(false); });
    return;
  }
  var text = input.value.trim();
  if (!text) return;
  input.value = '';
  if (text.charAt(0) === '/') {
    appendUser(text);
    fetch('/api/command', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ line: text }) })
      .then(function (r) { return r.json(); })
      .then(function (body) {
        if (body.error) appendPre(body.error, 'message error');
        else if (body.output) appendPre(body.output, 'message');
        // /refresh rescans manifests server-side; plugin code may have
        // changed, and module identities can't be swapped in place — the
        // Session Log is the truth, so reload rebuilds everything (ADR-0010).
        var cmd = text.replace(/^\//, '').split(/\s+/)[0];
        if (cmd === 'refresh') {
          location.reload();
        }
      });
    return;
  }
  appendUser(text);
  beginTurn();
  setRunning(true, state.currentSessionId);
  fetch('/api/message', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text: text, sessionId: state.currentSessionId }) })
    .then(function (r) { return r.json(); })
    .then(function (b) {
      if (b && b.ok === false) {
        setRunning(false);
        appendPre(b.error || 'send failed', 'message error');
      }
    }).catch(function () { setRunning(false); });
}

btnSend.onclick = sendOrStop;
input.addEventListener('keydown', function (e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendOrStop(); }
});

// History first, then live SSE (avoids replay double-paint on refresh).
pageLoader.loadPluginUIs();
loadHistory().then(function () { refreshRunState(); openSSE(); });
