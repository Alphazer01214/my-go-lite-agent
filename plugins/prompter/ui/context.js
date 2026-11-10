import {
	callPrompter,
	clear,
	errorMessage,
	node,
	stringify,
} from "./client.js";
import { panelStyles } from "./styles.js";

const CONTEXT_CSS = `
:host {
	display: block;
	height: 100%;
	min-height: 0;
	overflow: hidden;
	color: inherit;
	font: 12px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}
.panel {
	display: flex;
	flex-direction: column;
	height: 100%;
	min-height: 0;
	padding: 0;
	gap: 0;
}
.head {
	flex: none;
	display: flex;
	flex-direction: column;
	gap: 7px;
	padding: 10px 12px 8px;
	border-bottom: 1px solid var(--border, #e5e8ec);
}
.head-line {
	display: flex;
	align-items: center;
	gap: 6px;
	min-width: 0;
}
.title { font-size: 13px; font-weight: 600; }
.head-line .spacer { flex: 1; }
.head-line .updated {
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	white-space: nowrap;
}
.head-line button {
	flex: none;
	width: 24px;
	height: 24px;
	padding: 0;
	border-radius: 6px;
	font-size: 13px;
	line-height: 1;
}
.ctx-meta {
	display: flex;
	align-items: center;
	gap: 6px;
	min-width: 0;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
}
.ctx-meta span {
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
}
.ctx-meta .ctx-model { flex: 0 1 auto; }
.ctx-meta .ctx-dot { opacity: .45; }
.ctx-usage { display: flex; flex-direction: column; gap: 4px; }
.ctx-usage-line {
	display: flex;
	align-items: baseline;
	gap: 5px;
	font-size: 11px;
}
.ctx-usage-line strong { font-size: 14px; font-weight: 600; }
.ctx-usage-line .muted { color: var(--text-muted, #7a828d); }
.ctx-usage-line .ctx-percent {
	margin-left: auto;
	color: var(--accent-strong, #2f4bd8);
	font-size: 11px;
	font-weight: 600;
}
.ctx-progress {
	height: 4px;
	border-radius: 999px;
	background: rgba(127, 127, 127, .18);
	overflow: hidden;
}
.ctx-progress-fill {
	height: 100%;
	border-radius: inherit;
	background: var(--accent, #4d6bfe);
	opacity: .75;
}
.ctx-usage-meta {
	color: var(--text-muted, #7a828d);
	font-size: 10px;
}
.tabs {
	flex: none;
	display: flex;
	gap: 2px;
	padding: 5px 8px;
	border-bottom: 1px solid var(--border, #e5e8ec);
	overflow-x: auto;
}
.tab {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	flex: none;
	padding: 3px 8px;
	border: 0;
	border-radius: 6px;
	background: transparent;
	color: var(--text-muted, #7a828d);
	font: inherit;
	font-size: 11px;
	cursor: pointer;
	white-space: nowrap;
}
.tab:hover { background: var(--surface-muted, #eef1f4); color: var(--text, #1f2329); }
.tab.active {
	background: var(--accent-soft, #eef3ff);
	color: var(--accent-strong, #2f4bd8);
	font-weight: 600;
}
.tab .count {
	min-width: 14px;
	padding: 0 4px;
	border-radius: 999px;
	background: rgba(127, 127, 127, .12);
	font-size: 10px;
	line-height: 14px;
	text-align: center;
	font-weight: 400;
}
.scroll {
	flex: 1 1 auto;
	min-height: 0;
	overflow: auto;
}
.ctx-body {
	display: flex;
	flex-direction: column;
	gap: 6px;
	padding: 8px 10px 14px;
}
.ctx-empty {
	padding: 28px 18px;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
	line-height: 1.6;
	text-align: center;
}
.error { margin: 8px 10px 0; }

.msg {
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 7px;
	background: var(--surface, #fff);
	overflow: hidden;
}
.msg-head {
	display: flex;
	align-items: center;
	gap: 6px;
	width: 100%;
	padding: 6px 8px;
	border: 0;
	border-radius: 0;
	background: transparent;
	color: inherit;
	font: inherit;
	text-align: left;
	cursor: pointer;
}
.msg-head:hover { background: var(--surface-muted, #eef1f4); }
.role {
	flex: none;
	padding: 1px 5px;
	border-radius: 4px;
	background: rgba(127, 127, 127, .12);
	font-size: 10px;
	font-weight: 600;
	letter-spacing: .02em;
	opacity: .9;
}
.role-system { color: #5b6470; }
.role-user { color: var(--accent-strong, #2f4bd8); background: var(--accent-soft, #eef3ff); }
.role-assistant { color: #2e7d32; background: #eaf5ea; }
.role-tool { color: #a15c00; background: #fff4e0; }
.msg-seq {
	flex: none;
	color: var(--text-muted, #7a828d);
	font: 10px/1.4 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	opacity: .65;
}
.msg-preview {
	flex: 1 1 auto;
	min-width: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
}
.chevron {
	flex: none;
	color: var(--text-muted, #7a828d);
	font-size: 12px;
	line-height: 1;
	transition: transform .15s ease;
}
.msg.open .chevron { transform: rotate(90deg); }
.msg-body {
	display: flex;
	flex-direction: column;
	gap: 6px;
	padding: 8px;
	border-top: 1px solid var(--border, #e5e8ec);
}
.msg-body[hidden] { display: none; }
.msg-text {
	margin: 0;
	padding: 8px;
	max-height: 320px;
	overflow: auto;
	border-radius: 6px;
	background: var(--surface-muted, #eef1f4);
	font: 11px/1.55 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.msg-note {
	display: flex;
	flex-wrap: wrap;
	gap: 4px;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
}
.msg-sub summary {
	cursor: pointer;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
}
.msg-sub pre {
	margin: 6px 0 0;
	padding: 6px;
	max-height: 200px;
	overflow: auto;
	border-radius: 5px;
	background: var(--surface-muted, #eef1f4);
	font-size: 10px;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.msg-more {
	display: flex;
	justify-content: center;
	padding: 2px 0;
}
.msg-more button { font-size: 11px; }

.ctx-tool {
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 7px;
	background: var(--surface, #fff);
	overflow: hidden;
}
.ctx-tool summary {
	display: flex;
	align-items: baseline;
	gap: 6px;
	padding: 6px 8px;
	list-style: none;
	cursor: pointer;
}
.ctx-tool summary::-webkit-details-marker { display: none; }
.ctx-tool-name {
	flex: none;
	font-size: 11px;
	font-weight: 600;
}
.ctx-tool-desc {
	flex: 1 1 auto;
	min-width: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
}
.ctx-tool pre {
	margin: 0;
	padding: 8px;
	max-height: 240px;
	overflow: auto;
	border-top: 1px solid var(--border, #e5e8ec);
	background: var(--surface-muted, #eef1f4);
	font-size: 10px;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}

.ctx-snapshot {
	display: grid;
	grid-template-columns: auto minmax(0, 1fr);
	gap: 6px 10px;
	margin: 0;
	padding: 8px;
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 7px;
	background: var(--surface, #fff);
	font-size: 11px;
}
.ctx-snapshot dt { color: var(--text-muted, #7a828d); white-space: nowrap; }
.ctx-snapshot dd {
	margin: 0;
	overflow: hidden;
	text-overflow: ellipsis;
	white-space: nowrap;
	font: 10px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.ctx-snapshot dd.wrap {
	white-space: normal;
	overflow-wrap: anywhere;
}
`;

