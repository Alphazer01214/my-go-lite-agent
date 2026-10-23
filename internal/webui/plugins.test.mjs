import test from "node:test";
import assert from "node:assert/strict";

import {
	dependencyGraphModel,
	relationshipGraphModel,
	renderPluginDetail,
} from "./static/plugins.mjs";

function fakeDocument() {
	return {
		createElement(tag) {
			return {
				tag,
				children: [],
				className: "",
				dataset: {},
				title: "",
				textContent: "",
				attributes: {},
				type: "",
				value: "",
				disabled: false,
				setAttribute(name, value) {
					this.attributes[name] = String(value);
				},
				listeners: {},
				addEventListener(name, listener) {
					this.listeners[name] = listener;
				},
				replaceChildren(...children) {
					this.children = children;
				},
				append(...children) {
					this.children.push(...children);
				},
			};
		},
	};
}

function textOf(node) {
	if (!node) return "";
	if (typeof node === "string") return node;
	return [node.textContent, ...node.children.map(textOf)].filter(Boolean).join(" ");
}

function findNode(node, predicate) {
	if (predicate(node)) return node;
	for (const child of node.children || []) {
		const found = findNode(child, predicate);
		if (found) return found;
	}
	return null;
}

test("derives a deterministic dependency graph model", () => {
	const model = dependencyGraphModel([
		{ name: "agent", state: "active", depends_on: ["session", "llm-openai"] },
		{ name: "session", state: "active", depends_on: [] },
		{ name: "llm-openai", state: "inactive", depends_on: [] },
		{ name: "missing-user", state: "failed", depends_on: ["missing"] },
	]);

	assert.deepEqual(model.nodes.map((node) => node.id).sort(), [
		"agent", "llm-openai", "missing", "missing-user", "session",
	]);
	assert.equal(model.edges.length, 3);
	assert.equal(model.nodes.find((node) => node.id === "agent").depth, 1);
	assert.equal(model.nodes.find((node) => node.id === "session").depth, 0);
	assert.equal(model.nodes.find((node) => node.id === "missing").state, "missing");
});

test("adds Host lifecycle edges without changing plugin dependency depth", () => {
	const model = relationshipGraphModel([
		{ name: "agent", state: "active", provides: ["loop"], depends_on: ["session", "llm-openai"] },
		{ name: "session", state: "active", provides: ["session"], depends_on: [], mounts: [{ id: "chat" }] },
		{ name: "llm-openai", state: "active", provides: ["llm"], depends_on: [] },
	]);

	assert.equal(model.nodes[0].id, "Host");
	assert.equal(model.nodes[0].kind, "host");
	assert.equal(model.edges.filter((edge) => edge.type === "mount").length, 3);
	assert.equal(model.edges.filter((edge) => edge.type === "depends").length, 2);

	const session = model.nodes.find((node) => node.id === "session");
	assert.deepEqual(session.usedBy, ["agent"]);
	assert.equal(session.mounts.length, 1);
});

test("renders plugin info, lifecycle action, instances and relations", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const plugins = [
		{ name: "agent", state: "active", provides: ["loop"], depends_on: ["session"] },
		{
			name: "session",
			version: "0.1.0",
			description: "Session storage and chat UI",
			state: "active",
			selected_instance_id: "session",
			enabled: true,
			provides: ["session"],
			depends_on: [],
			mounts: [{ plugin: "session", id: "chat" }],
			instances: [{
				instance_id: "session",
				generation: 3,
				state: "active",
				registrations: [{ capability: "session", method: "history" }],
			}],
			settings: { schema: [{ key: "limit", type: "number" }] },
		},
	];
	const model = relationshipGraphModel(plugins);
	let stopped = "";
	const result = renderPluginDetail(document, root, model, plugins, "session", {
		onStop: (plugin) => {
			stopped = plugin.name;
		},
	});

	assert.equal(result.plugin.name, "session");
	assert.ok(result.settingsRoot);
	const rendered = textOf(root);
	assert.match(rendered, /session/);
	assert.match(rendered, /0\.1\.0/);
	assert.match(rendered, /Session storage and chat UI/);
	assert.match(rendered, /运行中/);
	assert.match(rendered, /启用/);
	assert.match(rendered, /G3/);
	assert.match(rendered, /session\.history/);
	assert.match(rendered, /Capability/);
	assert.match(rendered, /被依赖/);
	assert.match(rendered, /agent/);
	assert.match(rendered, /session:chat/);

	const stop = findNode(root, (node) => node.dataset?.action === "stop");
	assert.ok(stop);
	stop.listeners.click();
	assert.equal(stopped, "session");
});

