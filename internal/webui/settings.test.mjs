import test from "node:test";
import assert from "node:assert/strict";

import {
	collectSettingsValues,
	pluginSettingsAvailability,
	renderSettingsForm,
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
				listeners: {},
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
				addEventListener(name, listener) {
					this.listeners[name] = listener;
				},
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

test("keeps select values inside list rows", () => {
	const values = collectSettingsValues(
		[{
			key: "rules",
			type: "list",
			item_fields: [
				{ key: "action", type: "select" },
				{ key: "priority", type: "number" },
			],
		}],
		{ rules: [{ action: "deny", priority: "5" }] },
	);
	assert.deepEqual(values, { rules: [{ action: "deny", priority: 5 }] });
});

test("renders settings fields and calls lifecycle hooks", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	const calls = [];
	const plugin = {
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
	};
	const form = renderSettingsForm(document, root, plugin, {
		onLoad(loadedPlugin, loadedForm) {
			calls.push(["load", loadedPlugin.name, loadedForm]);
		},
		onSave(savedPlugin, values, savedForm) {
			calls.push(["save", savedPlugin.name, values, savedForm]);
		},
	});

	assert.equal(root.children.length, 1);
	assert.equal(root.children[0], form);
	assert.equal(calls[0][0], "load");
	assert.equal(form.dataset.plugin, "demo");
	assert.equal(form.elements.namedItem("token").type, "password");
	assert.equal(form.elements.namedItem("enabled").type, "checkbox");
	assert.match(form.querySelector(".settings-status").textContent, /加载中/);
});

test("submits collected settings values", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	let saved = null;
	const plugin = {
		name: "demo",
		settings: {
			get: { capability: "demo", method: "settings.get" },
			save: { capability: "demo", method: "settings.save" },
			schema: [
				{ key: "endpoint", type: "string" },
				{ key: "retries", type: "number" },
				{ key: "token", type: "secret" },
			],
		},
	};
	const form = renderSettingsForm(document, root, plugin, {
		onSave(_plugin, values) {
			saved = values;
		},
	});
	form.elements.namedItem("endpoint").value = "https://example.test";
	form.elements.namedItem("retries").value = "3";
	form.elements.namedItem("token").value = "";
	form.listeners.submit({ preventDefault() {} });
	assert.deepEqual(saved, { endpoint: "https://example.test", retries: 3 });
});

test("maps plugin lifecycle states to settings availability", () => {
	assert.deepEqual(pluginSettingsAvailability({ state: "active" }), { available: true });
	assert.deepEqual(pluginSettingsAvailability({}), { available: true });
	assert.equal(pluginSettingsAvailability({ state: "inactive" }).available, false);
	assert.match(pluginSettingsAvailability({ state: "loading" }).message, /加载中/);
	assert.match(pluginSettingsAvailability({ state: "stopping" }).message, /停止中/);
	assert.match(pluginSettingsAvailability({ state: "failed" }).message, /启动失败/);
	assert.match(pluginSettingsAvailability({ missing: true }).message, /缺失/);
});

test("does not render a form or load settings while the plugin is inactive", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	const calls = [];
	const form = renderSettingsForm(document, root, {
		name: "sandbox",
		state: "inactive",
		settings: {
			get: { capability: "sandbox", method: "settings.get" },
			save: { capability: "sandbox", method: "settings.save" },
			schema: [{ key: "default_action", label: "Default action", type: "string" }],
		},
	}, {
		availability: pluginSettingsAvailability,
		onLoad(plugin, loadedForm) {
			calls.push(["load", plugin.name, loadedForm]);
		},
		onSave(plugin, values, savedForm) {
			calls.push(["save", plugin.name, values, savedForm]);
		},
	});

	assert.equal(form, null);
	assert.deepEqual(calls, []);
	assert.equal(root.children.length, 1);
	assert.match(root.children[0].className, /settings-unavailable/);
	assert.match(root.children[0].textContent, /插件未启动/);
});

test("renders a stable empty state for a plugin without settings", () => {
	const document = fakeDocument();
	const root = document.createElement("div");
	const form = renderSettingsForm(document, root, { name: "demo", settings: { schema: [] } });
	assert.equal(form, null);
	assert.equal(root.children.length, 1);
	assert.equal(root.children[0].textContent, "该插件未提供可配置项。");
});



