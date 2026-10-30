function isObject(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function settingsValuesFromResponse(payload) {
	return isObject(payload?.values) ? payload.values : {};
}

// Settings are plugin methods: a stopped plugin has no route owner, so the
// renderer must not call settings.get for it. Unknown state stays available
// for callers that do not track lifecycle.
export function pluginSettingsAvailability(plugin) {
	if (!plugin || plugin.missing) {
		return { available: false, message: "插件缺失，无法读取设置" };
	}
	switch (plugin.state) {
		case undefined:
		case "":
		case "active":
			return { available: true };
		case "loading":
			return { available: false, message: "插件加载中，稍后可读取设置" };
		case "stopping":
			return { available: false, message: "插件停止中，停止完成后可重新启动" };
		case "failed":
			return { available: false, message: "插件启动失败，请先重新启动插件" };
		default:
			return { available: false, message: "插件未启动，启动后可编辑设置" };
	}
}

function normalizeListItem(field, rawItem) {
	const item = {};
	let hasValue = false;
	for (const itemField of Array.isArray(field.item_fields) ? field.item_fields : []) {
		const raw = rawItem?.[itemField.key];
		if (raw === null || raw === undefined || raw === "") {
			if (itemField.type === "number") item[itemField.key] = 0;
			else if (itemField.type === "boolean") item[itemField.key] = false;
			else item[itemField.key] = "";
			continue;
		}
		hasValue = true;
		if (itemField.type === "number") {
			const number = Number(raw);
			if (Number.isFinite(number)) item[itemField.key] = number;
		} else if (itemField.type === "boolean") {
			item[itemField.key] = Boolean(raw);
		} else if (itemField.type === "secret") {
			item[itemField.key] = typeof raw === "string" ? raw : "";
		} else {
			item[itemField.key] = String(raw);
		}
	}
	return hasValue ? item : null;
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
			case "list": {
				if (!Array.isArray(value)) break;
				const items = [];
				for (const rawItem of value) {
					if (!isObject(rawItem)) continue;
					const item = normalizeListItem(field, rawItem);
					if (item) items.push(item);
				}
				result[field.key] = items;
				break;
			}
			default:
				if (value !== null && value !== undefined) result[field.key] = String(value);
		}
	}
	return result;
}

function selectOptions(field, listValues) {
	if (!field.options_from) {
		return (Array.isArray(field.options) ? field.options : [])
			.map((option) => ({
				value: String(option?.value ?? ""),
				label: String(option?.label || option?.value || ""),
			}))
			.filter((option) => option.value);
	}
	const valueKey = field.option_value || "id";
	const labelKey = field.option_label || "label";
	return (Array.isArray(listValues) ? listValues : [])
		.map((item) => ({
			value: String(item?.[valueKey] ?? ""),
			label: String(item?.[labelKey] || item?.[valueKey] || ""),
		}))
		.filter((option) => option.value);
}

function readRawListValues(container) {
	if (!container) return [];
	const rows = container.querySelectorAll?.(".settings-list-row") || [];
	return [...rows].map((row) => {
		const item = {};
		const inputs = row.querySelectorAll?.(".settings-list-input") || [];
		for (const input of inputs) {
			item[input.dataset?.itemKey] = input.type === "checkbox" ? input.checked : input.value;
		}
		return item;
	});
}

function syncDependentSelects(form, schema, preferredValues) {
	for (const field of Array.isArray(schema) ? schema : []) {
		if (field.type !== "select") continue;
		const select = form.elements.namedItem(field.key);
		if (!select) continue;
		let listValues = [];
		if (field.options_from) {
			const container = form.querySelector?.(`[data-list-key="${field.options_from}"]`);
			listValues = readRawListValues(container);
		}
		const options = selectOptions(field, listValues);
		const preferred = preferredValues?.[field.key] !== undefined
			? String(preferredValues[field.key])
			: String(select.value || "");
		select.replaceChildren(...options.map((option) => {
			const node = select.ownerDocument.createElement("option");
			node.value = option.value;
			node.textContent = option.label;
			return node;
		}));
		select.value = options.some((option) => option.value === preferred)
			? preferred
			: options[0]?.value || "";
		select.disabled = options.length === 0;
	}
}

