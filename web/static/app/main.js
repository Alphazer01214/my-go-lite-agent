// Shell entry: composer and boot order. The faces (session-view/rail/trace
// and any plugin Panel Components) hold the rendering; this module conducts
// the composer and announces composer events to the view over private
// topics (__turn-start / __notice).

import { state, setRunning, setSessionId } from './state.js';
import { setPageLoader, createLoader } from './loader.js';
import { openSSE } from './events.js';

var input = document.getElementById('input');
var btnSend = document.getElementById('btn-send');
var sessionLabel = document.getElementById('session-label');

function notice(text, cls) {
  // The session-view paints notices; before it is mounted, fall back to console.
  if (window.LiteAgent && window.LiteAgent.emit) {
    window.LiteAgent.emit('__notice', { text: text, cls: cls });
  } else {
    console.error(text);
  }
}

// This page's Panel mounts (ADR-0011): sidebar/chat/trace/toolbar/overlay,
// filled by plugin components.
const pageLoader = createLoader('main', function panelHost(slot) {
  if (slot === 'sidebar') return document.getElementById('rail');
  if (slot === 'main-overlay') return document.getElementById('main-overlay');
  if (slot === 'trace') return document.getElementById('slot-trace');
  if (slot === 'chat') return document.getElementById('chat');
  return document.getElementById('slot-toolbar-right');
}, function (msg) { notice(msg, 'message error'); });
setPageLoader(pageLoader);

function refreshRunState() {
  fetch('/api/session').then(function (r) { return r.json(); }).then(function (b) {
    if (b.sessionId !== undefined) setSessionId(b.sessionId);
    if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
    setRunning(b.status === 'running', state.currentSessionId);
  }).catch(function () { });
}

LiteAgent.on('__session', function (sid) {
  sid = sid || '';
  state.currentSessionId = sid;
  window.__liteSessionId = sid;
  sessionLabel.textContent = sid || '(default)';
  setRunning(false);
  // The session-view reloads its history on the same __session event.
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
    window.LiteAgent.emit('__turn-start', { text: text, sessionId: state.currentSessionId });
    fetch('/api/command', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ line: text }) })
      .then(function (r) { return r.json(); })
      .then(function (body) {
        if (body.error) notice(body.error, 'message error');
        else if (body.output) notice(body.output, 'message');
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
  window.LiteAgent.emit('__turn-start', { text: text, sessionId: state.currentSessionId });
  setRunning(true, state.currentSessionId);
  fetch('/api/message', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ text: text, sessionId: state.currentSessionId }) })
    .then(function (r) { return r.json(); })
    .then(function (b) {
      if (b && b.ok === false) {
        setRunning(false);
        notice(b.error || 'send failed', 'message error');
      }
    }).catch(function () { setRunning(false); });
}

btnSend.onclick = sendOrStop;
input.addEventListener('keydown', function (e) {
  if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendOrStop(); }
});

// Components load their own history; the shell only opens the live bridge.
pageLoader.loadPluginUIs();
refreshRunState();
openSSE();
