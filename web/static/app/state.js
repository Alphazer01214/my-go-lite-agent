// Shared Shell state (module form of the former inline-IIFE globals).
// DOM lookups are lazy so importing modules never races page parse.

export var state = {
  currentSessionId: '',
  historyReady: false,
  running: false,
};

export function sameSession(sid) {
  // Missing sessionId means the default Session (""), not "any session".
  if (sid === undefined || sid === null) return !state.currentSessionId;
  return String(sid) === String(state.currentSessionId || '');
}

export function setRunning(on, sid) {
  var show = !!on && sameSession(sid);
  state.running = show;
  var btnSend = document.getElementById('btn-send');
  btnSend.classList.toggle('running', show);
  btnSend.textContent = show ? 'Stop' : 'Send';
  btnSend.title = show ? 'Stop generation' : 'Send';
}

// Session channel for Panel Components (LiteAgent.onSessionChange rides on this).
export function setSessionId(id) {
  id = id || '';
  if (window.__liteSessionId === id && state.currentSessionId === id) return;
  state.currentSessionId = id;
  window.__liteSessionId = id;
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit('__session', id);
}
