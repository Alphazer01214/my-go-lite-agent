// SSE bridge: one EventSource for the Shell and every Panel Component.
// Live-only — history/trace rehydrate durable state so refresh never
// double-paints old presentation events.

import { state, sameSession, setRunning } from './state.js';
import { onPresentation, onStreamDelta, clearTurnUI } from './chat.js';
import { applyPanel } from './loader.js';

function applySSE(env) {
  if (!state.historyReady && env.topic !== 'panel') return;
  var topic = env.topic;
  var d = env.data !== undefined ? env.data : env;
  // Fan live events out to Panel Components subscribed via LiteAgent.on —
  // including topic=session, which the session plugin's trace view consumes.
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit(topic, d);
  if (topic === 'presentation') { onPresentation(d); return; }
  if (topic === 'panel') { applyPanel(d); return; }
  if (topic === 'stream') {
    if (!sameSession(d.sessionId)) return;
    onStreamDelta(d.channel === 'reasoning' ? 'reasoning' : 'content', d.delta || '');
    return;
  }
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
      if (sameSession(sid)) {
        setRunning(false);
        clearTurnUI();
      }
      // Session list refresh is owned by the session rail component.
    }
  }
}

function openSSE() {
  // No replay: history/trace rehydrate durable state; SSE is live-only so
  // refresh does not double-paint old presentation events.
  var es = new EventSource('/events');
  es.addEventListener('presentation', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('panel', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('stream', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('session', function (e) { applySSE(JSON.parse(e.data)); });
  es.addEventListener('status', function (e) { applySSE(JSON.parse(e.data)); });
  es.onerror = function () { document.getElementById('status').textContent = 'sse reconnecting…'; };
}

export { applySSE, openSSE };