function createListRow(document, field, item) {
	const row = document.createElement("div");
	row.className = "settings-list-row";
	const itemFields = Array.isArray(field.item_fields) ? field.item_fields : [];
	const scalarCount = itemFields.filter((itemField) => itemField.type !== "boolean").length;
	const actionRow = Math.floor(scalarCount / 2) + 1;
	const booleanStartRow = actionRow + 1;
	let scalarIndex = 0;
	let booleanIndex = 0;
	for (const itemField of itemFields) {
		const value = item?.[itemField.key];
		let input;
		if (itemField.type === "select") {
			input = document.createElement("select");
			const options = (Array.isArray(itemField.options) ? itemField.options : [])
				.map((option) => ({
					value: String(option?.value ?? ""),
					label: String(option?.label || option?.value || ""),
				}))
				.filter((option) => option.value);
			// A blank placeholder keeps an untouched new row out of the save
			// payload; otherwise the first option would make the row look filled.
			const blank = document.createElement("option");
			blank.value = "";
			blank.textContent = "请选择";
			input.replaceChildren(blank, ...options.map((option) => {
				const node = document.createElement("option");
				node.value = option.value;
				node.textContent = option.label;
				return node;
			}));
			input.value = value === null || value === undefined ? "" : String(value);
		} else if (itemField.type === "boolean") {
			input = document.createElement("input");
			input.type = "checkbox";
			input.checked = Boolean(value);
			input.placeholder = itemField.label || itemField.key;
		} else if (itemField.type === "secret") {
			input = document.createElement("input");
			input.type = "password";
			input.autocomplete = "new-password";
			const configured = Boolean(
				value?.configured ||
				(typeof value === "string" && value !== ""),
			);
			input.value = "";
			input.placeholder = configured ? "已配置，留空保持不变" : "未配置";
		} else {
			input = document.createElement("input");
			input.type = itemField.type === "number" ? "number" : "text";
			input.placeholder = itemField.label || itemField.key;
			input.value = value === null || value === undefined ? "" : String(value);
		}
		input.className = "settings-list-input";
		input.dataset.itemKey = itemField.key;
		input.setAttribute?.("aria-label", `${field.label || field.key} ${itemField.label || itemField.key}`);
		if (itemField.required) input.setAttribute?.("aria-required", "true");
		if (itemField.min !== undefined) input.min = String(itemField.min);
		if (itemField.step !== undefined) input.step = String(itemField.step);

		const cell = document.createElement("label");
		cell.className = itemField.type === "boolean"
			? "settings-list-cell settings-list-boolean"
			: "settings-list-cell";
		const title = document.createElement("span");
		title.className = "settings-list-label";
		title.textContent = itemField.label || itemField.key;
		if (itemField.required) {
			const required = document.createElement("span");
			required.className = "settings-list-required";
			required.textContent = "必填";
			title.append(required);
		}
		if (itemField.type === "boolean") {
			cell.append(input, title);
			cell.style.gridRow = String(booleanStartRow + Math.floor(booleanIndex / 3));
			cell.style.gridColumn = String(1 + (booleanIndex % 3) * 2) + " / span 2";
			booleanIndex += 1;
		} else {
			cell.append(title, input);
			cell.style.gridRow = String(1 + Math.floor(scalarIndex / 2));
			cell.style.gridColumn = scalarIndex % 2 === 0 ? "1 / span 3" : "4 / span 3";
			scalarIndex += 1;
		}
		row.append(cell);
	}
	const actions = document.createElement("div");
	actions.className = "settings-list-actions";
	actions.style.gridRow = String(actionRow);
	actions.style.gridColumn = "4 / -1";
	const remove = document.createElement("button");
	remove.type = "button";
	remove.className = "settings-list-remove";
	remove.textContent = "删除";
	remove.addEventListener("click", () => {
		const parent = row.parentElement;
		row.remove();
		parent?.dispatchEvent?.(new Event("input", { bubbles: true }));
	});
	actions.append(remove);
	row.append(actions);
	return row;
}

