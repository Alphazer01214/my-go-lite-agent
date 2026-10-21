import test from "node:test";
import assert from "node:assert/strict";

import {
	collectSettingsValues,
	renderSettingsList,
	settingsValuesFromResponse,
} from "./static/settings.mjs";

function fakeDocument() {
	return {
		createElement(tag) {
			const element = {
				tag,
				children: [],
				className: "",
				dataset: {},
				id: "",
				textContent: "",
				type: "",
				value: "",
				checked: false,
				required: false,
				autocomplete: "",
				placeholder: "",
				disabled: false,
				elements: {
					items: new Map(),
					namedItem(name) {
						return this.items.get(name);
					},
				},
				replaceChildren(...children) {
					this.children = children;
				},
				append(...children) {
					this.children.push(...children);
					if (tag !== "form") return;
					const collect = (node) => {
						if (node?.name) this.elements.items.set(node.name, node);
						for (const child of node?.children ?? []) collect(child);
					};
					for (const child of children) collect(child);
				},
				addEventListener() {},
				querySelector(selector) {
					const className = selector.slice(1);
					const stack = [...this.children];
					while (stack.length) {
						const node = stack.pop();
						if (String(node?.className ?? "").split(/\s+/).includes(className)) return node;
						stack.push(...(node?.children ?? []));
					}
					return null;
				},
			};
			return element;
		},
	};
}

test("normalizes settings get and save envelopes", () => {
	assert.deepEqual(settingsValuesFromResponse({ values: { model: "m" } }), { model: "m" });
	assert.deepEqual(settingsValuesFromResponse({ model: "m" }), {});

	const values = collectSettingsValues(
		[
			{ key: "endpoint", type: "string" },
			{ key: "token", type: "secret" },
			{ key: "retries", type: "number" },
			{ key: "enabled", type: "boolean" },
		],
		{ endpoint: "https://example.test", token: "", retries: "3", enabled: true },
	);
	assert.deepEqual(values, {
		endpoint: "https://example.test",
		retries: 3,
		enabled: true,
	});
});

test("normalizes list fields and keeps select values", () => {
	const values = collectSettingsValues(
		[
			{
				key: "models",
				type: "list",
				item_fields: [
					{ key: "id", type: "string" },
					{ key: "label", type: "string" },
					{ key: "context_window", type: "number" },
				],
			},
			{ key: "default_model", type: "select" },
		],
		{
			models: [
				{ id: "a", label: "Model A", context_window: "8192" },
				{ id: "b", label: "Model B", context_window: "" },
				{ id: "", label: "", context_window: "" },
			],
			default_model: "a",
		},
	);
	assert.deepEqual(values, {
		models: [
			{ id: "a", label: "Model A", context_window: 8192 },
			{ id: "b", label: "Model B", context_window: 0 },
		],
		default_model: "a",
	});
});

test("renders settings fields and calls lifecycle hooks", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	const calls = [];
	renderSettingsList(document, root, [
		{
			name: "demo",
			version: "1.0.0",
			settings: {
				get: { capability: "demo", method: "settings.get" },
				save: { capability: "demo", method: "settings.save" },
				schema: [
					{ key: "endpoint", label: "Endpoint", type: "string", required: true },
					{ key: "token", label: "Token", type: "secret" },
					{ key: "enabled", label: "Enabled", type: "boolean" },
				],
			},
		},
	], {
		onLoad(plugin, form) {
			calls.push(["load", plugin.name, form]);
		},
		onSave(plugin, values, form) {
			calls.push(["save", plugin.name, values, form]);
		},
	});

	assert.equal(root.children.length, 1);
	assert.equal(calls[0][0], "load");
	const form = calls[0][2];
	assert.equal(form.elements.namedItem("token").type, "password");
	assert.equal(form.elements.namedItem("enabled").type, "checkbox");
	assert.match(form.querySelector(".settings-status").textContent, /加载中/);
});

test("renders a stable empty settings state", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	renderSettingsList(document, root, []);
	assert.equal(root.children.length, 1);
	assert.equal(root.children[0].textContent, "暂无可配置插件");
});



