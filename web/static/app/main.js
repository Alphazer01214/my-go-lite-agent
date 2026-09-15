// Shell entry: boot order only. Every face — chat view, session rail, trace,
// composer — is a plugin Panel Component; the layout provides slots, Design
// Tokens, and this loader. Navigation is rendered from the merged layout.

import { state, setSessionId } from './state.js';
import { setPageLoader, createLoader, setPages } from './loader.js';
import { openSSE } from './events.js';
import { openPluginsPanel } from './plugins-panel.js';

var sessionLabel = document.getElementById('session-label');

function notice(text, cls) {
  // The session-view paints notices; before it is mounted, fall back to console.
  if (window.LiteAgent && window.LiteAgent.emit) {
    window.LiteAgent.emit('__notice', { text: text, cls: cls });
  } else {
    console.error(text);
  }
}

// This page's Panel mounts (ADR-0012): sidebar/chat/trace/toolbar/overlay,
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
  // Current Session comes from the session Capability (ADR-0012).
  LiteAgent.call('session', 'current', {}).then(function (b) {
    var id = (b && b.ok !== false && b.result && b.result.sessionId) || '';
    setSessionId(id);
    if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
  }).catch(function () {
    fetch('/api/session').then(function (r) { return r.json(); }).then(function (b) {
      if (b.sessionId !== undefined) setSessionId(b.sessionId);
      if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
    }).catch(function () { });
  });
}

LiteAgent.on('__session', function (sid) {
  sid = sid || '';
  state.currentSessionId = sid;
  window.__liteSessionId = sid;
  sessionLabel.textContent = sid || '(default)';
  // The session-view reloads its history on the same __session event.
});

function renderNav(pages) {
  var host = document.getElementById('layout-nav');
  if (!host || !pages) return;
  host.textContent = '';
  pages.forEach(function (p) {
    if (p.slug === 'main') return;
    var a = document.createElement('a');
    a.href = p.path || ('/' + p.slug);
    a.textContent = p.title || p.slug;
    a.target = '_blank';
    a.rel = 'noreferrer';
    host.appendChild(a);
  });
}

// Components load their own history; the shell only opens the live bridge.
fetch('/api/layout').then(function (r) { return r.json(); }).then(function (lay) {
  if (lay && lay.pages) {
    setPages(lay.pages.map(function (p) { return p.slug; }));
    renderNav(lay.pages);
  }
}).catch(function () { }).then(function () {
  pageLoader.loadPluginUIs();
  refreshRunState();
  openSSE();
});

var btnPlugins = document.getElementById('btn-plugins');
if (btnPlugins) btnPlugins.addEventListener('click', function () { openPluginsPanel(); });
