import { createMountLoader } from "./loader.mjs";
import { LiteAgent } from "./lite-agent.js";
import {
	renderDependencyDetails,
	renderDependencyEdgeList,
	renderDependencyGraph,
	renderPluginList,
} from "./plugins.mjs";
import {
	renderSettingsList,
	setSettingsFormValues,
	setSettingsStatus,
	settingsValuesFromResponse,
} from "./settings.mjs";

const elements = {
	hostStatus: document.querySelector("#host-status"),
	diagnosticButton: document.querySelector("#diagnostic-toggle"),
	diagnosticCount: document.querySelector("#diagnostic-count"),
	diagnosticPanel: document.querySelector("#diagnostics"),
	diagnosticList: document.querySelector("#diagnostic-list"),
	diagnosticClose: document.querySelector("#diagnostic-close"),
	pluginsButton: document.querySelector("#plugins-toggle"),
	pluginCount: document.querySelector("#plugin-count"),
	pluginPanel: document.querySelector("#plugins"),
	pluginsClose: document.querySelector("#plugins-close"),
	pluginSummary: document.querySelector("#plugin-summary"),
	pluginList: document.querySelector("#plugin-list"),
	pluginRescan: document.querySelector("#plugin-rescan"),
	pluginActionStatus: document.querySelector("#plugin-action-status"),
	settingsSummary: document.querySelector("#settings-summary"),
	settingsList: document.querySelector("#settings-list"),
	pluginGraph: document.querySelector("#plugin-graph"),
	pluginGraphDetails: document.querySelector("#plugin-graph-details"),
	pluginEdgeList: document.querySelector("#plugin-edge-list"),
};

const localDiagnostics = [];
let mountLoader = null;
let currentMounts = [];
let selectedPluginName = "";
const selectedPluginInstances = new Map();

function diagnosticKey(diagnostic) {
	return [diagnostic.plugin ?? "", diagnostic.path ?? "", diagnostic.code ?? ""].join("\u0000");
}

function renderDiagnostics(diagnostics) {
	const merged = new Map();
	for (const diagnostic of diagnostics) {
		const key = diagnosticKey(diagnostic);
		const existing = merged.get(key);
		if (existing) {
			existing.count += 1;
			continue;
		}
		merged.set(key, { ...diagnostic, count: 1 });
	}

	elements.diagnosticList.replaceChildren();
	if (merged.size === 0) {
		const empty = document.createElement("li");
		empty.className = "diagnostic-empty";
		empty.textContent = "暂无诊断信息";
		elements.diagnosticList.append(empty);
	}
	for (const diagnostic of merged.values()) {
		const item = document.createElement("li");
		item.className = "diagnostic";

		const title = document.createElement("strong");
		title.textContent = diagnostic.plugin || "Host";
		item.append(title);

		const location = document.createElement("span");
		location.className = "diagnostic-location";
		location.textContent = diagnostic.path || diagnostic.code;
		item.append(location);

		const code = document.createElement("code");
		code.textContent = diagnostic.code;
		item.append(code);

		const message = document.createElement("p");
		message.textContent = diagnostic.count > 1
			? `${diagnostic.message} (${diagnostic.count})`
			: diagnostic.message;
		item.append(message);

		elements.diagnosticList.append(item);
	}

	const count = merged.size;
	elements.diagnosticCount.textContent = String(count);
	elements.diagnosticButton.disabled = count === 0;
	elements.diagnosticButton.title = count === 0 ? "诊断：暂无诊断信息" : `诊断：${count} 条诊断信息（Esc 关闭）`;
}

function addLocalDiagnostic(diagnostic) {
	localDiagnostics.push(diagnostic);
	renderDiagnostics(localDiagnostics);
}

function settingsButton(form) {
	return form.querySelector(".settings-save");
}

async function loadPluginSettings(plugin, form) {
	const request = plugin.settings?.get;
	const button = settingsButton(form);
	if (!request?.capability || !request?.method) {
		setSettingsStatus(form, "Settings.get 未配置", "error");
		return;
	}
	button.disabled = true;
	setSettingsStatus(form, "加载中…", "info");
	try {
		const payload = await LiteAgent.call(request.capability, request.method, {});
		setSettingsFormValues(form, plugin.settings.schema, settingsValuesFromResponse(payload));
		setSettingsStatus(form, "已加载", "success");
	} catch (error) {
		setSettingsStatus(form, error instanceof Error ? error.message : String(error), "error");
	} finally {
		button.disabled = false;
	}
}

