import { el } from "./dom.js";
import { icon } from "./icons.js";
import { renderMarkdown } from "./markdown.js";
import { CHAT_CSS } from "./styles.js";
import {
	state,
	on,
	init,
	resync,
	sendTurn,
	stopTurn,
	currentTurn,
	isBusy,
	isSessionRunning,
	setSelectedModel,
	clearError,
	reportError,
	workspaceLabel,
	newChat,
	activateTab,
	closeTab,
	tabTitle,
	friendlyErrorMessage,
} from "./store.js";

function shouldRenderTurn(turn, messages) {
	if (!turn) return false;
	if (turn.status !== "done") return true;
	const text = String(turn.assistantText || "").trim();
	const reasoning = String(turn.assistantReasoning || "").trim();
	if (!text && !reasoning) return true;
	return !messages.some((message) =>
		message?.role === "assistant"
		&& String(message.text || "").trim() === text
		&& String(message.reasoning || "").trim() === reasoning);
}

function factMeta(fact) {
	let meta = fact?.meta;
	if (typeof meta === "string") {
		try {
			meta = JSON.parse(meta);
		} catch {
			meta = null;
		}
	}
	return meta && typeof meta === "object" ? meta : {};
}

function turnEndStatus(fact) {
	if (fact?.type !== "turn_end") return "";
	const status = String(factMeta(fact).status || "");
	return status === "error" || status === "cancelled" ? status : "";
}

function buildTimeline(messages, facts) {
	const ends = (Array.isArray(facts) ? facts : [])
		.filter((fact) => fact?.type === "turn_end")
		.sort((a, b) => Number(a.seq) - Number(b.seq));
	const ordered = [...(Array.isArray(messages) ? messages : [])]
		.sort((a, b) => Number(a.seq) - Number(b.seq));
	const items = [];
	let lastUser = "";
	let endIndex = 0;
	const flush = (beforeSeq) => {
		while (endIndex < ends.length && Number(ends[endIndex].seq) < beforeSeq) {
			const fact = ends[endIndex++];
			items.push({ kind: "turn_end", fact, status: turnEndStatus(fact), retryText: lastUser });
			lastUser = "";
		}
	};
	for (const message of ordered) {
		flush(Number(message.seq));
		items.push({ kind: "message", message });
		if (message?.role === "user") lastUser = String(message.text || "");
	}
	flush(Infinity);
	return items;
}

const MESSAGE_WINDOW = 80;
const TOOL_PREVIEW_LIMIT = 8 * 1024;
const REASONING_RENDER_LIMIT = 32 * 1024;
const REASONING_DOM_LIMIT = 32 * 1024;
const REASONING_HINT_AFTER_MS = 120_000;

function reasoningSummaryText(length, live = false) {
	const count = Number(length) || 0;
	const suffix = count > 0 ? ` · ${count.toLocaleString()} 字符` : "";
	return `思考过程${suffix}${live ? " · 生成中" : ""}`;
}

function renderReasoningBody(body, text) {
	if (text.length <= REASONING_RENDER_LIMIT) {
		body.replaceChildren(renderMarkdown(text));
		return;
	}
	body.replaceChildren(renderMarkdown(text.slice(0, REASONING_RENDER_LIMIT)));
	const omitted = text.length - REASONING_RENDER_LIMIT;
	const note = el("div", {
		class: "more-note",
		text: `已省略 ${omitted.toLocaleString()} 字符，共 ${text.length.toLocaleString()} 字符`,
	});
	const button = el("button", {
		class: "reasoning-full",
		type: "button",
		text: "展开全部",
	});
	button.addEventListener("click", () => {
		note.remove();
		button.remove();
		body.replaceChildren(renderMarkdown(text));
	});
	body.append(note, button);
}

function formatBytes(size) {
	const value = Number(size) || 0;
	if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
	if (value >= 1024) return `${Math.round(value / 1024)} KB`;
	return `${value} B`;
}

