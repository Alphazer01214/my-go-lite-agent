import test from "node:test";
import assert from "node:assert/strict";

import { createMountLoader } from "./static/loader.mjs";

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
				},
			},
		]),
	);
	const customElements = {
		defined: new Set(overrides.definedTags ?? []),
		pending: new Map(),
		get(tag) {
			return this.defined.has(tag) ? {} : undefined;
		},
		whenDefined(tag) {
			if (this.defined.has(tag)) return Promise.resolve();
			const promise = this.pending.get(tag) ?? new Promise((resolve) => {
				this.pending.set(tag, resolve);
			});
			return promise;
		},
		define(tag) {
			this.defined.add(tag);
			this.pending.get(tag)?.({});
			this.pending.delete(tag);
		},
	};
	const document = {
		createElement(tag) {
			const element = {
				tag,
				dataset: {},
				removed: false,
				remove() {
					this.removed = true;
				},
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
		...overrides,
	});
	return { customElements, diagnostics, elements, imports, loader, slots };
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


