import test from "node:test";
import assert from "node:assert/strict";

import { createMountLoader, TAB_SHOW_EVENT } from "./static/loader.mjs";

function createStorage(initial = {}) {
	return {
		store: new Map(Object.entries(initial)),
		getItem(key) {
			return this.store.has(key) ? this.store.get(key) : null;
		},
		setItem(key, value) {
			this.store.set(key, String(value));
		},
	};
}

function harness(overrides = {}) {
	const imports = [];
	const elements = [];
	const diagnostics = [];
	const slots = new Map(
		["top", "left", "center", "right", "bottom"].map((name) => [
			name,
			{
				children: [],
				append(element) {
					this.children.push(element);
					element.parentElement = this;
				},
				prepend(element) {
					this.children.unshift(element);
					element.parentElement = this;
				},
			},
		]),
	);
	const ctors = new Map(
		(overrides.definedTags ?? []).map((tag) => [tag, overrides.ctors?.[tag] ?? {}]),
	);
	const customElements = {
		defined: new Set(overrides.definedTags ?? []),
		pending: new Map(),
		get(tag) {
			return this.defined.has(tag) ? ctors.get(tag) : undefined;
		},
		whenDefined(tag) {
			if (this.defined.has(tag)) return Promise.resolve();
			const promise = this.pending.get(tag) ?? new Promise((resolve) => {
				this.pending.set(tag, resolve);
			});
			return promise;
		},
		define(tag, ctor = {}) {
			this.defined.add(tag);
			ctors.set(tag, ctor);
			this.pending.get(tag)?.({});
			this.pending.delete(tag);
		},
	};
		const document = {
		createElement(tag) {
			const listeners = new Map();
			const children = [];
			const element = {
				tag,
				className: "",
				textContent: "",
				dataset: {},
				attributes: {},
				style: {},
				children,
				listeners,
				parentElement: null,
				addEventListener(type, handler) {
					const list = listeners.get(type) ?? [];
					list.push(handler);
					listeners.set(type, list);
				},
				removeEventListener() {},
				dispatchEvent(event) {
					for (const handler of listeners.get(event.type) ?? []) handler(event);
					return true;
				},
				setAttribute(name, value) {
					this.attributes[name] = String(value);
				},
				append(...nodes) {
					children.push(...nodes);
				},
				replaceChildren(...nodes) {
					children.length = 0;
					children.push(...nodes);
				},
				remove() {
					this.removed = true;
					if (this.parentElement) {
						const index = this.parentElement.children.indexOf(this);
						if (index >= 0) this.parentElement.children.splice(index, 1);
						this.parentElement = null;
					}
				},
				removed: false,
			};
			elements.push(element);
			return element;
		},
	};
	const loader = createMountLoader({
		importModule: async (entry) => {
			imports.push(entry);
			return {};
		},
		customElements,
		document,
		slots,
		onDiagnostic: (diagnostic) => diagnostics.push(diagnostic),
		whenDefinedTimeoutMs: overrides.whenDefinedTimeoutMs ?? 20,
		resyncRetryDelaysMs: overrides.resyncRetryDelaysMs ?? [0],
		tabStorage: overrides.tabStorage ?? null,
		overlayTags: overrides.overlayTags ?? [],
		...overrides,
	});
	return { customElements, diagnostics, elements, imports, loader, slots };
}

function mount(plugin, id, tag, slot, entry) {
	return { mount: { plugin, id }, tag, slot, entry: entry ?? `/plugins/${plugin}/ui/main.js` };
}

