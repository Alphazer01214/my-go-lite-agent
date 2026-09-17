// Shell entry: boot order only. Every content face is a plugin Panel Component
// (ADR-0011). Settings chrome is medium framework; per-plugin settings faces
// are <name>-settings custom elements owned by each plugin.
// L0 only (ADR-0030): no session/turn domain orchestration in the Shell.

import { setPageLoader, createLoader, setPages } from './loader.js';
import { openSSE } from './events.js';
import { openPluginsPanel, prefetchPlugins } from './plugins-panel.js';
import { openSettingsPanel } from './settings.js';

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
  prefetchPlugins();
  openSSE();
});

// Re-export settings opener for the chrome button.
window.__liteOpenSettings = openSettingsPanel;
