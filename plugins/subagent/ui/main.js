import { callSubagent, clear, errorMessage, node } from "./client.js";

const ACTIVE_SESSION_EVENT = "liteagent:session-changed";
const ACTIVE_SESSION_REQUEST = "liteagent:session-request";
const POLL_INTERVAL_MS = 3000;

const PANEL_CSS = `
:host {
	display: block;
	height: 100%;
	color: inherit;
	font: 12px/1.45 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}
.dock {
	display: flex;
	align-items: center;
	height: 100%;
}
.dock-button {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	height: 26px;
	padding: 0 10px;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 6px;
	background: transparent;
	color: var(--text, #1f2329);
	font: inherit;
	font-size: 12px;
	cursor: pointer;
}
.dock-button:hover { background: rgba(127, 127, 127, .12); }
.dock-button[aria-expanded="true"] { border-color: var(--accent, #4d6bfe); background: var(--accent-soft, #eef3ff); }
.dock-dot {
	width: 7px;
	height: 7px;
	border-radius: 50%;
	background: var(--text-muted, #7a828d);
}
.dock-dot.running { background: var(--accent, #4d6bfe); animation: pulse 1.2s ease-in-out infinite; }
.dock-badge {
	min-width: 16px;
	height: 16px;
	padding: 0 5px;
	border-radius: 999px;
	background: rgba(127, 127, 127, .14);
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	line-height: 16px;
	text-align: center;
}
.dock-badge.running { background: var(--accent-soft, #eef3ff); color: var(--accent-strong, #2f4bd8); }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .35; } }

.subagent-dialog {
	width: min(720px, calc(100vw - 40px));
	max-height: min(640px, calc(100vh - 80px));
	padding: 0;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 10px;
	background: var(--surface, #fff);
	color: inherit;
	box-shadow: 0 18px 60px rgba(0, 0, 0, .28);
	overflow: hidden;
}
.subagent-dialog::backdrop { background: rgba(15, 23, 42, .28); }
.panel {
	display: flex;
	flex-direction: column;
	max-height: inherit;
}
.head {
	position: sticky;
	top: 0;
	z-index: 1;
	display: flex;
	align-items: center;
	gap: 8px;
	padding: 10px 14px;
	border-bottom: 1px solid var(--border, #e5e8ec);
	background: var(--surface, #fff);
}
.title { font-size: 13px; font-weight: 600; }
.summary { color: var(--text-muted, #7a828d); }
.spacer { flex: 1; }
.updated { color: var(--text-muted, #7a828d); font-size: 11px; }
button {
	font: inherit;
	color: inherit;
	background: transparent;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 5px;
	padding: 2px 8px;
	cursor: pointer;
}
button:hover:not(:disabled) { background: rgba(127, 127, 127, .12); }
button:disabled { opacity: .5; cursor: default; }
.close-button { margin-left: 2px; padding: 2px 7px; font-size: 15px; line-height: 1.2; }
.error {
	margin: 10px 14px 0;
	padding: 6px 8px;
	border: 1px solid #f0c7c3;
	border-radius: 6px;
	background: var(--danger-surface, #fff5f5);
	color: var(--danger, #b42318);
	white-space: pre-wrap;
}
.list {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding: 10px 14px 14px;
	overflow: auto;
	min-height: 0;
}
.empty {
	padding: 14px 2px;
	color: var(--text-muted, #7a828d);
}
.run {
	display: grid;
	grid-template-columns: 10px minmax(0, 1fr) auto;
	gap: 8px;
	align-items: start;
	padding: 8px 10px;
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 8px;
	background: var(--background, #f7f9fb);
}
.run-dot {
	width: 8px;
	height: 8px;
	margin-top: 4px;
	border-radius: 50%;
	background: var(--text-muted, #7a828d);
}
.run.st-running .run-dot { background: var(--accent, #4d6bfe); animation: pulse 1.2s ease-in-out infinite; }
.run.st-completed .run-dot { background: #2e7d32; }
.run.st-failed .run-dot { background: var(--danger, #b42318); }
.run.st-cancelled .run-dot { background: #9aa1ab; }
.run-main { min-width: 0; }
.run-head {
	display: flex;
	align-items: baseline;
	gap: 6px;
	min-width: 0;
}
.run-title {
	overflow: hidden;
	color: var(--text, #1f2329);
	font-size: 12px;
	font-weight: 550;
	text-overflow: ellipsis;
	white-space: nowrap;
}
.run-state {
	flex: none;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
}
.run.st-running .run-state { color: var(--accent-strong, #2f4bd8); }
.run.st-failed .run-state { color: var(--danger, #b42318); }
.run-meta {
	margin-top: 2px;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
	line-height: 1.5;
}
.run-error {
	margin-top: 3px;
	color: var(--danger, #b42318);
	font-size: 11px;
	line-height: 1.5;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.run-id {
	align-self: center;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	white-space: nowrap;
}
`;

