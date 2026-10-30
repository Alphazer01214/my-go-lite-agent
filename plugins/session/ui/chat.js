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
	clearError,
	reportError,
	workspaceLabel,
} from "./store.js";

function shouldRenderTurn(turn, messages) {
	if (!turn) return false;
	if (turn.status !== "done") return true;
	const text = String(turn.assistantText || "").trim();
	if (!text) return true;
	return !messages.some((message) =>
		message?.role === "assistant" && String(message.text || "").trim() === text);
}

function messageText(message) {
	const text = String(message?.text || "");
	if (text) return text;
	if (message?.tool_calls?.length) return `[tool calls] ${message.tool_calls.length}`;
	return "(空)";
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
		this.draftKey = "";
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

		const root = el("div", { class: "chat" },
			el("div", { class: "head" },
				el("div", { class: "head-main" }, this.headTitle, this.headMeta),
			),
			banner,
			el("div", { class: "stage" }, this.scroll, this.jump),
			el("div", { class: "composer" },
				el("div", { class: "card" },
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
		const key = state.sessionId || "new";
		let keyChanged = false;
		if (this.inputKey !== key) {
			if (this.inputKey !== null) this.drafts.set(this.inputKey, this.textarea.value);
			this.inputKey = key;
			this.textarea.value = this.drafts.get(key) || "";
			this.autosize();
			keyChanged = true;
		}

		const meta = state.meta;
		this.headTitle.textContent = String(meta?.title || "").trim() || "新会话";
		const workspace = workspaceLabel(meta?.workspace || state.workspace);
		const count = state.messages.length;
		this.headMeta.textContent = state.sessionId
			? `${workspace} · ${count} 条消息${isBusy() ? " · 生成中" : ""}`
			: `工作区：${workspace}`;
		this.hint.textContent = workspace;

		if (state.error) {
			this.banner.hidden = false;
			this.bannerText.textContent = state.error;
		} else {
			this.banner.hidden = true;
			this.bannerText.textContent = "";
		}

		this.renderMessages();
		this.updateButtons();
		if (keyChanged || this.pinned) this.scrollToBottom();
		else this.updateJump();
	}

	renderMessages() {
		const turn = currentTurn();
		const messages = state.messages;
		this.column.replaceChildren();
		this.draftBody = null;
		this.draftText = null;
		this.draftKey = "";

		if (messages.length === 0 && !turn) {
			this.appendEmpty();
			return;
		}

		for (const message of messages) {
			this.appendMessage(message);
		}
		if (turn && !turn.userPersisted) {
			this.appendUserRow(turn.userText);
		}
		if (turn && shouldRenderTurn(turn, messages)) {
			this.appendDraft(turn);
		}
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
			body.append(renderMarkdown(text));
			this.column.append(el("div", { class: "msg assistant" }, body));
			return;
		}
		this.column.append(el("div", { class: "msg other" },
			el("div", { class: "body", text: `${role.toUpperCase()} · ${text}` }),
		));
	}

	appendDraft(turn) {
		const body = el("div", { class: "body" });
		this.draftText = document.createTextNode(turn.assistantText || "");
		body.append(this.draftText);
		if (turn.status === "streaming" || turn.status === "stopping") {
			body.append(el("span", { class: "caret" }));
		}
		const status = turn.status === "streaming"
			? "正在生成…"
			: turn.status === "stopping"
				? "正在停止…"
				: turn.status === "stopped"
					? "已停止 · 本轮内容未保存"
					: turn.status === "done"
						? "已完成"
						: turn.error || "本轮失败";
		this.column.append(el("div", { class: `msg assistant draft ${turn.status}` },
			body,
			el("div", { class: "draft-status", text: status }),
		));
		this.draftBody = body;
		this.draftKey = turn.key;
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
		sendTurn(text);
	}

	updateButtons() {
		const busy = isBusy();
		const turn = currentTurn();
		this.sendButton.hidden = busy;
		this.stopButton.hidden = !busy;
		this.sendButton.disabled = !this.textarea.value.trim() || busy;
		this.stopButton.disabled = busy && !turn?.sessionId;
		this.stopButton.title = turn?.sessionId ? "停止生成" : "会话建立后可停止";
	}

	onChunk(detail) {
		const turn = currentTurn();
		if (!turn || turn.key !== detail.key) return;
		if (!this.draftBody || this.draftKey !== detail.key) {
			this.render();
			return;
		}
		if (this.draftText) {
			this.draftText.appendData(detail.delta);
		} else {
			this.draftText = document.createTextNode(detail.delta);
			this.draftBody.prepend(this.draftText);
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
