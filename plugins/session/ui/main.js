// Session WebUI. Mounts render Runtime-pushed State; the browser never decides
// which session is current. Theme hooks are plugin-owned (--session-*).
const styles = `
*, *::before, *::after { box-sizing: border-box; }
:host {
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  height: 100%;
  min-height: 0;
  padding: 12px;
  color: var(--session-text, #202124);
  font: 14px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif;
}
h1, h2, h3, h4, p, ul, li, pre { margin: 0; }
button, input, textarea, select { font: inherit; color: inherit; }
button {
  padding: 7px 11px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
  cursor: pointer;
}
button:hover:not(:disabled) { border-color: var(--session-accent, #147d64); }
button:disabled { cursor: not-allowed; opacity: .55; }
button:focus-visible, input:focus-visible, textarea:focus-visible, select:focus-visible {
  outline: 2px solid var(--session-accent, #147d64);
  outline-offset: 2px;
}
.hint { color: var(--session-muted-text, #5c6875); font-size: 12px; }
.empty { padding: 24px 8px; color: var(--session-muted-text, #5c6875); font-size: 13px; text-align: center; }
.error { margin-top: 6px; color: var(--session-danger, #b91c1c); font-size: 12px; overflow-wrap: anywhere; }

.history-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.history-head .grow { flex: 1; min-width: 0; }
.list {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  align-content: start;
  gap: 6px;
  padding: 0;
  margin: 0;
  overflow: auto;
  list-style: none;
}
.item {
  display: block;
  width: 100%;
  padding: 8px 10px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  text-align: left;
}
.item:hover:not(:disabled) { border-color: var(--session-border, #d5dbe1); background: var(--session-surface-muted, #eef1f4); }
.item.active { border-color: var(--session-accent, #147d64); background: #e6f5f0; }
.item .name { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.item.active .name { color: var(--session-accent-strong, #0c604c); }
.item .meta { display: block; margin-top: 2px; color: var(--session-muted-text, #5c6875); font-size: 12px; }
.item .open-badge { float: right; margin-left: 6px; color: var(--session-accent, #147d64); font-size: 11px; }

.toolbar { display: flex; align-items: flex-end; gap: 8px; margin-bottom: 8px; }
.toolbar textarea {
  flex: 1;
  min-width: 0;
  max-height: 132px;
  padding: 8px 10px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
  resize: none;
}
.toolbar textarea:disabled { background: var(--session-surface-muted, #eef1f4); }
.status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  color: var(--session-muted-text, #5c6875);
  font-size: 12px;
}
.status[hidden] { display: none; }
.status::before {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--session-accent, #147d64);
  content: "";
  animation: pulse 1.1s ease-in-out infinite;
}
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .25; } }
.history-search {
  width: 100%;
  min-height: 34px;
  padding: 6px 9px;
  margin-bottom: 8px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
}
.workspace-picker {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  margin-bottom: 8px;
}
.workspace-picker select {
  flex: 1;
  min-width: 0;
  padding: 6px 8px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
}
.workspace-picker button {
  flex: 0 0 auto;
  white-space: nowrap;
}
.model-row {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}
.model-row[hidden] { display: none; }
.model-row label {
  flex: 0 0 auto;
  color: var(--session-muted-text, #5c6875);
  font-size: 12px;
}
.model-row select {
  flex: 1;
  min-width: 0;
  padding: 6px 8px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
}
.chat-tabs {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  min-height: 36px;
  padding-bottom: 8px;
  margin-bottom: 8px;
  overflow-x: auto;
  border-bottom: 1px solid var(--session-border, #d5dbe1);
}
.chat-tabs:empty { display: none; }
.chat-tab {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  padding: 2px 3px 2px 9px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
}
.chat-tab.active {
  border-color: var(--session-accent, #147d64);
  background: #e6f5f0;
}
.chat-tab .tab-select {
  max-width: 190px;
  overflow: hidden;
  padding: 4px 2px;
  border: 0;
  background: transparent;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.chat-tab .tab-close {
  min-width: 24px;
  padding: 2px 5px;
  border: 0;
  background: transparent;
  color: var(--session-muted-text, #5c6875);
}
.chat-tab .tab-running {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--session-accent, #147d64);
  animation: pulse 1.1s ease-in-out infinite;
}
.workspace-row {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.workspace-row label {
  flex: 0 0 auto;
  color: var(--session-muted-text, #5c6875);
  font-size: 12px;
}
.workspace-path {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: var(--session-muted-text, #5c6875);
  font: 12px/1.35 ui-monospace, SFMono-Regular, Consolas, monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.workspace-warning {
  margin-bottom: 8px;
  color: var(--session-warning, #a45b12);
}
.no-pane {
  display: flex;
  flex: 1 1 auto;
  align-items: center;
  justify-content: center;
  color: var(--session-muted-text, #5c6875);
  font-size: 13px;
}
.chat-stage {
  position: relative;
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
}
.load-earlier {
  align-self: center;
  flex: 0 0 auto;
  margin-bottom: 6px;
  font-size: 12px;
}
.scroll-latest {
  position: absolute;
  right: 12px;
  bottom: 12px;
  min-height: 30px;
  padding: 4px 10px;
  border-color: var(--session-accent, #147d64);
  box-shadow: 0 4px 14px rgba(20, 32, 43, .14);
}
.toolbar.composer {
  align-items: flex-end;
  margin: 8px 0 0;
  padding-top: 8px;
  border-top: 1px solid var(--session-border, #d5dbe1);
}
.message {
  position: relative;
  border-left-width: 2px;
}
.message.user {
  margin-left: 8%;
  background: color-mix(in srgb, var(--session-accent, #147d64) 7%, var(--session-surface, #ffffff));
}
.message.assistant {
  margin-right: 4%;
}
.message-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.message-header .role {
  margin: 0;
}
.message-time {
  color: var(--session-muted-text, #5c6875);
  font-size: 11px;
}
.message-copy {
  min-height: 24px;
  padding: 2px 7px;
  margin-left: auto;
  border: 0;
  color: var(--session-muted-text, #5c6875);
  background: transparent;
  font-size: 11px;
  opacity: 0;
}
.message:hover .message-copy,
.message-copy:focus-visible {
  opacity: 1;
}
.tool-calls,
.reasoning {
  margin-top: 6px;
}
.tool-calls summary {
  cursor: pointer;
  color: var(--session-warning, #a45b12);
  font-size: 12px;
}
.tool-json {
  max-height: 240px;
  overflow: auto;
  margin-top: 4px;
  padding: 7px 9px;
  border-radius: 5px;
  background: var(--session-surface-muted, #eef1f4);
  white-space: pre-wrap;
  font: 11px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace;
}
.tool-content .content {
  max-height: 320px;
  overflow: auto;
}
.tool-more {
  min-height: 24px;
  margin-top: 4px;
  padding: 2px 7px;
  border: 0;
  background: transparent;
  font-size: 11px;
}
.code-block-wrap {
  margin: 6px 0;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  overflow: hidden;
  background: var(--session-surface-muted, #eef1f4);
}
.code-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 28px;
  padding: 2px 8px;
  border-bottom: 1px solid var(--session-border, #d5dbe1);
  color: var(--session-muted-text, #5c6875);
  font-size: 11px;
}
.code-copy {
  min-height: 22px;
  padding: 1px 7px;
  border: 0;
  background: transparent;
  font-size: 11px;
}
.code-block-wrap .code-block {
  margin: 0;
  border: 0;
  border-radius: 0;
}
.message.draft {
  border-left-style: dashed;
}

.turn-fold { display: flex; flex: 0 0 auto; }
.turn-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px 6px;
  border: 0;
  background: transparent;
  color: var(--session-muted-text, #5c6875);
  font: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}
.turn-toggle:hover:not(:disabled) { color: var(--session-accent, #147d64); }
.turn-toggle:disabled { cursor: default; opacity: 1; }
.turn-chevron {
  flex: 0 0 12px;
  width: 12px;
  color: var(--session-muted-text, #5c6875);
}
.turn-title { color: var(--session-text, #202124); font-weight: 500; }
.turn-meta { color: var(--session-muted-text, #5c6875); }
.turn-fold.running .turn-title { color: var(--session-accent, #147d64); }
.turn-fold.running .turn-title::before {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 50%;
  background: currentColor;
  content: "";
  animation: pulse 1.1s ease-in-out infinite;
}
.reasoning-preview {
  margin-left: 6px;
  color: var(--session-muted-text, #5c6875);
  font-size: 11px;
}
.tool-result summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  cursor: pointer;
}
.tool-result .badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
  font-size: 11px;
}
.tool-result .badge.error { color: var(--session-danger, #b91c1c); background: #fbecec; }
.tool-result .badge.warn { color: #a45b12; background: #fdf3e7; }
.tool-result .tool-preview {
  flex: 1 1 160px;
  min-width: 0;
  overflow: hidden;
  color: var(--session-muted-text, #5c6875);
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tool-result .tool-output {
  max-height: 300px;
  margin: 6px 0 0;
  padding: 8px;
  overflow: auto;
  border-radius: 5px;
  background: var(--session-surface-muted, #eef1f4);
  white-space: pre-wrap;
  font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace;
}

.messages {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  gap: 8px;
  overflow: auto;
  padding-right: 4px;
}
.message {
  padding: 8px 10px;
  border-left: 3px solid var(--session-border, #d5dbe1);
  border-radius: 0 6px 6px 0;
  background: var(--session-surface, #ffffff);
}
.message.user { border-left-color: var(--session-accent, #147d64); }
.message.assistant { border-left-color: var(--session-assistant, #22c55e); }
.message.tool { border-left-color: var(--session-warning, #a45b12); }
.message.context-block {
  border-left-color: var(--session-context, #64748b);
  border-left-style: dashed;
  background: var(--session-surface-muted, #f6f8fa);
}
.message.context-block summary { cursor: pointer; }
.message.context-block .content { margin-top: 4px; }
.message .role { display: block; margin-bottom: 3px; color: var(--session-muted-text, #5c6875); font-size: 12px; }
.message .role.thinking { color: var(--session-thinking, #7c3aed); }
.content { overflow-wrap: anywhere; }
.message .content > * {
  content-visibility: auto;
  contain-intrinsic-size: auto 1.5em;
}
.rich-line { min-height: 1.45em; }
.rich-heading { margin: 4px 0 2px; font-size: 1.05em; }
.rich-list { margin: 2px 0; padding-left: 20px; }
.inline-code {
  padding: 1px 4px;
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: .92em;
}
.code-block {
  max-height: 340px;
  overflow: auto;
  margin: 6px 0;
  padding: 8px 10px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface-muted, #eef1f4);
  white-space: pre;
  font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace;
}
.reasoning { margin-top: 6px; }
.reasoning summary { cursor: pointer; color: var(--session-thinking, #7c3aed); font-size: 12px; }
.reasoning .reasoning-body {
  margin-top: 4px;
  color: var(--session-muted-text, #5c6875);
  font-size: 12px;
  white-space: pre-wrap;
}

.trace-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}
.trace-filter {
  padding: 4px 6px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
  font-size: 12px;
}
.trace-list {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  align-content: start;
  gap: 4px;
  padding: 0;
  margin: 0;
  overflow: auto;
  list-style: none;
}
.fact { border: 1px solid var(--session-border, #d5dbe1); border-radius: 6px; background: var(--session-surface, #ffffff); }
.fact summary { padding: 6px 8px; cursor: pointer; overflow-wrap: anywhere; font-size: 12px; }
.fact .seq { color: var(--session-muted-text, #5c6875); }
.fact .badge {
  display: inline-block;
  margin: 0 4px;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
  font-size: 11px;
}
.fact .preview { color: var(--session-muted-text, #5c6875); }
.fact pre {
  max-height: 300px;
  overflow: auto;
  padding: 8px;
  border-top: 1px solid var(--session-border, #d5dbe1);
  background: var(--session-surface-muted, #eef1f4);
  white-space: pre;
  font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace;
}
.trace-more { margin-top: 6px; width: 100%; font-size: 12px; }
`;

