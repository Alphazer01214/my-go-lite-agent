import { createMountLoader, mountKeyFor } from "./loader.mjs";
import { LiteAgent } from "./lite-agent.js";
import {
	renderDependencyEdgeList,
	renderDependencyGraph,
	renderPluginDetail,
} from "./plugins.mjs";
import {
	pluginSettingsAvailability,
	renderSettingsForm,
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
	pluginDrawer: document.querySelector("#plugins"),
	pluginsClose: document.querySelector("#plugins-close"),
	pluginSummary: document.querySelector("#plugin-summary"),
	pluginRescan: document.querySelector("#plugin-rescan"),
	pluginActionStatus: document.querySelector("#plugin-action-status"),
	pluginGraph: document.querySelector("#plugin-graph"),
	pluginEdgeList: document.querySelector("#plugin-edge-list"),
	pluginEdgeCount: document.querySelector("#plugin-edge-count"),
	pluginDetail: document.querySelector("#plugin-detail"),
	pluginSelectionStatus: document.querySelector("#plugin-selection-status"),
};

const localDiagnostics = [];
const serverDiagnostics = [];
let mountLoader = null;
let currentMounts = [];
let latestPlugins = [];
let latestGraphModel = { nodes: [], edges: [] };
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

function combinedDiagnostics() {
	return [...serverDiagnostics, ...localDiagnostics];
}