test("deduplicates entry imports and mounts into declared slots", async () => {
	const h = harness({
		definedTags: ["demo-chat", "demo-trace"],
		importModule: async (entry) => {
			h.imports.push(entry);
			return {};
		},
	});

	await h.loader.load([
		{ mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" },
		{ mount: { plugin: "demo", id: "trace" }, tag: "demo-trace", slot: "right", entry: "/plugins/demo/ui/main.js" },
	]);

	assert.equal(h.imports.length, 1);
	assert.equal(h.elements.length, 2);
	assert.equal(h.slots.get("center").children.length, 1);
	assert.equal(h.slots.get("right").children.length, 1);
	assert.equal(h.elements[0].dataset.mountKey, "demo:chat");
	assert.equal(h.elements[1].dataset.mountKey, "demo:trace");
});

test("mounts a key only once", async () => {
	const h = harness({ definedTags: ["demo-chat"] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };

	await h.loader.load([mount, mount]);

	assert.equal(h.elements.length, 1);
	assert.equal(h.slots.get("center").children.length, 1);
});

test("reloading keeps unchanged mounted elements", async () => {
	const h = harness({ definedTags: ["demo-chat"] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };

	await h.loader.load([mount]);
	const first = h.loader.get("demo:chat");
	await h.loader.load([mount]);

	assert.equal(h.elements.length, 1);
	assert.equal(h.loader.get("demo:chat"), first);
	assert.equal(first.removed, false);
});

test("isolates entry import failure", async () => {
	const h = harness({
		definedTags: ["demo-ok"],
		importModule: async (entry) => {
			h.imports.push(entry);
			if (entry.endsWith("bad.js")) throw new Error("import failed");
			return {};
		},
	});

	await h.loader.load([
		{ mount: { plugin: "bad", id: "main" }, tag: "bad-main", slot: "left", entry: "/plugins/bad/ui/bad.js" },
		{ mount: { plugin: "demo", id: "ok" }, tag: "demo-ok", slot: "center", entry: "/plugins/demo/ui/main.js" },
	]);

	assert.equal(h.elements.length, 1);
	assert.equal(h.slots.get("center").children.length, 1);
	assert.equal(h.diagnostics.length, 1);
	assert.equal(h.diagnostics[0].code, "ui_mount_failed");
});

test("isolates custom element definition timeout", async () => {
	const h = harness({ definedTags: ["demo-ok"], whenDefinedTimeoutMs: 1 });

	await h.loader.load([
		{ mount: { plugin: "bad", id: "main" }, tag: "bad-main", slot: "left", entry: "/plugins/bad/ui/main.js" },
		{ mount: { plugin: "demo", id: "ok" }, tag: "demo-ok", slot: "center", entry: "/plugins/demo/ui/main.js" },
	]);

	assert.equal(h.elements.length, 1);
	assert.equal(h.slots.get("center").children.length, 1);
	assert.equal(h.diagnostics.length, 1);
	assert.match(h.diagnostics[0].message, /timed out/i);
});

test("resync asks mounted components to refresh", async () => {
	const h = harness({ definedTags: ["demo-chat"] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };
	await h.loader.load([mount]);

	let refreshed = 0;
	h.elements[0].resync = async () => {
		refreshed += 1;
	};
	await h.loader.resync([mount]);

	assert.equal(refreshed, 1);
	assert.equal(h.elements.length, 1);
	assert.equal(h.loader.has("demo:chat"), true);
});

test("resync remounts components without resync", async () => {
	const h = harness({ definedTags: ["demo-chat"] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };
	await h.loader.load([mount]);
	const first = h.elements[0];

	await h.loader.resync([mount]);

	assert.equal(first.removed, true);
	assert.equal(h.elements.length, 2);
	assert.notEqual(h.loader.get("demo:chat"), first);
});

test("resync keeps the mount when component resync fails", async () => {
	const h = harness({ definedTags: ["demo-chat"] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };
	await h.loader.load([mount]);

	h.elements[0].resync = async () => {
		throw new Error("resync failed");
	};
	await h.loader.resync([mount]);

	assert.equal(h.loader.has("demo:chat"), true);
	assert.equal(h.diagnostics.length, 1);
	assert.equal(h.diagnostics[0].code, "ui_resync_failed");
});

test("resync retries transient component failure", async () => {
	const h = harness({ definedTags: ["demo-chat"], resyncRetryDelaysMs: [0, 0, 0] });
	const mount = { mount: { plugin: "demo", id: "chat" }, tag: "demo-chat", slot: "center", entry: "/plugins/demo/ui/main.js" };
	await h.loader.load([mount]);

	let calls = 0;
	h.elements[0].resync = async () => {
		calls += 1;
		if (calls < 3) throw new Error("plugin not ready");
	};
	await h.loader.resync([mount]);

	assert.equal(calls, 3);
	assert.equal(h.diagnostics.length, 0);
});

test("two same-slot mounts form a tab group hiding the inactive member", async () => {
	const storage = createStorage();
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
		},
		tabStorage: storage,
	});
	await h.loader.load([
		mount("session", "trace", "session-trace", "right"),
		mount("git-tools", "panel", "git-tools-panel", "right"),
	]);

	const rightChildren = h.slots.get("right").children;
	const bar = rightChildren[0];
	assert.equal(bar.className, "slot-tabs");
	assert.equal(bar.attributes["role"], "tablist");
	const buttons = bar.children;
	assert.equal(buttons.length, 2);
	assert.deepEqual(buttons.map((button) => button.textContent), ["会话", "Git"]);
	assert.deepEqual(buttons.map((button) => button.className), ["active", ""]);

	const trace = h.elements.find((element) => element.tag === "session-trace");
	const panel = h.elements.find((element) => element.tag === "git-tools-panel");
	assert.equal(trace.hidden, false);
	assert.equal(trace.style.display, "");
	assert.equal(panel.hidden, true);
	assert.equal(panel.style.display, "none");
	assert.equal(storage.getItem("liteagent.tab.right"), "session:trace");
});

test("tabOrder sorts members and a missing tabTitle falls back to the plugin name", async () => {
	const h = harness({
		definedTags: ["demo-b", "demo-a"],
		ctors: {
			"demo-b": { tabTitle: "Second", tabOrder: 40 },
			"demo-a": { tabTitle: "First", tabOrder: 5 },
		},
		tabStorage: null,
	});
	await h.loader.load([
		mount("demo", "b", "demo-b", "left"),
		mount("demo", "a", "demo-a", "left"),
	]);

	const bar = h.slots.get("left").children[0];
	assert.deepEqual(bar.children.map((button) => button.textContent), ["First", "Second"]);
});

test("single-slot mounts stay visible without a tab bar", async () => {
	const h = harness({ definedTags: ["demo-chat", "demo-trace"] });
	await h.loader.load([
		mount("demo", "chat", "demo-chat", "center"),
		mount("demo", "trace", "demo-trace", "right"),
	]);

	assert.equal(h.slots.get("center").children.length, 1);
	assert.equal(h.slots.get("right").children.length, 1);
	assert.equal(h.elements[0].hidden, false);
	assert.equal(h.elements[1].hidden, false);
});

test("overlay statics and host-side overlayTags are excluded from tabs and never hidden", async () => {
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel", "plan-progress", "sandbox-confirm"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
			"plan-progress": { overlay: true },
		},
		overlayTags: ["sandbox-confirm"],
	});
	await h.loader.load([
		mount("session", "trace", "session-trace", "right"),
		mount("git-tools", "panel", "git-tools-panel", "right"),
		mount("plan", "progress", "plan-progress", "bottom"),
		mount("sandbox", "confirm", "sandbox-confirm", "right"),
	]);

	const bar = h.slots.get("right").children.find((child) => child.className === "slot-tabs");
	assert.deepEqual(bar.children.map((button) => button.textContent), ["会话", "Git"]);
	assert.equal(h.slots.get("bottom").children.length, 1);
	assert.equal(h.slots.get("bottom").children[0].hidden, undefined);

	const overlay = h.elements.find((element) => element.tag === "sandbox-confirm");
	assert.equal(overlay.hidden, undefined);
	assert.equal(overlay.style.display, undefined);
});

