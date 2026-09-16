// Shell entry: boot order only. Every content face is a plugin Panel Component
// (ADR-0011). Settings chrome is medium framework; per-plugin settings faces
// are <name>-settings custom elements owned by each plugin.

import { state, setSessionId } from './state.js';
import { setPageLoader, createLoader, setPages } from './loader.js';
import { openSSE } from './events.js';
import { openPluginsPanel, prefetchPlugins } from './plugins-panel.js';
import { openSettingsPanel } from './settings.js';

var sessionLabel = document.getElementById('session-label');

function notice(text, cls) {
  if (window.LiteAgent && window.LiteAgent.emit) {
    window.LiteAgent.emit('__notice', { text: text, cls: cls });
  } else {
    console.error(text);
  }
}

const pageLoader = createLoader('main', function panelHost(slot) {
  if (slot === 'sidebar') return document.getElementById('rail');
  if (slot === 'main-overlay') return document.getElementById('main-overlay');
  if (slot === 'trace') return document.getElementById('slot-trace');
  if (slot === 'statusbar') return document.getElementById('statusbar');
  if (slot === 'chat') return document.getElementById('chat');
  return document.getElementById('slot-toolbar-right');
}, function (msg) { notice(msg, 'message error'); });
setPageLoader(pageLoader);

function refreshRunState() {
  // The Host still knows a Current Session, but the Shell must NOT announce it:
  // the Session View opens on its new-session face and only loads a Session
  // when the user picks one (or sends the first message). Hence silent=true.
  LiteAgent.call('session', 'current', {}).then(function (b) {
    var id = (b && b.ok !== false && b.result && b.result.sessionId) || '';
    setSessionId(id, true);
    if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
  }).catch(function () {
    fetch('/api/session').then(function (r) { return r.json(); }).then(function (b) {
      if (b.sessionId !== undefined) setSessionId(b.sessionId, true);
      if (state.currentSessionId) sessionLabel.textContent = state.currentSessionId;
    }).catch(function () { });
  });
}

LiteAgent.on('__session', function (sid) {
  sid = sid || '';
  state.currentSessionId = sid;
  window.__liteSessionId = sid;
  sessionLabel.textContent = sid || '(default)';
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

fetch('/api/layout').then(function (r) { return r.json(); }).then(function (lay) {
  if (lay && lay.pages) {
    setPages(lay.pages.map(function (p) { return p.slug; }));
    renderNav(lay.pages);
  }
}).catch(function () { }).then(function () {
  pageLoader.loadPluginUIs();
  // Warm the plugin-graph cache now: under Autostart+dependsOn most plugins
  // mount lazily on the first Turn, so waiting would show a half-empty graph.
  prefetchPlugins();
  refreshRunState();
  openSSE();
});

var btnPlugins = document.getElementById('btn-plugins');
if (btnPlugins) btnPlugins.addEventListener('click', function () { openPluginsPanel(); });
var btnSettings = document.getElementById('btn-settings');
if (btnSettings) btnSettings.addEventListener('click', function () { openSettingsPanel(); });