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
.workspace-create-row {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  margin-bottom: 8px;
}
.workspace-create-row[hidden] { display: none; }
.workspace-create-row input {
  flex: 1;
  min-width: 0;
  padding: 6px 8px;
  border: 1px solid var(--session-border, #d5dbe1);
  border-radius: 6px;
  font: 12px/1.35 ui-monospace, SFMono-Regular, Consolas, monospace;
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
  min-height: 0;
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
.message .role { display: block; margin-bottom: 3px; color: var(--session-muted-text, #5c6875); font-size: 12px; }
.message .role.thinking { color: var(--session-thinking, #7c3aed); }
.content { overflow-wrap: anywhere; }
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

const roleLabels = Object.freeze({
	system: "系统",
	user: "用户",
	assistant: "助手",
	tool: "工具",
	host: "过程",
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
	session_meta: "会话信息",
});

const TERMINAL_TURN_STATUS = Object.freeze(["completed", "cancelled", "max_steps", "error"]);
const TRACE_PAGE_SIZE = 60;

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

class SessionHistory extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
        <style>${styles}</style>
        <div class="workspace-picker">
          <select class="workspace-select" aria-label="当前工作区"></select>
          <button type="button" class="workspace-add" title="添加工作区">添加</button>
        </div>
        <div class="workspace-create-row" hidden>
          <input class="workspace-create-input" type="text" placeholder="输入已存在的绝对目录">
          <button type="button" class="workspace-open">打开</button>
          <button type="button" class="workspace-cancel">取消</button>
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
		this.workspaceCreateRow = this.shadowRoot.querySelector(".workspace-create-row");
		this.workspaceCreateInput = this.shadowRoot.querySelector(".workspace-create-input");
		this.workspaceOpen = this.shadowRoot.querySelector(".workspace-open");
		this.workspaceCancel = this.shadowRoot.querySelector(".workspace-cancel");
		this.searchInput = this.shadowRoot.querySelector(".history-search");
		this.createButton = this.shadowRoot.querySelector(".create");
		this.list = this.shadowRoot.querySelector(".list");
		this.error = this.shadowRoot.querySelector(".error");
		this.state = { sessions: [], revision: 0 };
		this.query = "";
		this.creating = false;
		this.workspaceBusy = false;
		this.workspaceSelect.addEventListener("change", () => this.selectWorkspace(this.workspaceSelect.value));
		this.workspaceAdd.addEventListener("click", () => this.showWorkspaceCreate());
		this.workspaceCancel.addEventListener("click", () => this.hideWorkspaceCreate());
		this.workspaceOpen.addEventListener("click", () => this.createWorkspace());
		this.workspaceCreateInput.addEventListener("keydown", (event) => {
			if (event.key === "Enter" && !event.isComposing) {
				event.preventDefault();
				this.createWorkspace();
			}
		});
		this.createButton.addEventListener("click", () => this.createSession());
		this.searchInput.addEventListener("input", () => {
			this.query = this.searchInput.value.trim().toLowerCase();
			this.render();
		});
		this.render();
		this.refresh();
	}

	async refresh() {
		try {
			const state = await call("state", {});
			this.setData(state.history);
		} catch (error) {
			this.error.textContent = describeError(error);
		}
	}

	async resync() {
		const state = await call("state", {});
		this.setData(state.history);
	}

	async createSession() {
		const workspaceId = this.state.workspace_id;
		if (this.creating || this.workspaceBusy || !workspaceId) return;
		this.creating = true;
		this.error.textContent = "";
		this.render();
		try {
			const created = await call("create", { workspace_id: workspaceId });
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

	showWorkspaceCreate() {
		this.workspaceCreateRow.hidden = false;
		this.workspaceCreateInput.focus();
	}

	hideWorkspaceCreate() {
		this.workspaceCreateRow.hidden = true;
		this.workspaceCreateInput.value = "";
	}

	async createWorkspace() {
		const path = this.workspaceCreateInput.value.trim();
		if (!path) {
			this.workspaceCreateInput.focus();
			return;
		}
		this.workspaceBusy = true;
		this.error.textContent = "";
		this.render();
		try {
			await call("workspace.create", { path });
			this.hideWorkspaceCreate();
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
		this.renderedSessionId = null;
		this.draftKey = "";
		this.tabsSignature = "";
		this.workspaceSignature = "";

		this.sendButton.addEventListener("click", () => this.send());
		this.cancelButton.addEventListener("click", () => this.cancel());
		this.scrollLatest.addEventListener("click", () => this.scrollToBottom());
		this.messages.addEventListener("scroll", () => this.updateScrollButton());
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
		this.saveInputDraft();
		if (Array.isArray(next.panes)) {
			const incoming = new Map();
			for (const pane of next.panes) {
				if (!pane?.session_id) continue;
				const previous = this.panes.get(pane.session_id);
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
			if (pane.stream) this.drafts.set(sessionId, pane.stream);
			this.reconcileDraft(sessionId);
		}
		for (const sessionId of this.drafts.keys()) {
			if (!this.panes.has(sessionId)) this.drafts.delete(sessionId);
		}
		this.restoreInputDraft();
		this.render();
	}

	runCommand(command, payload) {
		if (command !== "stream") return;
		const sessionId = payload?.session_id;
		if (!sessionId || !this.panes.has(sessionId)) return;
		this.drafts.set(sessionId, payload);
		this.renderTabs();
		if (sessionId !== this.selectedId) return;
		// Stream frames only touch the draft. Rebuilding the landed list per
		// frame would drop text selections and collapse open details.
		if (payload?.message) this.renderDraft();
		else this.renderMessages();
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
		this.scrollLatest.hidden = this.nearBottom();
	}

	autoGrow() {
		this.input.style.height = "auto";
		this.input.style.height = `${Math.min(this.input.scrollHeight, 148)}px`;
	}

	nearBottom() {
		return this.messages.scrollHeight - this.messages.scrollTop - this.messages.clientHeight <= 48;
	}

	scrollToBottom() {
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
		body.append(renderRichText(message.content));
		if (message.reasoning) {
			const details = el("details", "reasoning");
			details.append(el("summary", "", "思考过程"));
			details.append(el("div", "reasoning-body", message.reasoning));
			body.append(details);
		}
		if (message.tool_calls) {
			const details = el("details", "tool-calls");
			details.append(el("summary", "", "工具调用"));
			const pre = el("pre", "tool-json");
			pre.textContent = JSON.stringify(message.tool_calls, null, 2);
			details.append(pre);
			body.append(details);
		}
		item.append(body);
		return item;
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
			return;
		}
		this.draftNode.remove();
		this.draftNode = null;
		this.draftBody = null;
		this.draftText = null;
		this.draftReasoning = null;
		this.draftReasoningText = null;
		this.draftKey = "";
	}

	// syncMessageNodes reconciles the landed list in place. Rows are cached by
	// seq like the trace pane's, and unchanged nodes never move: re-parenting a
	// node clears any text selection inside it.
	syncMessageNodes(messages) {
		const order = [];
		const wanted = new Set();
		const live = new Set();
		for (const message of messages) {
			live.add(message.seq);
			let row = this.messageRows.get(message.seq);
			const json = JSON.stringify(message);
			if (!row || row.json !== json) {
				row = { json, node: this.messageNode(message) };
				this.messageRows.set(message.seq, row);
			}
			order.push(row.node);
			wanted.add(row.node);
		}
		for (const seq of [...this.messageRows.keys()]) {
			if (!live.has(seq)) this.messageRows.delete(seq);
		}
		for (let child = this.messages.firstChild; child;) {
			const next = child.nextSibling;
			if (!wanted.has(child) && child !== this.draftNode && child !== this.emptyNode) {
				child.remove();
			}
			child = next;
		}
		let cursor = this.messages.firstChild;
		for (const node of order) {
			if (cursor === node) {
				cursor = cursor.nextSibling;
				continue;
			}
			this.messages.insertBefore(node, cursor);
		}
	}

	render() {
		if (!this.messages) return;
		this.renderTabs();
		this.renderWorkspace();
		this.renderMessages();
	}

	renderMessages() {
		const stick = this.nearBottom();
		const pane = this.activePane();
		const draft = this.activeDraft();
		const messages = pane ? pane.messages || [] : [];
		if (this.renderedSessionId !== this.selectedId) {
			// A different session owns the container: cached rows and draft
			// references leave with the old one.
			this.renderedSessionId = this.selectedId;
			this.messageRows.clear();
			this.messages.replaceChildren();
			this.emptyNode = null;
			this.dropDraft();
		}
		this.syncMessageNodes(messages);
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
		const stick = this.nearBottom();
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
			item.append(this.draftBody);
			this.messages.append(item);
			this.draftNode = item;
		}
		const content = draft.message.content || "";
		if (content !== this.draftText) {
			this.draftBody.replaceChildren(renderRichText(content));
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
		this.state = { facts: [] };
		this.limit = TRACE_PAGE_SIZE;
		this.expanded = new Set();
		this.rows = new Map();
		this.filter.addEventListener("change", () => {
			this.limit = TRACE_PAGE_SIZE;
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
		const next = state || { facts: [], revision: 0 };
		if (next.session_id === this.state?.session_id &&
			Number.isFinite(next.revision) && Number.isFinite(this.state?.revision) &&
			next.revision < this.state.revision) return;
		if (next.session_id !== this.state?.session_id) {
			this.rows.clear();
			this.expanded.clear();
			this.limit = TRACE_PAGE_SIZE;
		}
		this.state = next;
		this.render();
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
		details.addEventListener("toggle", () => {
			if (details.open) this.expanded.add(fact.seq);
			else this.expanded.delete(fact.seq);
		});
		const summary = el("summary", "");
		summary.append(el("span", "seq", `#${fact.seq}`));
		summary.append(el("span", "badge", factTypeLabels[fact.type] || fact.type || "未知"));
		if (fact.role) summary.append(el("span", "badge", roleLabel(fact.role)));
		const preview = this.previewOf(fact);
		if (preview) summary.append(el("span", "preview", preview));
		details.append(summary);
		const pre = el("pre", "");
		pre.textContent = JSON.stringify(fact, null, 2);
		details.append(pre);
		return details;
	}

	previewOf(fact) {
		const text = String(fact.content || "").replace(/\s+/g, " ").trim();
		if (text) return text.length > 60 ? `${text.slice(0, 60)}…` : text;
		if (fact.tool_call_id) return `tool_call_id: ${fact.tool_call_id}`;
		const meta = fact.meta;
		if (meta && typeof meta === "object") {
			const parts = ["turn_id", "step_id", "reason", "step"]
				.filter((key) => meta[key] !== undefined && meta[key] !== "")
				.map((key) => `${key}=${meta[key]}`);
			if (parts.length > 0) return parts.join(" ");
		}
		return "";
	}

	render() {
		if (!this.list) return;
		const all = this.state.facts || [];
		const facts = this.visibleFacts();
		this.head.textContent = facts.length === all.length
			? `共 ${all.length} 条记录`
			: `共 ${all.length} 条记录，筛选出 ${facts.length} 条`;
		if (facts.length === 0) {
			this.rows.clear();
			this.list.replaceChildren(el("li", "empty", all.length === 0 ? "暂无记录" : "没有匹配的记录"));
			return;
		}
		const shown = facts.slice(0, this.limit);

		// Reuse the node of an unchanged fact. Re-parenting keeps <details>.open,
		// so an expanded record survives the pushes that arrive during a turn.
		const nodes = [];
		const live = new Set();
		for (const fact of shown) {
			live.add(fact.seq);
			const json = JSON.stringify(fact);
			const row = this.rows.get(fact.seq);
			if (row && row.json === json) {
				nodes.push(row.node);
				continue;
			}
			const node = this.factNode(fact);
			this.rows.set(fact.seq, { json, node });
			nodes.push(node);
		}
		for (const seq of this.rows.keys()) {
			if (!live.has(seq)) this.rows.delete(seq);
		}

		if (facts.length > shown.length) {
			const more = el("button", "trace-more", `显示更多（还有 ${facts.length - shown.length} 条）`);
			more.type = "button";
			more.addEventListener("click", () => {
				this.limit += TRACE_PAGE_SIZE;
				this.render();
			});
			const item = el("li", "");
			item.append(more);
			nodes.push(item);
		}
		this.list.replaceChildren(...nodes);
	}
}

customElements.define("session-history", SessionHistory);
customElements.define("session-chat", SessionChat);
customElements.define("session-trace", SessionTrace);
