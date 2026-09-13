// Shared Shell state (module form of the former inline-IIFE globals).
// The session-view owns Send/Stop state; the shell only tracks which
// Session is current so it can label the pane and route events.

export var state = {
  currentSessionId: '',
};

// Session channel for Panel Components (LiteAgent.onSessionChange rides on this).
export function setSessionId(id) {
  id = id || '';
  if (window.__liteSessionId === id && state.currentSessionId === id) return;
  state.currentSessionId = id;
  window.__liteSessionId = id;
  if (window.LiteAgent && window.LiteAgent.emit) window.LiteAgent.emit('__session', id);
}
