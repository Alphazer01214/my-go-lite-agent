import { el, relativeTime, dayBucket, shortPath } from "./dom.js";
import { icon } from "./icons.js";
import { HISTORY_CSS } from "./styles.js";
import {
	state,
	on,
	init,
	resync,
	selectSession,
	newChat,
	selectWorkspace,
	addWorkspace,
	renameSession,
	togglePin,
	deleteSession,
	isSessionRunning,
	clearError,
	notify,
} from "./store.js";

function sessionTitle(session) {
	return String(session?.title || "").trim() || "未命名会话";
}

function sessionTimestamp(session) {
	return Number(session?.last_ts || session?.created_at || 0);
}

function sessionMeta(session, childCount = 0) {
	const count = Number(session?.fact_count) || 0;
	const parts = [count > 0 ? `${count} 条记录` : "空会话"];
	if (childCount > 0) parts.push(`${childCount} 个子会话`);
	const time = relativeTime(sessionTimestamp(session));
	if (time) parts.push(time);
	return parts.join(" · ");
}

function buildTree(sessions) {
	const byId = new Map();
	for (const session of sessions) byId.set(session.session_id, session);
	const childrenOf = new Map();
	const roots = [];
	for (const session of sessions) {
		const parentId = String(session.parent_session_id || "");
		const parent = parentId ? byId.get(parentId) : null;
		if (parent && parent.session_id !== session.session_id) {
			const kids = childrenOf.get(parent.session_id);
			if (kids) kids.push(session);
			else childrenOf.set(parent.session_id, [session]);
		} else {
			roots.push(session);
		}
	}
	// 子会话按创建时间正序排列，父会话的下级按委派顺序阅读。
	for (const kids of childrenOf.values()) {
		kids.sort((a, b) => (Number(a.created_at) || 0) - (Number(b.created_at) || 0)
			|| String(a.session_id).localeCompare(String(b.session_id)));
	}
	return { roots, childrenOf };
}

// 搜索命中的会话保留其祖先路径，并展开命中节点的整棵子树。
function searchVisible(sessions, childrenOf, query) {
	if (!query) return null;
	const byId = new Map();
	for (const session of sessions) byId.set(session.session_id, session);
	const direct = sessions.filter((session) => sessionTitle(session).toLowerCase().includes(query));
	const visible = new Set();
	for (const session of direct) {
		let cursor = session;
		while (cursor && !visible.has(cursor.session_id)) {
			visible.add(cursor.session_id);
			cursor = byId.get(cursor.parent_session_id);
		}
	}
	const stack = direct.map((session) => session.session_id);
	while (stack.length) {
		const kids = childrenOf.get(stack.pop()) || [];
		for (const kid of kids) {
			if (visible.has(kid.session_id)) continue;
			visible.add(kid.session_id);
			stack.push(kid.session_id);
		}
	}
	return visible;
}

