import { el, timeOfDay } from "./dom.js";
import { icon } from "./icons.js";
import { TRACE_CSS } from "./styles.js";
import { state, on, init, resync, refreshCurrent, setFactsFilter } from "./store.js";

function matchesFilter(fact, filter) {
	const isMessage = fact?.type === "message";
	if (filter === "message") return isMessage;
	if (filter === "other") return !isMessage;
	return true;
}

function factPreview(fact) {
	if (fact?.type === "message") return String(fact.text_content || "(空)");
	if (fact?.text_content) return String(fact.text_content);
	if (fact?.meta) {
		try {
			return JSON.stringify(fact.meta);
		} catch {
			return String(fact.meta);
		}
	}
	if (fact?.tool_calls?.length) return `${fact.tool_calls.length} 个工具调用`;
	return "";
}

export class SessionTrace extends HTMLElement {
	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.shadowRoot.append(el("style", { text: TRACE_CSS }));
		this.off = [];
		this.expanded = new Set();
		this.build();
	}

	build() {
		this.countBadge = el("span", { class: "count", text: "0" });
		this.refreshButton = el("button", {
			class: "icon-button",
			type: "button",
			title: "刷新 facts",
			onclick: () => refreshCurrent(),
		}, icon("refresh", 15));

		this.filterButtons = new Map();
		const filters = el("div", { class: "filters" });
		for (const [value, label] of [["all", "全部"], ["message", "消息"], ["other", "非消息"]]) {
			const button = el("button", {
				class: "filter",
				type: "button",
				dataset: { filter: value },
				text: label,
				onclick: () => setFactsFilter(value),
			});
			this.filterButtons.set(value, button);
			filters.append(button);
		}

		this.list = el("div", { class: "list" });
		this.list.addEventListener("click", (event) => this.handleClick(event));
		this.list.addEventListener("keydown", (event) => this.handleKey(event));

		const root = el("div", { class: "trace" },
			el("div", { class: "head" },
				el("span", { class: "head-title", text: "Facts" }),
				this.countBadge,
				this.refreshButton,
			),
			filters,
			this.list,
		);
		this.shadowRoot.append(root);
	}

	connectedCallback() {
		this.off.push(on("state", () => this.render()));
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
		const facts = state.facts;
		const messages = facts.filter((fact) => fact?.type === "message").length;
		const others = facts.length - messages;
		this.countBadge.textContent = String(state.factsTotal || facts.length);
		this.refreshButton.disabled = !state.sessionId || state.loading.facts;

		const counts = { all: facts.length, message: messages, other: others };
		for (const [value, button] of this.filterButtons) {
			button.classList.toggle("active", state.factsFilter === value);
			button.textContent = `${value === "all" ? "全部" : value === "message" ? "消息" : "非消息"} ${counts[value]}`;
		}

		const scrollTop = this.list.scrollTop;
		this.list.replaceChildren();

		if (!state.sessionId) {
			this.list.append(el("div", { class: "empty", text: "选择或新建一个会话后，这里会显示它的全部 facts。" }));
			return;
		}
		if (facts.length === 0) {
			this.list.append(el("div", {
				class: `empty${state.loading.facts ? " loading" : ""}`,
				text: state.loading.facts ? "加载中…" : "当前会话还没有 facts。",
			}));
			return;
		}

		const visible = facts.filter((fact) => matchesFilter(fact, state.factsFilter));
		if (visible.length === 0) {
			this.list.append(el("div", { class: "empty", text: "这个分类下没有 facts。" }));
			return;
		}
		for (const fact of visible) this.list.append(this.renderFact(fact));
		this.list.scrollTop = scrollTop;
	}

	renderFact(fact) {
		const seq = Number(fact?.seq) || 0;
		const open = this.expanded.has(seq);
		const row = el("div", { class: `fact${fact?.type === "message" ? " message" : ""}${open ? " open" : ""}` },
			el("div", {
				class: "fact-head",
				dataset: { seq },
				role: "button",
				tabIndex: 0,
				attrs: { "aria-expanded": String(open) },
			},
				el("span", { class: "seq", text: `#${seq}` }),
				el("span", { class: `type${fact?.type === "message" ? " message" : ""}`, text: String(fact?.type || "unknown") }),
				fact?.role ? el("span", { class: "role", text: String(fact.role) }) : null,
				el("span", { class: "time", text: timeOfDay(fact?.timestamp) }),
				el("span", { class: "chevron" }, icon("chevron", 12)),
			),
			el("div", { class: "preview", text: factPreview(fact) }),
		);
		if (open) {
			row.append(el("pre", { class: "raw", text: JSON.stringify(fact, null, 2) }));
		}
		return row;
	}

	handleClick(event) {
		const target = event.target;
		if (!(target instanceof Element)) return;
		const head = target.closest(".fact-head[data-seq]");
		if (!head) return;
		this.toggle(Number(head.dataset.seq));
	}

	handleKey(event) {
		const head = event.target.closest?.(".fact-head[data-seq]");
		if (!head) return;
		if (event.key !== "Enter" && event.key !== " ") return;
		event.preventDefault();
		this.toggle(Number(head.dataset.seq));
	}

	toggle(seq) {
		if (!seq) return;
		if (this.expanded.has(seq)) this.expanded.delete(seq);
		else this.expanded.add(seq);
		this.render();
	}
}

customElements.define("session-trace", SessionTrace);