function formatCompact(value) {
	const number = Number(value) || 0;
	if (number < 1000) return String(number);
	if (number < 1_000_000) {
		const scaled = number / 1000;
		return `${scaled >= 100 ? scaled.toFixed(0) : scaled.toFixed(1)}k`;
	}
	return `${(number / 1_000_000).toFixed(1)}M`;
}

function shortId(value) {
	const text = String(value || "");
	if (!text) return "—";
	if (text.length <= 14) return text;
	return `${text.slice(0, 6)}…${text.slice(-4)}`;
}

function shortHash(value) {
	const text = String(value || "");
	if (!text) return "—";
	if (text.length <= 12) return text;
	return `${text.slice(0, 10)}…`;
}

function clockTime(value) {
	if (!value) return "";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return String(value);
	return date.toLocaleTimeString([], { hour12: false });
}

function oneLine(value, limit = 180) {
	const text = String(value ?? "").replace(/\s+/g, " ").trim();
	if (!text) return "";
	return text.length > limit ? `${text.slice(0, limit)}…` : text;
}

function sourceLabel(source) {
	if (source === "live") return "实时";
	if (source === "derived") return "推导";
	return "无";
}

function messagePreview(message) {
	const text = String(message?.text || "").trim();
	if (text) return text;
	if (Array.isArray(message?.tool_calls) && message.tool_calls.length > 0) {
		return `${message.tool_calls.length} 个工具调用`;
	}
	if (message?.reasoning) return "思考过程";
	return "(空)";
}

