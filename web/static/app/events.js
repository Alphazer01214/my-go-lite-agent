// SSE bridge: one EventSource for the Shell and every Panel Component.
// L0 only (ADR-0030): fan-out of opaque events; no domain topic semantics
// beyond panel loader + a status line. Generic plugin evts (topic "evt",
// ADR-0034) relay cap/method/payload as-is — e.g. session choice cards.

import { applyPanel } from './loader.js';

function applySSE(env) {
  var topic = env.topic;
  var d = env.data !== undefined ? env.data : env;
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit(topic, d);
  if (topic === 'panel') { applyPanel(d); return; }
  if (topic === 'status') {
    var st = d.status || '';
    var statusEl = document.getElementById('status');
    statusEl.textContent = st;
  }
}

function openSSE() {
  var es = new EventSource('/events');
  es.addEventListener('presentation', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('panel', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('stream', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('status', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('evt', function (e) { applySSE(JSON.parse(e.data)); });
  es.onerror = function () { document.getElementById('status').textContent = 'sse reconnecting…'; };
}

export { applySSE, openSSE };