async function savePluginSettings(plugin, values, form) {
	const request = plugin.settings?.save;
	const button = settingsButton(form);
	if (!request?.capability || !request?.method) {
		setSettingsStatus(form, "Settings.save 未配置", "error");
		return;
	}
	button.disabled = true;
	setSettingsStatus(form, "保存中…", "info");
	try {
		await LiteAgent.call(request.capability, request.method, { values });
		await loadPluginSettings(plugin, form);
		setSettingsStatus(form, "已保存", "success");
	} catch (error) {
		setSettingsStatus(form, error instanceof Error ? error.message : String(error), "error");
	} finally {
		button.disabled = false;
	}
}

function setPluginActionStatus(message, kind = "info") {
	elements.pluginActionStatus.textContent = message || "";
	elements.pluginActionStatus.className = `plugin-action-status plugin-action-${kind}`;
}

function renderSettings(plugins) {
	const configurable = plugins.filter((plugin) => plugin.settings?.schema?.length);
	renderSettingsList(document, elements.settingsList, configurable, {
		onLoad: loadPluginSettings,
		onSave: savePluginSettings,
	});
	elements.settingsSummary.textContent = `${configurable.length} 个可配置插件`;
}

function renderPluginGraphView(plugins) {
	if (!plugins.some((plugin) => plugin.name === selectedPluginName)) selectedPluginName = "";
	const graphModel = renderDependencyGraph(document, elements.pluginGraph, plugins, {
		selectedId: selectedPluginName,
		onSelect: (name) => {
			selectedPluginName = name;
			renderPluginGraphView(plugins);
		},
	});
	renderDependencyDetails(document, elements.pluginGraphDetails, graphModel, selectedPluginName);
}

function renderPluginCenter(plugins) {
	for (const plugin of plugins) {
		const instances = Array.isArray(plugin?.instances) ? plugin.instances : [];
		const selectedID = selectedPluginInstances.get(plugin?.name);
		if (selectedID && !instances.some((instance) => instance.instance_id === selectedID)) {
			selectedPluginInstances.delete(plugin.name);
		}
	}
	renderPluginList(document, elements.pluginList, plugins, {
		selectedInstanceIds: selectedPluginInstances,
		onSelectInstance: (name, instanceID) => {
			selectedPluginInstances.set(name, instanceID);
			renderPluginCenter(plugins);
		},
		onStart: (plugin, button) => runPluginAction(plugin, "start", button),
		onStop: (plugin, button) => runPluginAction(plugin, "stop", button),
	});
	renderSettings(plugins);
	renderDependencyEdgeList(document, elements.pluginEdgeList, plugins);
	renderPluginGraphView(plugins);
	elements.pluginCount.textContent = String(plugins.length);
	elements.pluginSummary.textContent = `${plugins.length} 个插件`;
}
async function parseManagementResponse(response) {
	let payload = null;
	try {
		payload = await response.json();
	} catch {
		payload = null;
	}
	if (!response.ok) {
		throw new Error(payload?.error?.message || `管理请求失败：HTTP ${response.status}`);
	}
	return payload;
}

async function applyBootstrap(payload, { loadMounts = true } = {}) {
	const plugins = Array.isArray(payload?.plugins) ? payload.plugins : [];
	currentMounts = Array.isArray(payload?.mounts) ? payload.mounts : [];
	renderPluginCenter(plugins);
	if (loadMounts && mountLoader) await mountLoader.load(currentMounts);
}

async function runPluginAction(plugin, action, button) {
	if (!plugin?.name) return;
	button.disabled = true;
	setPluginActionStatus(`${action === "start" ? "启动" : "停止"} ${plugin.name}…`, "info");
	try {
		const response = await fetch(`/bridge/plugins/${encodeURIComponent(plugin.name)}/${action}`, {
			method: "POST",
			headers: { Accept: "application/json" },
		});
		const payload = await parseManagementResponse(response);
		await applyBootstrap(payload);
		setPluginActionStatus(`${plugin.name} 已${action === "start" ? "启动" : "停止"}`, "success");
	} catch (error) {
		setPluginActionStatus(error instanceof Error ? error.message : String(error), "error");
		button.disabled = false;
	}
}

async function rescanPlugins() {
	elements.pluginRescan.disabled = true;
	setPluginActionStatus("正在重新扫描插件目录…", "info");
	try {
		const response = await fetch("/bridge/plugins/rescan", {
			method: "POST",
			headers: { Accept: "application/json" },
		});
		const payload = await parseManagementResponse(response);
		await applyBootstrap(payload);
		setPluginActionStatus(`扫描完成，共 ${payload.plugins?.length || 0} 个插件`, "success");
	} catch (error) {
		setPluginActionStatus(error instanceof Error ? error.message : String(error), "error");
	} finally {
		elements.pluginRescan.disabled = false;
	}
}