function formatDuration(ms) {
	const value = Math.max(0, Number(ms) || 0);
	if (value < 1000) return `${Math.round(value)} 毫秒`;
	const seconds = value / 1000;
	if (seconds < 60) return `${seconds >= 10 ? Math.round(seconds) : seconds.toFixed(1)} 秒`;
	const minutes = Math.floor(seconds / 60);
	const rest = Math.round(seconds % 60);
	if (minutes < 60) return `${minutes} 分 ${rest} 秒`;
	const hours = Math.floor(minutes / 60);
	return `${hours} 时 ${minutes % 60} 分`;
}

function statusLabel(state) {
	switch (state) {
		case "running": return "运行中";
		case "completed": return "已完成";
		case "failed": return "失败";
		case "cancelled": return "已取消";
		default: return String(state || "未知");
	}
}

function displayTitle(run) {
	const title = String(run?.title || "").trim();
	const stripped = title.replace(/^subagent:\s*/i, "").trim();
	return stripped || String(run?.child_session_id || "子代理");
}

function shortId(id) {
	const value = String(id || "");
	if (value.length <= 16) return value;
	return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function summaryText(runs) {
	if (!runs.length) return "0 次派出";
	const counts = new Map();
	for (const run of runs) counts.set(run.state, (counts.get(run.state) || 0) + 1);
	const parts = [`${runs.length} 次派出`];
	for (const state of ["running", "completed", "failed", "cancelled"]) {
		const count = counts.get(state) || 0;
		if (count > 0) parts.push(`${count} ${statusLabel(state)}`);
	}
	return parts.join(" · ");
}

class SubagentPanel extends HTMLElement {
	static overlay = true;

	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.sessionId = "";
		this.data = null;
		this.error = "";
		this.loading = false;
		this.loadingPromise = null;
		this.pending = false;
		this.loadSeq = 0;
		this.refreshTimer = 0;
		this.pollTimer = 0;
		this.tickTimer = 0;
		this.elapsedNodes = [];
		this.onSessionChanged = (event) => this.applySession(event?.detail?.session_id);
		this.onTurnSettled = (event) => {
			const sessionId = String(event?.detail?.session_id || "");
			if (sessionId && sessionId !== this.sessionId) return;
			this.schedule(0);
		};
	}

	connectedCallback() {
		this.shadowRoot.innerHTML = `<style>${PANEL_CSS}</style>
			<div class="dock">
				<button type="button" class="dock-button" data-open aria-haspopup="dialog" aria-expanded="false">
					<span class="dock-dot" data-dot></span>
					<span>子代理</span>
					<span class="dock-badge" data-badge hidden></span>
				</button>
			</div>
			<dialog class="subagent-dialog" data-dialog aria-label="子代理">
				<section class="panel">
					<header class="head">
						<span class="title">子代理</span>
						<span class="summary" data-summary>0 次派出</span>
						<span class="spacer"></span>
						<span class="updated" data-updated></span>
						<button type="button" data-refresh>刷新</button>
						<button type="button" class="close-button" data-close aria-label="关闭">×</button>
					</header>
					<div class="error" data-error hidden></div>
					<div class="list" data-list></div>
				</section>
			</dialog>`;
		this.els = {
			open: this.shadowRoot.querySelector("[data-open]"),
			dot: this.shadowRoot.querySelector("[data-dot]"),
			badge: this.shadowRoot.querySelector("[data-badge]"),
			dialog: this.shadowRoot.querySelector("[data-dialog]"),
			close: this.shadowRoot.querySelector("[data-close]"),
			refresh: this.shadowRoot.querySelector("[data-refresh]"),
			summary: this.shadowRoot.querySelector("[data-summary]"),
			updated: this.shadowRoot.querySelector("[data-updated]"),
			error: this.shadowRoot.querySelector("[data-error]"),
			list: this.shadowRoot.querySelector("[data-list]"),
		};
		this.els.open.addEventListener("click", () => this.openDialog());
		this.els.close.addEventListener("click", () => this.closeDialog());
		this.els.dialog.addEventListener("close", () => {
			this.els.open.setAttribute("aria-expanded", "false");
			this.stopTicking();
		});
		this.els.refresh.addEventListener("click", () => this.load());
		globalThis.addEventListener?.(ACTIVE_SESSION_EVENT, this.onSessionChanged);
		globalThis.addEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		this.requestActiveSession();
		if (!this.loading) this.load();
	}

	disconnectedCallback() {
		globalThis.removeEventListener?.(ACTIVE_SESSION_EVENT, this.onSessionChanged);
		globalThis.removeEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		clearTimeout(this.refreshTimer);
		this.stopPolling();
		this.stopTicking();
	}

	openDialog() {
		if (!this.els?.dialog) return;
		this.els.open.setAttribute("aria-expanded", "true");
		if (!this.els.dialog.open) this.els.dialog.showModal();
		this.syncTicking();
		this.load();
	}

	closeDialog() {
		this.els?.dialog?.close();
	}

	requestActiveSession() {
		try {
			globalThis.dispatchEvent?.(new CustomEvent(ACTIVE_SESSION_REQUEST));
		} catch {
			// Session UI may not be mounted yet; its next change notification wins.
		}
	}

	applySession(value) {
		const next = String(value || "");
		if (next === this.sessionId) return;
		this.sessionId = next;
		this.data = null;
		this.error = "";
		this.loadSeq += 1;
		clearTimeout(this.refreshTimer);
		this.stopPolling();
		this.stopTicking();
		this.render();
		this.load();
	}

	schedule(delay = 150) {
		clearTimeout(this.refreshTimer);
		this.refreshTimer = setTimeout(() => this.load(), delay);
	}

	setData(state) {
		if (!state) return;
		const sessionId = String(state.session_id || "");
		if (sessionId && sessionId !== this.sessionId) return;
		this.schedule();
	}

	runCommand(command) {
		if (command === "refresh" || command === "subagent.refresh") this.load();
	}

	resync() {
		return this.load();
	}

	async load() {
		if (!this.els) return undefined;
		if (this.loading) {
			this.pending = true;
			return this.loadingPromise;
		}
		if (!this.sessionId) {
			this.data = { session_id: "", runs: [] };
			this.error = "";
			this.render();
			return undefined;
		}
		const seq = ++this.loadSeq;
		const sessionId = this.sessionId;
		this.loading = true;
		this.els.refresh.setAttribute("disabled", "");
		this.loadingPromise = callSubagent("runs", { session_id: sessionId })
			.then((payload) => {
				if (seq !== this.loadSeq) return;
				this.data = payload && typeof payload === "object"
					? payload
					: { session_id: sessionId, runs: [] };
				this.error = "";
				this.loadedAt = Date.now();
			})
			.catch((error) => {
				if (seq !== this.loadSeq) return;
				this.error = errorMessage(error);
			})
			.finally(() => {
				this.loading = false;
				this.els?.refresh?.removeAttribute("disabled");
				this.render();
				if (this.pending) {
					this.pending = false;
					this.load();
				}
			});
		return this.loadingPromise;
	}

	runs() {
		const runs = Array.isArray(this.data?.runs) ? this.data.runs : [];
		return runs.filter((run) => run && typeof run === "object");
	}

	render() {
		if (!this.els) return;
		const runs = this.runs();
		const summary = summaryText(runs);
		const running = runs.some((run) => run.state === "running");
		this.els.summary.textContent = summary;
		this.els.updated.textContent = this.loadedAt ? `更新于 ${new Date(this.loadedAt).toLocaleTimeString()}` : "";
		this.els.error.hidden = !this.error;
		this.els.error.textContent = this.error;

		this.els.badge.hidden = runs.length === 0;
		this.els.badge.textContent = String(runs.length);
		this.els.badge.classList.toggle("running", running);
		this.els.dot.classList.toggle("running", running);
		this.els.open.title = this.sessionId ? summary : "选择一个会话后查看子代理";
		if (this.els.dialog.open) this.els.open.setAttribute("aria-expanded", "true");

		this.elapsedNodes = [];
		clear(this.els.list);
		if (!this.sessionId) {
			this.els.list.append(node("div", "empty", "选择一个会话后，这里会显示它派出的子代理。"));
			this.syncPolling(runs);
			this.syncTicking(runs);
			return;
		}
		if (this.loading && !this.data) {
			this.els.list.append(node("div", "empty", "加载中…"));
			this.syncPolling(runs);
			this.syncTicking(runs);
			return;
		}
		if (runs.length === 0) {
			this.els.list.append(node("div", "empty", "当前会话还没有派出子代理。"));
			this.syncPolling(runs);
			this.syncTicking(runs);
			return;
		}
		for (const run of runs) this.els.list.append(this.renderRun(run));
		this.syncPolling(runs);
		this.syncTicking(runs);
	}

	renderRun(run) {
		const state = ["running", "completed", "failed", "cancelled"].includes(run.state)
			? run.state
			: "unknown";
		const row = node("div", `run st-${state}`);
		const main = node("div", "run-main");
		const head = node("div", "run-head");
		const title = displayTitle(run);
		head.append(
			node("span", "run-title", title),
			node("span", "run-state", statusLabel(state)),
		);
		main.append(head);

		const meta = node("div", "run-meta");
		const chips = [];
		const depth = Number(run.delegation_depth) || 0;
		if (depth > 0) chips.push(document.createTextNode(`深度 ${depth}`));
		if (state === "running") {
			const started = Number(run.started_ts) || Date.now();
			const elapsed = node("span", "", `已运行 ${formatDuration(Date.now() - started)}`);
			elapsed.dataset.started = String(started);
			this.elapsedNodes.push(elapsed);
			chips.push(elapsed);
		} else if (Number(run.duration_ms) > 0) {
			chips.push(document.createTextNode(`耗时 ${formatDuration(run.duration_ms)}`));
		}
		if (Number(run.turn_count) > 0) chips.push(document.createTextNode(`${run.turn_count} 轮`));
		if (Number(run.message_count) > 0) chips.push(document.createTextNode(`${run.message_count} 条消息`));
		chips.forEach((chip, index) => {
			if (index > 0) meta.append(document.createTextNode(" · "));
			meta.append(chip);
		});
		if (chips.length > 0) main.append(meta);
		if (run.error) main.append(node("div", "run-error", String(run.error)));

		const id = node("code", "run-id", shortId(run.child_session_id));
		id.title = String(run.child_session_id || "");
		row.append(node("span", "run-dot"), main, id);
		row.title = title;
		return row;
	}

	syncPolling(runs = this.runs()) {
		const running = runs.some((run) => run.state === "running");
		if (running) {
			if (!this.pollTimer) {
				this.pollTimer = setInterval(() => this.load(), POLL_INTERVAL_MS);
			}
			return;
		}
		this.stopPolling();
	}

	syncTicking(runs = this.runs()) {
		const running = runs.some((run) => run.state === "running");
		if (running && this.els?.dialog?.open) {
			if (!this.tickTimer) this.tickTimer = setInterval(() => this.updateElapsed(), 1000);
			return;
		}
		this.stopTicking();
	}

	updateElapsed() {
		const now = Date.now();
		for (const element of this.elapsedNodes) {
			const started = Number(element.dataset.started) || now;
			element.textContent = `已运行 ${formatDuration(now - started)}`;
		}
	}

	stopPolling() {
		if (!this.pollTimer) return;
		clearInterval(this.pollTimer);
		this.pollTimer = 0;
	}

	stopTicking() {
		if (!this.tickTimer) return;
		clearInterval(this.tickTimer);
		this.tickTimer = 0;
	}
}

customElements.define("subagent-panel", SubagentPanel);
