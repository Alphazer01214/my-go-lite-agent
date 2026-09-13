// Session rail (left column): list + selection entries.

export function loadSessions() {
  fetch('/api/sessions').then(function (r) { return r.json(); }).then(function (b) {
    var cur = b.current || '';
    var list = b.sessions || [];
    var sessionList = document.getElementById('session-list');
    sessionList.innerHTML = '';
    if (!list.length) {
      var empty = document.createElement('div');
      empty.className = 'sess-item';
      empty.textContent = 'No sessions yet';
      sessionList.appendChild(empty);
      return;
    }
    list.forEach(function (s) {
      var id = s.id || '';
      var title = s.title || id;
      var el = document.createElement('div');
      el.className = 'sess-item' + (id === cur ? ' active' : '');
      el.textContent = title;
      el.title = id;
      el.onclick = function () { selectSession(id); };
      sessionList.appendChild(el);
    });
  }).catch(function () { });
}

// selectSession is wired by main.js (it orchestrates chat/trace/history);
// rail.js only renders the entries.
var selectSession = function () { };

export function onSelectSession(fn) {
  selectSession = fn;
}