const panels = [
	{ panel: elements.diagnosticPanel, button: elements.diagnosticButton },
	{ panel: elements.pluginPanel, button: elements.pluginsButton },
];

function setPanel(entry, open) {
	entry.panel.hidden = !open;
	entry.button.setAttribute("aria-expanded", String(open));
	entry.button.classList.toggle("is-active", open);
	if (open) entry.panel.focus({ preventScroll: true });
}

function closePanels(except) {
	for (const entry of panels) {
		if (entry !== except) setPanel(entry, false);
	}
}

function togglePanel(entry) {
	const open = entry.panel.hidden;
	if (open) closePanels(entry);
	setPanel(entry, open);
	if (!open) entry.button.focus({ preventScroll: true });
}

function setPluginView(name) {
	for (const tab of document.querySelectorAll("[data-plugin-view]")) {
		const active = tab.dataset.pluginView === name;
		tab.classList.toggle("is-active", active);
		tab.setAttribute("aria-selected", String(active));
	}
	for (const view of document.querySelectorAll(".plugin-view")) {
		view.hidden = view.id !== `plugin-view-${name}`;
	}
}

function slotElements() {
	const slots = new Map();
	for (const element of document.querySelectorAll("[data-slot]")) {
		slots.set(element.dataset.slot, element);
	}
	return slots;
}

function setHostStatus(state, text) {
	if (!elements.hostStatus) return;
	elements.hostStatus.dataset.state = state;
	elements.hostStatus.textContent = text;
}

async function bootstrap() {
	try {
		const response = await fetch("/bridge/bootstrap", {
			headers: { Accept: "application/json" },
		});
		if (!response.ok) throw new Error(`bootstrap failed with HTTP ${response.status}`);
		const payload = await response.json();
		localDiagnostics.push(...(Array.isArray(payload.diagnostics) ? payload.diagnostics : []));
		renderDiagnostics(localDiagnostics);

		mountLoader = createMountLoader({
			importModule: (entry) => import(entry),
			customElements,
			document,
			slots: slotElements(),
			onDiagnostic: addLocalDiagnostic,
		});
		LiteAgent.onUI((frame) => {
			if (!frame.mount) return;
			const key = `${frame.mount.plugin}:${frame.mount.id}`;
			const element = mountLoader?.get(key);
			if (!element) return;
			if (frame.kind === "state" && typeof element.setData === "function") {
				element.setData(frame.payload);
			} else if (frame.kind === "command" && typeof element.runCommand === "function") {
				element.runCommand(frame.command, frame.payload);
			}
		});
		LiteAgent.onDiagnostic(addLocalDiagnostic);
		LiteAgent.onEvicted(() => {
			setHostStatus("evicted", "已由其他标签页接管");
			addLocalDiagnostic({
				plugin: "host",
				path: "/bridge/ui",
				code: "ui_connection_evicted",
				message: "另一个标签页接管了 UI 推送连接，本页不再接收实时更新。刷新本页可重新接管。",
			});
		});
		await applyBootstrap(payload);
		setHostStatus("ready", "已连接");
	} catch (error) {
		setHostStatus("error", "连接失败");
		addLocalDiagnostic({
			plugin: "host",
			path: "/bridge/bootstrap",
			code: "ui_bootstrap_failed",
			message: error instanceof Error ? error.message : String(error),
		});
	}
}

elements.diagnosticButton.addEventListener("click", () => togglePanel(panels[0]));
elements.pluginsButton.addEventListener("click", () => togglePanel(panels[1]));
elements.diagnosticClose.addEventListener("click", () => {
	setPanel(panels[0], false);
	elements.diagnosticButton.focus({ preventScroll: true });
});
elements.pluginsClose.addEventListener("click", () => {
	setPanel(panels[1], false);
	elements.pluginsButton.focus({ preventScroll: true });
});
elements.pluginRescan.addEventListener("click", rescanPlugins);

for (const tab of document.querySelectorAll("[data-plugin-view]")) {
	tab.addEventListener("click", () => setPluginView(tab.dataset.pluginView));
}

document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") return;
	const open = panels.find((entry) => !entry.panel.hidden);
	if (open) {
		setPanel(open, false);
		open.button.focus({ preventScroll: true });
	}
});

document.addEventListener("click", (event) => {
	const target = event.target;
	if (!(target instanceof Element)) return;
	if (panels.some((entry) => entry.panel.contains(target) || entry.button.contains(target))) return;
	closePanels();
});

renderDiagnostics([]);
renderPluginCenter([]);
setPluginView("manage");
bootstrap();