export class SessionHistory extends HTMLElement {
	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.shadowRoot.append(el("style", { text: HISTORY_CSS }));
		this.off = [];
		this.query = "";
		this.editing = "";
		this.confirming = "";
		this.collapsed = new Set();
		this.workspaceSignature = "";
		this.listSignature = "";
		this.build();
	}

	build() {
		this.workspaceSelect = el("select", {
			class: "workspace-select",
			title: "选择工作区",
			onchange: () => selectWorkspace(this.workspaceSelect.value),
		});
		this.list = el("div", { class: "list" });
		this.list.addEventListener("click", (event) => this.handleClick(event));
		this.list.addEventListener("keydown", (event) => this.handleKey(event));
		this.status = el("div", { class: "status", hidden: true });
		this.searchInput = el("input", {
			type: "search",
			placeholder: "搜索会话",
			oninput: () => {
				this.query = this.searchInput.value.trim();
				this.renderList();
			},
		});

		const root = el("div", { class: "history" },
			el("div", { class: "head" },
				el("div", { class: "eyebrow" },
					el("span", { text: "会话" }),
					this.countBadge = el("span", { class: "count", text: "0" }),
				),
				el("button", { class: "new-chat", type: "button", onclick: () => newChat() },
					icon("plus", 15),
					el("span", { text: "新建会话" }),
				),
				el("div", { class: "workspace-row" },
					this.workspaceSelect,
					el("button", {
						class: "icon-button",
						type: "button",
						title: "添加工作区",
						onclick: () => addWorkspace(),
					}, icon("folder", 16)),
				),
			),
			el("div", { class: "search" }, icon("search", 14), this.searchInput),
			this.list,
			this.status,
		);
		this.shadowRoot.append(root);
	}

	connectedCallback() {
		this.off.push(on("state", () => this.render()));
		this.off.push(on("progress", () => this.renderList()));
		init();
		this.render();
	}

	disconnectedCallback() {
		for (const off of this.off) off();
		this.off = [];
	}

	resync() {
		return resync();
	}

	render() {
		this.renderWorkspace();
		this.renderStatus();
		this.renderList();
	}

	renderWorkspace() {
		const signature = state.workspaces
			.map((workspace) => `${workspace.path}\u0000${workspace.name || ""}`)
			.join("\u0001") + `\u0002${state.workspace}`;
		if (signature !== this.workspaceSignature) {
			this.workspaceSignature = signature;
			const options = state.workspaces.map((workspace) => el("option", {
				value: workspace.path,
				text: workspace.name || shortPath(workspace.path) || workspace.path,
				title: workspace.path,
			}));
			if (options.length === 0) {
				options.push(el("option", { value: "", text: "未选择工作区" }));
			}
			this.workspaceSelect.replaceChildren(...options);
			this.workspaceSelect.disabled = state.workspaces.length === 0;
		}
		if (this.workspaceSelect.value !== state.workspace) {
			this.workspaceSelect.value = state.workspace;
		}
	}

	renderStatus() {
		const message = state.error || state.notice;
		if (!message) {
			this.status.hidden = true;
			this.status.replaceChildren();
			return;
		}
		this.status.hidden = false;
		this.status.className = `status ${state.error ? "error" : "notice"}`;
		this.status.replaceChildren(
			el("span", { text: message }),
			el("button", {
				class: "icon-button",
				type: "button",
				title: "关闭",
				onclick: () => {
					if (state.error) clearError();
					else notify("");
				},
			}, icon("x", 12)),
		);
	}

	renderList() {
		this.countBadge.textContent = String(state.sessions.length);
		const signature = JSON.stringify({
			query: this.query,
			editing: this.editing,
			confirming: this.confirming,
			active: state.sessionId,
			loading: state.loading.sessions && state.sessions.length === 0,
			collapsed: [...this.collapsed].sort(),
			sessions: state.sessions.map((session) => [
				session.session_id,
				session.parent_session_id || "",
				session.origin || "",
				session.title || "",
				!!session.pinned,
				Number(session.fact_count) || 0,
				Number(session.last_ts || session.created_at) || 0,
				isSessionRunning(session.session_id),
			]),
		});
		if (signature === this.listSignature) return;
		this.listSignature = signature;
		this.list.replaceChildren();

		const query = this.query.toLowerCase();
		const { roots, childrenOf } = buildTree(state.sessions);
		const visible = searchVisible(state.sessions, childrenOf, query);
		const shown = visible ? roots.filter((root) => visible.has(root.session_id)) : roots;

		if (shown.length === 0) {
			const text = state.loading.sessions && state.sessions.length === 0
				? "加载中…"
				: state.sessions.length === 0
					? "还没有会话\n点击上方「新建会话」开始。"
					: "没有匹配的会话。";
			this.list.append(el("div", { class: "empty-list", text }));
			return;
		}

		const ctx = { childrenOf, visible, query };
		const pinned = shown.filter((session) => session.pinned);
		if (pinned.length) {
			this.appendGroup("置顶", pinned, ctx);
		}
		const buckets = new Map();
		for (const session of shown) {
			if (session.pinned) continue;
			const bucket = dayBucket(sessionTimestamp(session));
			if (!buckets.has(bucket)) buckets.set(bucket, []);
			buckets.get(bucket).push(session);
		}
		for (const label of ["今天", "昨天", "7 天内", "更早"]) {
			const items = buckets.get(label);
			if (items?.length) this.appendGroup(label, items, ctx);
		}
	}

	appendGroup(label, sessions, ctx) {
		const group = el("div", { class: "group" },
			el("div", { class: "group-title", text: label }),
		);
		for (const session of sessions) group.append(this.renderNode(session, ctx));
		this.list.append(group);
	}

	renderNode(session, ctx) {
		const id = session.session_id;
		const total = ctx.childrenOf.get(id) || [];
		let kids = total;
		if (ctx.visible) kids = kids.filter((kid) => ctx.visible.has(kid.session_id));
		const open = Boolean(ctx.query) || !this.collapsed.has(id);
		const node = el("div", { class: "node" });
		node.append(this.renderRow(session, {
			childCount: total.length,
			toggle: !ctx.query && kids.length > 0,
			open,
		}));
		if (kids.length && open) {
			const box = el("div", { class: "children" });
			for (const kid of kids) box.append(this.renderNode(kid, ctx));
			node.append(box);
		}
		return node;
	}

	renderRow(session, tree = {}) {
		const id = session.session_id;
		const active = state.sessionId === id;
		const running = isSessionRunning(id);
		const row = el("div", {
			class: `row${active ? " active" : ""}${this.confirming === id ? " confirming" : ""}`,
			dataset: { id },
			role: "button",
			tabIndex: 0,
		});

		if (this.editing === id) {
			const input = el("input", {
				class: "rename-input",
				value: sessionTitle(session),
				attrs: { "aria-label": "重命名会话" },
			});
			row.append(el("div", { class: "row-main" }, input));
			const commit = () => {
				if (this.editing !== id) return;
				const value = input.value;
				this.editing = "";
				this.renderList();
				renameSession(id, value);
			};
			input.addEventListener("keydown", (event) => {
				if (event.isComposing) return;
				if (event.key === "Enter") {
					event.preventDefault();
					commit();
				} else if (event.key === "Escape") {
					event.preventDefault();
					this.editing = "";
					this.renderList();
				}
			});
			input.addEventListener("blur", () => {
				if (this.editing === id) {
					this.editing = "";
					this.renderList();
				}
			});
			queueMicrotask(() => {
				input.focus();
				input.select();
			});
			return row;
		}

		row.append(
			el("div", { class: "row-main" },
				tree.toggle
					? el("button", {
						class: `toggle${tree.open ? " open" : ""}`,
						type: "button",
						title: tree.open ? "收起子会话" : "展开子会话",
						dataset: { action: "toggle", id },
						attrs: { "aria-expanded": tree.open ? "true" : "false" },
					}, icon("chevron", 12))
					: null,
				running ? el("span", { class: "running-dot", title: "正在生成" }) : null,
				session.origin === "subagent"
					? el("span", {
						class: "origin-badge",
						text: "子代理",
						title: session.parent_session_id
							? `父会话：${session.parent_session_id}`
							: "Subagent session",
					})
					: null,
				el("span", { class: "row-title", text: sessionTitle(session), title: sessionTitle(session) }),
			),
			el("div", { class: "row-meta", text: sessionMeta(session, tree.childCount) }),
		);

		const actions = el("div", { class: "actions" });
		if (this.confirming === id) {
			actions.append(
				el("div", { class: "confirm" },
					el("button", { class: "danger", type: "button", dataset: { action: "confirm-delete", id }, text: "删除" }),
					el("button", { type: "button", dataset: { action: "cancel-delete", id }, text: "取消" }),
				),
			);
		} else {
			actions.append(
				session.origin === "subagent"
					? null
					: el("button", {
						class: "icon-button",
						type: "button",
						title: session.pinned ? "取消置顶" : "置顶",
						dataset: { action: "pin", id },
					}, icon("bookmark", 14)),
				el("button", {
					class: "icon-button",
					type: "button",
					title: "重命名",
					dataset: { action: "rename", id },
				}, icon("edit", 14)),
				el("button", {
					class: "icon-button",
					type: "button",
					title: running ? "生成中不可删除" : "删除会话",
					dataset: { action: "delete", id },
					disabled: running,
				}, icon("trash", 14)),
			);
		}
		row.append(actions);
		return row;
	}

	handleClick(event) {
		const target = event.target;
		if (!(target instanceof Element)) return;
		const actionNode = target.closest("[data-action]");
		if (actionNode) {
			event.stopPropagation();
			const id = actionNode.dataset.id || "";
			switch (actionNode.dataset.action) {
				case "toggle":
					if (this.collapsed.has(id)) this.collapsed.delete(id);
					else this.collapsed.add(id);
					this.renderList();
					break;
				case "pin":
					togglePin(id);
					break;
				case "rename":
					this.editing = id;
					this.confirming = "";
					this.renderList();
					break;
				case "delete":
					this.editing = "";
					this.confirming = id;
					this.renderList();
					break;
				case "cancel-delete":
					this.confirming = "";
					this.renderList();
					break;
				case "confirm-delete":
					this.confirming = "";
					this.renderList();
					deleteSession(id);
					break;
				default:
					break;
			}
			return;
		}
		const row = target.closest(".row[data-id]");
		if (!row) return;
		this.editing = "";
		this.confirming = "";
		selectSession(row.dataset.id);
	}

	handleKey(event) {
		if (event.target instanceof HTMLInputElement) return;
		const row = event.target.closest?.(".row[data-id]");
		if (!row || row !== event.target) return;
		if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			selectSession(row.dataset.id);
		}
	}
}

customElements.define("session-history", SessionHistory);
