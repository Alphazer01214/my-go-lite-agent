import { el, timeOfDay } from "./dom.js";
import { icon } from "./icons.js";
import { TRACE_CSS } from "./styles.js";
import { state, on, init, resync, refreshCurrent, setFactsFilter, notify, reportError } from "./store.js";
import { factsExportFilename, serializeFactsExport } from "./facts-export.js";

function matchesFilter(fact, filter) {
	const isMessage = fact?.type === "message";
	if (filter === "message") return isMessage;
	if (filter === "other") return !isMessage;
	return true;
}

async function writeClipboard(text) {
	if (navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return;
		} catch {
			// Safari may reject clipboard access outside a direct user gesture.
		}
	}
	const textarea = document.createElement("textarea");
	textarea.value = text;
	textarea.setAttribute("readonly", "");
	textarea.style.position = "fixed";
	textarea.style.opacity = "0";
	document.body.append(textarea);
	textarea.select();
	const copied = document.execCommand("copy");
	textarea.remove();
	if (!copied) throw new Error("浏览器不支持复制");
}

const FACT_WINDOW = 200;
const FACT_PREVIEW_LIMIT = 4096;

function previewText(value) {
	const text = String(value ?? "");
	if (text.length <= FACT_PREVIEW_LIMIT) return text;
	return `已截断：显示前 ${FACT_PREVIEW_LIMIT.toLocaleString()} 字符，共 ${text.length.toLocaleString()} 字符\n${text.slice(0, FACT_PREVIEW_LIMIT)}`;
}

function factPreview(fact) {
	if (fact?.type === "message") return previewText(fact.text_content || "(空)");
	if (fact?.text_content) return previewText(fact.text_content);
	if (fact?.meta) {
		try {
			return previewText(JSON.stringify(fact.meta));
		} catch {
			return previewText(fact.meta);
		}
	}
	if (fact?.tool_calls?.length) return `${fact.tool_calls.length} 个工具调用`;
	return "";
}

export class SessionTrace extends HTMLElement {
	static tabTitle = "Facts";
	static tabOrder = 10;

	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.shadowRoot.append(el("style", { text: TRACE_CSS }));
		this.off = [];
		this.expanded = new Set();
		this.selected = new Set();
		this.selectionSessionId = "";
		this.factWindow = FACT_WINDOW;
		this.factWindowSession = "";
		this.factWindowFilter = "";
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

