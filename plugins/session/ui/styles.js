export const BASE = `
:host {
	--sx-font: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
	--sx-mono: "SF Mono", "JetBrains Mono", "Fira Code", Consolas, "Microsoft YaHei", monospace;
	--sx-bg: #f7f9fb;
	--sx-surface: #ffffff;
	--sx-side: #f8fafc;
	--sx-text: #1f2329;
	--sx-muted: #7a828d;
	--sx-faint: #a8afb9;
	--sx-border: #e5e8ec;
	--sx-border-strong: #d6dbe2;
	--sx-hover: #eef2f6;
	--sx-active: #e8efff;
	--sx-accent: #4d6bfe;
	--sx-accent-strong: #2f4bd8;
	--sx-accent-soft: #eef3ff;
	--sx-danger: #d64545;
	--sx-user-bubble: #eff6ff;

	display: block;
	height: 100%;
	min-width: 0;
	color: var(--sx-text);
	background: var(--sx-bg);
	font: 13px/1.5 var(--sx-font);
	-webkit-font-smoothing: antialiased;
}

*, *::before, *::after { box-sizing: border-box; }

button, input, textarea, select { font: inherit; color: inherit; }

button { cursor: pointer; }

button:focus-visible,
input:focus-visible,
textarea:focus-visible,
select:focus-visible {
	outline: 2px solid var(--sx-accent);
	outline-offset: 1px;
}

::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-thumb { background: #d8dde4; border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: #c4cbd4; }
::-webkit-scrollbar-track { background: transparent; }

.icon-button {
	display: inline-grid;
	place-items: center;
	width: 28px;
	height: 28px;
	padding: 0;
	border: none;
	border-radius: 8px;
	background: transparent;
	color: var(--sx-muted);
	flex: none;
}
.icon-button:hover { background: var(--sx-hover); color: var(--sx-text); }
.icon-button:disabled { opacity: .45; cursor: default; }
`;