const dockStyles = `
*, *::before, *::after { box-sizing: border-box; }
:host {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 40px;
  padding: 0 12px;
  color: var(--session-text, #202124);
  background: var(--session-surface, #ffffff);
  font: 12px/1.3 system-ui, -apple-system, "Segoe UI", sans-serif;
}
:host([hidden]) { display: none; }
.dock-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  overflow-x: auto;
}
.chip {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  min-height: 26px;
  padding: 3px 9px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 13px;
  background: var(--session-surface, #ffffff);
  color: var(--session-text, #202124);
  font: inherit;
  white-space: nowrap;
}
button.chip { cursor: pointer; }
button.chip:hover { border-color: var(--session-accent, #147d64); }
button.chip:focus-visible, .panel-close:focus-visible {
  outline: 2px solid var(--session-accent, #147d64);
  outline-offset: 2px;
}
.chip .label { color: var(--session-muted-text, #5c6875); }
.chip strong { font-weight: 600; }
.chip .value { font-variant-numeric: tabular-nums; }
.chip.warn { border-color: #d9a441; color: #8a5a00; }
.ring { width: 14px; height: 14px; flex: 0 0 auto; }
.ring .track { fill: none; stroke: var(--session-border, #d5dbe1); stroke-width: 2; }
.ring .fill {
  fill: none;
  stroke: var(--session-accent, #147d64);
  stroke-width: 2;
  stroke-linecap: round;
  transition: stroke-dasharray .18s ease;
}
.ring.warn .fill { stroke: #d97706; }
.spacer { flex: 1 1 auto; }
.turn-status {
  flex: 0 0 auto;
  color: var(--session-muted-text, #5c6875);
  white-space: nowrap;
}
.turn-status.running { color: var(--session-accent, #147d64); }
.turn-status.running::before {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 6px;
  border-radius: 50%;
  background: currentColor;
  content: "";
  animation: dock-pulse 1.1s ease-in-out infinite;
}
.turn-status-row {
  margin: 8px 0 12px;
  padding: 7px 10px;
  border-left: 3px solid var(--session-border, #d5dbe1);
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
  color: var(--session-muted-text, #5c6875);
  font-size: 12px;
  overflow-wrap: anywhere;
}
.turn-status-row.error {
  border-left-color: var(--session-danger, #b91c1c);
  background: #fbecec;
  color: var(--session-danger, #b91c1c);
}
.turn-status-row.cancelled {
  border-left-color: var(--session-warning, #a45b12);
  background: #fff6e8;
  color: var(--session-warning, #a45b12);
}
@keyframes dock-pulse { 0%, 100% { opacity: 1; } 50% { opacity: .25; } }
.dock-backdrop {
  position: fixed;
  z-index: 40;
  inset: 0;
  background: rgba(24, 33, 43, .18);
}
.dock-backdrop[hidden] { display: none; }
.dock-panel {
  position: fixed;
  z-index: 41;
  left: 50%;
  bottom: 50px;
  transform: translateX(-50%);
  width: min(600px, calc(100vw - 24px));
  max-height: min(70vh, 640px);
  overflow: auto;
  padding: 14px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 10px;
  background: var(--session-surface, #ffffff);
  box-shadow: 0 18px 48px rgba(24, 33, 43, .22);
}
.dock-panel[hidden] { display: none; }
.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 10px;
}
.panel-close {
  min-width: 28px;
  padding: 2px 8px;
  border: 0;
  background: transparent;
  color: var(--session-text, #202124);
  font: 18px/1 system-ui, sans-serif;
  cursor: pointer;
}
.context-line { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.progress {
  flex: 1 1 auto;
  height: 7px;
  overflow: hidden;
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
}
.progress > span {
  display: block;
  height: 100%;
  background: var(--session-accent, #147d64);
  transition: width .18s ease;
}
.progress.warn > span { background: #d97706; }
.context-reading { flex: 0 0 auto; font-variant-numeric: tabular-nums; }
.summary { display: grid; grid-template-columns: 1fr auto; gap: 5px 14px; margin: 0; }
.summary dt { color: var(--session-muted-text, #5c6875); }
.summary dd { margin: 0; text-align: right; font-variant-numeric: tabular-nums; }
.section-title { margin: 14px 0 6px; font-weight: 600; }
.empty-line { padding: 6px 0; color: var(--session-muted-text, #5c6875); }
.warnings { margin: 6px 0 0; padding-left: 18px; color: var(--session-warning, #a45b12); }
.note { margin-top: 12px; color: var(--session-muted-text, #5c6875); font-size: 11px; }
.inspect-tabs { display: flex; align-items: center; gap: 6px; margin: 8px 0; }
.panel-tab, .panel-refresh {
  padding: 4px 9px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 13px;
  background: var(--session-surface, #ffffff);
  color: var(--session-text, #202124);
  font: inherit;
  cursor: pointer;
}
.panel-tab.active {
  border-color: var(--session-accent, #147d64);
  background: #e6f5f0;
  color: var(--session-accent-strong, #0c604c);
}
.panel-refresh { margin-left: auto; }
.inspect-status { padding: 6px 0; color: var(--session-muted-text, #5c6875); }
.inspect-error { padding: 6px 0; color: var(--session-danger, #b91c1c); overflow-wrap: anywhere; }
.inspect-meta { margin: 4px 0 6px; color: var(--session-muted-text, #5c6875); font-size: 11px; }
.inspect-integrity { margin: 4px 0; color: var(--session-muted-text, #5c6875); font-size: 11px; }
.inspect-integrity.bad { color: var(--session-warning, #a45b12); }
.inspect-list { display: grid; gap: 5px; margin: 0; padding: 0; list-style: none; }
.inspect-message {
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  background: var(--session-surface, #ffffff);
}
.inspect-message summary {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  cursor: pointer;
  overflow-wrap: anywhere;
}
.inspect-message .seq, .inspect-message .meta {
  color: var(--session-muted-text, #5c6875);
  font-size: 11px;
}
.inspect-message .badge {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--session-surface-muted, #eef1f4);
  font-size: 11px;
}
.inspect-message .badge.warn { color: #a45b12; background: #fdf3e7; }
.inspect-message .inspect-preview {
  flex: 1 1 160px;
  min-width: 0;
  overflow: hidden;
  color: var(--session-muted-text, #5c6875);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.inspect-message .inspect-body { padding: 0 8px 8px; }
.inspect-content {
  max-height: 260px;
  margin: 0 0 6px;
  padding: 8px;
  overflow: auto;
  border-radius: 5px;
  background: var(--session-surface-muted, #eef1f4);
  white-space: pre-wrap;
  font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, monospace;
}
@media (max-width: 620px) {
  :host { height: auto; min-height: 40px; padding: 4px 8px; }
  .dock-panel { bottom: 8px; }
}
`;

function formatTokenCount(value) {
	const n = Number(value) || 0;
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
	return String(n);
}

function formatExact(value) {
	return (Number(value) || 0).toLocaleString("zh-CN");
}

const ringCircumference = 2 * Math.PI * 5.5;

const roleLabels = Object.freeze({
	system: "系统",
	user: "用户",
	assistant: "助手",
	tool: "工具",
	host: "过程",
});

const sourceLabels = Object.freeze({
	system_prompt: "系统提示",
	runtime_context: "运行上下文",
	summary: "压缩摘要",
	session: "会话消息",
});

const warningLabels = Object.freeze({
	compacted: "已触发上下文压缩",
	agents_md_truncated: "AGENTS.md 已截断",
	tools_unavailable: "工具不可用",
	messages_hash_mismatch: "消息 hash 与快照不一致",
	truncation_inferred_from_current_derive: "旧快照缺少截断信息，已按当前 derive 推断",
	reconstruction_unavailable: "无法读取 Session 日志",
});

const factTypeLabels = Object.freeze({
	session_start: "会话开始",
	turn_start: "Turn 开始",
	turn_end: "Turn 结束",
	step_start: "Step 开始",
	step_end: "Step 结束",
	request_header: "请求摘要",
	llm_usage: "Token 用量",
	message: "消息",
	context_summary: "上下文摘要",
	context_compaction: "上下文压缩",
	context_snapshot: "上下文快照",
	session_meta: "会话信息",
});

const TERMINAL_TURN_STATUS = Object.freeze(["completed", "cancelled", "max_steps", "error"]);
const VISIBLE_TURN_STATUS_ROW = Object.freeze(["cancelled", "max_steps", "error"]);
const TRACE_PAGE_SIZE = 60;
const CHAT_WINDOW_MESSAGES = 200;
const TRACE_WINDOW_FACTS = 200;
const CHAT_WINDOW_BYTES = 2 << 20;
const TRACE_WINDOW_BYTES = 2 << 20;
const chatMessageByteCache = new WeakMap();
const factByteCache = new WeakMap();

function chatMessageBytes(message) {
	if (message && typeof message === "object") {
		const cached = chatMessageByteCache.get(message);
		if (cached !== undefined) return cached;
	}
	const toolCalls = message?.tool_calls ? JSON.stringify(message.tool_calls).length : 0;
	const size = String(message?.content || "").length + String(message?.reasoning || "").length + toolCalls;
	if (message && typeof message === "object") chatMessageByteCache.set(message, size);
	return size;
}

function factBytes(fact) {
	if (fact && typeof fact === "object") {
		const cached = factByteCache.get(fact);
		if (cached !== undefined) return cached;
	}
	const meta = fact?.meta ? JSON.stringify(fact.meta).length : 0;
	const toolCalls = fact?.tool_calls ? JSON.stringify(fact.tool_calls).length : 0;
	const size = String(fact?.content || "").length + meta + toolCalls;
	if (fact && typeof fact === "object") factByteCache.set(fact, size);
	return size;
}

function trimChatWindow(messages) {
	let bytes = messages.reduce((sum, message) => sum + chatMessageBytes(message), 0);
	let drop = 0;
	while (drop < messages.length - 1 &&
		(messages.length - drop > CHAT_WINDOW_MESSAGES || bytes > CHAT_WINDOW_BYTES)) {
		bytes -= chatMessageBytes(messages[drop]);
		drop += 1;
	}
	return drop;
}

function trimTraceWindow(facts) {
	let bytes = facts.reduce((sum, fact) => sum + factBytes(fact), 0);
	let drop = 0;
	while (drop < facts.length - 1 &&
		(facts.length - drop > TRACE_WINDOW_FACTS || bytes > TRACE_WINDOW_BYTES)) {
		bytes -= factBytes(facts[drop]);
		drop += 1;
	}
	return drop;
}

function el(tag, className, text) {
	const node = document.createElement(tag);
	if (className) node.className = className;
	if (text !== undefined) node.textContent = text;
	return node;
}

function call(method, payload) {
	return window.LiteAgent.call("session", method, payload);
}

function describeError(error) {
	return error instanceof Error ? error.message : String(error);
}

function sessionLabel(session) {
	if (session?.name) return session.name;
	const id = String(session?.session_id || "");
	return id.length > 14 ? `${id.slice(0, 14)}…` : id;
}

async function copyText(text) {
	const value = String(text ?? "");
	if (navigator.clipboard?.writeText) {
		await navigator.clipboard.writeText(value);
		return;
	}
	const textarea = document.createElement("textarea");
	textarea.value = value;
	textarea.style.position = "fixed";
	textarea.style.opacity = "0";
	document.body.append(textarea);
	textarea.select();
	document.execCommand("copy");
	textarea.remove();
}

function codeBlockNode(lines) {
	const wrap = el("div", "code-block-wrap");
	const actions = el("div", "code-actions");
	const language = el("span", "code-language", "代码");
	const copy = el("button", "code-copy", "复制");
	copy.type = "button";
	copy.addEventListener("click", async () => {
		await copyText(lines.join("\n"));
		copy.textContent = "已复制";
		setTimeout(() => { copy.textContent = "复制"; }, 1200);
	});
	actions.append(language, copy);
	const pre = el("pre", "code-block");
	pre.append(el("code", "", lines.join("\n")));
	wrap.append(actions, pre);
	return wrap;
}

function roleLabel(role) {
	return roleLabels[role] || role || "未知";
}

function formatTime(ts) {
	const value = Number(ts);
	if (!Number.isFinite(value) || value <= 0) return "";
	const date = new Date(value);
	const pad = (n) => String(n).padStart(2, "0");
	return `${date.getMonth() + 1}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatDuration(start, end) {
	const ms = Number(end) - Number(start);
	if (!Number.isFinite(ms) || ms <= 0) return "";
	const seconds = Math.round(ms / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}m${String(seconds % 60).padStart(2, "0")}s`;
	return `${Math.floor(minutes / 60)}h${String(minutes % 60).padStart(2, "0")}m`;
}

function truncateText(text, max) {
	const value = String(text ?? "").trim();
	return value.length > max ? `${value.slice(0, max)}…` : value;
}

function messageHasToolCalls(message) {
	return Array.isArray(message?.tool_calls) && message.tool_calls.length > 0;
}

const turnStatusLabels = Object.freeze({
	running: "生成中…",
	completed: "已完成",
	cancelled: "已取消",
	max_steps: "达到步数上限",
	error: "出错",
	idle: "空闲",
});

function renderInline(text) {
	const fragment = document.createDocumentFragment();
	const pattern = /(`[^`]+`)|(\*\*[^*]+\*\*)/g;
	let cursor = 0;
	let match;
	while ((match = pattern.exec(text)) !== null) {
		if (match.index > cursor) fragment.append(document.createTextNode(text.slice(cursor, match.index)));
		const token = match[0];
		if (token.startsWith("`")) {
			fragment.append(el("code", "inline-code", token.slice(1, -1)));
		} else {
			fragment.append(el("strong", "", token.slice(2, -2)));
		}
		cursor = match.index + token.length;
	}
	if (cursor < text.length) fragment.append(document.createTextNode(text.slice(cursor)));
	return fragment;
}

