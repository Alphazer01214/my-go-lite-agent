import {
	callCapability,
	callGit,
	clear,
	errorMessage,
	node,
} from "./client.js";

const ACTIVE_SESSION_EVENT = "liteagent:session-changed";
const DIFF_LINE_LIMIT = 2000;

const GIT_CSS = `
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
}
.head {
	flex: none;
	display: flex;
	align-items: center;
	gap: 6px;
	padding: 10px 12px 8px;
	border-bottom: 1px solid var(--border, #e5e8ec);
}
.title { font-size: 13px; font-weight: 600; }
.head .spacer { flex: 1; }
.head .updated {
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	white-space: nowrap;
}
.head button {
	flex: none;
	width: 24px;
	height: 24px;
	padding: 0;
	border: 1px solid rgba(127, 127, 127, .3);
	border-radius: 6px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 13px;
	line-height: 1;
	cursor: pointer;
}
.head button:hover:not(:disabled) { background: var(--surface-muted, #eef1f4); }
.head button:disabled { opacity: .45; cursor: default; }
.error {
	margin: 8px 10px 0;
	padding: 6px 8px;
	border: 1px solid #f0c7c3;
	border-radius: 6px;
	background: var(--danger-surface, #fff5f5);
	color: var(--danger, #b42318);
	font-size: 11px;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.error button {
	margin-top: 4px;
	padding: 1px 8px;
	border: 1px solid currentColor;
	border-radius: 5px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 10px;
	cursor: pointer;
}
.scroll {
	flex: 1 1 auto;
	min-height: 0;
	overflow: auto;
}
.body {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding: 8px 10px 14px;
}
.empty {
	padding: 26px 14px;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
	line-height: 1.6;
	text-align: center;
}
.empty-path {
	margin-top: 6px;
	color: var(--text-muted, #7a828d);
	font: 10px/1.5 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	overflow-wrap: anywhere;
	opacity: .7;
}

.branches {
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 8px;
	background: var(--surface, #fff);
	overflow: hidden;
}
.branch-head {
	display: flex;
	align-items: stretch;
}
.branch-toggle {
	flex: 1 1 auto;
	min-width: 0;
	display: flex;
	align-items: center;
	gap: 6px;
	padding: 7px 9px;
	border: 0;
	background: transparent;
	color: inherit;
	font: inherit;
	text-align: left;
	cursor: pointer;
}
.branch-toggle:hover { background: var(--surface-muted, #eef1f4); }
.branch-mark { flex: none; color: var(--accent, #4d6bfe); font-size: 13px; }
.branch-name {
	overflow: hidden;
	color: var(--text, #1f2329);
	font-size: 12px;
	font-weight: 600;
	text-overflow: ellipsis;
	white-space: nowrap;
}
.branch-upstream {
	margin-left: auto;
	max-width: 42%;
	overflow: hidden;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	text-overflow: ellipsis;
	white-space: nowrap;
}
.branch-chevron {
	flex: none;
	color: var(--text-muted, #7a828d);
	font-size: 12px;
	transition: transform .15s ease;
}
.branches.open .branch-chevron { transform: rotate(90deg); }
.branch-add {
	flex: none;
	width: 28px;
	border: 0;
	border-left: 1px solid var(--border, #e5e8ec);
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 16px;
	cursor: pointer;
}
.branch-add:hover:not(:disabled) { background: var(--surface-muted, #eef1f4); }
.branch-add:disabled { opacity: .45; cursor: default; }
.branch-tag {
	flex: none;
	padding: 0 4px;
	border-radius: 4px;
	background: var(--surface-muted, #eef1f4);
	color: var(--text-muted, #7a828d);
	font-size: 9px;
	font-weight: 400;
}
.branch-list {
	max-height: 200px;
	overflow: auto;
	border-top: 1px solid var(--border, #e5e8ec);
}
.branch-row {
	display: flex;
	align-items: center;
	padding: 1px 3px;
}
.branch-pick {
	flex: 1 1 auto;
	min-width: 0;
	display: flex;
	align-items: baseline;
	gap: 6px;
	padding: 5px 6px;
	border: 0;
	border-radius: 5px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 11px;
	text-align: left;
	cursor: pointer;
}
.branch-pick:hover:not(:disabled) { background: var(--surface-muted, #eef1f4); }
.branch-pick:disabled { opacity: .55; cursor: default; }
.branch-row.current .branch-pick {
	background: var(--accent-soft, #eef3ff);
	color: var(--accent-strong, #2f4bd8);
	font-weight: 600;
}
.branch-row.current .branch-name { color: inherit; }
.branch-pick .branch-name { font-size: 11px; }
.branch-remove {
	flex: none;
	width: 22px;
	height: 22px;
	border: 0;
	border-radius: 4px;
	background: transparent;
	color: var(--text-muted, #7a828d);
	font: inherit;
	font-size: 13px;
	cursor: pointer;
}
.branch-remove:hover { background: var(--danger-surface, #fff5f5); color: var(--danger, #b42318); }
.branch-remove:disabled { opacity: .45; cursor: default; }
.branch-confirm {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 5px;
	padding: 6px 8px;
	border-top: 1px solid var(--border, #e5e8ec);
	color: var(--danger, #b42318);
	font-size: 11px;
}
.branch-confirm button {
	padding: 1px 7px;
	border: 1px solid currentColor;
	border-radius: 5px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 10px;
	cursor: pointer;
}
.branch-confirm button.danger { background: var(--danger, #b42318); color: #fff; }
.branch-form {
	display: flex;
	gap: 4px;
	padding: 6px;
	border-top: 1px solid var(--border, #e5e8ec);
}
.branch-form input {
	flex: 1 1 auto;
	min-width: 0;
	height: 26px;
	padding: 0 7px;
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 5px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 11px;
}
.branch-form input:focus { outline: 2px solid var(--accent, #4d6bfe); outline-offset: 0; }
.branch-form button {
	padding: 2px 8px;
	border: 1px solid rgba(127, 127, 127, .3);
	border-radius: 5px;
	background: transparent;
	color: inherit;
	font: inherit;
	font-size: 11px;
	cursor: pointer;
}
.branch-form button:hover:not(:disabled) { background: var(--surface-muted, #eef1f4); }
.branch-form button:disabled { opacity: .5; cursor: default; }
.action-error {
	padding: 5px 8px;
	border-top: 1px solid #f0c7c3;
	background: var(--danger-surface, #fff5f5);
	color: var(--danger, #b42318);
	font-size: 11px;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}

.summary {
	display: flex;
	flex-wrap: wrap;
	align-items: center;
	gap: 5px;
	color: var(--text-muted, #7a828d);
	font-size: 10px;
}
.chip {
	padding: 1px 5px;
	border-radius: 999px;
	background: var(--surface-muted, #eef1f4);
	color: var(--text-muted, #7a828d);
	font-size: 10px;
	white-space: nowrap;
}
.chip.ahead { color: #2f7d32; background: #eaf5ea; }
.chip.behind { color: #a15c00; background: #fff4e0; }
.summary .add { color: #2e7d32; }
.summary .del { color: var(--danger, #b42318); }
.summary .clean { color: #2e7d32; }

.view-tabs {
	display: flex;
	gap: 2px;
	padding: 2px;
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 7px;
	background: var(--surface-muted, #eef1f4);
}
.view-tab {
	flex: 1 1 50%;
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: 4px;
	padding: 4px 6px;
	border: 0;
	border-radius: 5px;
	background: transparent;
	color: var(--text-muted, #7a828d);
	font: inherit;
	font-size: 11px;
	cursor: pointer;
}
.view-tab.active {
	background: var(--surface, #fff);
	color: var(--text, #1f2329);
	font-weight: 600;
	box-shadow: 0 1px 2px rgba(15, 23, 42, .06);
}
.view-tab .count {
	min-width: 14px;
	padding: 0 4px;
	border-radius: 999px;
	background: rgba(127, 127, 127, .12);
	font-size: 10px;
	line-height: 14px;
	font-weight: 400;
	text-align: center;
}
.files {
	display: flex;
	flex-direction: column;
	gap: 5px;
}
.file {
	border: 1px solid var(--border, #e5e8ec);
	border-radius: 7px;
	background: var(--surface, #fff);
	overflow: hidden;
}
.file-main {
	display: flex;
	align-items: center;
	gap: 6px;
	width: 100%;
	padding: 6px 8px;
	border: 0;
	background: transparent;
	color: inherit;
	font: inherit;
	text-align: left;
	cursor: pointer;
}
.file-main:hover { background: var(--surface-muted, #eef1f4); }
.file-status {
	flex: none;
	width: 18px;
	height: 18px;
	border-radius: 4px;
	font-size: 10px;
	font-weight: 700;
	line-height: 18px;
	text-align: center;
	background: var(--surface-muted, #eef1f4);
	color: var(--text-muted, #7a828d);
}
.st-M { background: #fff4e0; color: #a15c00; }
.st-A { background: #eaf5ea; color: #2e7d32; }
.st-D { background: var(--danger-surface, #fff5f5); color: var(--danger, #b42318); }
.st-R, .st-C { background: var(--accent-soft, #eef3ff); color: var(--accent-strong, #2f4bd8); }
.st-U { background: var(--danger-surface, #fff5f5); color: var(--danger, #b42318); }
.st-untracked { background: var(--surface-muted, #eef1f4); color: var(--text-muted, #7a828d); }
.file-path {
	flex: 1 1 auto;
	min-width: 0;
	overflow: hidden;
	color: var(--text, #1f2329);
	font-size: 11px;
	text-overflow: ellipsis;
	white-space: nowrap;
}
.file-stats {
	flex: none;
	display: inline-flex;
	gap: 4px;
	font: 10px/1.4 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.file-stats .add { color: #2e7d32; }
.file-stats .del { color: var(--danger, #b42318); }
.file-stats .binary { color: var(--text-muted, #7a828d); }
.file-chevron {
	flex: none;
	color: var(--text-muted, #7a828d);
	font-size: 12px;
	transition: transform .15s ease;
}
.file.open .file-chevron { transform: rotate(90deg); }
.diff-wrap {
	border-top: 1px solid var(--border, #e5e8ec);
	background: var(--surface-muted, #eef1f4);
}
.diff-note {
	padding: 8px;
	color: var(--text-muted, #7a828d);
	font-size: 11px;
	line-height: 1.5;
}
.diff-error {
	padding: 8px;
	color: var(--danger, #b42318);
	font-size: 11px;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
}
.diff {
	max-height: 340px;
	overflow: auto;
	padding: 4px 0;
	font: 10px/1.55 ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
.diff-line {
	min-width: max-content;
	padding: 0 8px;
	white-space: pre;
}
.diff-line.add { background: rgba(46, 125, 50, .1); color: #1f5f24; }
.diff-line.del { background: rgba(180, 35, 24, .08); color: #8a1c14; }
.diff-line.hunk { background: var(--accent-soft, #eef3ff); color: var(--accent-strong, #2f4bd8); }
.diff-line.meta { color: var(--text-muted, #7a828d); }
.diff-note.truncated { border-top: 1px solid var(--border, #e5e8ec); }
`;

