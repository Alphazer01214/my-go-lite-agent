// Trace column (center-right): fact rows, live session facts, dump.

import { state, sameSession } from './state.js';
import { esc } from './md.js';
import { mergeReasoningFacts } from './facts.js';
import { appendPre } from './chat.js';

var traceList = document.getElementById('trace-list');

function traceSummary(f) {
  var t = f.type || '';
  var role = f.role || '';
  if (t === 'message') return (role || 'msg') + ' · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'reasoning') return 'thinking · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'tool_call') {
    var names = ((f.meta && f.meta.tool_calls) || []).map(function (tc) { return tc.name; }).join(',');
    return 'call ' + (names || '?');
  }
  if (t === 'tool_result') return 'result · ' + String(f.content || '').replace(/\s+/g, ' ').slice(0, 80);
  if (t === 'request_header') return 'llm request';
  if (t === 'step_start') return 'step ' + ((f.meta && f.meta.step) || '');
  if (t === 'step_end') return 'step end · ' + ((f.meta && f.meta.reason) || '');
  if (t === 'turn_start') return 'turn start';
  if (t === 'turn_end') return 'turn end · ' + ((f.meta && f.meta.reason) || '');
  return t;
}

function shortSid(sid) {
  var s = String(sid === undefined || sid === null ? '' : sid);
  if (!s) return '·';
  return s.length > 8 ? s.slice(0, 8) : s;
}

function factBelongs(f) {
  if (!f) return false;
  return sameSession(f.sessionId);
}

function appendTraceFact(f) {
  if (!f || !f.type) return;
  if (!factBelongs(f)) return;
  var t = f.type;
  var role = f.role || '';
  var cls = t === 'message' && role ? role : t;
  var row = document.createElement('div');
  row.className = 'trace-row ' + cls;
  var kind = t === 'message' ? (role || 'msg') : t;
  row.innerHTML = '<span class="t-kind">' + esc(kind) + '</span><span class="t-sum">' + esc(traceSummary(f)) + '</span><span class="t-seq">' + esc(shortSid(f.sessionId)) + ' #' + (f.seq || '') + '</span>';
  var det = document.createElement('div');
  det.className = 'trace-detail';
  var detail = f.content || '';
  if (f.sessionId !== undefined) detail = 'sessionId: ' + (f.sessionId || '(default)') + '\n\n' + detail;
  if (f.meta) detail += (detail ? '\n\n' : '') + JSON.stringify(f.meta, null, 2);
  det.textContent = detail || '(empty)';
  row.onclick = function () { row.classList.toggle('open'); det.classList.toggle('open'); };
  traceList.appendChild(row); traceList.appendChild(det);
  traceList.scrollTop = traceList.scrollHeight;
}

function dumpTrace() {
  fetch('/api/trace').then(function (r) { return r.json(); }).then(function (b) {
    var sid = b.sessionId !== undefined && b.sessionId !== null ? b.sessionId : (state.currentSessionId || '');
    var payload = {
      exportedAt: new Date().toISOString(),
      sessionId: sid,
      currentViewSessionId: state.currentSessionId || '',
      facts: b.facts || []
    };
    var name = 'trace-' + (sid || 'default') + '-' + new Date().toISOString().replace(/[:.]/g, '-') + '.json';
    var blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(function () { URL.revokeObjectURL(url); }, 1000);
    appendPre('dumped ' + (b.facts || []).length + ' facts → ' + name, 'message');
  }).catch(function (e) { appendPre('dump failed: ' + e, 'message error'); });
}

function loadTrace() {
  fetch('/api/trace').then(function (r) { return r.json(); }).then(function (b) {
    var facts = mergeReasoningFacts(b.facts || []);
    if (b.sessionId) document.getElementById('session-label').textContent = b.sessionId;
    traceList.innerHTML = '';
    facts.forEach(appendTraceFact);
    traceList.scrollTop = traceList.scrollHeight;
  }).catch(function () { });
}

export { appendTraceFact, dumpTrace, loadTrace };