export const HISTORY_CSS = `${BASE}
.history { display: flex; flex-direction: column; height: 100%; background: var(--sx-side); }
.head { padding: 14px 12px 8px; flex: none; }
.eyebrow {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 0 4px 10px;
	color: var(--sx-muted);
	font-size: 11px;
	letter-spacing: .04em;
}
.count {
	padding: 1px 6px;
	border-radius: 999px;
	background: #edf1f5;
	color: var(--sx-muted);
	font-size: 10px;
	font-variant-numeric: tabular-nums;
}
.new-chat {
	display: flex;
	align-items: center;
	justify-content: center;
	gap: 6px;
	width: 100%;
	height: 38px;
	border: 1px solid var(--sx-border);
	border-radius: 12px;
	background: var(--sx-surface);
	color: var(--sx-text);
	font-size: 13px;
	font-weight: 500;
	box-shadow: 0 1px 2px rgba(16, 24, 40, .04);
}
.new-chat:hover { background: #f1f4f8; }
.workspace-row { display: flex; align-items: center; gap: 4px; margin-top: 8px; }
.workspace-select {
	flex: 1;
	min-width: 0;
	height: 34px;
	padding: 0 8px;
	border: 1px solid transparent;
	border-radius: 8px;
	background: transparent;
	color: var(--sx-text);
	font-size: 12px;
	cursor: pointer;
}
.workspace-select:hover { background: var(--sx-hover); }
.search { position: relative; margin: 4px 12px 6px; flex: none; }
.search svg { position: absolute; left: 9px; top: 8px; color: var(--sx-faint); pointer-events: none; }
.search input {
	width: 100%;
	height: 32px;
	padding: 0 10px 0 30px;
	border: none;
	border-radius: 8px;
	background: #edf1f5;
	color: var(--sx-text);
	font-size: 12px;
}
.search input::placeholder { color: var(--sx-faint); }
.list { flex: 1; min-height: 0; overflow-y: auto; padding: 2px 8px 12px; }
.group { margin-top: 8px; }
.group-title { padding: 4px 8px; color: var(--sx-faint); font-size: 11px; }
.node { display: flex; flex-direction: column; }
.node .children {
	margin-left: 14px;
	padding-left: 5px;
	border-left: 1px solid var(--sx-border);
}
.children .row { padding-left: 6px; }
.children .row-title { font-size: 12px; }
.children .row-meta { font-size: 10px; }
.toggle {
	display: grid;
	place-items: center;
	width: 16px;
	height: 16px;
	flex: none;
	margin-left: -3px;
	padding: 0;
	border: none;
	border-radius: 4px;
	background: transparent;
	color: var(--sx-faint);
}
.toggle:hover { background: rgba(31, 35, 41, .08); color: var(--sx-text); }
.toggle svg { transition: transform .12s ease; }
.toggle.open svg { transform: rotate(90deg); }
.row {
	position: relative;
	display: flex;
	flex-direction: column;
	gap: 2px;
	padding: 8px;
	border-radius: 8px;
	cursor: pointer;
}
.row:hover { background: var(--sx-hover); }
.row.active { background: var(--sx-active); }
.row:focus-visible { outline: 2px solid var(--sx-accent); outline-offset: -2px; }
.row-main { display: flex; align-items: center; gap: 6px; min-width: 0; padding-right: 62px; }
.row-title {
	flex: 1;
	min-width: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	font-size: 13px;
	font-weight: 500;
}
.row.active .row-title { color: var(--sx-accent-strong); }
.row-meta {
	display: flex;
	align-items: center;
	gap: 6px;
	padding-right: 62px;
	color: var(--sx-muted);
	font-size: 11px;
}
.running-dot {
	width: 6px;
	height: 6px;
	border-radius: 50%;
	background: var(--sx-accent);
	animation: sx-pulse 1.2s ease-in-out infinite;
	flex: none;
}
@keyframes sx-pulse { 0%, 100% { opacity: .3; } 50% { opacity: 1; } }
.actions {
	position: absolute;
	right: 4px;
	top: 50%;
	transform: translateY(-50%);
	display: flex;
	gap: 2px;
	opacity: 0;
	transition: opacity .12s ease;
}
.row:hover .actions, .row.active .actions, .row.confirming .actions { opacity: 1; }
.actions .icon-button { width: 24px; height: 24px; border-radius: 6px; }
.row:hover .actions .icon-button:hover { background: var(--sx-surface); box-shadow: 0 1px 2px rgba(16, 24, 40, .08); }
.confirm { display: flex; gap: 4px; }
.confirm button {
	padding: 3px 7px;
	border: none;
	border-radius: 6px;
	background: var(--sx-surface);
	color: var(--sx-muted);
	font-size: 11px;
	box-shadow: 0 1px 2px rgba(16, 24, 40, .08);
}
.confirm .danger { color: #b42318; }
.rename-input {
	width: 100%;
	height: 28px;
	padding: 0 7px;
	border: 1px solid var(--sx-accent);
	border-radius: 6px;
	background: var(--sx-surface);
	font-size: 13px;
}
.empty-list { padding: 28px 12px; text-align: center; color: var(--sx-faint); font-size: 12px; line-height: 1.8; white-space: pre-line; }
.status {
	display: flex;
	align-items: flex-start;
	gap: 6px;
	flex: none;
	padding: 8px 12px;
	border-top: 1px solid var(--sx-border);
	font-size: 11px;
	line-height: 1.5;
}
.status.error { color: #b42318; background: #fff5f5; }
.status.notice { color: #1f6f4a; background: #f2fbf6; }
.status[hidden] { display: none; }
.status span { flex: 1; min-width: 0; word-break: break-word; }
.status .icon-button { width: 20px; height: 20px; border-radius: 4px; }
`;

