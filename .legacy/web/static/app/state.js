// Shared Shell state (module form of the former inline-IIFE globals).
// The session-view owns Send/Stop state; the shell only tracks which
// Session is current so it can label the pane and route events.

export var state = {
  currentSessionId: '',
};

// Session channel for Panel Components (LiteAgent.onSessionChange rides on this).
// silent=true records the Host's Current Session without announcing a switch:
// the Shell uses it at boot so the Session View stays on its new-session face.
export function setSessionId(id, silent) {
  id = id || '';
  if (window.__liteSessionId === id && state.currentSessionId === id) return;
  state.currentSessionId = id;
  window.__liteSessionId = id;
  if (silent) return;
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit('__session', id);
}
