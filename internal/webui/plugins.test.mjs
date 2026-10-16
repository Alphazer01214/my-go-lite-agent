import test from "node:test";
import assert from "node:assert/strict";

import { dependencyGraphModel, renderPluginList } from "./static/plugins.mjs";

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
				addEventListener() {},
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
			state: "mounted",
			enabled: true,
			provides: ["session"],
			depends_on: [],
			mounts: [{ plugin: "session", id: "chat" }],
		},
		{
			name: "missing-user",
			state: "missing",
			enabled: true,
			depends_on: ["missing"],
		},
	]);

	assert.equal(list.children.length, 2);
	const rendered = list.children.map(textOf).join("\n");
	assert.match(rendered, /session/);
	assert.match(rendered, /0\.1\.0/);
	assert.match(rendered, /已挂载/);
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
		{ name: "agent", state: "mounted", depends_on: ["session", "llm-openai"] },
		{ name: "session", state: "mounted", depends_on: [] },
		{ name: "llm-openai", state: "available", depends_on: [] },
		{ name: "missing-user", state: "degraded", depends_on: ["missing"] },
	]);

	assert.deepEqual(model.nodes.map((node) => node.id).sort(), [
		"agent", "llm-openai", "missing", "missing-user", "session",
	]);
	assert.equal(model.edges.length, 3);
	assert.equal(model.nodes.find((node) => node.id === "agent").depth, 1);
	assert.equal(model.nodes.find((node) => node.id === "session").depth, 0);
	assert.equal(model.nodes.find((node) => node.id === "missing").state, "missing");
});