function addLocalDiagnostic(diagnostic) {
	localDiagnostics.push(diagnostic);
	renderDiagnostics(combinedDiagnostics());
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
		// Other plugin UIs may cache data derived from this config, such as
		// session-history's model catalog.
		window.dispatchEvent(new CustomEvent("liteagent:settings-saved", {
			detail: { plugin: plugin.name },
		}));
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

const summaryLabels = Object.freeze({
	active: "运行中",
	loading: "加载中",
	stopping: "停止中",
	inactive: "未激活",
	failed: "失败",
	missing: "缺失",
});

function pluginSummary(plugins) {
	const counts = new Map();
	for (const plugin of plugins) {
		const state = plugin?.missing ? "missing" : (plugin?.state || "inactive");
		counts.set(state, (counts.get(state) || 0) + 1);
	}
	const parts = [`${plugins.length} 个插件`];
	for (const state of ["active", "inactive", "loading", "stopping", "failed", "missing"]) {
		const count = counts.get(state) || 0;
		if (count > 0) parts.push(`${count} ${summaryLabels[state]}`);
	}
	return parts.join(" · ");
}

function renderPluginDetails({ preserveSettings = false } = {}) {
	const previousForm = preserveSettings
		? elements.pluginDetail.querySelector?.(".plugin-settings form")
		: null;
	const previousPlugin = previousForm?.dataset?.plugin || "";
	const result = renderPluginDetail(
		document,
		elements.pluginDetail,
		latestGraphModel,
		latestPlugins,
		selectedPluginName,
		{
			selectedInstanceIds: selectedPluginInstances,
			onSelectPlugin: selectPlugin,
			onSelectInstance: (name, instanceID) => {
				selectedPluginInstances.set(name, instanceID);
				renderPluginDetails({ preserveSettings: true });
			},
			onClear: () => selectPlugin(""),
			onStart: (plugin, button) => runPluginAction(plugin, "start", button),
			onStop: (plugin, button) => runPluginAction(plugin, "stop", button),
		},
	);
	elements.pluginSelectionStatus.textContent = selectedPluginName
		? `已选择插件 ${selectedPluginName}`
		: "未选择插件";
	if (!result?.settingsRoot || !result.plugin) return;
	if (previousForm && previousPlugin === result.plugin.name) {
		result.settingsRoot.append(previousForm);
		return;
	}
	renderSettingsForm(document, result.settingsRoot, result.plugin, {
		availability: pluginSettingsAvailability,
		onLoad: loadPluginSettings,
		onSave: savePluginSettings,
	});
}

function renderPluginCenter(plugins) {
	latestPlugins = Array.isArray(plugins) ? plugins : [];
	if (!latestPlugins.some((plugin) => plugin.name === selectedPluginName)) selectedPluginName = "";
	for (const plugin of latestPlugins) {
		const instances = Array.isArray(plugin?.instances) ? plugin.instances : [];
		const selectedID = selectedPluginInstances.get(plugin?.name);
		if (selectedID && !instances.some((instance) => instance.instance_id === selectedID)) {
			selectedPluginInstances.delete(plugin.name);
		}
	}
	latestGraphModel = renderDependencyGraph(document, elements.pluginGraph, latestPlugins, {
		selectedId: selectedPluginName,
		onSelect: selectPlugin,
	});
	renderDependencyEdgeList(document, elements.pluginEdgeList, latestGraphModel);
	elements.pluginEdgeCount.textContent = String(latestGraphModel.edges.length);
	renderPluginDetails();
	elements.pluginCount.textContent = String(latestPlugins.length);
	elements.pluginSummary.textContent = pluginSummary(latestPlugins);
}

function selectPlugin(name) {
	const next = name || "";
	if (next === selectedPluginName) return;
	selectedPluginName = next;
	renderPluginCenter(latestPlugins);
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

async function applyBootstrap(payload, { loadMounts = true, resync = false } = {}) {
	const plugins = Array.isArray(payload?.plugins) ? payload.plugins : [];
	currentMounts = Array.isArray(payload?.mounts) ? payload.mounts : [];
	renderPluginCenter(plugins);
	if (loadMounts && mountLoader) {
		if (resync) await mountLoader.resync(currentMounts);
		else await mountLoader.load(currentMounts);
	}
}

// StartPlugin only spawns the process; the plugin becomes active once it
// registers. Poll so the settings panel does not stay stuck on "loading"
// after an explicit start.
async function waitForPluginSettled(name) {
	const deadline = Date.now() + 5000;
	let payload = null;
	while (Date.now() < deadline) {
		await new Promise((resolve) => setTimeout(resolve, 150));
		try {
			payload = await fetchBootstrap();
		} catch {
			return payload;
		}
		const plugin = payload?.plugins?.find((entry) => entry.name === name);
		if (!plugin || plugin.state !== "loading") return payload;
	}
	return payload;
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
		let payload = await parseManagementResponse(response);
		if (action === "start") payload = (await waitForPluginSettled(plugin.name)) || payload;
		// A lifecycle action can restart a plugin without changing its mount.
		// Resync mounted components so they re-read the new plugin generation.
		await applyBootstrap(payload, { resync: true });
		const state = payload?.plugins?.find((entry) => entry.name === plugin.name)?.state;
		if (action === "start" && state !== "active") {
			setPluginActionStatus(`${plugin.name} 启动未完成（${state || "unknown"}），请稍后刷新`, "error");
			button.disabled = false;
			return;
		}
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
		await applyBootstrap(payload, { resync: true });
		setPluginActionStatus(`扫描完成，共 ${payload.plugins?.length || 0} 个插件`, "success");
	} catch (error) {
		setPluginActionStatus(error instanceof Error ? error.message : String(error), "error");
	} finally {
		elements.pluginRescan.disabled = false;
	}
}

function setDiagnosticPanel(open) {
	elements.diagnosticPanel.hidden = !open;
	elements.diagnosticButton.setAttribute("aria-expanded", String(open));
	elements.diagnosticButton.classList.toggle("is-active", open);
	if (open) elements.diagnosticPanel.focus({ preventScroll: true });
}

function closeDiagnostics() {
	setDiagnosticPanel(false);
}

function toggleDiagnostics() {
	const open = elements.diagnosticPanel.hidden;
	setDiagnosticPanel(open);
	if (!open) elements.diagnosticButton.focus({ preventScroll: true });
}

function syncPluginDrawerState() {
	const open = elements.pluginDrawer.open;
	elements.pluginsButton.classList.toggle("is-active", open);
	elements.pluginsButton.setAttribute("aria-expanded", String(open));
	document.body.classList.toggle("plugin-drawer-open", open);
	if (!open) elements.pluginsButton.focus({ preventScroll: true });
}

function openPluginDrawer() {
	if (elements.pluginDrawer.open) return;
	closeDiagnostics();
	elements.pluginDrawer.showModal();
	syncPluginDrawerState();
	elements.pluginsClose.focus({ preventScroll: true });
}

function closePluginDrawer() {
	if (!elements.pluginDrawer.open) return;
	elements.pluginDrawer.close();
	// Some embedded browser surfaces do not dispatch the dialog close event.
	syncPluginDrawerState();
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

async function fetchBootstrap() {
	const response = await fetch("/bridge/bootstrap", {
		headers: { Accept: "application/json" },
	});
	if (!response.ok) throw new Error(`bootstrap failed with HTTP ${response.status}`);
	const payload = await response.json();
	serverDiagnostics.length = 0;
	serverDiagnostics.push(...(Array.isArray(payload.diagnostics) ? payload.diagnostics : []));
	renderDiagnostics(combinedDiagnostics());
	return payload;
}

async function syncFromServer({ resync = false } = {}) {
	const payload = await fetchBootstrap();
	await applyBootstrap(payload, { resync });
	setHostStatus("ready", "已连接");
}

async function bootstrap() {
	try {
		mountLoader = createMountLoader({
			importModule: (entry) => import(entry),
			customElements,
			document,
			slots: slotElements(),
			onDiagnostic: addLocalDiagnostic,
		});
		LiteAgent.onUI((frame) => {
			if (!frame.mount) return;
			const element = mountLoader?.get(mountKeyFor(frame.mount));
			if (!element) return;
			if (frame.kind === "state" && typeof element.setData === "function") {
				element.setData(frame.payload);
			} else if (frame.kind === "command" && typeof element.runCommand === "function") {
				element.runCommand(frame.command, frame.payload);
			}
		});
		LiteAgent.onDiagnostic(addLocalDiagnostic);
		LiteAgent.onStatus((next) => {
			if (next === "connected") setHostStatus("ready", "已连接");
			else if (next === "reconnecting") setHostStatus("connecting", "重连中…");
			else if (next === "connecting") setHostStatus("connecting", "连接中…");
		});
		LiteAgent.onResync(async () => {
			try {
				await syncFromServer({ resync: true });
			} catch (error) {
				setHostStatus("error", "重连失败");
				addLocalDiagnostic({
					plugin: "host",
					path: "/bridge/bootstrap",
					code: "ui_resync_failed",
					message: error instanceof Error ? error.message : String(error),
				});
			}
		});
		await syncFromServer();
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

elements.diagnosticButton.addEventListener("click", toggleDiagnostics);
elements.diagnosticClose.addEventListener("click", () => {
	closeDiagnostics();
	elements.diagnosticButton.focus({ preventScroll: true });
});
elements.pluginsButton.addEventListener("click", () => {
	if (elements.pluginDrawer.open) closePluginDrawer();
	else openPluginDrawer();
});
elements.pluginsClose.addEventListener("click", closePluginDrawer);
elements.pluginDrawer.addEventListener("close", syncPluginDrawerState);
elements.pluginDrawer.addEventListener("cancel", (event) => {
	event.preventDefault();
	closePluginDrawer();
});
elements.pluginDrawer.addEventListener("click", (event) => {
	if (event.target !== elements.pluginDrawer) return;
	const rect = elements.pluginDrawer.getBoundingClientRect();
	const outside = event.clientX < rect.left || event.clientX > rect.right
		|| event.clientY < rect.top || event.clientY > rect.bottom;
	if (outside) closePluginDrawer();
});
elements.pluginRescan.addEventListener("click", rescanPlugins);

document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape" || elements.pluginDrawer.open) return;
	if (elements.diagnosticPanel.hidden) return;
	closeDiagnostics();
	elements.diagnosticButton.focus({ preventScroll: true });
});

document.addEventListener("click", (event) => {
	const target = event.target;
	if (!(target instanceof Element)) return;
	if (elements.diagnosticPanel.contains(target) || elements.diagnosticButton.contains(target)) return;
	closeDiagnostics();
});

renderDiagnostics(combinedDiagnostics());
renderPluginCenter([]);
bootstrap();
