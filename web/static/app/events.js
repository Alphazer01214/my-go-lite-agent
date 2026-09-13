// SSE bridge: one EventSource for the Shell and every Panel Component.
// Everything except the composer status line fans out through LiteAgent —
// the faces (session-view etc.) consume their own topics and queue until
// their history is ready.

import { setRunning } from './state.js';
import { applyPanel } from './loader.js';

function applySSE(env) {
  var topic = env.topic;
  var d = env.data !== undefined ? env.data : env;
  // Fan live events out to Panel Components subscribed via LiteAgent.on —
  // presentation/stream/session go to the session-view; panel ops go to
  // the page loader; status also drives the composer's Send/Stop state.
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit(topic, d);
  if (topic === 'panel') { applyPanel(d); return; }
  if (topic === 'status') {
    var st = d.status || '';
    var sid = d.sessionId !== undefined ? d.sessionId : '';
    var statusEl = document.getElementById('status');
    if (st.indexOf('session:') === 0) {
      statusEl.textContent = st;
      return;
    }
    if (st === 'running') {
      statusEl.textContent = 'running';
      setRunning(true, sid);
      return;
    }
    if (st === 'idle' || st.indexOf('error:') === 0 || st === 'cancelling') {
      statusEl.textContent = st;
      if (!sid || String(sid) === String(window.__liteSessionId || '')) {
        setRunning(false);
      }
    }
  }
}

function openSSE() {
  // No replay: durable state rehydrates from the Session Log via
  // capabilities; SSE is live-only so refresh never double-paints.
  var es = new EventSource('/events');
  es.addEventListener('presentation', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('panel', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('stream', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('session', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('status', function (e) { applySSE(JSON.parse(e.data)); });
  es.onerror = function () { document.getElementById('status').textContent = 'sse reconnecting…'; };
}

export { applySSE, openSSE };
