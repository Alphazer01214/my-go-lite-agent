import {
	callCapability,
	callPrompter,
	clear,
	errorMessage,
	formatNumber,
	formatTime,
	node,
	stringify,
} from "./client.js";
import { panelStyles } from "./styles.js";

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
		this.onTabShow = () => this.resync();
	}

	connectedCallback() {
		this.shadowRoot.innerHTML = `<style>${panelStyles}</style>
			<div class="panel">
				<div class="head">
					<span class="title">Context</span>
					<span class="badge" data-source>—</span>
					<span class="spacer"></span>
					<span class="muted" data-updated></span>
					<button type="button" data-refresh>Refresh</button>
				</div>
				<div class="meta" data-meta></div>
				<div class="usage" data-usage></div>
				<div class="error" data-error hidden></div>
				<div class="body" data-body></div>
				<div class="empty" data-empty hidden>No context yet. Run a turn to populate the model-visible context.</div>
			</div>`;
		this.els = {
			source: this.shadowRoot.querySelector("[data-source]"),
			updated: this.shadowRoot.querySelector("[data-updated]"),
			refresh: this.shadowRoot.querySelector("[data-refresh]"),
			meta: this.shadowRoot.querySelector("[data-meta]"),
			usage: this.shadowRoot.querySelector("[data-usage]"),
			error: this.shadowRoot.querySelector("[data-error]"),
			body: this.shadowRoot.querySelector("[data-body]"),
			empty: this.shadowRoot.querySelector("[data-empty]"),
		};
		this.els.refresh.addEventListener("click", () => this.resync());
		this.addEventListener("liteagent:tab-show", this.onTabShow);
		this.resync();
	}

	disconnectedCallback() {
		this.removeEventListener("liteagent:tab-show", this.onTabShow);
		clearTimeout(this.refreshTimer);
	}

	async resync() {
		if (!this.els) return undefined;
		if (this.loading) {
			this.pending = true;
			return this.loadingPromise;
		}
		this.loading = true;
		this.els.refresh.setAttribute("disabled", "");
		this.loadingPromise = this.load()
			.then((data) => {
				this.data = data;
				this.error = "";
			})
			.catch((error) => {
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

	async load() {
		let data = await callPrompter("context.get", {});
		if (data?.source !== "none") return data;
		try {
			const current = await callCapability("workspace", "current", {});
			const workspace = current?.workspace?.path || "";
			const list = await callCapability("session", "list", workspace ? { workspace } : {});
			const sessions = Array.isArray(list?.sessions) ? [...list.sessions] : [];
			// Empty newly-created sessions have last_ts=0; fall back to created_at like the sidebar.
			sessions.sort(
				(a, b) => Number(b.last_ts || b.created_at || 0) - Number(a.last_ts || a.created_at || 0),
			);
			const session = sessions[0];
			if (session?.session_id) {
				data = await callPrompter("context.get", { session_id: session.session_id });
			}
		} catch {
			// Keep the empty view when no workspace or session is available.
		}
		return data;
	}

	setData(state) {
		if (!state || this.hidden) return;
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
		this.els.source.textContent = source;
		this.els.source.dataset.kind = source;
		this.els.updated.textContent = data?.assembled_at ? formatTime(data.assembled_at) : "";

		clear(this.els.meta);
		if (data && source !== "none") {
			this.els.meta.append(
				kv("session", data.session_id || "—"),
				kv("turn", data.turn_id || "—"),
				kv("model", data.model || "—"),
			);
		}

		this.renderUsage(data?.usage);
		this.renderError();
		clear(this.els.body);

		const hasMessages = Array.isArray(data?.messages) && data.messages.length > 0;
		const hasTools = Array.isArray(data?.tools) && data.tools.length > 0;
		this.els.empty.hidden = Boolean(this.error) || hasMessages || hasTools || source === "none";
		if (!data || source === "none") {
			this.els.empty.hidden = Boolean(this.error);
			return;
		}

		if (hasMessages) {
			this.els.body.append(this.renderMessages(data.messages));
		}
		if (hasTools) {
			this.els.body.append(this.renderTools(data.tools));
		}
		if (data.snapshot) {
			this.els.body.append(this.renderSnapshot(data.snapshot));
		}
	}

	renderUsage(usage) {
		clear(this.els.usage);
		if (!usage) return;
		const total = Number(usage.token_estimate) || 0;
		const window = Number(usage.context_window) || 0;
		const percent = Number(usage.percent) || 0;
		const parts = [`≈${formatNumber(total)} tokens`];
		if (window > 0) parts.push(`${formatNumber(window)} window · ${percent}%`);
		parts.push(`${formatNumber(usage.message_count)} messages`);
		parts.push(`${formatNumber(usage.tool_count)} tools`);
		this.els.usage.append(node("div", "usage-text", parts.join(" · ")));
		if (window > 0) {
			const bar = node("div", "progress");
			const fill = node("div", "progress-fill");
			fill.style.width = `${Math.min(100, Math.max(0, percent))}%`;
			bar.append(fill);
			this.els.usage.append(bar);
		}
	}

	renderError() {
		this.els.error.hidden = !this.error;
		this.els.error.textContent = this.error;
	}

	renderMessages(messages) {
		const section = node("section");
		section.append(node("h3", "section-title", `Messages (${formatNumber(messages.length)})`));
		messages.forEach((message) => {
			const details = node("details");
			details.open = message.role === "system";
			const summary = node("summary");
			summary.append(
				node("span", "role", message.role || "unknown"),
				node("span", "tag", `seq ${message.seq ?? "—"}`),
				node("span", "tag", `${formatNumber(message.tokens)} tok`),
			);
			if (message.truncated) summary.append(node("span", "tag", "truncated"));
			if (message.tool_call_id) summary.append(node("span", "tag", message.tool_call_id));
			details.append(summary);

			if (message.text) {
				details.append(node("pre", "", message.text));
			}
			if (message.reasoning) {
				details.append(node("div", "hint", "reasoning"));
				details.append(node("pre", "", message.reasoning));
			}
			if (Array.isArray(message.tool_calls) && message.tool_calls.length > 0) {
				details.append(node("div", "hint", "tool calls"));
				details.append(node("pre", "", stringify(message.tool_calls)));
			}
			if (Array.isArray(message.files) && message.files.length > 0) {
				details.append(node("div", "hint", "files"));
				details.append(node("pre", "", stringify(message.files)));
			}
			section.append(details);
		});
		return section;
	}

	renderTools(tools) {
		const section = node("section");
		section.append(node("h3", "section-title", `Tools (${formatNumber(tools.length)})`));
		tools.forEach((tool) => {
			const details = node("details");
			const summary = node("summary");
			summary.append(node("span", "role", tool.name || "unknown"));
			details.append(summary);
			if (tool.description) details.append(node("div", "item-desc", tool.description));
			if (tool.parameters) {
				details.append(node("pre", "", stringify(tool.parameters)));
			}
			section.append(details);
		});
		return section;
	}

	renderSnapshot(snapshot) {
		const section = node("section");
		section.append(node("h3", "section-title", "Snapshot"));
		const meta = node("div", "meta");
		const fields = [
			["surface", snapshot.surface_seqs?.join(", ") || "—"],
			["messages_hash", snapshot.messages_hash || "—"],
			["system_hash", snapshot.system_prompt_hash || "—"],
			["runtime_hash", snapshot.runtime_context_hash || "—"],
			["tool_generation", snapshot.tool_catalog_generation || "—"],
		];
		for (const [key, value] of fields) {
			meta.append(kv(key, value));
		}
		section.append(meta);
		if (Array.isArray(snapshot.tool_names) && snapshot.tool_names.length > 0) {
			section.append(node("div", "hint", "tool names"));
			section.append(node("pre", "", snapshot.tool_names.join("\n")));
		}
		return section;
	}
}

function kv(key, value) {
	const item = node("span", "kv");
	item.append(node("span", "k", key), node("span", "v", value));
	return item;
}

customElements.define("prompter-context", PrompterContext);
