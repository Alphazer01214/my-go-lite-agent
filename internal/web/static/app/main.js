// Shell entry: boot order only. Content faces are plugin Panel Components
// (ADR-0011/0031). Shell exposes top/bottom + left|center|right; domain UI is plugin-owned.
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

// ADR-0031: only top/bottom/left/center/right are Shell-hosted slots.
const pageLoader = createLoader('main', function panelHost(slot) {
  if (slot === 'top') return document.getElementById('region-top');
  if (slot === 'bottom') return document.getElementById('region-bottom');
  if (slot === 'left') return document.getElementById('region-left');
  if (slot === 'center') return document.getElementById('region-center');
  if (slot === 'right') return document.getElementById('region-right');
  return null;
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
  wireChromeButtons();
});

// Shell chrome buttons (framework overlays — not plugin region mounts).
function wireChromeButtons() {
  var pluginsBtn = document.getElementById('btn-plugins');
  if (pluginsBtn) pluginsBtn.addEventListener('click', function () { openPluginsPanel(); });
  var settingsBtn = document.getElementById('btn-settings');
  if (settingsBtn) settingsBtn.addEventListener('click', function () { openSettingsPanel(); });
}

window.__liteOpenSettings = openSettingsPanel;
window.__liteOpenPlugins = openPluginsPanel;