		this.selectAllCheckbox = el("input", {
			class: "select-all-box",
			type: "checkbox",
			attrs: { "aria-label": "全选当前筛选结果" },
			onchange: () => this.toggleVisibleSelection(this.selectAllCheckbox.checked),
		});
		this.selectionCount = el("span", { class: "selection-count", text: "未选择" });
		this.clearButton = el("button", {
			class: "text-button",
			type: "button",
			text: "清空",
			onclick: () => this.clearSelection(),
		});
		this.copyButton = el("button", {
			class: "text-button",
			type: "button",
			text: "复制",
			title: "复制已选 facts JSON",
			onclick: () => this.copySelected(),
		});
		this.exportButton = el("button", {
			class: "export-button",
			type: "button",
			text: "导出",
			title: "导出已选 facts",
			onclick: () => this.exportSelected(),
		});
		this.selectionBar = el("div", { class: "selection-bar" },
			el("label", { class: "select-all" }, this.selectAllCheckbox, el("span", { text: "全选" })),
			this.selectionCount,
			el("div", { class: "selection-actions" }, this.clearButton, this.copyButton, this.exportButton),
		);

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
			this.selectionBar,
			this.list,
		);
		this.shadowRoot.append(root);
	}

	connectedCallback() {
		this.off.push(on("state", () => this.render()));
		this.off.push(on("progress", () => this.renderLiveCount()));
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

	renderLiveCount() {
		const facts = state.facts;
		this.countBadge.textContent = String(state.factsTotal || facts.length);
	}

	render() {
		if (this.selectionSessionId !== state.sessionId) {
			this.selectionSessionId = state.sessionId;
			this.selected.clear();
			this.expanded.clear();
		}
		const facts = state.facts;
		const messages = facts.filter((fact) => fact?.type === "message").length;
		const others = facts.length - messages;
		const visible = facts.filter((fact) => matchesFilter(fact, state.factsFilter));
		if (this.factWindowSession !== state.sessionId || this.factWindowFilter !== state.factsFilter) {
			this.factWindowSession = state.sessionId;
			this.factWindowFilter = state.factsFilter;
			this.factWindow = FACT_WINDOW;
		}
		const selectedCount = facts.filter((fact) => this.selected.has(Number(fact?.seq))).length;
		const selectedVisible = visible.filter((fact) => this.selected.has(Number(fact?.seq))).length;
		this.countBadge.textContent = String(state.factsTotal || facts.length);
		this.refreshButton.disabled = !state.sessionId || state.loading.facts;
		this.selectAllCheckbox.checked = visible.length > 0 && selectedVisible === visible.length;
		this.selectAllCheckbox.indeterminate = selectedVisible > 0 && selectedVisible < visible.length;
		this.selectAllCheckbox.disabled = !state.sessionId || state.loading.facts || visible.length === 0;
		this.selectionCount.textContent = selectedCount > 0
			? `已选 ${selectedCount} / ${facts.length}`
			: "未选择";
		this.clearButton.disabled = selectedCount === 0;
		this.copyButton.disabled = selectedCount === 0 || state.loading.facts;
		this.exportButton.disabled = selectedCount === 0 || state.loading.facts;

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

		if (visible.length === 0) {
			this.list.append(el("div", { class: "empty", text: "这个分类下没有 facts。" }));
			return;
		}
		const shown = visible.slice(Math.max(0, visible.length - this.factWindow));
		const hidden = visible.length - shown.length;
		if (hidden > 0) {
			this.list.append(el("button", {
				class: "load-earlier",
				type: "button",
				text: `加载更早的 ${hidden} 条`,
				onclick: () => this.loadEarlierFacts(),
			}));
		}
		for (const fact of shown) this.list.append(this.renderFact(fact));
		this.list.scrollTop = scrollTop;
	}

	loadEarlierFacts() {
		const before = this.list.scrollHeight;
		const top = this.list.scrollTop;
		this.factWindow += FACT_WINDOW;
		this.render();
		const after = this.list.scrollHeight;
		this.list.scrollTop = top + Math.max(0, after - before);
	}

	renderFact(fact) {
		const seq = Number(fact?.seq) || 0;
		const open = this.expanded.has(seq);
		const selected = this.selected.has(seq);
		const checkbox = el("input", {
			class: "select-box",
			type: "checkbox",
			checked: selected,
			attrs: { "aria-label": `选择 fact #${seq}` },
			onchange: (event) => this.setSelected(seq, event.currentTarget.checked),
		});
		const row = el("div", { class: `fact${fact?.type === "message" ? " message" : ""}${open ? " open" : ""}${selected ? " selected" : ""}` },
			el("div", { class: "fact-main" },
				checkbox,
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

	setSelected(seq, selected) {
		if (!seq) return;
		if (selected) this.selected.add(seq);
		else this.selected.delete(seq);
		this.render();
	}

	toggleVisibleSelection(select) {
		const visible = state.facts.filter((fact) => matchesFilter(fact, state.factsFilter));
		for (const fact of visible) {
			const seq = Number(fact?.seq);
			if (!seq) continue;
			if (select) this.selected.add(seq);
			else this.selected.delete(seq);
		}
		this.render();
	}

	clearSelection() {
		this.selected.clear();
		this.render();
	}

	selectedFacts() {
		return state.facts.filter((fact) => this.selected.has(Number(fact?.seq)));
	}

	async copySelected() {
		const facts = this.selectedFacts();
		if (!state.sessionId || facts.length === 0) return;
		try {
			await writeClipboard(serializeFactsExport(state.sessionId, facts));
			notify(`已复制 ${facts.length} 条 facts`);
		} catch (error) {
			reportError("复制 Facts", error);
		}
	}

	exportSelected() {
		const facts = this.selectedFacts();
		if (!state.sessionId || facts.length === 0) return;
		try {
			const exportedAt = new Date();
			const blob = new Blob([serializeFactsExport(state.sessionId, facts, exportedAt)], {
				type: "application/json;charset=utf-8",
			});
			const url = URL.createObjectURL(blob);
			const link = document.createElement("a");
			link.href = url;
			link.download = factsExportFilename(state.sessionId, facts.length, exportedAt);
			document.body.append(link);
			link.click();
			link.remove();
			setTimeout(() => URL.revokeObjectURL(url), 0);
			notify(`已导出 ${facts.length} 条 facts`);
		} catch (error) {
			reportError("导出 Facts", error);
		}
	}
}

customElements.define("session-trace", SessionTrace);
