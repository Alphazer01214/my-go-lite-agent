// /trace debug page entry (ADR-0011): a pure slot grid filled by plugin
// components — the session plugin's session-trace provides the content.

import { createLoader } from './loader.js';

const loader = createLoader('trace', function panelHost(slot) {
  if (slot === 'main') return document.getElementById('slot-main');
  return null; // this page provides no other slots
}, function (msg) { console.error('trace page:', msg); });

loader.loadPluginUIs();
