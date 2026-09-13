// Shell entry: composer, session orchestration, and boot order. The face
// modules (chat/trace/panels/rail/events) hold the rendering; this module
// conducts them exactly as the former inline IIFE did.

import { state, setRunning, setSessionId } from './state.js';
import { appendUser, appendPre, clearTurnUI, beginTurn, resetChatUI, loadHistory, maybeResumeLiveThinking } from './chat.js';
import { dumpTrace, loadTrace } from './trace.js';
import { loadPluginUIs } from './panels.js';
import { loadSessions, onSelectSession } from './rail.js';
import { openSSE } from './events.js';

var input = document.getElementById('input');
var btnSend = document.getElementById('btn-send');
var btnNew = document.getElementById('btn-new');
var sessionLabel = document.getElementById('session-label');

function selectSession(id) {
  fetch('/api/session/select', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ sessionId: id }) })
    .then(function (r) { return r.json(); }).then(function (b) {
      setSessionId(id);
      sessionLabel.textContent = id || '(default)';
      state.historyReady = false;
      clearTurnUI();
      setRunning(false);
      return loadHistory();
    }).then(function () {
      state.historyReady = true;
      loadTrace(); loadSessions(); refreshRunState();
    });
}

function refreshRunState() {
  fetch('/api/session').then(function (r) { return r.json(); }).then(function (b) {
    if (b.sessionId !== undefined) setSessionId(b.sessionId);
    if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
    setRunning(b.status === 'running', state.currentSessionId);
    // Mid-run switch/refresh: resume live Thinking from rebuilt facts.
    maybeResumeLiveThinking(b.status === 'running');
  }).catch(function () { });
}

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
var btnDump = document.getElementById('btn-dump');
if (btnDump) btnDump.onclick = dumpTrace;
input.addEventListener('keydown', function (e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendOrStop(); }
});
btnNew.onclick = function () {
  fetch('/api/session/new', { method: 'POST' }).then(function (r) { return r.json(); }).then(function (b) {
    if (!b.ok) { appendPre(b.error || 'new session failed', 'message error'); return; }
    setSessionId(b.sessionId);
    sessionLabel.textContent = b.sessionId || '(default)';
    resetChatUI();
    setRunning(false);
    state.historyReady = true;
    loadTrace(); loadSessions(); refreshRunState();
    appendPre('new session ' + (b.sessionId || ''), 'message');
  });
};

onSelectSession(selectSession);

// History first, then live SSE (avoids replay double-paint on refresh).
loadSessions();
loadPluginUIs();
loadHistory().then(function () { loadTrace(); refreshRunState(); openSSE(); });