function appendOtherBody(role, text) {
	const body = el("div", { class: "body" });
	const prefix = `${role.toUpperCase()} · `;
	if (text.length <= TOOL_PREVIEW_LIMIT) {
		body.textContent = prefix + text;
		return body;
	}
	let expanded = false;
	const content = document.createTextNode(prefix + text.slice(0, TOOL_PREVIEW_LIMIT));
	const omitted = text.length - TOOL_PREVIEW_LIMIT;
	const note = el("div", {
		class: "more-note",
		text: `已省略 ${omitted.toLocaleString()} 字符，共 ${formatBytes(text.length)}`,
	});
	const toggle = el("button", {
		class: "load-full",
		type: "button",
		text: `展开全文（${formatBytes(text.length)}）`,
		onclick: () => {
			expanded = !expanded;
			content.data = prefix + (expanded ? text : text.slice(0, TOOL_PREVIEW_LIMIT));
			note.hidden = expanded;
			toggle.textContent = expanded ? "收起" : `展开全文（${formatBytes(text.length)}）`;
		},
	});
	body.append(content, note, toggle);
	return body;
}

function messageText(message) {
	const text = String(message?.text || "");
	if (text) return text;
	if (message?.tool_calls?.length) return `[tool calls] ${message.tool_calls.length}`;
	return "(空)";
}

function reasoningBlock(reasoning, open = false) {
	const text = String(reasoning);
	const body = el("div", { class: "reasoning-body" });
	const details = el("details", { class: "reasoning", open },
		el("summary", { text: reasoningSummaryText(text.length) }),
		body,
	);
	let rendered = false;
	const ensure = () => {
		if (rendered) return;
		rendered = true;
		renderReasoningBody(body, text);
	};
	details.addEventListener("toggle", () => {
		if (details.open) ensure();
	});
	if (open) ensure();
	return details;
}

