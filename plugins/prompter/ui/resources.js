import {
	callCapability,
	callPrompter,
	clear,
	errorMessage,
	formatNumber,
	formatTime,
	node,
} from "./client.js";
import { panelStyles } from "./styles.js";

const dockStyles = `
:host {
	display: block;
	height: 100%;
	overflow: visible;
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
	color: inherit;
	font: inherit;
	font-size: 12px;
	cursor: pointer;
}
.dock-button:hover { background: rgba(127, 127, 127, .12); }
.dock-button[aria-expanded="true"] { border-color: var(--accent, #4d6bfe); background: var(--accent-soft, #eef3ff); }
.resources-dialog {
	width: min(980px, calc(100vw - 40px));
	max-height: min(760px, calc(100vh - 40px));
	padding: 0;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 10px;
	background: var(--surface, #fff);
	color: inherit;
	box-shadow: 0 18px 60px rgba(0, 0, 0, .28);
	overflow: hidden;
}
.resources-dialog::backdrop { background: rgba(0, 0, 0, .35); }
.resources-dialog .panel {
	max-height: min(760px, calc(100vh - 40px));
	overflow: auto;
}
.resources-dialog .head { position: sticky; top: 0; z-index: 1; padding-bottom: 8px; background: var(--surface, #fff); }
.close-button {
	margin-left: 4px;
	padding: 2px 7px;
	font-size: 15px;
	line-height: 1.2;
}
`;

class PrompterResources extends HTMLElement {
	static overlay = true;
	static tabTitle = "MCP / Skills";
	static tabOrder = 10;

	constructor() {
		super();
		this.attachShadow({ mode: "open" });
		this.data = null;
		this.error = "";
		this.loading = false;
		this.loadingPromise = null;
		this.pending = false;
		this.refreshTimer = 0;
		this.mcpConfig = null;
		this.configDirty = false;
		this.configError = "";
		this.onSettingsSaved = () => {
			if (this.els?.dialog?.open) this.resync();
		};
	}

	connectedCallback() {
		this.shadowRoot.innerHTML = `<style>${panelStyles}${dockStyles}</style>
			<div class="dock">
				<button type="button" class="dock-button" data-open aria-haspopup="dialog" aria-expanded="false">
					MCP / Skills
				</button>
			</div>
			<dialog class="resources-dialog" data-dialog aria-label="MCP / Skills">
				<div class="panel">
					<div class="head">
						<span class="title">MCP / Skills</span>
						<span class="spacer"></span>
						<span class="muted" data-updated></span>
						<button type="button" data-refresh>Refresh</button>
						<button type="button" class="close-button" data-close aria-label="关闭">×</button>
					</div>
					<div class="dirs" data-dirs></div>
					<div class="error" data-error hidden></div>
					<div class="grid">
						<section>
							<h3 class="section-title">Skills</h3>
							<div data-skills></div>
						</section>
						<section>
							<h3 class="section-title">MCP Servers</h3>
							<div data-mcp></div>
						</section>
					</div>
					<details class="config">
						<summary>MCP JSON config</summary>
						<textarea data-mcp-config spellcheck="false" placeholder='{"servers":[]}'></textarea>
						<div class="config-actions">
							<button type="button" data-mcp-save>Save config</button>
							<span class="config-status" data-config-status></span>
						</div>
						<div class="hint">Edits the servers array. Enable MCP and set the global timeout in plugin settings.</div>
					</details>
				</div>
			</dialog>`;
		this.els = {
			open: this.shadowRoot.querySelector("[data-open]"),
			dialog: this.shadowRoot.querySelector("[data-dialog]"),
			close: this.shadowRoot.querySelector("[data-close]"),
			updated: this.shadowRoot.querySelector("[data-updated]"),
			refresh: this.shadowRoot.querySelector("[data-refresh]"),
			dirs: this.shadowRoot.querySelector("[data-dirs]"),
			error: this.shadowRoot.querySelector("[data-error]"),
			skills: this.shadowRoot.querySelector("[data-skills]"),
			mcp: this.shadowRoot.querySelector("[data-mcp]"),
			configText: this.shadowRoot.querySelector("[data-mcp-config]"),
			configSave: this.shadowRoot.querySelector("[data-mcp-save]"),
			configStatus: this.shadowRoot.querySelector("[data-config-status]"),
		};
		this.els.open.addEventListener("click", () => this.openDialog());
		this.els.close.addEventListener("click", () => this.closeDialog());
		this.els.dialog.addEventListener("close", () => {
			this.els.open.setAttribute("aria-expanded", "false");
		});
		this.els.refresh.addEventListener("click", () => this.refresh());
		this.els.configSave.addEventListener("click", () => this.saveMCPConfig());
		this.els.configText.addEventListener("input", () => {
			this.configDirty = true;
		});
		globalThis.addEventListener?.("liteagent:settings-saved", this.onSettingsSaved);
	}

	disconnectedCallback() {
		globalThis.removeEventListener?.("liteagent:settings-saved", this.onSettingsSaved);
		clearTimeout(this.refreshTimer);
	}

	openDialog() {
		if (!this.els?.dialog) return;
		this.els.open.setAttribute("aria-expanded", "true");
		if (!this.els.dialog.open) this.els.dialog.showModal();
		this.resync();
	}

	closeDialog() {
		this.els?.dialog?.close();
	}

