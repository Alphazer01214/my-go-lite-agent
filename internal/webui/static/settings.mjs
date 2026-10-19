function isObject(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function settingsValuesFromResponse(payload) {
	return isObject(payload?.values) ? payload.values : {};
}

export function collectSettingsValues(schema, values) {
	const result = {};
	for (const field of Array.isArray(schema) ? schema : []) {
		const value = values?.[field.key];
		switch (field.type) {
			case "secret":
				if (typeof value === "string" && value !== "") result[field.key] = value;
				break;
			case "number":
				if (value !== "" && value !== null && value !== undefined && Number.isFinite(Number(value))) {
					result[field.key] = Number(value);
				}
				break;
			case "boolean":
				result[field.key] = Boolean(value);
				break;
			default:
				if (value !== null && value !== undefined) result[field.key] = String(value);
		}
	}
	return result;
}

export function setSettingsFormValues(form, schema, values) {
	for (const field of Array.isArray(schema) ? schema : []) {
		const input = form.elements.namedItem(field.key);
		if (!input) continue;
		if (field.type === "secret") {
			const configured = Boolean(
				values?.[field.key]?.configured ||
				(typeof values?.[field.key] === "string" && values[field.key] !== ""),
			);
			input.value = "";
			input.placeholder = configured ? "已配置，留空保持不变" : "未配置";
			continue;
		}
		if (field.type === "boolean") {
			input.checked = Boolean(values?.[field.key]);
			continue;
		}
		const value = values?.[field.key];
		input.value = value === null || value === undefined ? "" : String(value);
	}
}

export function setSettingsStatus(form, message, kind = "info") {
	const status = form.querySelector(".settings-status");
	if (!status) return;
	status.textContent = message;
	status.className = `settings-status settings-status-${kind}`;
}

function createInput(document, pluginName, field) {
	const input = document.createElement("input");
	input.id = `${pluginName}-${field.key}`;
	input.name = field.key;
	switch (field.type) {
		case "secret":
			input.type = "password";
			input.autocomplete = "new-password";
			break;
		case "number":
			input.type = "number";
			break;
		case "boolean":
			input.type = "checkbox";
			break;
		default:
			input.type = "text";
	}
	return input;
}

// renderSettingsList renders the plugins it is given. Filtering by schema is the
// caller's job so the summary count and the list cannot drift apart.
export function renderSettingsList(document, root, plugins, handlers = {}) {
	const entries = Array.isArray(plugins) ? plugins : [];
	root.replaceChildren();

	if (entries.length === 0) {
		const empty = document.createElement("p");
		empty.className = "settings-empty";
		empty.textContent = "暂无可配置插件";
		root.append(empty);
		return;
	}

	for (const plugin of entries) {
		const section = document.createElement("section");
		section.className = "settings-plugin";

		const header = document.createElement("div");
		header.className = "settings-plugin-header";
		const name = document.createElement("strong");
		name.textContent = plugin.name || "未命名插件";
		header.append(name);
		if (plugin.version) {
			const version = document.createElement("span");
			version.textContent = plugin.version;
			header.append(version);
		}
		section.append(header);

		const form = document.createElement("form");
		form.className = "settings-form";
		const fields = document.createElement("div");
		fields.className = "settings-fields";

		for (const field of plugin.settings.schema) {
			const row = document.createElement("label");
			row.className = "setting-field";
			row.htmlFor = `${plugin.name}-${field.key}`;

			const title = document.createElement("span");
			title.className = "setting-label";
			title.textContent = field.label || field.key;
			row.append(title);
			if (field.required) {
				const required = document.createElement("span");
				required.className = "setting-required";
				required.textContent = "必填";
				row.append(required);
			}

			const input = createInput(document, plugin.name || "plugin", field);
			row.append(input);
			fields.append(row);
		}
		form.append(fields);

		const actions = document.createElement("div");
		actions.className = "settings-actions";
		const status = document.createElement("span");
		status.className = "settings-status settings-status-info";
		status.textContent = "加载中…";
		actions.append(status);
		const save = document.createElement("button");
		save.type = "submit";
		save.className = "settings-save";
		save.textContent = "保存";
		actions.append(save);
		form.append(actions);

		form.addEventListener("submit", (event) => {
			event.preventDefault();
			const values = {};
			for (const field of plugin.settings.schema) {
				const input = form.elements.namedItem(field.key);
				values[field.key] = field.type === "boolean" ? input?.checked : input?.value;
			}
			handlers.onSave?.(plugin, collectSettingsValues(plugin.settings.schema, values), form);
		});
		section.append(form);
		root.append(section);
		handlers.onLoad?.(plugin, form);
	}
}


