// Standalone /trace page renderer (extracted verbatim from trace.html).

import { esc } from './md.js';

function render(facts) {
  var list = document.getElementById('list');
  list.innerHTML = '';
  document.getElementById('count').textContent = facts.length + ' facts';
  facts.forEach(function (f) {
    var t = f.type || 'message';
    var role = f.role || '';
    var el = document.createElement('div');
    el.className = 'item ' + t + (role === 'user' || role === 'assistant' || role === 'system' ? ' ' + role : '');
    var title = t;
    if (t === 'message') title = role || 'message';
    var body = '';
    var mono = false;
    if (t === 'message') {
      body = f.content || '';
    } else if (t === 'tool_call') {
      var tcs = (f.meta && f.meta.tool_calls) || [];
      body = tcs.map(function (tc) {
        return (tc.name || 'tool') + ' ' + (tc.arguments ? JSON.stringify(tc.arguments) : '');
      }).join('\n');
      if (!body) body = f.content || '';
      mono = true;
    } else if (t === 'tool_result') {
      body = f.content || '';
      mono = true;
    } else if (t === 'request_header') {
      body = JSON.stringify(f.meta || {}, null, 0);
      mono = true;
    } else if (t === 'step_start' || t === 'step_end' || t === 'turn_start' || t === 'turn_end') {
      body = JSON.stringify(f.meta || {});
      mono = true;
    } else {
      body = f.content || JSON.stringify(f.meta || {});
      mono = true;
    }
    el.innerHTML = '<span class="seq">#' + (f.seq || '') + '</span><div class="kind">' + esc(title) + '</div>' +
      (f.content && t !== 'message' && t !== 'tool_result' ? '<div class="role">' + esc(String(f.content).slice(0, 120)) + '</div>' : '') +
      '<div class="body' + (mono ? ' mono' : '') + '">' + esc(body) + '</div>';
    list.appendChild(el);
  });
}

function load() {
  fetch('/api/trace').then(function (r) { return r.json(); }).then(function (b) {
    render(b.facts || []);
  });
}

load();
setInterval(load, 2000);