export const CHAT_CSS = `${BASE}
.chat { display: flex; flex-direction: column; height: 100%; background: var(--sx-surface); }
.tabs-bar {
	display: flex;
	align-items: center;
	gap: 4px;
	flex: none;
	height: 40px;
	min-width: 0;
	padding: 6px 12px 0;
	border-bottom: 1px solid var(--sx-border);
	background: var(--sx-surface);
}
.tabs-bar[hidden] { display: none; }
.tabs-list {
	display: flex;
	align-items: center;
	gap: 4px;
	flex: 1;
	min-width: 0;
	overflow-x: auto;
	overflow-y: hidden;
	scrollbar-width: none;
}
.tabs-list::-webkit-scrollbar { display: none; }
.session-tab {
	display: flex;
	align-items: center;
	gap: 2px;
	flex: none;
	max-width: 220px;
	height: 30px;
	padding: 0 4px 0 10px;
	border: 1px solid transparent;
	border-radius: 8px;
	background: transparent;
	color: var(--sx-muted);
}
.session-tab:hover { background: var(--sx-hover); color: var(--sx-text); }
.session-tab.active { background: var(--sx-active); color: var(--sx-accent-strong); }
.tab-select {
	display: flex;
	align-items: center;
	gap: 5px;
	min-width: 0;
	max-width: 180px;
	height: 100%;
	padding: 0;
	border: none;
	background: transparent;
	color: inherit;
	font-size: 12px;
}
.tab-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tab-running {
	width: 6px;
	height: 6px;
	flex: none;
	border-radius: 50%;
	background: var(--sx-accent);
	animation: sx-tab-pulse 1.2s ease-in-out infinite;
}
@keyframes sx-tab-pulse { 50% { opacity: .35; } }
.tab-close {
	display: grid;
	place-items: center;
	width: 20px;
	height: 20px;
	flex: none;
	padding: 0;
	border: none;
	border-radius: 5px;
	background: transparent;
	color: inherit;
	opacity: .55;
}
.tab-close:hover { background: rgba(31, 35, 41, .08); opacity: 1; }
.tab-close:disabled { opacity: .3; cursor: default; }
.tab-add {
	display: grid;
	place-items: center;
	width: 28px;
	height: 28px;
	flex: none;
	padding: 0;
	border: none;
	border-radius: 8px;
	background: transparent;
	color: var(--sx-muted);
}
.tab-add:hover { background: var(--sx-hover); color: var(--sx-text); }
.head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 12px;
	flex: none;
	height: 54px;
	padding: 0 20px;
	border-bottom: 1px solid var(--sx-border);
}
.head-main { min-width: 0; display: flex; flex-direction: column; gap: 1px; }
.head-title {
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	font-size: 14px;
	font-weight: 600;
}
.head-meta {
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--sx-muted);
	font-size: 11px;
}
.banner {
	display: flex;
	align-items: flex-start;
	gap: 8px;
	margin: 8px 20px 0;
	padding: 8px 10px;
	border: 1px solid #f1d2d2;
	border-radius: 10px;
	background: #fff6f6;
	color: #b42318;
	font-size: 12px;
}
.banner span { flex: 1; min-width: 0; word-break: break-word; }
.banner[hidden] { display: none; }
.banner .icon-button { width: 20px; height: 20px; border-radius: 4px; color: #b42318; }
.stage { position: relative; display: flex; flex: 1; min-height: 0; }
.scroll { flex: 1; min-width: 0; overflow-y: auto; overflow-x: hidden; overscroll-behavior: contain; }
.column {
	display: flex;
	flex-direction: column;
	gap: 18px;
	max-width: 780px;
	margin: 0 auto;
	padding: 28px 28px 24px;
}
.empty {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	min-height: 46vh;
	gap: 8px;
	text-align: center;
}
.empty h1 { margin: 0; font-size: 24px; font-weight: 600; letter-spacing: -.01em; }
.empty p { margin: 0; color: var(--sx-muted); font-size: 13px; }
.chips { display: flex; flex-wrap: wrap; justify-content: center; gap: 8px; margin-top: 14px; }
.chip {
	padding: 6px 12px;
	border: 1px solid var(--sx-border);
	border-radius: 999px;
	background: var(--sx-surface);
	color: var(--sx-text);
	font-size: 12px;
}
.chip:hover { background: var(--sx-hover); border-color: var(--sx-border-strong); }
.load-earlier {
	align-self: center;
	padding: 6px 12px;
	border: 1px solid var(--sx-border);
	border-radius: 999px;
	background: var(--sx-surface);
	color: var(--sx-muted);
	font-size: 12px;
}
.load-earlier:hover { background: var(--sx-hover); border-color: var(--sx-border-strong); color: var(--sx-text); }
.more-note { margin-top: 6px; color: var(--sx-faint); font-size: 11px; }
.more-note[hidden] { display: none; }
.msg.other .body .load-full {
	display: block;
	margin-top: 6px;
	padding: 3px 9px;
	border: 1px solid var(--sx-border);
	border-radius: 6px;
	background: var(--sx-surface);
	color: var(--sx-accent-strong);
	font: 11px/1.5 var(--sx-font);
	white-space: normal;
}
.msg.other .body .load-full:hover { background: var(--sx-hover); }
.msg { display: flex; min-width: 0; }
.msg.user { justify-content: flex-end; }
.msg.user .bubble {
	max-width: 78%;
	padding: 10px 14px;
	border-radius: 18px;
	background: var(--sx-user-bubble);
	font-size: 14px;
	line-height: 22px;
	white-space: pre-wrap;
	word-break: break-word;
}
.msg.assistant { flex-direction: column; align-items: flex-start; }
.msg.assistant .body {
	font-size: 14px;
	line-height: 24px;
	white-space: pre-wrap;
	word-break: break-word;
}
.msg.assistant .reasoning {
	margin: 0 0 12px;
	border: 1px solid var(--sx-border);
	border-radius: 10px;
	background: #f8fafc;
	color: var(--sx-muted);
	overflow: hidden;
}
.msg.assistant .reasoning[hidden] { display: none; }
.msg.assistant .reasoning summary {
	cursor: pointer;
	padding: 8px 10px;
	color: var(--sx-muted);
	font-size: 12px;
	font-weight: 600;
	user-select: none;
}
.msg.assistant .reasoning summary::marker { color: var(--sx-faint); }
.msg.assistant .reasoning-body {
	padding: 0 12px 10px;
	max-height: 360px;
	overflow: auto;
	font-size: 13px;
	line-height: 21px;
	white-space: pre-wrap;
	word-break: break-word;
}
.msg.assistant .reasoning-body .md-p { margin: 0 0 8px; }
.msg.assistant .reasoning-body .md-p:last-child { margin-bottom: 0; }
.msg.assistant .reasoning-full {
	display: block;
	margin-top: 6px;
	padding: 3px 9px;
	border: 1px solid var(--sx-border);
	border-radius: 6px;
	background: var(--sx-surface);
	color: var(--sx-accent-strong);
	font: 11px/1.5 var(--sx-font);
}
.msg.assistant .reasoning-full:hover { background: var(--sx-hover); }
.msg.assistant .md-p { margin: 0 0 10px; }
.msg.assistant .md-p:last-child { margin-bottom: 0; }
.msg.assistant .md-heading { margin: 14px 0 8px; font-size: 15px; font-weight: 600; }
.msg.assistant .md-list { margin: 0 0 10px; padding-left: 20px; }
.msg.assistant .md-list li { margin: 2px 0; white-space: pre-wrap; }
.msg.assistant .md-quote {
	margin: 0 0 10px;
	padding-left: 10px;
	border-left: 3px solid var(--sx-border-strong);
	color: var(--sx-muted);
}
.msg.assistant .md-code {
	margin: 0 0 12px;
	padding: 10px 12px;
	border: 1px solid var(--sx-border);
	border-radius: 10px;
	background: #f6f8fa;
	overflow: auto;
	font: 12px/1.6 var(--sx-mono);
	font-family: var(--sx-mono);
}
.msg.assistant .md-code-lang { margin-bottom: 4px; color: var(--sx-faint); font-size: 10px; }
.msg.assistant .md-inline {
	padding: 1px 4px;
	border-radius: 4px;
	background: #f2f4f7;
	font-family: var(--sx-mono);
	font-size: .92em;
}
.msg.assistant a { color: var(--sx-accent-strong); text-decoration: none; }
.msg.assistant a:hover { text-decoration: underline; }
.msg.other { justify-content: flex-start; }
.msg.other .body {
	max-width: 100%;
	padding: 8px 10px;
	border: 1px solid var(--sx-border);
	border-radius: 10px;
	background: #f6f8fa;
	color: var(--sx-muted);
	font: 12px/1.6 var(--sx-mono);
	white-space: pre-wrap;
	word-break: break-word;
}
.draft-status { display: flex; align-items: center; gap: 6px; margin-top: 8px; color: var(--sx-muted); font-size: 12px; }
.draft.stopped .draft-status { color: #a15c00; }
.draft.error .draft-status { color: #b42318; }
.caret {
	display: inline-block;
	width: 7px;
	height: 16px;
	margin-left: 2px;
	border-radius: 1px;
	background: var(--sx-accent);
	vertical-align: -2px;
	animation: sx-blink 1s steps(2, start) infinite;
}
@keyframes sx-blink { 50% { opacity: 0; } }
.error-row {
	padding: 8px 10px;
	border: 1px solid #f1d2d2;
	border-radius: 10px;
	background: #fff6f6;
	color: #b42318;
	font-size: 13px;
	word-break: break-word;
}
.turn-status {
	display: flex;
	align-items: flex-start;
	gap: 8px;
	padding: 9px 10px;
	border: 1px solid #f1d2d2;
	border-radius: 10px;
	background: #fff6f6;
	color: #b42318;
	font-size: 12px;
	line-height: 1.5;
}
.turn-status.cancelled { border-color: #f0dfc0; background: #fffaf2; color: #a15c00; }
.turn-status-mark {
	display: grid;
	place-items: center;
	width: 18px;
	height: 18px;
	flex: none;
	border-radius: 50%;
	background: rgba(180, 35, 24, .08);
	font-size: 11px;
	font-weight: 700;
}
.turn-status.cancelled .turn-status-mark { background: rgba(161, 92, 0, .08); }
.turn-status-main { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; }
.turn-status-title { font-size: 12px; font-weight: 600; }
.turn-status-note { color: inherit; opacity: .75; }
.turn-status-actions { display: flex; align-items: center; gap: 6px; }
.turn-retry {
	padding: 3px 9px;
	border: 1px solid #f1d2d2;
	border-radius: 6px;
	background: var(--sx-surface);
	color: #b42318;
	font-size: 11px;
}
.turn-retry:hover { background: #fff0f0; }
.turn-retry:disabled { opacity: .5; cursor: default; }
.turn-details { color: inherit; font-size: 11px; }
.turn-details summary { cursor: pointer; opacity: .85; }
.turn-error-text {
	margin-top: 4px;
	padding: 6px 8px;
	max-height: 160px;
	overflow: auto;
	border-radius: 6px;
	background: rgba(180, 35, 24, .04);
	font: 11px/1.5 var(--sx-mono);
	white-space: pre-wrap;
	word-break: break-word;
}
.jump {
	position: absolute;
	right: 24px;
	bottom: 14px;
	width: 32px;
	height: 32px;
	border: 1px solid var(--sx-border);
	border-radius: 50%;
	background: var(--sx-surface);
	color: var(--sx-muted);
	box-shadow: 0 4px 14px rgba(15, 23, 42, .12);
	display: grid;
	place-items: center;
}
.jump:hover { color: var(--sx-text); }
.jump[hidden] { display: none; }
.composer { flex: none; padding: 10px 24px 14px; background: linear-gradient(to bottom, rgba(255, 255, 255, 0), var(--sx-surface) 20px); }
.card {
	max-width: 780px;
	margin: 0 auto;
	padding: 10px 10px 8px 14px;
	border: 1px solid var(--sx-border);
	border-radius: 18px;
	background: var(--sx-surface);
	box-shadow: 0 8px 30px rgba(15, 23, 42, .07);
	transition: border-color .15s ease, box-shadow .15s ease;
}
.card:focus-within { border-color: #c9d5ff; box-shadow: 0 8px 30px rgba(77, 107, 254, .12); }
.card textarea {
	display: block;
	width: 100%;
	min-height: 26px;
	max-height: 200px;
	padding: 2px 0;
	border: none;
	outline: none;
	resize: none;
	background: transparent;
	font-size: 14px;
	line-height: 22px;
}
.card textarea::placeholder { color: var(--sx-faint); }
.model-row {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-bottom: 6px;
}
.model-label {
	flex: none;
	color: var(--sx-faint);
	font-size: 11px;
}
.model-select {
	min-width: 0;
	max-width: 240px;
	height: 28px;
	padding: 0 8px;
	border: 1px solid var(--sx-border);
	border-radius: 7px;
	background: var(--sx-surface);
	color: var(--sx-text);
	font-size: 12px;
}
.model-select:disabled {
	cursor: not-allowed;
	opacity: .65;
}
.actions { display: flex; align-items: center; gap: 8px; margin-top: 6px; }
.hint {
	flex: 1;
	min-width: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--sx-faint);
	font-size: 11px;
}
.send, .stop {
	display: grid;
	place-items: center;
	width: 32px;
	height: 32px;
	border-radius: 50%;
	flex: none;
}
.send { border: none; background: var(--sx-accent); color: #fff; }
.send:hover { background: var(--sx-accent-strong); }
.send:disabled { background: #e5e8ec; color: #a9b0ba; cursor: default; }
.stop { border: 1px solid var(--sx-border); background: var(--sx-surface); color: #4b5563; }
.stop:hover { background: #f4f6f8; }
.stop[hidden], .send[hidden] { display: none; }
.footer { padding-top: 8px; text-align: center; color: var(--sx-faint); font-size: 11px; }
`;