function clockTime(value) {
	if (!value) return "";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return "";
	return date.toLocaleTimeString([], { hour12: false });
}

function statusMeta(status) {
	switch (status) {
		case "M": return ["M", "修改", "st-M"];
		case "A": return ["A", "新增", "st-A"];
		case "D": return ["D", "删除", "st-D"];
		case "R": return ["R", "重命名", "st-R"];
		case "C": return ["C", "复制", "st-C"];
		case "T": return ["T", "类型变更", "st-M"];
		case "?": return ["?", "未跟踪", "st-untracked"];
		case "U": return ["!", "冲突", "st-U"];
		default: return [status || "?", status || "变更", "st-untracked"];
	}
}

function diffLineClass(line) {
	if (line.startsWith("@@")) return "hunk";
	if (line.startsWith("+++") || line.startsWith("---") ||
		line.startsWith("diff --git") || line.startsWith("index ") ||
		line.startsWith("new file mode") || line.startsWith("deleted file mode") ||
		line.startsWith("similarity index") || line.startsWith("rename from") || line.startsWith("rename to")) {
		return "meta";
	}
	if (line.startsWith("+")) return "add";
	if (line.startsWith("-")) return "del";
	return "";
}

class GitBoard extends HTMLElement {
	static tabTitle = "Git";
	static tabOrder = 15;

	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.workspace = "";
		this.status = null;
		this.branches = null;
		this.error = "";
		this.actionError = "";
		this.loading = false;
		this.loadingPromise = null;
		this.pending = false;
		this.loadSeq = 0;
		this.view = "unstaged";
		this.branchesOpen = false;
		this.newBranchOpen = false;
		this.newBranchName = "";
		this.confirmBranch = "";
		this.busy = false;
		this.openDiff = "";
		this.diffs = new Map();
		this.diffLoading = new Set();
		this.diffErrors = new Map();
		this.updatedAt = 0;
		this.refreshTimer = 0;
		this.onSessionChanged = (event) => this.applyWorkspace(event?.detail?.workspace);
		this.onTurnSettled = () => {
			if (this.hidden) return;
			this.schedule();
		};
		this.onTabShow = () => this.load();
	}

	connectedCallback() {
		this.shadowRoot.innerHTML = `<style>${GIT_CSS}</style>
			<div class="panel">
				<header class="head">
					<span class="title">Git</span>
					<span class="spacer"></span>
					<span class="updated" data-updated></span>
					<button type="button" data-refresh title="刷新 Git 状态" aria-label="刷新 Git 状态">↻</button>
				</header>
				<div class="error" data-error hidden>
					<div data-error-text></div>
					<button type="button" data-retry>重试</button>
				</div>
				<div class="scroll" data-scroll>
					<div class="body" data-body></div>
				</div>
			</div>`;
		this.els = {
			refresh: this.shadowRoot.querySelector("[data-refresh]"),
			updated: this.shadowRoot.querySelector("[data-updated]"),
			error: this.shadowRoot.querySelector("[data-error]"),
			errorText: this.shadowRoot.querySelector("[data-error-text]"),
			retry: this.shadowRoot.querySelector("[data-retry]"),
			scroll: this.shadowRoot.querySelector("[data-scroll]"),
			body: this.shadowRoot.querySelector("[data-body]"),
		};
		this.els.refresh.addEventListener("click", () => this.load());
		this.els.retry.addEventListener("click", () => this.load());
		this.addEventListener("liteagent:tab-show", this.onTabShow);
		globalThis.addEventListener?.(ACTIVE_SESSION_EVENT, this.onSessionChanged);
		globalThis.addEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		this.load();
	}

	disconnectedCallback() {
		this.removeEventListener("liteagent:tab-show", this.onTabShow);
		globalThis.removeEventListener?.(ACTIVE_SESSION_EVENT, this.onSessionChanged);
		globalThis.removeEventListener?.("liteagent:turn-settled", this.onTurnSettled);
		clearTimeout(this.refreshTimer);
	}

	schedule(delay = 250) {
		clearTimeout(this.refreshTimer);
		this.refreshTimer = setTimeout(() => this.load(), delay);
	}

	applyWorkspace(value) {
		const next = String(value || "");
		if (next === this.workspace) return;
		this.workspace = next;
		this.resetWorkspaceState();
		if (!next) {
			this.render();
			return;
		}
		this.load();
	}

	resetWorkspaceState() {
		this.status = null;
		this.branches = null;
		this.error = "";
		this.actionError = "";
		this.view = "unstaged";
		this.branchesOpen = false;
		this.newBranchOpen = false;
		this.newBranchName = "";
		this.confirmBranch = "";
		this.busy = false;
		this.diffs.clear();
		this.diffLoading.clear();
		this.diffErrors.clear();
		this.openDiff = "";
		this.updatedAt = 0;
		this.loadSeq += 1;
	}

	async currentWorkspace() {
		try {
			const current = await callCapability("workspace", "current", {});
			return String(current?.workspace?.path || "");
		} catch {
			return "";
		}
	}

	resync() {
		return this.load();
	}

	runCommand(command) {
		if (command === "refresh" || command === "git.refresh") this.load();
	}

	async load() {
		if (!this.els) return undefined;
		if (this.loading) {
			this.pending = true;
			return this.loadingPromise;
		}
		const seq = ++this.loadSeq;
		this.loading = true;
		this.render();
		this.loadingPromise = (async () => {
			let workspace = this.workspace;
			if (!workspace) {
				workspace = await this.currentWorkspace();
				if (seq !== this.loadSeq) return;
				this.workspace = workspace;
			}
			if (!workspace) {
				this.status = null;
				this.branches = null;
				this.error = "";
				return;
			}
			const status = await callGit("status", { workspace });
			if (seq !== this.loadSeq || workspace !== this.workspace) return;
			this.status = status && typeof status === "object" ? status : null;
			if (this.status?.repo) {
				const branches = await callGit("branches", { workspace });
				if (seq !== this.loadSeq || workspace !== this.workspace) return;
				this.branches = branches && typeof branches === "object" ? branches : null;
			} else {
				this.branches = null;
			}
			this.error = "";
			this.actionError = "";
			this.updatedAt = Date.now();
			this.clearDiffs();
		})()
			.catch((error) => {
				if (seq !== this.loadSeq) return;
				this.error = errorMessage(error);
			})
			.finally(() => {
				this.loading = false;
				this.render();
				if (this.pending) {
					this.pending = false;
					this.load();
				}
			});
		return this.loadingPromise;
	}

	clearDiffs() {
		this.diffs.clear();
		this.diffLoading.clear();
		this.diffErrors.clear();
		this.openDiff = "";
	}

	render() {
		if (!this.els) return;
		const scrollTop = this.els.scroll?.scrollTop || 0;
		this.els.refresh.disabled = this.loading || this.busy;
		this.els.updated.textContent = this.loading
			? "更新中…"
			: this.updatedAt ? clockTime(this.updatedAt) : "";
		this.els.error.hidden = !this.error;
		this.els.errorText.textContent = this.error;
		clear(this.els.body);

		if (!this.workspace) {
			this.els.body.append(node("div", "empty", "选择一个工作区后查看 Git 状态。"));
			return;
		}
		if (this.loading && !this.status) {
			this.els.body.append(node("div", "empty", "加载中…"));
			return;
		}
		if (this.status && !this.status.repo) {
			const empty = node("div", "empty", "当前工作区不是 Git 仓库。");
			empty.append(node("div", "empty-path", this.workspace));
			this.els.body.append(empty);
			return;
		}
		if (!this.status) {
			this.els.body.append(node("div", "empty", "暂时没有读到 Git 状态。"));
			return;
		}
		this.renderRepo(this.els.body);
		this.els.scroll.scrollTop = scrollTop;
	}

	renderRepo(parent) {
		parent.append(this.renderBranches());
		parent.append(this.renderSummary());
		parent.append(this.renderViewTabs());
		parent.append(this.renderFiles());
	}

	renderBranches() {
		const container = node("div", `branches${this.branchesOpen ? " open" : ""}`);
		const head = node("div", "branch-head");
		const toggle = node("button", "branch-toggle");
		toggle.type = "button";
		toggle.append(node("span", "branch-mark", "⑂"));
		const current = this.status?.branch || this.branches?.current || "";
		const detached = Boolean(this.status?.detached && !this.status?.unborn);
		toggle.append(node("span", "branch-name", detached ? "detached HEAD" : (current || "未知分支")));
		if (this.status?.unborn) toggle.append(node("span", "branch-tag", "无提交"));
		if (this.status?.upstream) toggle.append(node("span", "branch-upstream", this.status.upstream));
		toggle.append(node("span", "branch-chevron", "›"));
		toggle.title = "展开或收起本地分支";
		toggle.addEventListener("click", () => {
			this.branchesOpen = !this.branchesOpen;
			this.confirmBranch = "";
			this.render();
		});
		head.append(toggle);
		const add = node("button", "branch-add", "+");
		add.type = "button";
		add.title = "新建分支";
		add.disabled = this.busy;
		add.addEventListener("click", () => {
			this.newBranchOpen = !this.newBranchOpen;
			this.confirmBranch = "";
			this.render();
		});
		head.append(add);
		container.append(head);

		if (this.branchesOpen) {
			const list = node("div", "branch-list");
			const branches = Array.isArray(this.branches?.branches) ? this.branches.branches : [];
			if (branches.length === 0) {
				list.append(node("div", "empty", "还没有本地分支。"));
			} else {
				for (const branch of branches) {
					if (this.confirmBranch === branch.name) {
						list.append(this.renderBranchConfirm(branch));
						continue;
					}
					const row = node("div", `branch-row${branch.current ? " current" : ""}`);
					const pick = node("button", "branch-pick");
					pick.type = "button";
					pick.disabled = this.busy;
					pick.append(node("span", "branch-name", branch.name));
					if (branch.current) pick.append(node("span", "branch-tag", "当前"));
					if (branch.upstream) pick.append(node("span", "branch-upstream", branch.upstream));
					pick.title = branch.subject ? `${branch.name} · ${branch.subject}` : branch.name;
					if (!branch.current) {
						pick.addEventListener("click", () => this.checkout(branch.name));
					}
					row.append(pick);
					if (!branch.current) {
						const remove = node("button", "branch-remove", "×");
						remove.type = "button";
						remove.title = `删除 ${branch.name}`;
						remove.disabled = this.busy;
						remove.addEventListener("click", () => {
							this.confirmBranch = branch.name;
							this.render();
						});
						row.append(remove);
					}
					list.append(row);
				}
			}
			container.append(list);
		}
		if (this.newBranchOpen) container.append(this.renderBranchForm());
		if (this.actionError) container.append(node("div", "action-error", this.actionError));
		return container;
	}

	renderBranchConfirm(branch) {
		const confirm = node("div", "branch-confirm");
		confirm.append(node("span", "", `删除 ${branch.name}？`));
		const remove = node("button", "danger", "删除");
		remove.type = "button";
		remove.disabled = this.busy;
		remove.addEventListener("click", () => this.deleteBranch(branch.name));
		const cancel = node("button", "", "取消");
		cancel.type = "button";
		cancel.disabled = this.busy;
		cancel.addEventListener("click", () => {
			this.confirmBranch = "";
			this.render();
		});
		confirm.append(remove, cancel);
		return confirm;
	}

	renderBranchForm() {
		const form = node("div", "branch-form");
		const input = node("input");
		input.type = "text";
		input.placeholder = "新分支名";
		input.value = this.newBranchName;
		input.dataset.branchInput = "true";
		input.disabled = this.busy;
		input.addEventListener("input", () => {
			this.newBranchName = input.value;
		});
		input.addEventListener("keydown", (event) => {
			if (event.isComposing) return;
			if (event.key === "Enter") {
				event.preventDefault();
				this.createBranch();
			} else if (event.key === "Escape") {
				event.preventDefault();
				this.newBranchOpen = false;
				this.newBranchName = "";
				this.render();
			}
		});
		const create = node("button", "", "创建");
		create.type = "button";
		create.disabled = this.busy;
		create.addEventListener("click", () => this.createBranch());
		form.append(input, create);
		queueMicrotask(() => {
			if (this.newBranchOpen && this.els?.body?.contains(form)) input.focus();
		});
		return form;
	}

	renderSummary() {
		const status = this.status;
		const staged = Array.isArray(status?.staged) ? status.staged : [];
		const unstaged = Array.isArray(status?.unstaged) ? status.unstaged : [];
		const summary = node("div", "summary");
		if (status?.ahead > 0) summary.append(node("span", "chip ahead", `↑${status.ahead}`));
		if (status?.behind > 0) summary.append(node("span", "chip behind", `↓${status.behind}`));
		const paths = new Set([...staged, ...unstaged].map((entry) => entry.path));
		if (paths.size === 0) {
			summary.append(node("span", "clean", "工作区干净"));
			return summary;
		}
		const insertions = [...staged, ...unstaged].reduce((sum, entry) => sum + (Number(entry.insertions) || 0), 0);
		const deletions = [...staged, ...unstaged].reduce((sum, entry) => sum + (Number(entry.deletions) || 0), 0);
		summary.append(node("span", "chip", `${paths.size} 个文件`));
		if (insertions > 0) summary.append(node("span", "add", `+${insertions}`));
		if (deletions > 0) summary.append(node("span", "del", `−${deletions}`));
		return summary;
	}

	renderViewTabs() {
		const tabs = node("div", "view-tabs");
		const staged = Array.isArray(this.status?.staged) ? this.status.staged.length : 0;
		const unstaged = Array.isArray(this.status?.unstaged) ? this.status.unstaged.length : 0;
		for (const [value, label, count] of [["unstaged", "工作区", unstaged], ["staged", "暂存", staged]]) {
			const button = node("button", `view-tab${this.view === value ? " active" : ""}`);
			button.type = "button";
			button.append(node("span", "", label), node("span", "count", String(count)));
			button.addEventListener("click", () => {
				if (this.view === value) return;
				this.view = value;
				this.openDiff = "";
				this.render();
			});
			tabs.append(button);
		}
		return tabs;
	}

	renderFiles() {
		const list = node("div", "files");
		const entries = Array.isArray(this.status?.[this.view]) ? this.status[this.view] : [];
		if (entries.length === 0) {
			list.append(node(
				"div",
				"empty",
				this.view === "staged" ? "暂存区没有变更。" : "工作区没有未暂存变更。",
			));
			return list;
		}
		for (const entry of entries) list.append(this.renderFile(entry));
		return list;
	}

	renderFile(entry) {
		const staged = this.view === "staged";
		const key = `${staged ? "staged" : "unstaged"}:${entry.path}`;
		const open = this.openDiff === key;
		const [label, title, className] = statusMeta(entry.status);
		const file = node("div", `file${open ? " open" : ""}`);
		const main = node("button", "file-main");
		main.type = "button";
		main.title = entry.orig_path ? `${entry.orig_path} → ${entry.path}` : entry.path;
		main.append(node("span", `file-status ${className}`, label));
		main.append(node("span", "file-path", entry.path));
		if (Number(entry.insertions) > 0 || Number(entry.deletions) > 0) {
			const stats = node("span", "file-stats");
			if (Number(entry.insertions) > 0) stats.append(node("span", "add", `+${entry.insertions}`));
			if (Number(entry.deletions) > 0) stats.append(node("span", "del", `−${entry.deletions}`));
			main.append(stats);
		} else if (entry.binary) {
			main.append(node("span", "file-stats binary", "BIN"));
		}
		main.append(node("span", "file-chevron", "›"));
		main.setAttribute("aria-expanded", String(open));
		main.addEventListener("click", () => this.toggleDiff(entry, staged, key));
		file.append(main);
		if (open) file.append(this.renderDiff(entry, staged, key));
		return file;
	}

	toggleDiff(entry, staged, key) {
		if (this.openDiff === key) {
			this.openDiff = "";
			this.render();
			return;
		}
		this.openDiff = key;
		this.render();
		if (!entry.binary && !this.diffs.has(key) && !this.diffLoading.has(key)) {
			void this.fetchDiff(entry, staged, key);
		}
	}

	async fetchDiff(entry, staged, key) {
		const workspace = this.workspace;
		this.diffLoading.add(key);
		this.render();
		try {
			const diff = await callGit("diff", {
				workspace,
				path: entry.path,
				orig_path: entry.orig_path || "",
				staged,
				untracked: entry.status === "?",
			});
			if (workspace !== this.workspace) return;
			this.diffs.set(key, diff && typeof diff === "object" ? diff : null);
		} catch (error) {
			if (workspace !== this.workspace) return;
			this.diffErrors.set(key, errorMessage(error));
		} finally {
			this.diffLoading.delete(key);
			if (workspace === this.workspace) this.render();
		}
	}

	renderDiff(entry, staged, key) {
		const wrap = node("div", "diff-wrap");
		if (entry.binary) {
			wrap.append(node("div", "diff-note", "二进制文件，无法显示文本 diff。"));
			return wrap;
		}
		if (this.diffLoading.has(key)) {
			wrap.append(node("div", "diff-note", "加载中…"));
			return wrap;
		}
		const error = this.diffErrors.get(key);
		if (error) {
			wrap.append(node("div", "diff-error", error));
			return wrap;
		}
		const diff = this.diffs.get(key);
		if (!diff) {
			wrap.append(node("div", "diff-note", "还没有读到 diff。"));
			return wrap;
		}
		if (diff.untracked || entry.status === "?") {
			wrap.append(node("div", "diff-note", "未跟踪文件没有可比较的 diff。"));
			return wrap;
		}
		const text = String(diff.diff || "");
		if (!text.trim()) {
			wrap.append(node("div", "diff-note", "没有文本差异。"));
			return wrap;
		}
		const lines = text.split(/\r?\n/);
		const shown = lines.slice(0, DIFF_LINE_LIMIT);
		const box = node("div", "diff");
		for (const line of shown) {
			const value = line === "" ? " " : line;
			box.append(node("div", `diff-line ${diffLineClass(line)}`.trim(), value));
		}
		wrap.append(box);
		if (diff.truncated) {
			wrap.append(node("div", "diff-note truncated", "diff 已截断。"));
		}
		if (lines.length > DIFF_LINE_LIMIT) {
			wrap.append(node("div", "diff-note truncated", `仅显示前 ${DIFF_LINE_LIMIT} 行。`));
		}
		return wrap;
	}

	async checkout(name) {
		if (this.busy || !name) return;
		this.busy = true;
		this.actionError = "";
		this.render();
		try {
			await callGit("checkout", { workspace: this.workspace, branch: name });
			this.branchesOpen = false;
			this.confirmBranch = "";
			await this.load();
		} catch (error) {
			this.actionError = errorMessage(error);
		} finally {
			this.busy = false;
			this.render();
		}
	}

	async createBranch() {
		const name = String(this.newBranchName || "").trim();
		if (this.busy || !name) return;
		this.busy = true;
		this.actionError = "";
		this.render();
		try {
			await callGit("branch.create", { workspace: this.workspace, name });
			this.newBranchOpen = false;
			this.newBranchName = "";
			this.branchesOpen = false;
			await this.load();
		} catch (error) {
			this.actionError = errorMessage(error);
		} finally {
			this.busy = false;
			this.render();
		}
	}

	async deleteBranch(name) {
		if (this.busy || !name) return;
		this.busy = true;
		this.actionError = "";
		this.render();
		try {
			await callGit("branch.delete", { workspace: this.workspace, name });
			this.confirmBranch = "";
			await this.load();
		} catch (error) {
			this.actionError = errorMessage(error);
		} finally {
			this.busy = false;
			this.render();
		}
	}
}

customElements.define("git-tool-board", GitBoard);