test("opens dependency links and clears the selection", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const plugins = [
		{ name: "agent", state: "active", provides: ["loop"], depends_on: ["session"] },
		{ name: "session", state: "active", provides: ["session"], depends_on: [] },
	];
	const model = relationshipGraphModel(plugins);
	let selected = "";
	let cleared = false;
	renderPluginDetail(document, root, model, plugins, "agent", {
		onSelectPlugin: (name) => {
			selected = name;
		},
		onClear: () => {
			cleared = true;
		},
	});

	const sessionLink = findNode(
		root,
		(node) => node.tag === "button" && node.textContent === "session" && node.className.includes("plugin-relation-link"),
	);
	assert.ok(sessionLink);
	sessionLink.listeners.click();
	assert.equal(selected, "session");

	const clear = findNode(root, (node) => node.className === "plugin-detail-clear");
	assert.ok(clear);
	clear.listeners.click();
	assert.equal(cleared, true);
});

test("selects one provider instance and reports changes", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const plugins = [{
		name: "demo",
		state: "active",
		selected_instance_id: "demo-a",
		enabled: true,
		instances: [
			{ instance_id: "demo-a", generation: 1, state: "active", registrations: [] },
			{ instance_id: "demo-b", generation: 2, state: "loading", registrations: [] },
		],
	}];
	const model = relationshipGraphModel(plugins);
	const selectedInstanceIds = new Map();
	let selection = null;
	renderPluginDetail(document, root, model, plugins, "demo", {
		selectedInstanceIds,
		onSelectInstance: (name, instanceID) => {
			selection = { name, instanceID };
			selectedInstanceIds.set(name, instanceID);
		},
	});

	let select = findNode(root, (node) => node.className === "plugin-instance-select");
	assert.ok(select);
	assert.equal(select.value, "demo-a");
	select.value = "demo-b";
	select.listeners.change();
	assert.deepEqual(selection, { name: "demo", instanceID: "demo-b" });

	root.replaceChildren();
	renderPluginDetail(document, root, model, plugins, "demo", { selectedInstanceIds });
	select = findNode(root, (node) => node.className === "plugin-instance-select");
	assert.equal(select.value, "demo-b");
	assert.match(textOf(root), /加载中/);
	assert.match(textOf(root), /设置作用于当前活跃实例/);
});

test("renders a missing dependency without actions or settings", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const plugins = [{ name: "agent", state: "active", provides: ["loop"], depends_on: ["missing"] }];
	const model = relationshipGraphModel(plugins);
	const result = renderPluginDetail(document, root, model, plugins, "missing", {});

	assert.equal(result.plugin, null);
	assert.equal(result.settingsRoot, null);
	const rendered = textOf(root);
	assert.match(rendered, /missing/);
	assert.match(rendered, /缺失/);
	assert.match(rendered, /未在插件目录中发现该依赖/);
	assert.match(rendered, /agent/);
	assert.equal(findNode(root, (node) => node.dataset?.action === "start"), null);
	assert.equal(findNode(root, (node) => node.tag === "form"), null);
});

test("renders an overview with configurable plugin shortcuts", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const plugins = [
		{ name: "agent", state: "active", enabled: true, settings: { schema: [{ key: "a" }] } },
		{ name: "session", state: "inactive", enabled: true },
	];
	const model = relationshipGraphModel(plugins);
	let selected = "";
	renderPluginDetail(document, root, model, plugins, "", {
		onSelectPlugin: (name) => {
			selected = name;
		},
	});

	const rendered = textOf(root);
	assert.match(rendered, /选择一个插件节点/);
	assert.match(rendered, /1 运行中/);
	assert.match(rendered, /1 未激活/);
	assert.match(rendered, /可配置插件/);
	const shortcut = findNode(root, (node) => node.tag === "button" && node.textContent === "agent");
	assert.ok(shortcut);
	shortcut.listeners.click();
	assert.equal(selected, "agent");
});