export const TRACE_CSS = `${BASE}
.trace { display: flex; flex-direction: column; height: 100%; background: var(--sx-side); }
.head {
	display: flex;
	align-items: center;
	gap: 8px;
	flex: none;
	height: 54px;
	padding: 0 14px;
	border-bottom: 1px solid var(--sx-border);
}
.head-title { font-size: 13px; font-weight: 600; }
.count {
	padding: 1px 6px;
	border-radius: 999px;
	background: #edf1f5;
	color: var(--sx-muted);
	font-size: 10px;
	font-variant-numeric: tabular-nums;
}
.head .icon-button { margin-left: auto; }
.filters {
	display: flex;
	gap: 2px;
	flex: none;
	margin: 10px 12px 8px;
	padding: 2px;
	border-radius: 8px;
	background: #edf1f5;
}
.filter {
	flex: 1;
	height: 26px;
	border: none;
	border-radius: 6px;
	background: transparent;
	color: var(--sx-muted);
	font-size: 12px;
}
.filter.active {
	background: var(--sx-surface);
	color: var(--sx-text);
	box-shadow: 0 1px 2px rgba(16, 24, 40, .08);
}
.selection-bar {
	display: flex;
	align-items: center;
	gap: 8px;
	flex: none;
	min-height: 34px;
	margin: 0 12px 8px;
	color: var(--sx-muted);
	font-size: 11px;
}
.select-all { display: inline-flex; align-items: center; gap: 5px; cursor: pointer; }
.select-all-box, .select-box { width: 14px; height: 14px; margin: 0; accent-color: var(--sx-accent); cursor: pointer; }
.selection-count { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.selection-actions { display: flex; align-items: center; gap: 4px; margin-left: auto; }
.text-button, .export-button {
	height: 26px;
	padding: 0 8px;
	border: 1px solid var(--sx-border);
	border-radius: 7px;
	background: var(--sx-surface);
	color: var(--sx-muted);
	font-size: 11px;
}
.text-button:hover, .export-button:hover { background: var(--sx-hover); color: var(--sx-text); }
.export-button { border-color: var(--sx-accent); color: var(--sx-accent-strong); }
.text-button:disabled, .export-button:disabled { opacity: .45; cursor: default; }
.load-earlier {
	display: block;
	margin: 6px auto 10px;
	padding: 5px 12px;
	border: 1px solid var(--sx-border);
	border-radius: 999px;
	background: var(--sx-surface);
	color: var(--sx-muted);
	font-size: 11px;
}
.load-earlier:hover { background: var(--sx-hover); border-color: var(--sx-border-strong); color: var(--sx-text); }
.list { flex: 1; min-height: 0; overflow-y: auto; padding: 0 10px 16px; }
.fact { border-bottom: 1px solid #eef1f4; padding: 7px 4px; }
.fact.selected { background: var(--sx-accent-soft); border-radius: 8px; border-bottom-color: transparent; }
.fact-main { display: flex; align-items: center; gap: 5px; min-width: 0; }
.fact-head {
	display: flex;
	align-items: center;
	gap: 6px;
	flex: 1;
	min-width: 0;
	cursor: pointer;
	border-radius: 6px;
}
.fact-head:focus-visible { outline: 2px solid var(--sx-accent); outline-offset: 1px; }
.seq {
	width: 34px;
	flex: none;
	color: var(--sx-faint);
	font: 11px/1.5 var(--sx-mono);
	font-variant-numeric: tabular-nums;
}
.type {
	flex: none;
	padding: 1px 6px;
	border-radius: 999px;
	background: #edf1f5;
	color: #52606d;
	font-size: 10px;
}
.type.message { background: var(--sx-accent-soft); color: var(--sx-accent-strong); }
.role { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--sx-muted); font-size: 11px; }
.time { margin-left: auto; flex: none; color: var(--sx-faint); font-size: 10px; font-variant-numeric: tabular-nums; }
.chevron { flex: none; color: var(--sx-faint); transition: transform .15s ease; }
.fact.open .chevron { transform: rotate(90deg); }
.preview {
	display: -webkit-box;
	-webkit-line-clamp: 2;
	-webkit-box-orient: vertical;
	overflow: hidden;
	margin: 3px 0 0 24px;
	color: var(--sx-muted);
	font-size: 11px;
	line-height: 1.55;
	white-space: pre-wrap;
	word-break: break-word;
}
.fact.message .preview { color: var(--sx-text); }
.raw {
	margin: 8px 0 2px 24px;
	padding: 8px;
	max-height: 320px;
	overflow: auto;
	border-radius: 8px;
	background: #f5f7fa;
	color: #3d4753;
	font: 11px/1.55 var(--sx-mono);
	white-space: pre-wrap;
	word-break: break-word;
}
.empty:not(.loading) { padding: 40px 14px; text-align: center; color: var(--sx-faint); font-size: 12px; line-height: 1.8; }
`;