export class SessionChat extends HTMLElement {
	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.shadowRoot.append(el("style", { text: CHAT_CSS }));
		this.off = [];
		this.pinned = true;
		this.drafts = new Map();
		this.inputKey = null;
		this.draftBody = null;
		this.draftText = null;
		this.draftReasoningText = null;
		this.reasoningDetails = null;
		this.reasoningSummary = null;
		this.reasoningDomTruncated = false;
		this.reasoningUserToggled = false;
		this.lastReasoningSummaryAt = 0;
		this.draftStatusEl = null;
		this.tickTimer = 0;
		this.draftKey = "";
		this.lastActiveTabKey = null;
		this.messageWindow = MESSAGE_WINDOW;
		this.build();
	}

	build() {
		this.headTitle = el("div", { class: "head-title", text: "新会话" });
		this.headMeta = el("div", { class: "head-meta", text: "" });
		this.bannerText = el("span", { text: "" });

		this.column = el("div", { class: "column" });
		this.column.addEventListener("click", (event) => this.handleColumnClick(event));
		this.scroll = el("div", { class: "scroll" }, this.column);
		this.scroll.addEventListener("scroll", () => this.handleScroll());
		this.jump = el("button", {
			class: "jump",
			type: "button",
			title: "回到最新",
			hidden: true,
			onclick: () => this.scrollToBottom(),
		}, icon("arrowDown", 16));

		this.textarea = el("textarea", {
			placeholder: "给 agent 发送消息…",
			rows: 1,
			attrs: { "aria-label": "消息输入框" },
		});
		this.textarea.addEventListener("input", () => {
			this.autosize();
			this.saveDraft(this.textarea.value);
			this.updateButtons();
		});
		this.textarea.addEventListener("keydown", (event) => this.handleKeydown(event));
		this.modelSelect = el("select", {
			class: "model-select",
			attrs: { "aria-label": "模型" },
			onchange: () => setSelectedModel(this.modelSelect.value),
		});

		this.sendButton = el("button", {
			class: "send",
			type: "button",
			title: "发送（Enter）",
			onclick: () => this.send(),
		}, icon("send", 16));
		this.stopButton = el("button", {
			class: "stop",
			type: "button",
			title: "停止生成",
			hidden: true,
			onclick: () => stopTurn(),
		}, icon("stop", 12));
		this.hint = el("span", { class: "hint", text: "" });

		const banner = el("div", { class: "banner", hidden: true },
			this.bannerText,
			el("button", {
				class: "icon-button",
				type: "button",
				title: "关闭",
				onclick: () => clearError(),
			}, icon("x", 12)),
		);
		this.banner = banner;

		this.tabsList = el("div", {
			class: "tabs-list",
			attrs: { role: "tablist", "aria-label": "会话标签" },
		});
		this.tabsAdd = el("button", {
			class: "tab-add",
			type: "button",
			title: "新建会话",
			attrs: { "aria-label": "新建会话" },
			onclick: () => newChat(),
		}, icon("plus", 14));
		this.tabsBar = el("div", { class: "tabs-bar", hidden: true },
			this.tabsList,
			this.tabsAdd,
		);

		const root = el("div", { class: "chat" },
			this.tabsBar,
			el("div", { class: "head" },
				el("div", { class: "head-main" }, this.headTitle, this.headMeta),
			),
			banner,
			el("div", { class: "stage" }, this.scroll, this.jump),
			el("div", { class: "composer" },
				el("div", { class: "card" },
					el("div", { class: "model-row" },
						el("span", { class: "model-label", text: "模型" }),
						this.modelSelect,
					),
					this.textarea,
					el("div", { class: "actions" },
						this.hint,
						this.stopButton,
						this.sendButton,
					),
				),
				el("div", { class: "footer", text: "Enter 发送 · Shift+Enter 换行 · 内容由模型生成" }),
			),
		);
		this.shadowRoot.append(root);
	}

	connectedCallback() {
		this.off.push(on("state", () => this.render()));
		this.off.push(on("chunk", (detail) => this.onChunk(detail)));
		this.off.push(on("progress", () => {
			this.renderHeader();
			this.renderTabs();
			this.updateTick();
		}));
		this.settingsSaved = () => resync();
		window.addEventListener("liteagent:settings-saved", this.settingsSaved);
		init();
		this.render();
	}

	disconnectedCallback() {
		if (this.settingsSaved) window.removeEventListener("liteagent:settings-saved", this.settingsSaved);
		if (this.tickTimer) {
			clearInterval(this.tickTimer);
			this.tickTimer = 0;
		}
		for (const off of this.off) off();
		this.off = [];
	}

	resync() {
		return resync();
	}

	render() {
		const key = state.sessionId || "new";
		let keyChanged = false;
		if (this.inputKey !== key) {
			if (this.inputKey !== null) this.drafts.set(this.inputKey, this.textarea.value);
			this.inputKey = key;
			this.textarea.value = this.drafts.get(key) || "";
			this.autosize();
			this.messageWindow = MESSAGE_WINDOW;
			keyChanged = true;
		}

		this.renderHeader();

		if (state.error) {
			this.banner.hidden = false;
			this.bannerText.textContent = state.error;
		} else {
			this.banner.hidden = true;
			this.bannerText.textContent = "";
		}

		this.renderTabs();
		this.renderMessages();
		this.renderModels();
		this.updateButtons();
		if (keyChanged || this.pinned) this.scrollToBottom();
		else this.updateJump();
	}

	renderHeader() {
		const meta = state.meta;
		this.headTitle.textContent = String(meta?.title || "").trim() || "新会话";
		const workspace = workspaceLabel(meta?.workspace || state.workspace);
		const count = state.messageCount || state.messages.length;
		this.headMeta.textContent = state.sessionId
			? `${workspace} · ${count} 条消息${isBusy() ? " · 生成中" : ""}`
			: `工作区：${workspace}`;
		this.hint.textContent = workspace;
	}

	renderTabs() {
		const tabs = Array.isArray(state.tabs) ? state.tabs : [];
		this.tabsBar.hidden = tabs.length === 0;
		if (tabs.length === 0) {
			this.tabsList.replaceChildren();
			this.lastActiveTabKey = null;
			return;
		}
		const activeKey = state.sessionId || "";
		this.tabsList.replaceChildren(...tabs.map((key) => this.buildTab(key, activeKey)));
		if (this.lastActiveTabKey !== activeKey) {
			this.lastActiveTabKey = activeKey;
			const active = [...this.tabsList.querySelectorAll(".session-tab")]
				.find((tab) => tab.dataset.tabKey === activeKey);
			active?.querySelector(".tab-select")?.scrollIntoView({ block: "nearest", inline: "nearest" });
		}
	}

	buildTab(key, activeKey) {
		const active = key === activeKey;
		const running = key ? isSessionRunning(key) : state.sessionId === "" && isBusy();
		const title = tabTitle(key);
		const select = el("button", {
			class: "tab-select",
			type: "button",
			title,
			attrs: { role: "tab", "aria-selected": String(active), "aria-label": title },
			onclick: () => activateTab(key),
		},
			el("span", { class: "tab-label", text: title }),
			running ? el("span", { class: "tab-running", attrs: { "aria-hidden": "true" } }) : null,
		);
		const close = el("button", {
			class: "tab-close",
			type: "button",
			title: running ? "生成中不可关闭" : "关闭标签",
			attrs: { "aria-label": running ? `${title} 生成中，不能关闭` : `关闭 ${title}` },
			disabled: running,
			onclick: (event) => {
				event.stopPropagation();
				closeTab(key);
			},
		}, icon("x", 12));
		return el("div", {
			class: `session-tab${active ? " active" : ""}${running ? " running" : ""}`,
			dataset: { tabKey: key },
		}, select, close);
	}

	renderModels() {
		const models = Array.isArray(state.models) ? state.models : [];
		const selected = String(state.meta?.model || state.selectedModel || state.defaultModel || "");
		const options = models.map((model) => ({
			value: String(model?.id || ""),
			label: String(model?.label || model?.id || ""),
		})).filter((option) => option.value);
		if (selected && !options.some((option) => option.value === selected)) {
			options.unshift({ value: selected, label: selected });
		}
		this.modelSelect.replaceChildren(...options.map((option) => {
			const node = document.createElement("option");
			node.value = option.value;
			node.textContent = option.label;
			return node;
		}));
		this.modelSelect.value = selected || options[0]?.value || "";
		this.updateButtons();
	}

	renderMessages() {
		const turn = currentTurn();
		const messages = state.messages;
		this.stopTick();
		this.column.replaceChildren();
		this.draftBody = null;
		this.draftText = null;
		this.draftReasoningText = null;
		this.reasoningDetails = null;
		this.reasoningSummary = null;
		this.reasoningDomTruncated = false;
		this.reasoningUserToggled = false;
		this.lastReasoningSummaryAt = 0;
		this.draftStatusEl = null;
		this.draftKey = "";

		if (messages.length === 0 && !turn) {
			this.appendEmpty();
			return;
		}

		const items = buildTimeline(messages, state.facts);
		const hidden = Math.max(0, items.length - this.messageWindow);
		const visibleItems = hidden ? items.slice(-this.messageWindow) : items;
		let suppressEndSeq = null;
		if (turn && turn.sessionId === state.sessionId && (turn.status === "error" || turn.status === "stopped")) {
			const terminalItems = items.filter((item) => item.kind === "turn_end" && item.status);
			const latest = terminalItems[terminalItems.length - 1];
			const lastUserSeq = [...messages].reverse().find((message) => message?.role === "user")?.seq;
			if (latest && turn.userPersisted
				&& (lastUserSeq === undefined || Number(latest.fact.seq) > Number(lastUserSeq))) {
				suppressEndSeq = Number(latest.fact.seq);
			}
		}
		if (hidden) this.column.append(this.buildLoadEarlier(hidden));
		for (const item of visibleItems) {
			if (item.kind === "message") this.appendMessage(item.message);
			else if (item.status && Number(item.fact.seq) !== suppressEndSeq) this.appendTurnStatus(item);
		}
		const userPersisted = Boolean(turn && (turn.userPersisted
			|| messages.some((message) =>
				message?.role === "user" && String(message.text || "") === String(turn.userText || ""))));
		if (turn && !userPersisted) {
			this.appendUserRow(turn.userText);
		}
		if (turn && shouldRenderTurn(turn, messages)) {
			this.appendDraft(turn);
		}
	}

	buildLoadEarlier(hidden) {
		return el("button", {
			class: "load-earlier",
			type: "button",
			text: `加载更早的 ${hidden} 条`,
			onclick: () => this.loadEarlierMessages(),
		});
	}

	loadEarlierMessages() {
		const before = this.scroll.scrollHeight;
		const top = this.scroll.scrollTop;
		this.messageWindow += MESSAGE_WINDOW;
		this.pinned = false;
		this.renderMessages();
		const after = this.scroll.scrollHeight;
		this.scroll.scrollTop = top + Math.max(0, after - before);
		this.updateJump();
	}

	appendEmpty() {
		const chips = [
			"介绍一下这个项目",
			"列出当前目录的结构",
			"总结最近一次改动",
		];
		this.column.append(el("div", { class: "empty" },
			el("h1", { text: "有什么可以帮忙的？" }),
			el("p", { text: `当前工作区：${workspaceLabel(state.meta?.workspace || state.workspace)}` }),
			el("div", { class: "chips" },
				...chips.map((prompt) => el("button", {
					class: "chip",
					type: "button",
					dataset: { prompt },
					text: prompt,
				})),
			),
		));
	}

	appendUserRow(text) {
		this.column.append(el("div", { class: "msg user" },
			el("div", { class: "bubble", text }),
		));
	}

	appendMessage(message) {
		const role = String(message?.role || "assistant");
		const text = messageText(message);
		if (role === "user") {
			this.appendUserRow(text);
			return;
		}
		if (role === "assistant") {
			const body = el("div", { class: "body" });
			if (message?.reasoning) body.append(reasoningBlock(String(message.reasoning)));
			body.append(renderMarkdown(text));
			this.column.append(el("div", { class: "msg assistant" }, body));
			return;
		}
		this.column.append(el("div", { class: "msg other" },
			appendOtherBody(role, text),
		));
	}

	appendTurnStatus(item) {
		const meta = factMeta(item.fact);
		const cancelled = item.status === "cancelled";
		const code = String(meta.error_code || "");
		const error = String(meta.error || "").trim();
		const title = cancelled ? "本轮已取消" : friendlyErrorMessage(code, "本轮执行失败");
		const note = cancelled ? "已保留部分输出" : code ? `错误码：${code}` : "";
		const details = !cancelled && error
			? el("details", { class: "turn-details" },
				el("summary", { text: "查看详情" }),
				el("div", { class: "turn-error-text", text: error }),
			)
			: null;
		const retryText = String(item.retryText || "");
		const actions = cancelled ? null : el("div", { class: "turn-status-actions" },
			el("button", {
				class: "turn-retry",
				type: "button",
				disabled: isBusy() || !retryText,
				title: retryText ? "重试本轮" : "缺少原始输入",
				onclick: () => {
					if (!retryText || isBusy()) return;
					this.pinned = true;
					this.scrollToBottom();
					sendTurn(retryText, state.meta?.model || "");
				},
			}, "重试"),
		);
		this.column.append(el("div", {
			class: `turn-status ${cancelled ? "cancelled" : "error"}`,
			attrs: { role: cancelled ? "status" : "alert" },
		},
			el("span", { class: "turn-status-mark", attrs: { "aria-hidden": "true" } },
				cancelled ? icon("stop", 12) : "!"),
			el("div", { class: "turn-status-main" },
				el("div", { class: "turn-status-title", text: title }),
				note ? el("div", { class: "turn-status-note", text: note }) : null,
				details,
				actions,
			),
		));
	}

	appendDraft(turn) {
		const body = el("div", { class: "body" });
		const reasoning = String(turn.assistantReasoning || "");
		this.reasoningDomTruncated = reasoning.length > REASONING_DOM_LIMIT;
		this.draftReasoningText = document.createTextNode(reasoning.slice(0, REASONING_DOM_LIMIT));
		const reasoningBody = el("div", { class: "reasoning-body live" }, this.draftReasoningText);
		const live = turn.status === "streaming" || turn.status === "stopping";
		this.reasoningSummary = el("summary", {
			text: reasoningSummaryText(reasoning.length, live) + (this.reasoningDomTruncated ? " · 显示已截断" : ""),
		});
		this.reasoningDetails = el("details", {
			class: "reasoning",
			hidden: !reasoning,
		},
			this.reasoningSummary,
			reasoningBody,
		);
		this.lastReasoningSummaryAt = 0;
		body.append(this.reasoningDetails);
		this.draftText = document.createTextNode(turn.assistantText || "");
		body.append(this.draftText);
		if (turn.status === "streaming" || turn.status === "stopping") {
			body.append(el("span", { class: "caret" }));
		}
		if (turn.status === "error") {
			body.append(this.buildDraftError(turn));
		}
		this.draftStatusEl = el("div", { class: "draft-status", text: this.draftStatusText(turn) });
		this.column.append(el("div", { class: `msg assistant draft ${turn.status}` },
			body,
			this.draftStatusEl,
		));
		this.draftBody = body;
		this.draftKey = turn.key;
		if (turn.status === "streaming" || turn.status === "stopping") this.startTick();
		else this.stopTick();
	}

	buildDraftError(turn) {
		const retryText = String(turn.userText || "");
		const code = String(turn.errorCode || "");
		const detail = String(turn.error || "");
		const retry = el("button", {
			class: "turn-retry",
			type: "button",
			disabled: isBusy() || !retryText,
			text: "重试",
			title: retryText ? "重试本轮" : "缺少原始输入",
			onclick: () => {
				if (!retryText || isBusy()) return;
				this.pinned = true;
				this.scrollToBottom();
				sendTurn(retryText, state.meta?.model || "");
			},
		});
		return el("div", { class: "turn-status error", attrs: { role: "alert" } },
			el("span", { class: "turn-status-mark", attrs: { "aria-hidden": "true" } }, "!"),
			el("div", { class: "turn-status-main" },
				el("div", { class: "turn-status-title", text: friendlyErrorMessage(code, "本轮执行失败") }),
				code ? el("div", { class: "turn-status-note", text: `错误码：${code}` }) : null,
				detail ? el("details", { class: "turn-details" },
					el("summary", { text: "查看详情" }),
					el("div", { class: "turn-error-text", text: detail }),
				) : null,
				el("div", { class: "turn-status-actions" }, retry),
			),
		);
	}

	formatElapsed(ms) {
		const seconds = Math.max(0, Math.floor(Number(ms || 0) / 1000));
		const minutes = Math.floor(seconds / 60);
		const rest = seconds % 60;
		return minutes > 0 ? `${minutes} 分 ${rest} 秒` : `${rest} 秒`;
	}

	draftStatusText(turn) {
		if (turn.status === "streaming" || turn.status === "stopping") {
			const elapsedMs = Date.now() - Number(turn.startedAt || Date.now());
			const base = turn.status === "streaming" ? "正在生成" : "正在停止";
			const hint = !turn.assistantText && elapsedMs >= REASONING_HINT_AFTER_MS
				? " · 模型推理中，可停止后重试"
				: "";
			return `${base} · 已用时 ${this.formatElapsed(elapsedMs)}${hint}`;
		}
		if (turn.status === "stopped") return "已停止";
		if (turn.status === "done") return "已完成";
		return "本轮失败";
	}

	startTick() {
		if (this.tickTimer) return;
		this.tickTimer = setInterval(() => this.updateTick(), 1000);
	}

	stopTick() {
		if (!this.tickTimer) return;
		clearInterval(this.tickTimer);
		this.tickTimer = 0;
	}

	updateTick() {
		const turn = currentTurn();
		if (!turn || !this.draftStatusEl || this.draftKey !== turn.key) {
			this.stopTick();
			return;
		}
		this.draftStatusEl.textContent = this.draftStatusText(turn);
		this.updateReasoningSummary(turn);
		if (turn.status !== "streaming" && turn.status !== "stopping") this.stopTick();
	}

	updateReasoningSummary(turn, force = false) {
		if (!this.reasoningSummary) return;
		const now = Date.now();
		if (!force && now - this.lastReasoningSummaryAt < 500) return;
		this.lastReasoningSummaryAt = now;
		const live = turn.status === "streaming" || turn.status === "stopping";
		const suffix = this.reasoningDomTruncated ? " · 显示已截断" : "";
		this.reasoningSummary.textContent =
			reasoningSummaryText(String(turn.assistantReasoning || "").length, live) + suffix;
	}

	handleColumnClick(event) {
		const target = event.target;
		if (!(target instanceof Element)) return;
		const chip = target.closest("[data-prompt]");
		if (!chip) return;
		this.textarea.value = String(chip.dataset.prompt || "");
		this.saveDraft(this.textarea.value);
		this.autosize();
		this.updateButtons();
		this.textarea.focus();
	}

	handleScroll() {
		const distance = this.scroll.scrollHeight - this.scroll.scrollTop - this.scroll.clientHeight;
		this.pinned = distance < 80;
		this.updateJump();
	}

	handleKeydown(event) {
		if (event.key !== "Enter" || event.shiftKey) return;
		if (event.isComposing || event.keyCode === 229) return;
		event.preventDefault();
		this.send();
	}

	send() {
		const text = this.textarea.value.trim();
		if (!text || isBusy()) return;
		if (!state.workspace && !state.meta?.workspace) {
			reportError("发送", new Error("请先选择工作区"));
			return;
		}
		this.textarea.value = "";
		this.saveDraft("");
		this.autosize();
		this.pinned = true;
		this.updateButtons();
		sendTurn(text, this.modelSelect.value);
	}

	updateButtons() {
		const busy = isBusy();
		const turn = currentTurn();
		this.sendButton.hidden = busy;
		this.stopButton.hidden = !busy;
		this.sendButton.disabled = !this.textarea.value.trim() || busy;
		this.stopButton.disabled = busy && !turn?.sessionId;
		this.stopButton.title = turn?.sessionId ? "停止生成" : "会话建立后可停止";
		const lockedModel = Boolean(state.sessionId && state.meta?.model);
		this.modelSelect.disabled = busy || lockedModel || state.loading.models || this.modelSelect.options.length === 0;
	}

	onChunk(detail) {
		const turn = currentTurn();
		if (!turn || turn.key !== detail.key) return;
		if (!this.draftBody || this.draftKey !== detail.key) {
			this.render();
			return;
		}
		if (detail.channel === "thinking") {
			const currentLength = this.draftReasoningText?.length || 0;
			if (currentLength >= REASONING_DOM_LIMIT) {
				this.reasoningDomTruncated = true;
			} else {
				const remaining = REASONING_DOM_LIMIT - currentLength;
				const slice = detail.delta.length > remaining ? detail.delta.slice(0, remaining) : detail.delta;
				if (!this.draftReasoningText) {
					this.draftReasoningText = document.createTextNode(slice);
					this.reasoningDetails?.querySelector(".reasoning-body")?.append(this.draftReasoningText);
				} else {
					this.draftReasoningText.appendData(slice);
				}
				if (detail.delta.length > remaining) this.reasoningDomTruncated = true;
			}
			if (this.reasoningDetails) this.reasoningDetails.hidden = false;
			this.updateReasoningSummary(turn);
		} else {
			if (this.draftText) {
				this.draftText.appendData(detail.delta);
			} else {
				this.draftText = document.createTextNode(detail.delta);
				this.draftBody.append(this.draftText);
			}
		}
		if (this.pinned) this.scrollToBottom();
	}

	scrollToBottom() {
		this.pinned = true;
		this.scroll.scrollTop = this.scroll.scrollHeight;
		this.updateJump();
	}

	updateJump() {
		const hasContent = state.messages.length > 0 || !!currentTurn();
		this.jump.hidden = this.pinned || !hasContent;
	}

	saveDraft(value) {
		this.drafts.set(this.inputKey || "new", value);
	}

	autosize() {
		this.textarea.style.height = "auto";
		this.textarea.style.height = `${Math.min(this.textarea.scrollHeight || 26, 200)}px`;
	}
}

customElements.define("session-chat", SessionChat);