	async resync() {
		if (!this.els?.dialog?.open) return undefined;
		if (this.loading) {
			this.pending = true;
			return this.loadingPromise;
		}
		this.loading = true;
		this.els.refresh.setAttribute("disabled", "");
		this.loadingPromise = this.load()
			.then((result) => {
				this.data = result.payload;
				if (result.config) this.mcpConfig = result.config;
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
		let workspace = "";
		try {
			const current = await callCapability("workspace", "current", {});
			workspace = current?.workspace?.path || "";
		} catch {
			// The resources panel still works without an active workspace.
		}
		const payload = await callPrompter("resources.list", workspace ? { workspace } : {});
		let config = null;
		try {
			config = await callPrompter("mcp.config.get", {});
			this.configError = "";
		} catch (error) {
			this.configError = errorMessage(error);
		}
		return { payload, config };
	}

	async refresh() {
		try {
			await callPrompter("resources.refresh", {});
		} catch {
			// A failed refresh still reloads the visible state below.
		}
		return this.resync();
	}

	async saveMCPConfig() {
		if (!this.els?.dialog?.open) return;
		this.setConfigStatus("Saving…", "info");
		try {
			const value = JSON.parse(this.els.configText.value || "{}");
			if (!value || typeof value !== "object" || !Array.isArray(value.servers)) {
				throw new Error('config must contain a "servers" array');
			}
			await callPrompter("mcp.config.save", value);
			this.configDirty = false;
			this.mcpConfig = value;
			this.configError = "";
			this.setConfigStatus("Saved", "ok");
			await this.resync();
		} catch (error) {
			this.setConfigStatus(errorMessage(error), "error");
		}
	}

	setConfigStatus(text, kind) {
		const target = this.els?.configStatus;
		if (!target) return;
		target.textContent = text;
		target.className = "config-status" + (kind ? " " + kind : "");
	}

	setData(state) {
		if (!state || !this.els?.dialog?.open) return;
		this.state = state;
		clearTimeout(this.refreshTimer);
		this.refreshTimer = setTimeout(() => this.resync(), 250);
	}

	runCommand(command) {
		if ((command === "refresh" || command === "resources.refresh") && this.els?.dialog?.open) {
			this.refresh();
		}
	}

	render() {
		if (!this.els) return;
		const data = this.data;
		this.els.updated.textContent = data?.updated_at ? formatTime(data.updated_at) : "";

		clear(this.els.dirs);
		if (data) {
			this.els.dirs.append(
				kv("global", data.skills_global_dir || "disabled"),
				kv("workspace", data.skills_workspace_dir || "disabled"),
				kv("mcp", data.mcp?.enabled ? "enabled" : "disabled"),
				kv("timeout", `${formatNumber(data.mcp?.timeout_ms)} ms`),
			);
		}

		this.els.error.hidden = !this.error;
		this.els.error.textContent = this.error;

		clear(this.els.skills);
		clear(this.els.mcp);
		const skills = Array.isArray(data?.skills) ? data.skills : [];
		const servers = Array.isArray(data?.mcp?.servers) ? data.mcp.servers : [];

		if (skills.length === 0) {
			this.els.skills.append(node("div", "empty", "No skills found."));
		} else {
			for (const skill of skills) {
				this.els.skills.append(renderSkill(skill));
			}
		}
		for (const issue of data?.skill_errors || []) {
			this.els.skills.append(node("div", "error", issue));
		}

		if (servers.length === 0) {
			this.els.mcp.append(node("div", "empty", "No MCP servers configured."));
		} else {
			for (const server of servers) {
				this.els.mcp.append(renderServer(server));
			}
		}

		if (this.els.configText) {
			if (this.configError) {
				this.setConfigStatus(this.configError, "error");
			}
			if (!this.configDirty) {
				this.els.configText.value = JSON.stringify(this.mcpConfig ?? { servers: [] }, null, 2);
			}
		}
	}
}

function renderSkill(skill) {
	const item = node("div", "item");
	const head = node("div", "item-head");
	head.append(node("span", "item-name", skill.name || "unknown"));
	head.append(node("span", "tag", skill.source || "config"));
	head.append(node("span", "tag", skill.enabled ? "enabled" : "disabled"));
	item.append(head);
	if (skill.description) item.append(node("div", "item-desc", skill.description));
	if (skill.path) item.append(node("div", "item-path", skill.path));
	if (skill.error) item.append(node("div", "error", skill.error));
	return item;
}

function renderServer(server) {
	const item = node("div", "item");
	const head = node("div", "item-head");
	head.append(node("span", "item-name", server.name || "unknown"));
	head.append(node("span", "tag", server.status || "unknown"));
	if (server.transport) head.append(node("span", "tag", server.transport));
	head.append(node("span", "tag", server.enabled ? "enabled" : "disabled"));
	item.append(head);
	if (server.description) item.append(node("div", "item-desc", server.description));
	if (Array.isArray(server.tools) && server.tools.length > 0) {
		const tools = node("ul", "tools");
		for (const tool of server.tools) {
			const label = tool.description ? `${tool.name}: ${tool.description}` : tool.name;
			tools.append(node("li", "", label));
		}
		item.append(tools);
	}
	if (server.last_error) item.append(node("div", "error", server.last_error));
	return item;
}

function kv(key, value) {
	const item = node("span", "kv");
	item.append(node("span", "k", key), node("span", "v", value));
	return item;
}

customElements.define("prompter-resources", PrompterResources);