function appendListRow(form, field, item) {
	const document = form.ownerDocument;
	const container = form.querySelector?.(`[data-list-key="${field.key}"]`);
	if (!document || !container) return null;
	const row = createListRow(document, field, item);
	container.append(row);
	return row;
}

export function setSettingsFormValues(form, schema, values) {
	for (const field of Array.isArray(schema) ? schema : []) {
		if (field.type === "list") {
			const container = form.querySelector?.(`[data-list-key="${field.key}"]`);
			if (!container) continue;
			container.replaceChildren();
			const items = Array.isArray(values?.[field.key]) ? values[field.key] : [];
			for (const item of items) appendListRow(form, field, item);
			if (items.length === 0) appendListRow(form, field, {});
			continue;
		}
		if (field.type === "select") continue;
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
	syncDependentSelects(form, schema, values);
}

export function setSettingsStatus(form, message, kind = "info") {
	const status = form.querySelector(".settings-status");
	if (!status) return;
	status.textContent = message;
	status.className = `settings-status settings-status-${kind}`;
}

function createInput(document, pluginName, field) {
	const input = document.createElement(field.type === "select" ? "select" : "input");
	input.id = `${pluginName}-${field.key}`;
	input.name = field.key;
	if (field.type === "select") return input;
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

function appendHint(document, row, field) {
	if (!field.description) return;
	const hint = document.createElement("span");
	hint.className = "setting-hint";
	hint.textContent = field.description;
	row.append(hint);
}

function renderListField(document, form, field) {
	const row = document.createElement("div");
	row.className = "setting-field setting-field-full";
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
	appendHint(document, row, field);

	const container = document.createElement("div");
	container.className = "settings-list";
	container.dataset.listKey = field.key;
	container.addEventListener("input", () => syncDependentSelects(form, form.__settingsSchema || [field]));
	row.append(container);

	const add = document.createElement("button");
	add.type = "button";
	add.className = "settings-list-add";
	add.textContent = "添加一项";
	add.addEventListener("click", () => appendListRow(form, field, {}));
	row.append(add);
	return row;
}

// renderSettingsForm renders one plugin's settings. The detail pane owns the
// plugin header and availability message so a stopped plugin never exposes form
// inputs that cannot be saved.
export function renderSettingsForm(document, root, plugin, handlers = {}) {
	root.replaceChildren();
	const availability = handlers.availability?.(plugin) ?? { available: true };
	if (!availability.available) {
		const message = document.createElement("p");
		message.className = "settings-unavailable";
		message.textContent = availability.message || "插件未启动";
		root.append(message);
		return null;
	}
	const schema = Array.isArray(plugin?.settings?.schema) ? plugin.settings.schema : [];
	if (schema.length === 0) {
		const empty = document.createElement("p");
		empty.className = "settings-empty";
		empty.textContent = "该插件未提供可配置项。";
		root.append(empty);
		return null;
	}

	const form = document.createElement("form");
	form.className = "settings-form";
	form.dataset.plugin = plugin.name || "";
	form.__settingsSchema = schema;
	const fields = document.createElement("div");
	fields.className = "settings-fields";

	for (const field of schema) {
		if (field.type === "list") {
			fields.append(renderListField(document, form, field));
			continue;
		}
		const row = document.createElement("label");
		row.className = "setting-field";
		row.htmlFor = `${plugin.name || "plugin"}-${field.key}`;

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
		appendHint(document, row, field);
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
		const raw = {};
		for (const field of schema) {
			if (field.type === "list") {
				const container = form.querySelector?.(`[data-list-key="${field.key}"]`);
				raw[field.key] = readRawListValues(container);
				continue;
			}
			const input = form.elements.namedItem(field.key);
			raw[field.key] = field.type === "boolean" ? Boolean(input?.checked) : input?.value;
		}
		handlers.onSave?.(plugin, collectSettingsValues(schema, raw), form);
	});
	root.append(form);
	handlers.onLoad?.(plugin, form);
	return form;
}