test("clicking a tab switches visibility and persists the choice", async () => {
	const storage = createStorage();
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
		},
		tabStorage: storage,
	});
	await h.loader.load([
		mount("session", "trace", "session-trace", "right"),
		mount("git-tools", "panel", "git-tools-panel", "right"),
	]);

	let shown = 0;
	const panel = h.elements.find((element) => element.tag === "git-tools-panel");
	panel.addEventListener(TAB_SHOW_EVENT, () => {
		shown += 1;
	});

	const bar = h.slots.get("right").children[0];
	bar.children[1].dispatchEvent(new Event("click"));

	const trace = h.elements.find((element) => element.tag === "session-trace");
	assert.equal(trace.hidden, true);
	assert.equal(panel.hidden, false);
	assert.equal(storage.getItem("liteagent.tab.right"), "git-tools:panel");
	assert.deepEqual(bar.children.map((button) => button.className), ["", "active"]);
	assert.equal(shown, 1);
});

test("a stored tab choice is restored on the next load", async () => {
	const storage = createStorage({ "liteagent.tab.right": "git-tools:panel" });
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
		},
		tabStorage: storage,
	});
	await h.loader.load([
		mount("session", "trace", "session-trace", "right"),
		mount("git-tools", "panel", "git-tools-panel", "right"),
	]);

	const trace = h.elements.find((element) => element.tag === "session-trace");
	const panel = h.elements.find((element) => element.tag === "git-tools-panel");
	assert.equal(trace.hidden, true);
	assert.equal(panel.hidden, false);
	const bar = h.slots.get("right").children[0];
	assert.deepEqual(bar.children.map((button) => button.className), ["", "active"]);
});

test("dropping below two members removes the bar and reveals the survivor", async () => {
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
		},
		tabStorage: null,
	});
	await h.loader.load([
		mount("session", "trace", "session-trace", "right"),
		mount("git-tools", "panel", "git-tools-panel", "right"),
	]);
	await h.loader.load([mount("session", "trace", "session-trace", "right")]);

	const rightChildren = h.slots.get("right").children;
	assert.equal(rightChildren.length, 1);
	assert.notEqual(rightChildren[0].className, "slot-tabs");
	assert.equal(rightChildren[0].hidden, false);
	assert.equal(rightChildren[0].style.display, "");
});

test("resync keeps the tab group and the active member visible", async () => {
	const h = harness({
		definedTags: ["session-trace", "git-tools-panel"],
		ctors: {
			"session-trace": { tabTitle: "会话", tabOrder: 10 },
			"git-tools-panel": { tabTitle: "Git", tabOrder: 20 },
		},
		tabStorage: null,
	});
	const traceMount = mount("session", "trace", "session-trace", "right");
	const panelMount = mount("git-tools", "panel", "git-tools-panel", "right");
	await h.loader.load([traceMount, panelMount]);
	h.elements.forEach((element) => {
		element.resync = async () => {};
	});
	await h.loader.resync([traceMount, panelMount]);

	const bar = h.slots.get("right").children.find((child) => child.className === "slot-tabs");
	assert.deepEqual(bar.children.map((button) => button.textContent), ["会话", "Git"]);
	const trace = h.elements.find((element) => element.tag === "session-trace");
	const panel = h.elements.find((element) => element.tag === "git-tools-panel");
	assert.equal(trace.hidden, false);
	assert.equal(panel.hidden, true);
});


