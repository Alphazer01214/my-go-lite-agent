import test from "node:test";
import assert from "node:assert/strict";

import {
	dependencyGraphModel,
	relationshipGraphModel,
	renderDependencyDetails,
	renderPluginList,
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

test("renders plugin lifecycle, capabilities and UI mounts", () => {
	const document = fakeDocument();
	const list = document.createElement("ul");
	renderPluginList(document, list, [
		{
			name: "session",
			version: "0.1.0",
			state: "active",
			enabled: true,
			provides: ["session"],
			depends_on: [],
			mounts: [{ plugin: "session", id: "chat" }],
		},
		{
			name: "missing-user",
			state: "inactive",
			missing: true,
			enabled: true,
			depends_on: ["missing"],
		},
	]);

	assert.equal(list.children.length, 2);
	const rendered = list.children.map(textOf).join("\n");
	assert.match(rendered, /session/);
	assert.match(rendered, /0\.1\.0/);
	assert.match(rendered, /运行中/);
	assert.match(rendered, /启用/);
	assert.match(rendered, /session/);
	assert.match(rendered, /Capability/);
	assert.match(rendered, /UI Mount/);
	assert.match(rendered, /session:chat/);
	assert.match(rendered, /缺失/);
	assert.match(rendered, /missing/);
});

test("renders a stable empty state", () => {
	const document = fakeDocument();
	const list = document.createElement("ul");
	renderPluginList(document, list, []);
	assert.equal(list.children.length, 1);
	assert.equal(textOf(list.children[0]), "暂无插件");
});



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

test("renders selected plugin relationship details", () => {
	const document = fakeDocument();
	const root = document.createElement("aside");
	const model = relationshipGraphModel([
		{ name: "agent", state: "active", provides: ["loop"], depends_on: ["session"], mounts: [] },
		{ name: "session", state: "active", provides: ["session"], depends_on: [], mounts: [{ plugin: "session", id: "chat" }] },
	]);

	renderDependencyDetails(document, root, model, "agent");
	const rendered = textOf(root);
	assert.match(rendered, /agent/);
	assert.match(rendered, /运行中/);
	assert.match(rendered, /Capability/);
	assert.match(rendered, /loop/);
	assert.match(rendered, /硬依赖/);
	assert.match(rendered, /session/);
	assert.match(rendered, /无 UI Mount/);
});


function findNode(node, predicate) {
	if (predicate(node)) return node;
	for (const child of node.children || []) {
		const found = findNode(child, predicate);
		if (found) return found;
	}
	return null;
}

test("selects one provider instance for display", () => {
	const document = fakeDocument();
	const list = document.createElement("ul");
	const selectedInstanceIds = new Map();
	let selection = null;
	const plugin = {
		name: "demo",
		state: "active",
		selected_instance_id: "demo-a",
		enabled: true,
		instances: [
			{ instance_id: "demo-a", generation: 1, state: "active", registrations: [{ capability: "agent", method: "run" }] },
			{ instance_id: "demo-b", generation: 2, state: "loading", registrations: [] },
		],
	};

	renderPluginList(document, list, [plugin], {
		selectedInstanceIds,
		onSelectInstance: (name, instanceID) => {
			selection = { name, instanceID };
			selectedInstanceIds.set(name, instanceID);
		},
	});

	let rendered = list.children.map(textOf).join("\n");
	assert.match(rendered, /demo-a/);
	assert.match(rendered, /demo-b/);
	assert.match(rendered, /运行中/);
	assert.match(rendered, /agent\.run/);

	const select = findNode(list, (node) => node.className === "plugin-instance-select");
	assert.ok(select);
	assert.equal(select.value, "demo-a");
	select.value = "demo-b";
	select.listeners.change();
	assert.deepEqual(selection, { name: "demo", instanceID: "demo-b" });

	renderPluginList(document, list, [plugin], { selectedInstanceIds });
	rendered = list.children.map(textOf).join("\n");
	assert.match(rendered, /加载中/);
});