class PrompterContext extends HTMLElement {
	static tabTitle = "Context";
	static tabOrder = 20;

	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.data = null;
		this.error = "";
		this.loading = false;
		this.loadingPromise = null;
		this.pending = false;
		this.refreshTimer = 0;
		this.sessionId = "";
		this.loadSeq = 0;
		this.section = "messages";
		this.expanded = new Set();
		this.messageWindow = 40;
		this.onTabShow = () => this.resync();
		this.onSessionChanged = (event) => this.applySession(event?.detail?.session_id);
		this.onTurnSettled = (event) => {
			if (this.hidden) return;
			const sessionId = String(event?.detail?.session_id || "");
			if (sessionId && sessionId !== this.sessionId) return;
			this.resync();
		};
	}

	connectedCallback() {
		this.shadowRoot.innerHTML = `<style>${panelStyles}${CONTEXT_CSS}</style>
			<div class="panel">
				<header class="head">
					<div class="head-line">
						<span class="title">Context</span>
						<span class="badge" data-source>—</span>
						<span class="spacer"></span>
						<span class="updated" data-updated></span>
						<button type="button" data-refresh title="刷新上下文" aria-label="刷新上下文">↻</button>
					</div>
					<div class="ctx-meta" data-meta></div>
					<div class="ctx-usage" data-usage></div>
				</header>
				<div class="tabs" data-tabs role="tablist" aria-label="Context 分区"></div>
				<div class="error" data-error hidden></div>
				<div class="scroll">
					<div class="ctx-body" data-body></div>
					<div class="ctx-empty" data-empty hidden></div>
				</div>
			</div>`;
		this.els = {
			source: this.shadowRoot.querySelector("[data-source]"),
			updated: this.shadowRoot.querySelector("[data-updated]"),
			refresh: this.shadowRoot.querySelector("[data-refresh]"),
			meta: this.shadowRoot.querySelector("[data-meta]"),
			usage: this.shadowRoot.querySelector("[data-usage]"),
			tabs: this.shadowRoot.querySelector("[data-tabs]"),
			error: this.shadowRoot.querySelector("[data-error]"),
			body: this.shadowRoot.querySelector("[data-body]"),
			empty: this.shadowRoot.querySelector("[data-empty]"),
		};
		this.els.refresh.addEventListener("click", () => this.resync());
		this.addEventListener("liteagent:tab-show", this.onTabShow);
		globalThis.addEventListener?.("liteagent:session-changed", this.onSessionChanged);
		globalThis.addEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		this.requestActiveSession();
		if (!this.loading) this.resync();
	}

	disconnectedCallback() {
		this.removeEventListener("liteagent:tab-show", this.onTabShow);
		globalThis.removeEventListener?.("liteagent:session-changed", this.onSessionChanged);
		globalThis.removeEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		clearTimeout(this.refreshTimer);
	}

	requestActiveSession() {
		try {
			globalThis.dispatchEvent?.(new CustomEvent("liteagent:session-request"));
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
		this.expanded.clear();
		this.messageWindow = 40;
		this.loadSeq += 1;
		clearTimeout(this.refreshTimer);
		this.render();
		if (!this.hidden) this.resync();
	}

	async resync() {
		if (!this.els) return undefined;
		if (this.loading) {
			this.pending = true;
			return this.loadingPromise;
		}
		const seq = ++this.loadSeq;
		const sessionId = this.sessionId;
		this.loading = true;
		this.els.refresh.setAttribute("disabled", "");
		this.render();
		this.loadingPromise = this.load(sessionId)
			.then((data) => {
				if (seq !== this.loadSeq) return;
				this.data = data;
				this.error = "";
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
					this.resync();
				}
			});
		return this.loadingPromise;
	}

	async load(sessionId = this.sessionId) {
		if (!sessionId) return { source: "none", messages: [], tools: [] };
		return callPrompter("context.get", { session_id: sessionId });
	}

	setData(state) {
		if (!state || this.hidden) return;
		const sessionId = String(state.session_id || "");
		if (sessionId && sessionId !== this.sessionId) return;
		this.state = state;
		clearTimeout(this.refreshTimer);
		this.refreshTimer = setTimeout(() => this.resync(), 250);
	}

	runCommand(command) {
		if (command === "refresh" || command === "context.refresh") {
			this.resync();
		}
	}

	render() {
		if (!this.els) return;
		const data = this.data;
		const source = data?.source || "none";
		this.els.source.textContent = sourceLabel(source);
		this.els.source.dataset.kind = source;
		this.els.source.title = source;
		this.els.updated.textContent = data?.assembled_at ? clockTime(data.assembled_at) : "";
		this.renderMeta(data, source);
		this.renderUsage(data?.usage);
		this.renderTabs(data);
		this.renderError();

		clear(this.els.body);
		const messages = Array.isArray(data?.messages) ? data.messages : [];
		const tools = Array.isArray(data?.tools) ? data.tools : [];
		const hasData = Boolean(data) && source !== "none";

		if (!this.sessionId) {
			this.showEmpty("选择一个会话后，这里会显示它最近一次模型可见的上下文。");
			return;
		}
		if (this.loading && !data) {
			this.showEmpty("加载中…");
			return;
		}
		if (!hasData) {
			this.showEmpty("还没有模型可见的上下文。运行一轮对话后再查看。");
			return;
		}
		this.els.empty.hidden = true;

		if (this.section === "tools") {
			this.renderTools(tools);
			return;
		}
		if (this.section === "snapshot") {
			this.renderSnapshot(data);
			return;
		}
		this.renderMessages(messages);
	}

	showEmpty(text) {
		this.els.empty.textContent = text;
		this.els.empty.hidden = Boolean(this.error);
	}

	renderMeta(data, source) {
		clear(this.els.meta);
		if (!data || source === "none") {
			this.els.meta.title = "";
			return;
		}
		const parts = [];
		if (data.session_id) parts.push(["ctx-session", shortId(data.session_id)]);
		if (data.model) parts.push(["ctx-model", data.model]);
		parts.forEach(([className, text], index) => {
			if (index > 0) this.els.meta.append(node("span", "ctx-dot", "·"));
			this.els.meta.append(node("span", className, text));
		});
		this.els.meta.title = `session ${data.session_id || "—"}${data.turn_id ? ` · turn ${data.turn_id}` : ""}`;
	}

	renderUsage(usage) {
		clear(this.els.usage);
		if (!usage) return;
		const prompt = Number(usage.prompt_tokens) || 0;
		const completion = Number(usage.completion_tokens) || 0;
		const total = Number(usage.total_tokens) || 0;
		const window = Number(usage.context_window) || 0;
		const percent = Number(usage.percent) || 0;
		const messages = Number(usage.message_count) || 0;
		const tools = Number(usage.tool_count) || 0;

		const line = node("div", "ctx-usage-line");
		if (total > 0 || prompt > 0) {
			line.append(node("strong", "", formatCompact(prompt || total)));
			if (window > 0) line.append(node("span", "muted", `/ ${formatCompact(window)} tokens`));
			else line.append(node("span", "muted", "prompt tokens"));
			if (window > 0 && prompt > 0) line.append(node("span", "ctx-percent", `${percent}%`));
		} else {
			line.append(node("span", "muted", "尚无真实用量记录"));
		}
		this.els.usage.append(line);
		if (window > 0 && prompt > 0) {
			const bar = node("div", "ctx-progress");
			const fill = node("div", "ctx-progress-fill");
			fill.style.width = `${Math.min(100, Math.max(0, percent))}%`;
			bar.append(fill);
			this.els.usage.append(bar);
		}
		const meta = [`${messages} 条消息`, `${tools} 个工具`];
		if (completion > 0) meta.push(`输出 ${formatCompact(completion)}`);
		this.els.usage.append(node("div", "ctx-usage-meta", meta.join(" · ")));
	}

	renderTabs(data) {
		clear(this.els.tabs);
		const messages = Array.isArray(data?.messages) ? data.messages.length : 0;
		const tools = Array.isArray(data?.tools) ? data.tools.length : 0;
		const entries = [
			["messages", "消息", messages],
			["tools", "工具", tools],
			["snapshot", "快照", null],
		];
		for (const [value, label, count] of entries) {
			const active = this.section === value;
			const button = node("button", `tab${active ? " active" : ""}`, label);
			button.type = "button";
			button.dataset.section = value;
			button.setAttribute("role", "tab");
			button.setAttribute("aria-selected", String(active));
			if (count !== null) button.append(node("span", "count", String(count)));
			button.addEventListener("click", () => {
				if (this.section === value) return;
				this.section = value;
				this.render();
			});
			this.els.tabs.append(button);
		}
	}

	renderError() {
		this.els.error.hidden = !this.error;
		this.els.error.textContent = this.error;
	}

	renderMessages(messages) {
		if (messages.length === 0) {
			this.showEmpty("模型可见消息为空。");
			return;
		}
		const pinned = [];
		if (messages[0]?.role === "system") pinned.push(messages[0]);
		if (messages[1]?.role === "user" && String(messages[1].text || "").trim().startsWith("<runtime_context>")) {
			pinned.push(messages[1]);
		}
		const rest = messages.slice(pinned.length);
		const start = Math.max(0, rest.length - this.messageWindow);
		const shownRest = rest.slice(start);

		for (const message of pinned) this.els.body.append(this.renderMessage(message));
		if (start > 0) {
			const more = node("div", "msg-more");
			const button = node("button", "", `显示更早的 ${start} 条`);
			button.type = "button";
			button.addEventListener("click", () => {
				this.messageWindow += 40;
				this.render();
			});
			more.append(button);
			this.els.body.append(more);
		}
		for (const message of shownRest) this.els.body.append(this.renderMessage(message));
	}

	renderMessage(message) {
		const seq = Number(message?.seq) || 0;
		const role = String(message?.role || "unknown");
		const open = this.expanded.has(seq);
		const row = node("div", `msg${open ? " open" : ""}`);
		const head = node("button", "msg-head");
		head.type = "button";
		head.setAttribute("aria-expanded", String(open));
		head.append(
			node("span", `role role-${role.replace(/[^a-z0-9_-]/gi, "")}`, role),
			node("span", "msg-seq", seq ? `#${seq}` : ""),
			node("span", "msg-preview", oneLine(messagePreview(message))),
			node("span", "chevron", "›"),
		);
		row.append(head);

		const body = node("div", "msg-body");
		body.hidden = !open;
		const text = String(message?.text || "");
		if (text) {
			body.append(node("pre", "msg-text", text));
		}
		const note = [];
		if (message?.truncated) note.push("正文已截断");
		if (message?.tool_call_id) note.push(`工具调用 ${message.tool_call_id}`);
		if (note.length) body.append(node("div", "msg-note", note.join(" · ")));

		if (message?.reasoning) {
			const details = node("details", "msg-sub");
			details.append(
				node("summary", "", `思考过程 · ${String(message.reasoning).length.toLocaleString()} 字符`),
				node("pre", "", String(message.reasoning)),
			);
			body.append(details);
		}
		if (Array.isArray(message?.tool_calls) && message.tool_calls.length > 0) {
			const details = node("details", "msg-sub");
			details.append(
				node("summary", "", `工具调用 · ${message.tool_calls.length}`),
				node("pre", "", stringify(message.tool_calls)),
			);
			body.append(details);
		}
		if (Array.isArray(message?.files) && message.files.length > 0) {
			const details = node("details", "msg-sub");
			details.append(
				node("summary", "", `文件 · ${message.files.length}`),
				node("pre", "", stringify(message.files)),
			);
			body.append(details);
		}
		row.append(body);

		head.addEventListener("click", () => {
			const next = !this.expanded.has(seq);
			if (next) this.expanded.add(seq);
			else this.expanded.delete(seq);
			row.classList.toggle("open", next);
			body.hidden = !next;
			head.setAttribute("aria-expanded", String(next));
		});
		return row;
	}

	renderTools(tools) {
		if (tools.length === 0) {
			this.showEmpty("本轮没有暴露工具。");
			return;
		}
		for (const tool of tools) {
			const details = node("details", "ctx-tool");
			const summary = node("summary");
			summary.append(
				node("span", "ctx-tool-name", tool.name || "tool"),
				node("span", "ctx-tool-desc", oneLine(tool.description || "")),
			);
			details.append(summary);
			if (tool.parameters) details.append(node("pre", "", stringify(tool.parameters)));
			this.els.body.append(details);
		}
	}

	renderSnapshot(data) {
		const snapshot = data?.snapshot;
		if (!snapshot) {
			this.showEmpty("本次请求没有快照。");
			return;
		}
		const grid = node("dl", "ctx-snapshot");
		const fields = [
			["turn", shortId(data.turn_id || "—"), data.turn_id || ""],
			["surface seqs", Array.isArray(snapshot.surface_seqs) && snapshot.surface_seqs.length
				? snapshot.surface_seqs.join(", ")
				: "—", ""],
			["messages hash", shortHash(snapshot.messages_hash), snapshot.messages_hash],
			["system hash", shortHash(snapshot.system_prompt_hash), snapshot.system_prompt_hash],
			["runtime hash", shortHash(snapshot.runtime_context_hash), snapshot.runtime_context_hash],
			["tool generation", String(snapshot.tool_catalog_generation ?? "—"), ""],
			["tool names", Array.isArray(snapshot.tool_names) && snapshot.tool_names.length
				? snapshot.tool_names.join(", ")
				: "—", ""],
		];
		for (const [label, value, full] of fields) {
			grid.append(node("dt", "", label));
			const wrap = label === "tool names" || label === "surface seqs";
			const cell = node("dd", wrap ? "wrap" : "", String(value));
			if (full && full !== value) cell.title = String(full);
			grid.append(cell);
		}
		this.els.body.append(grid);
	}
}

customElements.define("prompter-context", PrompterContext);
