// Shell entry: boot order only. Every face — chat view, session rail, trace,
// composer — is a plugin Panel Component; the layout provides slots, Design
// Tokens, and this loader. Composer events reach the session-view over the
// private __notice topic.

import { state, setSessionId } from './state.js';
import { setPageLoader, createLoader } from './loader.js';
import { openSSE } from './events.js';

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
  }).catch(function () { });
}

LiteAgent.on('__session', function (sid) {
  sid = sid || '';
  state.currentSessionId = sid;
  window.__liteSessionId = sid;
  sessionLabel.textContent = sid || '(default)';
  // The session-view reloads its history on the same __session event.
});

// Components load their own history; the shell only opens the live bridge.
pageLoader.loadPluginUIs();
refreshRunState();
openSSE();