// Minimal, node-based renderer: escapes by construction, no innerHTML.
function renderRichText(text) {
	const root = document.createDocumentFragment();
	const lines = String(text ?? "").split("\n");
	let codeLines = null;
	let list = null;
	const flushList = () => {
		if (list) {
			root.append(list);
			list = null;
		}
	};
	for (const line of lines) {
		if (line.trimStart().startsWith("```")) {
			flushList();
			if (codeLines) {
				root.append(codeBlockNode(codeLines));
				codeLines = null;
			} else {
				codeLines = [];
			}
			continue;
		}
		if (codeLines) {
			codeLines.push(line);
			continue;
		}
		const listItem = line.match(/^\s*[-*]\s+(.*)$/);
		if (listItem) {
			if (!list) list = el("ul", "rich-list");
			const li = el("li", "");
			li.append(renderInline(listItem[1]));
			list.append(li);
			continue;
		}
		flushList();
		const heading = line.match(/^\s*(#{1,6})\s+(.*)$/);
		if (heading) {
			const level = Math.min(heading[1].length + 2, 6);
			const node = el(`h${level}`, "rich-heading");
			node.append(renderInline(heading[2]));
			root.append(node);
			continue;
		}
		const paragraph = el("p", "rich-line");
		paragraph.append(renderInline(line));
		root.append(paragraph);
	}
	flushList();
	if (codeLines) root.append(codeBlockNode(codeLines));
	return root;
}

// A draft is append-only, so completed blocks never change. Freeze all but the
// trailing blocks and re-render only those, keeping streaming cost proportional
// to the tail instead of the whole accumulated reply.
const DRAFT_TAIL_BLOCKS = 2;

function splitRichBlocks(text) {
	const lines = String(text ?? "").split("\n");
	const blocks = [];
	let index = 0;
	while (index < lines.length) {
		const line = lines[index];
		if (line.trimStart().startsWith("```")) {
			let end = index + 1;
			let closed = false;
			while (end < lines.length) {
				if (lines[end].trimStart().startsWith("```")) {
					closed = true;
					break;
				}
				end += 1;
			}
			blocks.push({
				kind: "code",
				lines: lines.slice(index, closed ? end + 1 : lines.length),
				complete: closed,
			});
			index = closed ? end + 1 : lines.length;
			continue;
		}
		if (/^\s*[-*]\s+/.test(line)) {
			let end = index + 1;
			while (end < lines.length && /^\s*[-*]\s+/.test(lines[end])) end += 1;
			blocks.push({ kind: "list", lines: lines.slice(index, end), complete: true });
			index = end;
			continue;
		}
		const heading = line.match(/^\s*(#{1,6})\s+(.*)$/);
		blocks.push({
			kind: heading ? "heading" : "line",
			level: heading ? Math.min(heading[1].length + 2, 6) : 0,
			lines: [line],
			complete: true,
		});
		index += 1;
	}
	return blocks;
}

function renderRichBlock(block) {
	if (block.kind === "code") {
		const codeLines = block.lines.slice(1, block.complete ? block.lines.length - 1 : block.lines.length);
		return codeBlockNode(codeLines);
	}
	if (block.kind === "list") {
		const list = el("ul", "rich-list");
		for (const line of block.lines) {
			const item = el("li", "");
			const match = line.match(/^\s*[-*]\s+(.*)$/);
			item.append(renderInline(match ? match[1] : line));
			list.append(item);
		}
		return list;
	}
	if (block.kind === "heading") {
		const node = el(`h${block.level}`, "rich-heading");
		const heading = block.lines[0].match(/^\s*(#{1,6})\s+(.*)$/);
		node.append(renderInline(heading ? heading[2] : block.lines[0]));
		return node;
	}
	const paragraph = el("p", "rich-line");
	paragraph.append(renderInline(block.lines[0]));
	return paragraph;
}

class IncrementalRichText {
	constructor(container) {
		this.container = container;
		this.anchor = document.createComment("rich-tail");
		this.frozenCount = 0;
		this.tail = [];
		this.previousText = "";
		this.container.replaceChildren(this.anchor);
	}

	reset() {
		this.frozenCount = 0;
		this.tail = [];
		this.previousText = "";
		this.container.replaceChildren(this.anchor);
	}

	render(text) {
		const value = String(text ?? "");
		if (!value.startsWith(this.previousText)) this.reset();
		this.previousText = value;
		const blocks = splitRichBlocks(value);
		let freezeCount = Math.max(0, blocks.length - DRAFT_TAIL_BLOCKS);
		while (freezeCount > 0 && !blocks[freezeCount - 1].complete) freezeCount -= 1;
		// Frozen blocks are complete and before the unstable tail, so appended
		// text cannot reshape them. Only a shrinking frontier invalidates them.
		if (freezeCount < this.frozenCount) this.reset();
		if (freezeCount > this.frozenCount) {
			const fragment = document.createDocumentFragment();
			for (let i = this.frozenCount; i < freezeCount; i += 1) {
				fragment.append(renderRichBlock(blocks[i]));
			}
			this.container.insertBefore(fragment, this.tail[0] ?? this.anchor);
			this.frozenCount = freezeCount;
		}
		for (const node of this.tail) node.remove();
		this.tail = [];
		const fragment = document.createDocumentFragment();
		for (let i = freezeCount; i < blocks.length; i += 1) {
			const node = renderRichBlock(blocks[i]);
			fragment.append(node);
			this.tail.push(node);
		}
		this.container.insertBefore(fragment, this.anchor);
	}
}

function previewOfText(text, maxLines = 16, maxChars = 4096) {
	const value = String(text ?? "");
	const preview = value.split("\n", maxLines + 1).slice(0, maxLines).join("\n").slice(0, maxChars);
	return { preview, truncated: preview.length < value.length };
}

class SessionHistory extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
        <style>${styles}</style>
        <div class="workspace-picker">
          <select class="workspace-select" aria-label="当前工作区"></select>
          <button type="button" class="workspace-add" title="添加工作区（选择文件夹）">选择文件夹</button>
        </div>
        <div class="model-row" hidden>
          <label for="session-model">模型</label>
          <select id="session-model" class="model-select" aria-label="新会话模型" title="模型目录在插件中心 → 设置 → llm-openai 中管理"></select>
        </div>
        <div class="history-head">
          <span class="hint grow"></span>
          <button type="button" class="create">新建会话</button>
        </div>
        <input class="history-search" type="search" aria-label="搜索会话" placeholder="搜索标题、ID 或工作区">
        <ul class="list"></ul>
        <div class="error" aria-live="polite"></div>
      `;
		this.head = this.shadowRoot.querySelector(".hint");
		this.workspaceSelect = this.shadowRoot.querySelector(".workspace-select");
		this.workspaceAdd = this.shadowRoot.querySelector(".workspace-add");
		this.searchInput = this.shadowRoot.querySelector(".history-search");
		this.createButton = this.shadowRoot.querySelector(".create");
		this.list = this.shadowRoot.querySelector(".list");
		this.error = this.shadowRoot.querySelector(".error");
		this.modelRow = this.shadowRoot.querySelector(".model-row");
		this.modelSelect = this.shadowRoot.querySelector(".model-select");
		this.models = [];
		this.defaultModel = "";
		this.selectedModel = "";
		this.modelsSignature = "";
		this.modelsLoading = false;
		this.state = { sessions: [], revision: 0 };
		this.query = "";
		this.creating = false;
		this.workspaceBusy = false;
		this.workspaceSelect.addEventListener("change", () => this.selectWorkspace(this.workspaceSelect.value));
		this.workspaceAdd.addEventListener("click", () => this.addWorkspace());
		this.createButton.addEventListener("click", () => this.createSession());
		this.modelSelect.addEventListener("change", () => {
			this.selectedModel = this.modelSelect.value;
		});
		// The plugin center owns model catalog editing; refresh this picker when
		// llm-openai reports a successful settings save.
		this.onSettingsSaved = (event) => {
			if (event?.detail?.plugin === "llm-openai") this.refreshModels();
		};
		window.addEventListener("liteagent:settings-saved", this.onSettingsSaved);
		this.searchInput.addEventListener("input", () => {
			this.query = this.searchInput.value.trim().toLowerCase();
			this.render();
		});
		this.render();
		this.refresh();
		this.refreshModels();
	}

	disconnectedCallback() {
		window.removeEventListener("liteagent:settings-saved", this.onSettingsSaved);
	}

	async refresh() {
		try {
			const state = await call("state", {});
			this.error.textContent = "";
			this.setData(state.history);
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	async resync() {
		const state = await call("state", {});
		this.error.textContent = "";
		this.setData(state.history);
		await this.refreshModels();
	}

	async refreshModels() {
		this.modelsLoading = true;
		this.renderModels();
		try {
			const catalog = await window.LiteAgent.call("llm", "models.list", {});
			this.applyCatalog(catalog);
		} catch {
			// llm is an optional collaborator: without it a session still runs
			// on the LLM plugin's own default, so the picker simply disappears.
			this.applyCatalog({ models: [], default_model: "" });
		} finally {
			this.modelsLoading = false;
			this.renderModels();
		}
	}

	applyCatalog(catalog) {
		const models = Array.isArray(catalog?.models)
			? catalog.models
				.filter((model) => model && typeof model.id === "string" && model.id.trim())
				.map((model) => ({
					id: model.id.trim(),
					label: String(model.label || model.id),
					context_window: Number(model.context_window) || 0,
				}))
			: [];
		const previousDefault = this.defaultModel || "";
		const previousSelection = this.selectedModel || "";
		this.models = models;
		const fallback = catalog?.default_model || models[0]?.id || "";
		this.defaultModel = models.some((model) => model.id === fallback)
			? fallback
			: models[0]?.id || "";
		const selectionExists = models.some((model) => model.id === previousSelection);
		// Follow a changed default only when the user had not picked another
		// model. An explicit selection survives unrelated settings saves.
		if (!selectionExists || previousSelection === "" || previousSelection === previousDefault) {
			this.selectedModel = this.defaultModel;
		}
		this.modelRow.hidden = models.length === 0;
		this.modelsSignature = "";
		this.renderModels();
	}

	renderModels() {
		if (!this.modelSelect) return;
		const disabled = this.creating || this.workspaceBusy || this.modelsLoading;
		const signature = JSON.stringify([
			this.models.map((model) => [model.id, model.label, model.context_window]),
			this.selectedModel, disabled,
		]);
		if (signature === this.modelsSignature) return;
		this.modelsSignature = signature;
		const options = this.models.map((model) => {
			const option = document.createElement("option");
			option.value = model.id;
			option.textContent = model.label || model.id;
			if (model.context_window > 0) option.title = `上下文窗口 ${model.context_window}`;
			return option;
		});
		this.modelSelect.replaceChildren(...options);
		this.modelSelect.value = this.selectedModel;
		this.modelSelect.disabled = disabled || this.models.length === 0;
	}

	async createSession() {
		const workspaceId = this.state.workspace_id;
		if (this.creating || this.workspaceBusy || !workspaceId) return;
		this.creating = true;
		this.error.textContent = "";
		this.render();
		try {
			const model = this.models.find((item) => item.id === this.selectedModel);
			const payload = { workspace_id: workspaceId };
			if (model) {
				payload.model = model.id;
				payload.context_window = model.context_window || 0;
			}
			const created = await call("create", payload);
			await call("select", { session_id: created.session_id });
		} catch (error) {
			this.error.textContent = describeError(error);
		} finally {
			this.creating = false;
			this.render();
		}
	}

	async selectWorkspace(workspaceId) {
		const current = this.state.selected_workspace_id || this.state.workspace_id;
		if (!workspaceId || workspaceId === current) return;
		this.workspaceBusy = true;
		this.error.textContent = "";
		this.render();
		try {
			await call("workspace.select", { workspace_id: workspaceId });
			await call("refresh", {});
			await this.refresh();
		} catch (error) {
			this.error.textContent = describeError(error);
		} finally {
			this.workspaceBusy = false;
			this.render();
		}
	}

	async addWorkspace() {
		if (this.creating || this.workspaceBusy) return;
		this.workspaceBusy = true;
		this.error.textContent = "";
		this.render();
		try {
			const picked = await window.LiteAgent.pickDirectory();
			if (picked.cancelled) return;
			if (!picked.path) throw new Error("文件夹选择未返回路径");
			await call("workspace.create", { path: picked.path });
			await call("refresh", {});
			await this.refresh();
		} catch (error) {
			this.error.textContent = describeError(error);
		} finally {
			this.workspaceBusy = false;
			this.render();
		}
	}

	async select(sessionId) {
		if (!sessionId) return;
		this.error.textContent = "";
		try {
			await call("select", { session_id: sessionId });
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	setData(state) {
		const next = state || { sessions: [], revision: 0 };
		if (next.session_id && this.state?.session_id && next.session_id !== this.state.session_id) return;
		if (Number.isFinite(next.revision) && Number.isFinite(this.state?.revision) &&
			next.session_id === this.state?.session_id && next.revision < this.state.revision) return;
		this.state = next;
		this.render();
	}

	filteredSessions() {
		const sessions = this.state.sessions || [];
		if (!this.query) return sessions;
		return sessions.filter((session) => [
			session.name,
			session.session_id,
			session.workspace,
		].some((value) => String(value || "").toLowerCase().includes(this.query)));
	}

	render() {
		if (!this.list) return;
		this.renderModels();
		const sessions = this.filteredSessions();
		this.renderWorkspaces();
		this.head.textContent = sessions.length > 0 ? `${sessions.length} 个会话` : "";
		this.createButton.disabled = this.creating || this.workspaceBusy || !this.state.workspace_id;
		this.createButton.textContent = this.creating ? "创建中…" : "新建会话";
		this.createButton.title = this.state.workspace_id ? "" : "请先选择工作区";
		this.list.replaceChildren();
		if (sessions.length === 0) {
			this.list.append(el("li", "empty", this.query ? "没有匹配的会话" : "暂无会话，点上方按钮新建"));
			return;
		}
		for (const session of sessions) {
			const active = session.session_id === this.state.selected_id;
			const open = (this.state.open_ids || []).includes(session.session_id);
			const button = el("button", active ? "item active" : "item");
			button.type = "button";
			button.title = session.workspace ? `${session.session_id}\n${session.workspace}` : session.session_id;
			if (open) button.append(el("span", "open-badge", active ? "当前" : "已打开"));
			button.append(el("strong", "name", session.name || session.session_id));
			const meta = `${session.fact_count || 0} 条记录 · ${formatTime(session.last_ts || session.created_ts)}`;
			button.append(el("span", "meta", meta));
			button.addEventListener("click", () => this.select(session.session_id));
			this.list.append(button);
		}
	}

	renderWorkspaces() {
		const workspaces = this.state.workspaces || [];
		// The selector highlights the registry's current workspace, which is not
		// necessarily the workspace of the session on screen: after switching
		// workspaces the pane keeps showing its old session until one is picked.
		const current = this.state.selected_workspace_id || this.state.workspace_id || "";
		const addDisabled = this.creating || this.workspaceBusy;
		const selectDisabled = workspaces.length === 0 || this.workspaceBusy;
		// Rebuilding the <select> closes its dropdown mid-interaction, so only
		// rebuild when the option set or the enabled state actually changed.
		const signature = JSON.stringify([
			workspaces.map((workspace) => [workspace.workspace_id, workspace.name || workspace.path]),
			current, addDisabled, selectDisabled,
		]);
		if (signature === this.workspaceSignature) return;
		this.workspaceSignature = signature;
		const options = workspaces.map((workspace) => {
			const option = document.createElement("option");
			option.value = workspace.workspace_id;
			option.textContent = workspace.name || workspace.path;
			option.title = workspace.path;
			return option;
		});
		if (options.length === 0) {
			const option = document.createElement("option");
			option.value = "";
			option.textContent = "尚未添加工作区";
			options.push(option);
		}
		this.workspaceSelect.replaceChildren(...options);
		this.workspaceSelect.value = current;
		this.workspaceAdd.disabled = addDisabled;
		this.workspaceSelect.disabled = selectDisabled;
	}
}

class SessionChat extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
        <style>${styles}</style>
        <div class="chat-tabs" role="tablist" aria-label="打开的对话"></div>
        <div class="workspace-row">
          <label for="workspace">工作区</label>
          <code class="workspace-path"></code>
        </div>
        <div class="workspace-warning hint"></div>
        <div class="status" hidden></div>
        <div class="chat-stage">
          <button type="button" class="load-earlier" hidden>加载更早消息</button>
          <div class="messages" role="log" aria-live="polite"></div>
          <button type="button" class="scroll-latest" hidden>回到最新</button>
        </div>
        <div class="toolbar composer">
          <textarea rows="1" aria-label="消息输入" placeholder="输入消息，Enter 发送，Shift+Enter 换行"></textarea>
          <button type="button" class="send">发送</button>
          <button type="button" class="cancel" disabled>停止</button>
        </div>
        <div class="error" aria-live="polite"></div>
      `;
		this.tabs = this.shadowRoot.querySelector(".chat-tabs");
		this.workspacePath = this.shadowRoot.querySelector(".workspace-path");
		this.workspaceWarning = this.shadowRoot.querySelector(".workspace-warning");
		this.input = this.shadowRoot.querySelector("textarea");
		this.sendButton = this.shadowRoot.querySelector(".send");
		this.cancelButton = this.shadowRoot.querySelector(".cancel");
		this.messages = this.shadowRoot.querySelector(".messages");
		this.scrollLatest = this.shadowRoot.querySelector(".scroll-latest");
		this.loadEarlierButton = this.shadowRoot.querySelector(".load-earlier");
		this.status = this.shadowRoot.querySelector(".status");
		this.error = this.shadowRoot.querySelector(".error");
		this.state = { selected_id: "", panes: [], revision: 0 };
		this.panes = new Map();
		this.selectedId = "";
		this.drafts = new Map();
		this.busy = new Set();
		this.emptyNode = null;
		this.draftNode = null;
		this.draftBody = null;
		this.draftText = null;
		this.draftReasoning = null;
		this.draftReasoningText = null;
		this.inputDrafts = new Map();
		this.messageRows = new Map();
		this.turnFolds = new Map();
		this.turnHeaderCache = new Map();
		this.turnHeaders = new Map();
		this.turnStatusRows = new Map();
		this.processRows = new Map();
		this.turnInfoById = new Map();
		this.renderedSessionId = null;
		this.draftKey = "";
		this.tabsSignature = "";
		this.workspaceSignature = "";
		this.stickToBottom = true;
		this.streamFrame = 0;
		this.draftRich = null;
		this.resyncPromise = null;
		this.loadingEarlier = false;

		this.sendButton.addEventListener("click", () => this.send());
		this.cancelButton.addEventListener("click", () => this.cancel());
		this.scrollLatest.addEventListener("click", () => this.scrollToBottom());
		this.loadEarlierButton.addEventListener("click", () => this.loadEarlierMessages());
		this.messages.addEventListener("scroll", () => {
			this.stickToBottom = this.nearBottom();
			this.updateScrollButton();
		});
		this.input.addEventListener("input", () => {
			this.saveInputDraft();
			this.autoGrow();
			this.updateControls();
		});
		this.input.addEventListener("keydown", (event) => {
			if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
				event.preventDefault();
				this.send();
			}
		});

		this.render();
		this.refresh();
	}

	async refresh() {
		try {
			const state = await call("state", {});
			if (state.history) {
				this.setData({ selected_id: state.history?.selected_id, panes: state.chats || [] });
			} else if (state.chat) {
				this.setData(state.chat);
			}
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	async resync() {
		const state = await call("state", {});
		this.error.textContent = "";
		if (state.history) {
			this.setData({ selected_id: state.history?.selected_id, panes: state.chats || [] });
		} else if (state.chat) {
			this.setData(state.chat);
		}
	}

	activePane() {
		return this.panes.get(this.selectedId) || null;
	}

	activeDraft() {
		const draft = this.drafts.get(this.selectedId);
		return draft?.session_id === this.selectedId ? draft : null;
	}

	saveInputDraft() {
		const id = this.selectedId;
		if (id) this.inputDrafts.set(id, this.input.value);
	}

	restoreInputDraft() {
		this.input.value = this.inputDrafts.get(this.selectedId) || "";
		this.autoGrow();
	}

	reconcileDraft(sessionId) {
		const draft = this.drafts.get(sessionId);
		const pane = this.panes.get(sessionId);
		if (!draft || !pane) return;
		const messages = pane.messages || [];
		const landed = messages.some((message) =>
			message.role === "assistant" &&
			(!draft.turn_id || message.turn_id === draft.turn_id) &&
			(!draft.step_id || message.step_id === draft.step_id));
		if (landed || TERMINAL_TURN_STATUS.includes(pane.turn?.status)) this.drafts.delete(sessionId);
	}

	setData(state) {
		const next = state || {};
		this.cancelStreamRender();
		this.saveInputDraft();
		if (Array.isArray(next.panes)) {
			const incoming = new Map();
			for (const pane of next.panes) {
				if (!pane?.session_id) continue;
				const previous = this.panes.get(pane.session_id);
				if (previous && Number.isFinite(previous.through_seq) && Number.isFinite(pane.through_seq) &&
					pane.through_seq < previous.through_seq) {
					incoming.set(pane.session_id, previous);
					continue;
				}
				if (previous && Number.isFinite(previous.revision) && Number.isFinite(pane.revision) &&
					pane.revision < previous.revision) {
					incoming.set(pane.session_id, previous);
					continue;
				}
				incoming.set(pane.session_id, pane);
			}
			this.panes = incoming;
			// A push never yanks the view off a session it still carries: only
			// an explicit selected_id or a dropped pane moves the selection.
			this.selectedId = next.selected_id && incoming.has(next.selected_id)
				? next.selected_id
				: incoming.has(this.selectedId)
					? this.selectedId
					: incoming.keys().next().value || "";
			this.state = {
				selected_id: this.selectedId,
				revision: Number(next.revision) || 0,
				panes: [...incoming.values()],
			};
		} else if (next.session_id) {
			if (Number.isFinite(next.revision) && Number.isFinite(this.panes.get(next.session_id)?.revision) &&
				next.revision < this.panes.get(next.session_id).revision) return;
			this.panes = new Map([[next.session_id, next]]);
			this.selectedId = next.session_id;
			this.state = { selected_id: next.session_id, revision: 0, panes: [next] };
		} else {
			return;
		}
		for (const sessionId of this.panes.keys()) {
			const pane = this.panes.get(sessionId);
			const existing = this.drafts.get(sessionId);
			if (pane.stream && (!existing || (pane.stream.update_id ?? 0) >= (existing.update_id ?? 0))) {
				this.drafts.set(sessionId, pane.stream);
			}
			this.reconcileDraft(sessionId);
		}
		for (const sessionId of this.drafts.keys()) {
			if (!this.panes.has(sessionId)) this.drafts.delete(sessionId);
		}
		this.restoreInputDraft();
		this.render();
	}

	runCommand(command, payload) {
		if (command === "append") {
			this.applyChatAppend(payload);
			return;
		}
		if (command !== "stream") return;
		if (!payload?.message) {
			this.applyStreamDelta(payload);
			return;
		}
		const sessionId = payload?.session_id;
		if (!sessionId || !this.panes.has(sessionId)) return;
		this.drafts.set(sessionId, payload);
		this.renderTabs();
		if (sessionId !== this.selectedId) return;
		// Stream frames only touch the draft. Rebuilding the landed list per
		// frame would drop text selections and collapse open details. High-rate
		// frames share one render: the latest payload is read when the frame runs.
		this.scheduleStreamRender();
	}

	applyStreamDelta(payload) {
		const sessionId = payload?.session_id;
		if (!sessionId || !this.panes.has(sessionId)) return;
		const pane = this.panes.get(sessionId);
		const landed = (pane?.messages || []).some((message) =>
			message.role === "assistant" &&
			message.turn_id === payload.turn_id &&
			message.step_id === payload.step_id &&
			String(message.content || "").trim() &&
			!messageHasToolCalls(message));
		const terminal = TERMINAL_TURN_STATUS.includes(pane?.turn?.status) &&
			pane?.turn?.turn_id === payload.turn_id;
		if (landed || terminal) {
			this.drafts.delete(sessionId);
			return;
		}
		let draft = this.drafts.get(sessionId);
		const sameAttempt = draft && draft.turn_id === payload.turn_id &&
			draft.step_id === payload.step_id;
		if (!payload.reset && !sameAttempt) {
			this.requestResync();
			return;
		}
		if (payload.reset || !sameAttempt) {
			draft = {
				session_id: sessionId,
				turn_id: payload.turn_id,
				step_id: payload.step_id,
				update_id: Number.isFinite(payload.update_id) ? payload.update_id : 0,
				status: payload.status || "streaming",
				message: { role: "assistant", content: "", reasoning: "" },
			};
		} else {
			if (Number.isFinite(payload.update_id) && Number.isFinite(draft.update_id)) {
				if (payload.update_id <= draft.update_id) return;
				if (payload.update_id > draft.update_id + 1 && !payload.coalesced) {
					this.requestResync();
					return;
				}
			}
			draft = { ...draft, message: { ...draft.message } };
		}
		draft.update_id = Number.isFinite(payload.update_id) ? payload.update_id : draft.update_id;
		draft.status = payload.status || draft.status;
		draft.message.content = (draft.message.content || "") + (payload.content_delta || "");
		draft.message.reasoning = (draft.message.reasoning || "") + (payload.reasoning_delta || "");
		this.drafts.set(sessionId, draft);
		this.renderTabs();
		if (sessionId !== this.selectedId) return;
		this.scheduleStreamRender();
	}

	applyChatAppend(payload) {
		const sessionId = payload?.session_id;
		const pane = this.panes.get(sessionId);
		if (!pane) return;
		const expected = Number.isFinite(pane.through_seq) ? pane.through_seq : 0;
		if (Number.isFinite(payload.after_seq) && payload.after_seq !== expected) {
			this.requestResync();
			return;
		}
		const messages = pane.messages || [];
		const lastSeq = messages.length > 0 ? messages[messages.length - 1].seq : 0;
		const appended = (Array.isArray(payload.messages) ? payload.messages : [])
			.filter((message) => message && Number(message.seq) > lastSeq);
		pane.messages = appended.length > 0 ? [...messages, ...appended] : messages;
		let trimmed = false;
		const drop = trimChatWindow(pane.messages);
		if (drop > 0) {
			pane.messages = pane.messages.slice(drop);
			pane.first_seq = pane.messages[0]?.seq || 0;
			pane.has_more = true;
			trimmed = true;
		}
		pane.through_seq = Number(payload.through_seq) || expected;
		if (Number.isFinite(payload.revision)) pane.revision = payload.revision;
		if (payload.turn) pane.turn = { ...(pane.turn || {}), ...payload.turn };
		this.state.panes = [...this.panes.values()];
		this.reconcileDraft(sessionId);
		this.renderTabs();
		if (sessionId === this.selectedId) {
			this.renderWorkspace();
			const oldHeight = trimmed ? this.messages.scrollHeight : 0;
			const oldTop = trimmed ? this.messages.scrollTop : 0;
			this.renderMessages();
			if (trimmed && !this.stickToBottom) {
				this.messages.scrollTop = Math.max(0, oldTop - (oldHeight - this.messages.scrollHeight));
			}
		}
	}

	async loadEarlierMessages() {
		const pane = this.activePane();
		if (!pane || !pane.has_more || this.loadingEarlier) return;
		this.loadingEarlier = true;
		this.renderMessages();
		try {
			const page = await call("messages", {
				session_id: pane.session_id,
				before_seq: pane.first_seq || 0,
				limit: CHAT_WINDOW_MESSAGES,
			});
			const older = Array.isArray(page?.messages) ? page.messages : [];
			const current = pane.messages || [];
			const firstExisting = current.length > 0 ? current[0].seq : 0;
			const prepended = older.filter((message) =>
				message && (!firstExisting || Number(message.seq) < firstExisting));
			if (prepended.length > 0) {
				const oldHeight = this.messages.scrollHeight;
				const oldTop = this.messages.scrollTop;
				pane.messages = [...prepended, ...current];
				pane.first_seq = Number(page.first_seq) || prepended[0].seq;
				pane.has_more = page.has_more === true;
				this.state.panes = [...this.panes.values()];
				this.renderMessages();
				const added = this.messages.scrollHeight - oldHeight;
				this.messages.scrollTop = oldTop + added;
			} else {
				pane.has_more = false;
			}
		} catch (error) {
			this.error.textContent = describeError(error);
		} finally {
			this.loadingEarlier = false;
			this.renderMessages();
		}
	}

	requestResync() {
		if (this.resyncPromise) return;
		this.resyncPromise = this.resync()
			.catch((error) => {
				this.error.textContent = describeError(error);
			})
			.finally(() => {
				this.resyncPromise = null;
			});
	}

	cancelStreamRender() {
		if (!this.streamFrame) return;
		if (typeof cancelAnimationFrame === "function") cancelAnimationFrame(this.streamFrame);
		else clearTimeout(this.streamFrame);
		this.streamFrame = 0;
	}

	scheduleStreamRender() {
		if (this.streamFrame) return;
		const run = () => {
			this.streamFrame = 0;
			this.renderDraft();
		};
		if (typeof requestAnimationFrame === "function") {
			this.streamFrame = requestAnimationFrame(() => {
				this.streamFrame = requestAnimationFrame(run);
			});
			return;
		}
		this.streamFrame = setTimeout(run, 16);
	}

	updateControls() {
		const pane = this.activePane();
		const running = pane?.turn?.status === "running";
		const busy = !pane || this.busy.has(this.selectedId) || running;
		const cancelRequested = pane?.turn?.cancel_requested === true;
		// The composer stays usable while generating: only sending is gated,
		// so the next message can be drafted during a long turn.
		this.input.disabled = !pane;
		this.sendButton.disabled = busy || this.input.value.trim() === "" || !pane?.workspace;
		this.sendButton.title = !pane?.workspace
			? "请先设置工作区"
			: "";
		this.cancelButton.disabled = !running || cancelRequested;
		this.status.hidden = !pane || !busy;
		this.status.textContent = !pane || !busy ? "" : cancelRequested ? "正在取消…" : "生成中…";
	}

	updateScrollButton() {
		this.scrollLatest.hidden = this.stickToBottom;
	}

	autoGrow() {
		this.input.style.height = "auto";
		this.input.style.height = `${Math.min(this.input.scrollHeight, 148)}px`;
	}

	nearBottom() {
		return this.messages.scrollHeight - this.messages.scrollTop - this.messages.clientHeight <= 48;
	}

	scrollToBottom() {
		this.stickToBottom = true;
		this.messages.scrollTop = this.messages.scrollHeight;
		this.scrollLatest.hidden = true;
	}

	async selectSession(sessionId) {
		if (!sessionId || sessionId === this.selectedId) return;
		this.saveInputDraft();
		this.error.textContent = "";
		try {
			await call("select", { session_id: sessionId });
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	async closeSession(sessionId) {
		if (!sessionId) return;
		this.error.textContent = "";
		try {
			await call("close", { session_id: sessionId });
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	renderTabs() {
		if (!this.tabs) return;
		const isRunning = (pane) => this.busy.has(pane.session_id) || pane.turn?.status === "running" ||
			pane.stream?.status === "streaming" || pane.stream?.status === "cancel_requested";
		// Rebuild only when something visible changed: a per-frame rebuild
		// would close the dropdown and reset hover while the user interacts.
		const signature = JSON.stringify((this.state.panes || []).map((pane) => [
			pane.session_id, sessionLabel(pane), pane.session_id === this.selectedId, isRunning(pane),
		]));
		if (signature === this.tabsSignature) return;
		this.tabsSignature = signature;
		const nodes = [];
		for (const pane of this.state.panes || []) {
			const active = pane.session_id === this.selectedId;
			const running = isRunning(pane);
			const tab = el("div", active ? "chat-tab active" : "chat-tab");
			tab.setAttribute("role", "tab");
			tab.setAttribute("aria-selected", active ? "true" : "false");
			tab.addEventListener("click", (event) => {
				if (event.target.closest(".tab-close")) return;
				this.selectSession(pane.session_id);
			});
			if (running) tab.append(el("span", "tab-running", ""));
			const select = el("button", "tab-select", sessionLabel(pane));
			select.type = "button";
			select.title = pane.workspace ? `${pane.session_id}\n${pane.workspace}` : pane.session_id;
			select.addEventListener("click", () => this.selectSession(pane.session_id));
			const close = el("button", "tab-close", "×");
			close.type = "button";
			close.title = "关闭对话";
			close.setAttribute("aria-label", `关闭 ${pane.name || pane.session_id}`);
			close.addEventListener("click", (event) => {
				event.stopPropagation();
				this.closeSession(pane.session_id);
			});
			tab.append(select, close);
			nodes.push(tab);
		}
		this.tabs.replaceChildren(...nodes);
	}

	renderWorkspace() {
		const pane = this.activePane();
		this.workspacePath.textContent = pane?.workspace || "";
		this.workspacePath.title = pane?.workspace || "";
		this.workspaceWarning.textContent = !pane
			? ""
			: pane.workspace
				? ""
				: "会话缺少工作区，工具调用将被拒绝。";
		this.updateControls();
	}

	messageNode(message) {
		if (message.kind === "system_prompt" || message.kind === "runtime_context" || message.role === "system") {
			return this.contextNode(message);
		}
		if (message.role === "tool") return this.toolMessageNode(message);
		const item = el("div", `message ${message.role || ""}`);
		const header = el("div", "message-header");
		header.append(el("span", "role", roleLabel(message.role)));
		if (message.timestamp) header.append(el("time", "message-time", formatTime(message.timestamp)));
		const copy = el("button", "message-copy", "复制");
		copy.type = "button";
		copy.addEventListener("click", async () => {
			await copyText(message.content || "");
			copy.textContent = "已复制";
			setTimeout(() => { copy.textContent = "复制"; }, 1200);
		});
		header.append(copy);
		item.append(header);

		const body = el("div", "content");
		if (message.content) body.append(renderRichText(message.content));
		if (message.reasoning) {
			const details = el("details", "reasoning");
			const summary = el("summary", "");
			summary.append(el("span", "", "思考过程"));
			const firstLine = String(message.reasoning).split("\n").map((line) => line.trim()).find(Boolean) || "";
			if (firstLine) summary.append(el("span", "reasoning-preview", truncateText(firstLine, 60)));
			details.append(summary);
			let reasoningBody = null;
			const renderReasoning = () => {
				if (reasoningBody) return;
				reasoningBody = el("div", "reasoning-body", message.reasoning);
				details.append(reasoningBody);
			};
			details.addEventListener("toggle", () => {
				if (details.open) renderReasoning();
			});
			body.append(details);
		}
		const toolCalls = Array.isArray(message.tool_calls) ? message.tool_calls : [];
		if (toolCalls.length > 0) {
			const details = el("details", "tool-calls");
			const names = toolCalls.map((call) => call?.function?.name).filter(Boolean);
			const summary = el("summary", "");
			summary.textContent = `工具调用 · ${toolCalls.length} 个${names.length ? `（${names.join("、")}）` : ""}`;
			details.append(summary);
			let toolJSON = null;
			const renderToolCalls = () => {
				if (toolJSON) return;
				toolJSON = el("pre", "tool-json");
				toolJSON.textContent = JSON.stringify(message.tool_calls, null, 2);
				details.append(toolJSON);
			};
			details.addEventListener("toggle", () => {
				if (details.open) renderToolCalls();
			});
			body.append(details);
		}
		item.append(body);
		return item;
	}

	toolMessageNode(message) {
		const item = el("details", "message tool tool-result");
		const summary = el("summary", "");
		summary.append(el("span", "role", "工具结果"));
		if (message.tool_name) summary.append(el("span", "badge", message.tool_name));
		summary.append(el("span", message.is_error ? "badge error" : "badge", message.is_error ? "失败" : "成功"));
		if (message.truncated) summary.append(el("span", "badge warn", "已截断"));
		if (message.timestamp) summary.append(el("time", "message-time", formatTime(message.timestamp)));
		const preview = String(message.content || "").replace(/\s+/g, " ").trim();
		if (preview) summary.append(el("span", "tool-preview", truncateText(preview, 90)));
		const copy = el("button", "message-copy", "复制");
		copy.type = "button";
		copy.addEventListener("click", async (event) => {
			event.preventDefault();
			event.stopPropagation();
			await copyText(message.content || "");
			copy.textContent = "已复制";
			setTimeout(() => { copy.textContent = "复制"; }, 1200);
		});
		summary.append(copy);
		item.append(summary);
		let output = null;
		const renderOutput = () => {
			if (output) return;
			output = el("pre", "tool-output");
			output.textContent = message.content || "(空结果)";
			item.append(output);
		};
		item.addEventListener("toggle", () => {
			if (item.open) renderOutput();
		});
		return item;
	}

	contextNode(message) {
		const details = el("details", "message context-block");
		const summary = el("summary", "");
		const label = message.kind === "runtime_context"
			? "运行上下文"
			: message.kind === "system_prompt"
				? "系统提示"
				: "系统消息";
		summary.append(el("span", "role", label));
		if (message.source) summary.append(el("span", "badge", message.source));
		details.append(summary);
		const body = el("div", "content");
		let rendered = false;
		const renderBody = () => {
			if (rendered) return;
			rendered = true;
			body.append(renderRichText(message.content));
			details.append(body);
		};
		details.addEventListener("toggle", () => {
			if (details.open) renderBody();
		});
		return details;
	}

	showEmpty(text) {
		if (!this.emptyNode) {
			this.emptyNode = el("div", "empty", text);
			this.messages.append(this.emptyNode);
			return;
		}
		this.emptyNode.textContent = text;
	}

	dropEmpty() {
		if (!this.emptyNode) return;
		this.emptyNode.remove();
		this.emptyNode = null;
	}

	dropDraft() {
		if (!this.draftNode) {
			this.draftKey = "";
			this.draftRich = null;
			return;
		}
		this.draftNode.remove();
		this.draftNode = null;
		this.draftBody = null;
		this.draftText = null;
		this.draftReasoning = null;
		this.draftReasoningText = null;
		this.draftRich = null;
		this.draftKey = "";
	}

	buildTurnInfos(pane, messages) {
		const infos = new Map();
		for (const turn of pane?.turns || []) {
			if (!turn?.turn_id) continue;
			infos.set(turn.turn_id, {
				turnId: turn.turn_id,
				status: turn.status || "",
				startedAt: Number(turn.started_ts) || 0,
				endedAt: Number(turn.ended_ts) || 0,
				error: String(turn.error || ""),
			});
		}
		const groups = new Map();
		const order = [];
		for (const message of messages) {
			const turnId = message.turn_id;
			if (!turnId) continue;
			let group = groups.get(turnId);
			if (!group) {
				group = { messages: [], finalSeq: 0, lastSeq: 0, toolCount: 0 };
				groups.set(turnId, group);
				order.push(turnId);
			}
			group.messages.push(message);
			group.lastSeq = Math.max(group.lastSeq, message.seq || 0);
			if (message.role === "tool") group.toolCount++;
			if (message.role === "assistant" && String(message.content || "").trim() && !messageHasToolCalls(message)) {
				group.finalSeq = message.seq;
			}
		}
		order.forEach((turnId, index) => {
			const group = groups.get(turnId);
			const processSeqs = new Set();
			for (const message of group.messages) {
				if (message.role !== "assistant" && message.role !== "tool") continue;
				if (message.seq === group.finalSeq) continue;
				processSeqs.add(message.seq);
			}
			const known = infos.get(turnId) || {};
			let status = known.status || "";
			if (!status) {
				status = pane?.turn?.turn_id === turnId ? (pane.turn.status || "running") : "completed";
			}
			const currentError = pane?.turn?.turn_id === turnId ? (pane.turn.error || "") : "";
			infos.set(turnId, {
				turnId,
				index: index + 1,
				status,
				startedAt: known.startedAt || 0,
				endedAt: known.endedAt || 0,
				error: known.error || currentError,
				processCount: processSeqs.size,
				toolCount: group.toolCount,
				lastSeq: group.lastSeq,
				processSeqs,
			});
		});
		return infos;
	}

	messageRow(message) {
		// Facts are append-only and immutable by seq, so a resync can reuse the
		// node without stringifying the whole message on every push.
		let row = this.messageRows.get(message.seq);
		if (!row) {
			row = { node: this.messageNode(message) };
			this.messageRows.set(message.seq, row);
		}
		return row;
	}

	turnHeaderRecord(turnId) {
		let record = this.turnHeaderCache.get(turnId);
		if (record) return record;
		const node = el("div", "turn-fold");
		const toggle = el("button", "turn-toggle");
		toggle.type = "button";
		const chevron = el("span", "turn-chevron", "");
		chevron.setAttribute("aria-hidden", "true");
		const title = el("span", "turn-title", "");
		const meta = el("span", "turn-meta", "");
		toggle.append(chevron, title, meta);
		toggle.addEventListener("click", () => this.toggleTurn(turnId));
		node.append(toggle);
		record = { node, toggle, chevron, title, meta };
		this.turnHeaderCache.set(turnId, record);
		return record;
	}

	turnStatusRecord(turnId) {
		let record = this.turnStatusRows.get(turnId);
		if (record) return record;
		const node = el("div", "turn-status-row");
		const text = el("span", "turn-status-text");
		node.append(text);
		record = { node, text };
		this.turnStatusRows.set(turnId, record);
		return record;
	}

	updateTurnStatusRow(record, info) {
		const label = turnStatusLabels[info.status] || info.status || "过程";
		const noTools = info.status === "completed" && info.toolCount === 0 ? " · 未调用工具" : "";
		const detail = info.error ? ` · ${info.error}` : "";
		record.node.className = `turn-status-row ${info.status}`;
		record.node.title = info.error || "";
		record.text.textContent = `第 ${info.index} 轮 · ${label}${noTools}${detail}`;
	}

	isTurnFolded(turnId, info) {
		if (info.status === "running") return false;
		const key = `${this.renderedSessionId}:${turnId}`;
		if (this.turnFolds.has(key)) return this.turnFolds.get(key);
		return true;
	}

	toggleTurn(turnId) {
		const info = this.turnInfoById.get(turnId);
		if (!info || info.status === "running") return;
		const key = `${this.renderedSessionId}:${turnId}`;
		this.turnFolds.set(key, !this.isTurnFolded(turnId, info));
		this.applyFoldState();
	}

	updateTurnHeader(record, info, folded) {
		const running = info.status === "running";
		record.toggle.disabled = running;
		record.toggle.title = info.error || "";
		record.toggle.setAttribute("aria-expanded", String(!folded));
		record.chevron.hidden = running;
		record.chevron.textContent = folded ? "▸" : "▾";
		record.title.textContent = `第 ${info.index} 轮 · ${turnStatusLabels[info.status] || info.status || "过程"}`;
		const parts = [];
		if (info.processCount > 0) parts.push(`${info.processCount} 条过程`);
		if (info.toolCount > 0) parts.push(`${info.toolCount} 个工具`);
		const duration = formatDuration(info.startedAt, info.endedAt);
		if (duration) parts.push(duration);
		if (folded && !running) parts.push("已折叠");
		record.meta.textContent = parts.join(" · ");
		record.node.classList.toggle("running", running);
	}

	applyFoldState() {
		for (const [turnId, record] of this.turnHeaders) {
			const info = this.turnInfoById.get(turnId);
			if (!info) continue;
			const folded = this.isTurnFolded(turnId, info);
			for (const row of this.processRows.get(turnId) || []) row.hidden = folded;
			this.updateTurnHeader(record, info, folded);
		}
	}

	syncLayout(nodes) {
		const wanted = new Set(nodes);
		for (let child = this.messages.firstChild; child;) {
			const next = child.nextSibling;
			if (!wanted.has(child) && child !== this.draftNode && child !== this.emptyNode) {
				child.remove();
			}
			child = next;
		}
		let cursor = this.messages.firstChild;
		for (const node of nodes) {
			if (cursor === node) {
				cursor = cursor.nextSibling;
				continue;
			}
			this.messages.insertBefore(node, cursor);
		}
	}

	renderLayout(pane, messages) {
		this.turnInfoById = this.buildTurnInfos(pane, messages);
		this.processRows = new Map();
		this.turnHeaders = new Map();
		const live = new Set(messages.map((message) => message.seq));
		for (const seq of [...this.messageRows.keys()]) {
			if (!live.has(seq)) this.messageRows.delete(seq);
		}
		const nodes = [];
		const inserted = new Set();
		for (const message of messages) {
			const turnId = message.turn_id || "";
			const info = turnId ? this.turnInfoById.get(turnId) : null;
			const isProcess = Boolean(info && info.processSeqs.has(message.seq));
			if (isProcess && !inserted.has(turnId)) {
				inserted.add(turnId);
				const record = this.turnHeaderRecord(turnId);
				this.turnHeaders.set(turnId, record);
				nodes.push(record.node);
			}
			const row = this.messageRow(message);
			nodes.push(row.node);
			if (info && info.lastSeq === message.seq && info.processCount === 0 &&
				(VISIBLE_TURN_STATUS_ROW.includes(info.status) ||
					(info.status === "completed" && info.toolCount === 0))) {
				const status = this.turnStatusRecord(turnId);
				this.updateTurnStatusRow(status, info);
				nodes.push(status.node);
			}
			if (isProcess) {
				let rows = this.processRows.get(turnId);
				if (!rows) {
					rows = [];
					this.processRows.set(turnId, rows);
				}
				rows.push(row.node);
			}
		}
		this.syncLayout(nodes);
		this.applyFoldState();
	}

	render() {
		if (!this.messages) return;
		this.renderTabs();
		this.renderWorkspace();
		this.renderMessages();
	}

	renderMessages() {
		let stick = this.stickToBottom;
		const pane = this.activePane();
		const draft = this.activeDraft();
		const messages = pane ? pane.messages || [] : [];
		if (this.renderedSessionId !== this.selectedId) {
			// A different session owns the container: cached rows and draft
			// references leave with the old one.
			this.renderedSessionId = this.selectedId;
			this.messageRows.clear();
			this.turnHeaderCache.clear();
			this.turnHeaders.clear();
			this.turnStatusRows.clear();
			this.processRows.clear();
			this.turnInfoById.clear();
			this.messages.replaceChildren();
			this.emptyNode = null;
			this.dropDraft();
			stick = true;
			this.stickToBottom = true;
		}
		this.loadEarlierButton.hidden = !pane?.has_more;
		this.loadEarlierButton.disabled = this.loadingEarlier;
		this.loadEarlierButton.textContent = this.loadingEarlier ? "加载中…" : "加载更早消息";
		this.renderLayout(pane, messages);
		if (draft?.message) this.renderDraft();
		else this.dropDraft();
		if (!pane) {
			this.showEmpty("从左侧打开一个会话");
		} else if (messages.length === 0 && !draft?.message) {
			this.showEmpty("输入一条消息开始对话");
		} else {
			this.dropEmpty();
		}
		if (stick) this.scrollToBottom();
		this.updateScrollButton();
		this.updateControls();
	}

	renderDraft() {
		const draft = this.activeDraft();
		if (!this.messages || !draft?.message) return;
		const stick = this.stickToBottom;
		this.dropEmpty();
		// A frame for another turn/step starts a fresh draft block: leftover
		// reasoning from the previous step must not bleed into it.
		const key = `${draft.turn_id || ""}/${draft.step_id || ""}`;
		if (this.draftNode && this.draftKey !== key) this.dropDraft();
		if (!this.draftNode) {
			this.draftKey = key;
			const item = el("div", "message assistant draft");
			const header = el("div", "message-header");
			header.append(el("span", "role thinking", "生成中…"));
			item.append(header);
			this.draftBody = el("div", "content");
			this.draftRich = new IncrementalRichText(this.draftBody);
			item.append(this.draftBody);
			this.messages.append(item);
			this.draftNode = item;
		}
		const content = draft.message.content || "";
		if (content !== this.draftText) {
			if (!this.draftRich) this.draftRich = new IncrementalRichText(this.draftBody);
			this.draftRich.render(content);
			this.draftText = content;
		}
		const reasoning = draft.message.reasoning || "";
		if (reasoning && !this.draftReasoning) {
			const details = el("details", "reasoning");
			details.append(el("summary", "", "思考过程"));
			this.draftReasoningText = el("div", "reasoning-body");
			details.append(this.draftReasoningText);
			this.draftNode.append(details);
			this.draftReasoning = details;
		}
		if (this.draftReasoningText && this.draftReasoningText.textContent !== reasoning) {
			this.draftReasoningText.textContent = reasoning;
		}
		if (stick) this.scrollToBottom();
		this.updateScrollButton();
		this.updateControls();
	}

	async send() {
		const pane = this.activePane();
		if (!pane) return;
		if (!pane.workspace) {
			this.error.textContent = "请先设置会话工作区";
			return;
		}
		const text = this.input.value.trim();
		const sessionId = pane.session_id;
		if (!text || this.busy.has(sessionId) || pane.turn?.status === "running") return;
		this.error.textContent = "";
		this.busy.add(sessionId);
		this.input.value = "";
		this.inputDrafts.delete(sessionId);
		this.autoGrow();
		this.renderTabs();
		this.updateControls();
		try {
			await window.LiteAgent.call("loop", "turn", {
				input: text,
				session_id: sessionId,
				workspace: pane.workspace,
			});
		} catch (error) {
			this.error.textContent = describeError(error);
			if (this.selectedId === sessionId) {
				this.input.value = text;
				this.saveInputDraft();
				this.autoGrow();
			}
		} finally {
			this.busy.delete(sessionId);
			this.renderTabs();
			this.updateControls();
			if (this.selectedId === sessionId) this.input.focus();
		}
	}

	async cancel() {
		const sessionId = this.selectedId;
		if (this.cancelButton.disabled || !sessionId) return;
		this.cancelButton.disabled = true;
		try {
			await window.LiteAgent.call("loop", "cancel", { session_id: sessionId });
		} catch (error) {
			this.error.textContent = describeError(error);
		} finally {
			this.updateControls();
		}
	}
}

class SessionTrace extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
        <style>${styles}</style>
        <div class="trace-head">
          <span class="hint"></span>
          <select class="trace-filter" aria-label="筛选记录类型">
            <option value="all">全部</option>
            <option value="message">仅消息</option>
            <option value="process">仅过程</option>
          </select>
        </div>
        <ul class="trace-list"></ul>
      `;
		this.head = this.shadowRoot.querySelector(".hint");
		this.filter = this.shadowRoot.querySelector(".trace-filter");
		this.list = this.shadowRoot.querySelector(".trace-list");
		this.state = { facts: [], revision: 0, through_seq: 0, total: 0 };
		this.limit = TRACE_PAGE_SIZE;
		this.expanded = new Set();
		this.rows = new Map();
		this.resyncPromise = null;
		this.loadingEarlier = false;
		this.stickToBottom = true;
		this.list.addEventListener("scroll", () => {
			this.stickToBottom = this.list.scrollHeight - this.list.scrollTop - this.list.clientHeight <= 24;
		});
		this.filter.addEventListener("change", () => {
			this.limit = TRACE_PAGE_SIZE;
			this.stickToBottom = true;
			this.render();
		});
		this.render();
		this.refresh();
	}

	async refresh() {
		try {
			const state = await call("state", {});
			if (state.trace) this.setData(state.trace);
		} catch (error) {
			this.list.replaceChildren(el("li", "error", describeError(error)));
		}
	}

	async resync() {
		const state = await call("state", {});
		if (state.trace) this.setData(state.trace);
	}

	setData(state) {
		const next = state || { facts: [], revision: 0, through_seq: 0, total: 0 };
		if (next.session_id === this.state?.session_id &&
			Number.isFinite(next.through_seq) && Number.isFinite(this.state?.through_seq) &&
			next.through_seq < this.state.through_seq) return;
		if (next.session_id === this.state?.session_id &&
			Number.isFinite(next.revision) && Number.isFinite(this.state?.revision) &&
			next.revision < this.state.revision) return;
		if (next.session_id !== this.state?.session_id) {
			this.rows.clear();
			this.expanded.clear();
			this.limit = TRACE_PAGE_SIZE;
			this.stickToBottom = true;
		}
		this.state = next;
		this.render();
	}

	runCommand(command, payload) {
		if (command === "append") this.applyAppend(payload);
	}

	applyAppend(payload) {
		if (!payload?.session_id || payload.session_id !== this.state?.session_id) return;
		const expected = Number.isFinite(this.state.through_seq) ? this.state.through_seq : 0;
		if (Number.isFinite(payload.after_seq) && payload.after_seq !== expected) {
			this.requestResync();
			return;
		}
		const current = this.state.facts || [];
		const lastSeq = current.length > 0 ? current[current.length - 1].seq : 0;
		const appended = (Array.isArray(payload.facts) ? payload.facts : [])
			.filter((fact) => fact && Number(fact.seq) > lastSeq);
		let facts = appended.length > 0 ? [...current, ...appended] : current;
		let firstSeq = Number(this.state.first_seq) || 0;
		let hasMore = this.state.has_more === true;
		let trimmed = false;
		const drop = trimTraceWindow(facts);
		if (drop > 0) {
			facts = facts.slice(drop);
			firstSeq = facts[0]?.seq || 0;
			hasMore = true;
			trimmed = true;
		}
		const oldHeight = trimmed ? this.list.scrollHeight : 0;
		const oldTop = trimmed ? this.list.scrollTop : 0;
		this.state = {
			...this.state,
			facts,
			first_seq: firstSeq,
			has_more: hasMore,
			through_seq: Number(payload.through_seq) || expected,
			total: Number.isFinite(payload.total) ? payload.total : this.state.total,
			revision: Number.isFinite(payload.revision) ? payload.revision : this.state.revision,
		};
		this.render();
		if (trimmed && !this.stickToBottom) {
			this.list.scrollTop = Math.max(0, oldTop - (oldHeight - this.list.scrollHeight));
		}
	}

	async loadEarlierFacts() {
		if (!this.state?.session_id || !this.state.has_more || this.loadingEarlier) return;
		this.loadingEarlier = true;
		this.render();
		try {
			const page = await call("facts", {
				session_id: this.state.session_id,
				before_seq: this.state.first_seq || 0,
				limit: TRACE_WINDOW_FACTS,
			});
			const older = Array.isArray(page?.facts) ? page.facts : [];
			const current = this.state.facts || [];
			const firstExisting = current.length > 0 ? current[0].seq : 0;
			const prepended = older.filter((fact) =>
				fact && (!firstExisting || Number(fact.seq) < firstExisting));
			if (prepended.length > 0) {
				this.stickToBottom = false;
				this.state = {
					...this.state,
					facts: [...prepended, ...current],
					first_seq: Number(page.first_seq) || prepended[0].seq,
					has_more: page.has_more === true,
					total: Number.isFinite(page.total) ? page.total : this.state.total,
				};
				this.limit = Math.min(this.state.facts.length, this.limit + prepended.length);
			} else {
				this.state = { ...this.state, has_more: false };
			}
		} catch (error) {
			this.list.replaceChildren(el("li", "error", describeError(error)));
		} finally {
			this.loadingEarlier = false;
			this.render();
			if (!this.stickToBottom) this.list.scrollTop = 0;
		}
	}

	requestResync() {
		if (this.resyncPromise) return;
		this.resyncPromise = this.resync()
			.catch((error) => {
				this.list.replaceChildren(el("li", "error", describeError(error)));
			})
			.finally(() => {
				this.resyncPromise = null;
			});
	}

	visibleFacts() {
		const facts = this.state.facts || [];
		const mode = this.filter?.value || "all";
		if (mode === "message") return facts.filter((fact) => fact.type === "message");
		if (mode === "process") return facts.filter((fact) => fact.type !== "message");
		return facts;
	}

	factNode(fact) {
		const details = el("details", "fact");
		if (this.expanded.has(fact.seq)) details.open = true;
		const summary = el("summary", "");
		summary.append(el("span", "seq", `#${fact.seq}`));
		summary.append(el("span", "badge", factTypeLabels[fact.type] || fact.type || "未知"));
		if (fact.role) summary.append(el("span", "badge", roleLabel(fact.role)));
		const preview = this.previewOf(fact);
		if (preview) summary.append(el("span", "preview", preview));
		details.append(summary);
		let pre = null;
		const renderDetail = () => {
			if (pre) return;
			pre = el("pre", "");
			pre.textContent = JSON.stringify(fact, null, 2);
			details.append(pre);
		};
		details.addEventListener("toggle", () => {
			if (details.open) {
				this.expanded.add(fact.seq);
				renderDetail();
			} else {
				this.expanded.delete(fact.seq);
			}
		});
		if (details.open) renderDetail();
		return details;
	}

	previewOf(fact) {
		// Preview only a bounded prefix: a Fact can carry a megabyte of tool output.
		const source = String(fact.content || "");
		const text = source.slice(0, 2048).replace(/\s+/g, " ").trim();
		if (text) return text.length > 60 ? `${text.slice(0, 60)}…` : text;
		if (fact.tool_call_id) return `tool_call_id: ${fact.tool_call_id}`;
		const meta = fact.meta;
		if (meta && typeof meta === "object") {
			const parts = ["turn_id", "step_id", "reason", "step", "model_context_id", "mode", "message_count", "token_estimate"]
				.filter((key) => meta[key] !== undefined && meta[key] !== "")
				.map((key) => `${key}=${meta[key]}`);
			if (parts.length > 0) return parts.join(" ");
		}
		return "";
	}

	earlierNode(localHidden) {
		const button = el("button", "trace-more");
		button.type = "button";
		button.disabled = this.loadingEarlier;
		button.textContent = localHidden > 0
			? `显示更早（本地还有 ${localHidden} 条）`
			: this.loadingEarlier ? "加载中…" : "从服务端加载更早";
		button.addEventListener("click", () => {
			if (localHidden > 0) {
				this.limit += TRACE_PAGE_SIZE;
				this.render();
				return;
			}
			this.loadEarlierFacts();
		});
		const item = el("li", "");
		item.append(button);
		return item;
	}

	render() {
		if (!this.list) return;
		const all = this.state.facts || [];
		const facts = this.visibleFacts();
		const total = Number.isFinite(this.state.total) ? this.state.total : all.length;
		const windowText = all.length < total ? `，窗口 ${all.length} 条` : "";
		const filterText = facts.length === all.length ? "" : `，筛选 ${facts.length} 条`;
		this.head.textContent = `共 ${total} 条记录${windowText}${filterText}`;
		if (facts.length === 0) {
			this.rows.clear();
			const nodes = [];
			if (this.state.has_more) nodes.push(this.earlierNode(0));
			nodes.push(el("li", "empty", all.length === 0 ? "暂无记录" : "没有匹配的记录"));
			this.list.replaceChildren(...nodes);
			return;
		}
		const shown = facts.slice(-this.limit);
		const localHidden = facts.length - shown.length;
		const nodes = [];
		if (localHidden > 0 || this.state.has_more) nodes.push(this.earlierNode(localHidden));

		// Reuse the node of an unchanged fact. Facts are immutable by seq, and
		// re-parenting keeps <details>.open, so expanded records survive pushes.
		const live = new Set();
		for (const fact of shown) {
			live.add(fact.seq);
			const row = this.rows.get(fact.seq);
			if (row) {
				nodes.push(row.node);
				continue;
			}
			const node = this.factNode(fact);
			this.rows.set(fact.seq, { node });
			nodes.push(node);
		}
		for (const seq of this.rows.keys()) {
			if (!live.has(seq)) this.rows.delete(seq);
		}
		this.list.replaceChildren(...nodes);
		if (this.stickToBottom) this.list.scrollTop = this.list.scrollHeight;
	}
}

class SessionDock extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
        <style>${dockStyles}</style>
        <div class="dock-row">
          <span class="chip model-chip" title="会话创建时选择的模型">
            <span class="label">模型</span><strong class="model-text">—</strong>
          </span>
          <button type="button" class="chip context-chip" title="上下文占用来自最近一次 API 请求的 prompt_tokens">
            <svg class="ring" viewBox="0 0 14 14" aria-hidden="true">
              <circle class="track" cx="7" cy="7" r="5.5"></circle>
              <circle class="fill" cx="7" cy="7" r="5.5" transform="rotate(-90 7 7)"></circle>
            </svg>
            <span class="context-text">上下文 —</span>
          </button>
          <button type="button" class="chip usage-chip" title="会话累计 token 用量">
            <span class="label">用量</span><span class="value usage-text">—</span>
          </button>
          <span class="spacer"></span>
          <span class="turn-status" aria-live="polite"></span>
        </div>
        <div class="dock-backdrop" hidden></div>
        <div class="dock-panel" role="dialog" aria-modal="true" aria-label="上下文与用量" hidden>
          <div class="panel-head">
            <strong>上下文与用量</strong>
            <button type="button" class="panel-close" aria-label="关闭明细" title="关闭">×</button>
          </div>
          <div class="panel-body"></div>
        </div>
      `;
		this.modelText = this.shadowRoot.querySelector(".model-text");
		this.contextChip = this.shadowRoot.querySelector(".context-chip");
		this.contextText = this.shadowRoot.querySelector(".context-text");
		this.ring = this.shadowRoot.querySelector(".ring");
		this.ringFill = this.shadowRoot.querySelector(".ring .fill");
		this.usageChip = this.shadowRoot.querySelector(".usage-chip");
		this.usageText = this.shadowRoot.querySelector(".usage-text");
		this.statusText = this.shadowRoot.querySelector(".turn-status");
		this.backdrop = this.shadowRoot.querySelector(".dock-backdrop");
		this.panel = this.shadowRoot.querySelector(".dock-panel");
		this.panelBody = this.shadowRoot.querySelector(".panel-body");
		this.panelClose = this.shadowRoot.querySelector(".panel-close");
		this.state = { revision: 0, turn: {} };
		this.panelOpen = false;
		this.panelTrigger = null;
		this.panelView = "last";
		this.panelDataKey = "";
		this.inspectData = null;
		this.previewData = null;
		this.panelLoading = false;
		this.panelError = "";
		this.panelRequestToken = 0;
		this.contextChip.addEventListener("click", (event) => this.openPanel(event.currentTarget));
		this.usageChip.addEventListener("click", (event) => this.openPanel(event.currentTarget));
		this.panelClose.addEventListener("click", () => this.closePanel());
		this.backdrop.addEventListener("click", () => this.closePanel());
		this.onKeyDown = (event) => {
			if (event.key === "Escape" && this.panelOpen) {
				event.preventDefault();
				this.closePanel();
			}
		};
		window.addEventListener("keydown", this.onKeyDown);
		this.hidden = true;
		this.refresh();
	}

	disconnectedCallback() {
		window.removeEventListener("keydown", this.onKeyDown);
	}

	async refresh() {
		try {
			const state = await call("dock", {});
			this.setData(state);
		} catch {
			this.hidden = true;
		}
	}

	async resync() {
		await this.refresh();
	}

	setData(state) {
		const next = state || {};
		if (Number.isFinite(next.revision) && Number.isFinite(this.state.revision) &&
			next.revision < this.state.revision) return;
		const sessionChanged = (next.session_id || "") !== (this.state.session_id || "");
		this.state = next;
		if (sessionChanged) {
			this.resetPanelData();
			this.closePanel();
		}
		this.hidden = !next.session_id;
		if (this.hidden) {
			this.closePanel();
			return;
		}
		this.render();
		if (this.panelOpen) {
			this.renderPanel();
			this.loadPanelData();
		}
	}

	runCommand(command, payload) {
		if (command === "append") this.applyAppend(payload);
	}

	applyAppend(payload) {
		if (!payload?.session_id || payload.session_id !== this.state?.session_id) return;
		if (Number.isFinite(payload.revision) && Number.isFinite(this.state.revision) &&
			payload.revision < this.state.revision) return;
		if (payload.turn) this.state.turn = { ...(this.state.turn || {}), ...payload.turn };
		if (payload.usage) {
			const current = this.state.usage || {};
			const delta = payload.usage;
			this.state.usage = {
				...current,
				prompt_tokens: (Number(current.prompt_tokens) || 0) + (Number(delta.prompt_tokens) || 0),
				completion_tokens: (Number(current.completion_tokens) || 0) + (Number(delta.completion_tokens) || 0),
				total_tokens: (Number(current.total_tokens) || 0) + (Number(delta.total_tokens) || 0),
				requests: (Number(current.requests) || 0) + (Number(delta.requests) || 0),
				last_prompt_tokens: delta.last_prompt_tokens ?? current.last_prompt_tokens ?? 0,
				model: delta.model || current.model || "",
				context_window: delta.context_window || current.context_window || 0,
			};
		}
		if (payload.context) this.state.context = payload.context;
		if (payload.request) this.state.request = payload.request;
		if (Number.isFinite(payload.revision)) this.state.revision = payload.revision;
		this.render();
		if (this.panelOpen) this.renderPanel();
	}

	contextWindow() {
		const usage = this.state.usage || {};
		return Number(this.state.context_window) || Number(usage.context_window) || 0;
	}

	percent() {
		const usage = this.state.usage || {};
		const window = this.contextWindow();
		const last = Number(usage.last_prompt_tokens) || 0;
		if (!window || !last) return null;
		return Math.max(0, Math.min(100, Math.round(last / window * 100)));
	}

	render() {
		const usage = this.state.usage || {};
		const turn = this.state.turn || {};
		const model = this.state.model || "";
		this.modelText.textContent = model || "默认";
		this.modelText.title = model ? `会话模型 · ${model}` : "会话未指定模型，使用 llm 默认模型";
		const window = this.contextWindow();
		const percent = this.percent();
		const last = Number(usage.last_prompt_tokens) || 0;
		if (percent === null) {
			this.ring.style.display = "none";
			this.contextText.textContent = last ? `上下文 ${formatTokenCount(last)}` : "上下文 —";
		} else {
			this.ring.style.display = "";
			this.ringFill.setAttribute(
				"stroke-dasharray",
				`${(ringCircumference * percent / 100).toFixed(2)} ${ringCircumference.toFixed(2)}`,
			);
			this.contextText.textContent = `上下文 ${formatTokenCount(last)} / ${formatTokenCount(window)} · ${percent}%`;
		}
		this.contextChip.classList.toggle("warn", percent !== null && percent >= 90);
		this.ring.classList.toggle("warn", percent !== null && percent >= 90);
		if (usage.requests > 0) {
			this.usageText.textContent = formatTokenCount(usage.total_tokens);
			this.usageChip.title = `输入 ${formatExact(usage.prompt_tokens)} · 输出 ${formatExact(usage.completion_tokens)} · 合计 ${formatExact(usage.total_tokens)} · ${usage.requests} 次请求`;
		} else {
			this.usageText.textContent = "—";
			this.usageChip.title = "尚未收到 API 用量";
		}
		const running = turn.status === "running";
		const cancelling = turn.cancel_requested === true;
		this.statusText.className = running ? "turn-status running" : "turn-status";
		this.statusText.textContent = cancelling ? "正在取消…" : running ? "生成中…" : "";
	}

	openPanel(trigger) {
		if (!this.state.session_id) return;
		this.panelTrigger = trigger || null;
		this.panelOpen = true;
		this.panelView = "last";
		this.panelDataKey = "";
		this.panelError = "";
		this.panelLoading = false;
		this.backdrop.hidden = false;
		this.panel.hidden = false;
		this.renderPanel();
		this.panelClose.focus();
		this.loadPanelData();
	}

	closePanel() {
		if (!this.panelOpen && this.panel.hidden) return;
		this.panelOpen = false;
		this.panelRequestToken += 1;
		this.panelLoading = false;
		this.backdrop.hidden = true;
		this.panel.hidden = true;
		if (this.panelTrigger && this.panelTrigger.isConnected) this.panelTrigger.focus();
		this.panelTrigger = null;
	}

	resetPanelData() {
		this.panelRequestToken += 1;
		this.panelDataKey = "";
		this.inspectData = null;
		this.previewData = null;
		this.panelLoading = false;
		this.panelError = "";
	}

	panelKey() {
		const contextID = this.state.context?.model_context_id || "";
		const requests = this.state.usage?.requests || 0;
		return `${this.panelView}:${this.state.session_id || ""}:${contextID}:${requests}`;
	}

	switchPanelView(view) {
		if (this.panelView === view) return;
		this.panelView = view;
		this.panelDataKey = "";
		this.panelError = "";
		this.panelLoading = false;
		this.renderPanel();
		this.loadPanelData(true);
	}

	refreshPanel() {
		this.panelDataKey = "";
		this.loadPanelData(true);
	}

	async loadPanelData(force = false) {
		if (!this.panelOpen || !this.state.session_id) return;
		const key = this.panelKey();
		if (!force && key === this.panelDataKey) return;
		this.panelDataKey = key;
		const token = ++this.panelRequestToken;
		this.panelLoading = true;
		this.panelError = "";
		this.renderPanel();
		try {
			let data;
			if (this.panelView === "preview") {
				const request = this.state.request || {};
				data = await window.LiteAgent.call("model-context", "build", {
					session_id: this.state.session_id,
					mode: request.mode || "",
					scheme: request.scheme || "",
					workspace: this.state.workspace || "",
					workspace_id: this.state.workspace_id || "",
					model: this.state.model || "",
					context_window: this.contextWindow(),
				});
				if (token !== this.panelRequestToken) return;
				this.previewData = data;
			} else {
				data = await window.LiteAgent.call("model-context", "inspect", {
					session_id: this.state.session_id,
				});
				if (token !== this.panelRequestToken) return;
				this.inspectData = data;
			}
		} catch (error) {
			if (token !== this.panelRequestToken) return;
			this.panelError = describeError(error);
		} finally {
			if (token === this.panelRequestToken) {
				this.panelLoading = false;
				this.renderPanel();
			}
		}
	}

	panelRow(list, label, value) {
		list.append(el("dt", "", label), el("dd", "", value));
	}

	renderPanel() {
		const usage = this.state.usage || {};
		const context = this.state.context || null;
		const body = this.panelBody;
		body.replaceChildren();
		if (usage.requests > 0) {
			const percent = this.percent();
			const line = el("div", "context-line");
			if (percent !== null) {
				const bar = el("div", percent >= 90 ? "progress warn" : "progress");
				const fill = el("span");
				fill.style.width = `${percent}%`;
				bar.append(fill);
				line.append(bar, el("span", "context-reading", `${percent}%`));
			} else {
				line.append(el("span", "context-reading", "上下文窗口未知"));
			}
			body.append(line);
			const summary = el("dl", "summary");
			this.panelRow(summary, "上次请求输入", formatExact(usage.last_prompt_tokens));
			this.panelRow(summary, "会话累计输入", formatExact(usage.prompt_tokens));
			this.panelRow(summary, "会话累计输出", formatExact(usage.completion_tokens));
			this.panelRow(summary, "会话累计合计", formatExact(usage.total_tokens));
			this.panelRow(summary, "请求数", formatExact(usage.requests));
			if (this.contextWindow() > 0) {
				this.panelRow(summary, "模型上下文窗口", formatExact(this.contextWindow()));
			}
			if (this.state.model) this.panelRow(summary, "模型", this.state.model);
			body.append(summary);
		} else {
			body.append(el("div", "empty-line", "尚未收到 API 用量。"));
		}
		body.append(el("div", "section-title", "最近一次上下文快照"));
		if (context) {
			const summary = el("dl", "summary");
			this.panelRow(summary, "消息数", formatExact(context.message_count));
			this.panelRow(summary, "本地估算", `${formatExact(context.token_estimate)}（估算）`);
			if (context.model_context_id) this.panelRow(summary, "快照", context.model_context_id);
			if (context.model) this.panelRow(summary, "请求模型", context.model);
			body.append(summary);
			if (Array.isArray(context.warnings) && context.warnings.length > 0) {
				const warnings = el("ul", "warnings");
				for (const warning of context.warnings) warnings.append(el("li", "", warning));
				body.append(warnings);
			}
		} else {
			body.append(el("div", "empty-line", "尚无上下文快照。"));
		}
		body.append(el("div", "note", "上下文百分比使用最近一次 API 响应返回的 prompt_tokens；本地估算仅供参考，不参与计算。"));
		this.renderInspectSection(body);
	}

	renderInspectSection(body) {
		body.append(el("div", "section-title", "请求消息"));
		const tabs = el("div", "inspect-tabs");
		const last = el("button", this.panelView === "last" ? "panel-tab active" : "panel-tab", "最近一次请求");
		last.type = "button";
		last.setAttribute("aria-pressed", String(this.panelView === "last"));
		last.addEventListener("click", () => this.switchPanelView("last"));
		const preview = el("button", this.panelView === "preview" ? "panel-tab active" : "panel-tab", "下一次预览");
		preview.type = "button";
		preview.setAttribute("aria-pressed", String(this.panelView === "preview"));
		preview.addEventListener("click", () => this.switchPanelView("preview"));
		const refresh = el("button", "panel-refresh", "刷新");
		refresh.type = "button";
		refresh.addEventListener("click", () => this.refreshPanel());
		tabs.append(last, preview, refresh);
		body.append(tabs);
		if (this.panelLoading) {
			body.append(el("div", "inspect-status", "读取中…"));
			return;
		}
		if (this.panelError) {
			body.append(el("div", "inspect-error", this.panelError));
			return;
		}
		if (this.panelView === "preview") {
			this.renderPreview(body);
			return;
		}
		this.renderInspect(body);
	}

	renderInspect(body) {
		const data = this.inspectData;
		if (!data) {
			body.append(el("div", "inspect-status", "尚未读取。"));
			return;
		}
		if (!data.found) {
			body.append(el("div", "empty-line", "尚无请求快照。"));
			return;
		}
		if (data.integrity === "ok") {
			body.append(el("div", "inspect-integrity", "消息 hash 校验通过"));
		} else if (data.integrity === "mismatch") {
			body.append(el("div", "inspect-integrity bad", "消息 hash 与快照不一致，内容可能不精确"));
		}
		this.appendWarnings(body, [
			...(data.snapshot?.warnings || []),
			...(data.warnings || []),
		]);
		const breakdown = data.breakdown || {};
		const hasBreakdown = Object.values(breakdown).some((value) => Number(value) > 0);
		if (hasBreakdown) {
			body.append(el("div", "inspect-meta",
				`估算分布：系统 ${formatTokenCount(breakdown.system_prompt_tokens)} · 运行 ${formatTokenCount(breakdown.runtime_context_tokens)} · 摘要 ${formatTokenCount(breakdown.summary_tokens)} · 会话 ${formatTokenCount(breakdown.session_tokens)} · 工具 ${formatTokenCount(breakdown.tools_tokens)}`));
		}
		const messages = Array.isArray(data.messages) ? data.messages : [];
		body.append(el("div", "inspect-meta", `共 ${messages.length} 条消息 · 工具 ${(data.snapshot?.tools || []).length} 个`));
		const list = el("div", "inspect-list");
		messages.forEach((message, index) => list.append(this.inspectMessageNode(message, index)));
		body.append(list);
	}

	renderPreview(body) {
		const data = this.previewData;
		if (!data) {
			body.append(el("div", "inspect-status", "尚未读取。"));
			return;
		}
		body.append(el("div", "inspect-integrity", "预览基于当前 Session，尚未发送"));
		this.appendWarnings(body, data.warnings || []);
		const messages = Array.isArray(data.messages) ? data.messages : [];
		body.append(el("div", "inspect-meta",
			`共 ${messages.length} 条消息 · 工具 ${(data.tools || []).length} 个 · 估算 ${formatExact(data.token_estimate)} token`));
		const list = el("div", "inspect-list");
		messages.forEach((message, index) => list.append(this.inspectMessageNode(message, index)));
		body.append(list);
	}

	appendWarnings(body, warnings) {
		const unique = [...new Set((warnings || []).filter(Boolean))];
		if (unique.length === 0) return;
		const list = el("ul", "warnings");
		for (const warning of unique) list.append(el("li", "", warningLabels[warning] || warning));
		body.append(list);
	}

	inspectMessageNode(message, index) {
		const details = el("details", "inspect-message");
		const summary = el("summary");
		summary.append(el("span", "seq", message.seq ? `#${message.seq}` : `#${index + 1}`));
		summary.append(el("span", "badge", roleLabel(message.role)));
		if (message.source) summary.append(el("span", "badge", sourceLabels[message.source] || message.source));
		if (message.truncated) summary.append(el("span", "badge warn", "已截断"));
		if (message.token_estimate) summary.append(el("span", "meta", `~${formatExact(message.token_estimate)} tok`));
		const text = String(message.content || "").replace(/\s+/g, " ").trim();
		if (text) summary.append(el("span", "inspect-preview", text.length > 90 ? `${text.slice(0, 90)}…` : text));
		details.append(summary);
		const body = el("div", "inspect-body");
		if (message.content) {
			const pre = el("pre", "inspect-content");
			pre.textContent = message.content;
			body.append(pre);
		}
		if (message.tool_calls) {
			const pre = el("pre", "inspect-content");
			pre.textContent = JSON.stringify(message.tool_calls, null, 2);
			body.append(pre);
		}
		details.append(body);
		return details;
	}
}

customElements.define("session-history", SessionHistory);
customElements.define("session-chat", SessionChat);
customElements.define("session-trace", SessionTrace);
customElements.define("session-dock", SessionDock);
