const stateLabels = Object.freeze({
	inactive: "未激活",
	loading: "加载中",
	active: "运行中",
	stopping: "停止中",
	failed: "失败",
	missing: "缺失",
});

function selectedProviderInstance(plugin, handlers) {
	const instances = Array.isArray(plugin?.instances) ? plugin.instances : [];
	const selectedID = handlers?.selectedInstanceIds?.get(plugin?.name)
		|| plugin?.selected_instance_id;
	return instances.find((instance) => instance?.instance_id === selectedID)
		|| instances.find((instance) => instance?.instance_id === plugin?.selected_instance_id)
		|| instances[0]
		|| null;
}

function displayState(plugin, instance) {
	if (!plugin || plugin.missing) return "missing";
	return instance?.state || plugin?.state || "inactive";
}

function stateBadge(document, state, missing = false) {
	const badge = document.createElement("span");
	const name = missing ? "missing" : state || "unknown";
	badge.className = `state-badge state-${String(name).replace(/[^a-z0-9_-]/g, "-")}`;
	badge.textContent = missing ? stateLabels.missing : (stateLabels[name] || name);
	return badge;
}

function instanceDescription(instance) {
	return `${instance?.instance_id || "实例"} · G${instance?.generation || 0} · ${stateLabels[instance?.state] || instance?.state || "未知"}`;
}

function instanceSelector(document, plugin, instance, handlers) {
	const instances = Array.isArray(plugin?.instances) ? plugin.instances : [];
	if (instances.length === 0) return null;
	const wrap = document.createElement("div");
	wrap.className = "plugin-instance-row";
	const select = document.createElement("select");
	select.className = "plugin-instance-select";
	select.dataset.plugin = plugin.name || "";
	select.setAttribute("aria-label", `${plugin.name || "插件"} 实例`);
	for (const item of instances) {
		const option = document.createElement("option");
		option.value = item.instance_id || "";
		option.textContent = instanceDescription(item);
		select.append(option);
	}
	select.value = instance?.instance_id || "";
	select.addEventListener("change", () => {
		handlers?.onSelectInstance?.(plugin.name, select.value);
	});
	wrap.append(select);
	return wrap;
}

const edgeLabels = Object.freeze({
	mount: "装载",
	depends: "硬依赖",
});

const nodeWidth = 180;
const nodeHeight = 76;
const columnGap = 230;
const rowGap = 96;

function appendChipList(document, parent, className, label, values, emptyText, options = {}) {
	const list = document.createElement("div");
	list.className = className;
	if (label) {
		list.dataset.label = label;
		const heading = document.createElement("span");
		heading.className = "plugin-detail-label";
		heading.textContent = label;
		list.append(heading);
	}
	const entries = Array.isArray(values) ? values.filter(Boolean) : [];
	if (entries.length === 0) {
		const empty = document.createElement("span");
		empty.className = "plugin-muted";
		empty.textContent = emptyText;
		list.append(empty);
		parent.append(list);
		return list;
	}
	const format = typeof options.format === "function" ? options.format : (value) => value;
	for (const value of entries) {
		const item = document.createElement(options.onSelect ? "button" : "span");
		item.className = `${className}-item`;
		if (options.onSelect) {
			item.type = "button";
			item.className += " plugin-relation-link";
			item.title = `查看 ${format(value)}`;
			item.addEventListener("click", () => options.onSelect(value));
		}
		item.textContent = format(value);
		list.append(item);
	}
	parent.append(list);
	return list;
}

function appendMuted(document, parent, text) {
	const muted = document.createElement("span");
	muted.className = "plugin-muted";
	muted.textContent = text;
	parent.append(muted);
	return muted;
}

function appendDetailSection(document, parent, title) {
	const section = document.createElement("section");
	section.className = "plugin-detail-section";
	const heading = document.createElement("h3");
	heading.className = "plugin-detail-heading";
	heading.textContent = title;
	section.append(heading);
	parent.append(section);
	return section;
}

function pluginAction(document, plugin, state, handlers) {
	if (!plugin || plugin.missing) return null;
	const busy = state === "loading" || state === "stopping";
	const stopping = state === "active";
	const blocked = !stopping && plugin.enabled === false;
	const button = document.createElement("button");
	button.type = "button";
	button.className = stopping ? "plugin-action plugin-stop" : "plugin-action plugin-start";
	button.textContent = busy
		? state === "loading" ? "加载中…" : "停止中…"
		: stopping ? "停止" : "启动";
	button.dataset.action = stopping ? "stop" : "start";
	button.dataset.plugin = plugin.name || "";
	button.disabled = busy || blocked;
	if (busy) {
		button.title = state === "loading" ? `${plugin.name || "插件"} 加载中` : `${plugin.name || "插件"} 停止中`;
	} else if (blocked) {
		button.title = `${plugin.name || "插件"} 已禁用，无法启动`;
	} else {
		button.title = stopping ? `停止 ${plugin.name || "插件"}` : `启动 ${plugin.name || "插件"}`;
	}
	button.setAttribute("aria-label", button.title);
	if (!busy && !blocked) {
		button.addEventListener("click", () => {
			const action = stopping ? handlers.onStop : handlers.onStart;
			action?.(plugin, button);
		});
	}
	return button;
}

function buildRelationshipGraphModel(plugins, includeHost) {
	const items = Array.isArray(plugins) ? plugins : [];
	const names = [...new Set(items.map((plugin) => plugin?.name).filter(Boolean))];
	const byName = new Map(items.filter((plugin) => plugin?.name).map((plugin) => [plugin.name, plugin]));
	for (const plugin of items) {
		for (const dependency of plugin?.depends_on || []) {
			if (dependency && !names.includes(dependency)) names.push(dependency);
		}
	}
	names.sort();

	const depths = new Map();
	const visiting = new Set();
	const depthOf = (name) => {
		if (depths.has(name)) return depths.get(name);
		if (visiting.has(name)) return 0;
		visiting.add(name);
		const dependencies = byName.get(name)?.depends_on || [];
		const depth = dependencies.length === 0
			? 0
			: 1 + Math.max(...dependencies.map((dependency) => depthOf(dependency)));
		visiting.delete(name);
		depths.set(name, depth);
		return depth;
	};
	for (const name of names) depthOf(name);

	const groups = new Map();
	for (const name of names) {
		const depth = depths.get(name) || 0;
		if (!groups.has(depth)) groups.set(depth, []);
		groups.get(depth).push(name);
	}

	const dependencyEdges = [];
	const usedBy = new Map(names.map((name) => [name, []]));
	for (const plugin of items) {
		for (const dependency of plugin?.depends_on || []) {
			if (!plugin?.name || !dependency) continue;
			dependencyEdges.push({ from: plugin.name, to: dependency, type: "depends" });
			if (usedBy.has(dependency)) usedBy.get(dependency).push(plugin.name);
		}
	}

	const pluginLeft = includeHost ? 250 : 70;
	const maxRows = Math.max(1, ...[...groups.values()].map((group) => group.length));
	const maxDepth = Math.max(0, ...names.map((name) => depths.get(name) || 0));
	const width = Math.max(620, pluginLeft + nodeWidth + maxDepth * columnGap);
	const height = Math.max(300, 80 + maxRows * rowGap);
	const nodes = names.map((name) => {
		const plugin = byName.get(name);
		const depth = depths.get(name) || 0;
		const group = groups.get(depth) || [name];
		const index = group.indexOf(name);
		const groupHeight = group.length * rowGap;
		return {
			id: name,
			kind: "plugin",
			state: displayState(plugin, null),
			enabled: plugin?.enabled !== false,
			depth,
			x: pluginLeft + depth * columnGap,
			y: 44 + (height - 80 - groupHeight) / 2 + index * rowGap,
			missing: !plugin || plugin.missing === true,
			provides: [...(plugin?.provides || [])],
			dependsOn: [...(plugin?.depends_on || [])],
			usedBy: [...(usedBy.get(name) || [])].sort(),
			mounts: [...(plugin?.mounts || [])],
		};
	});

	const edges = [];
	if (includeHost) {
		nodes.unshift({
			id: "Host",
			kind: "host",
			state: "active",
			enabled: true,
			depth: -1,
			x: 70,
			y: height / 2,
			missing: false,
			provides: [],
			dependsOn: [],
			usedBy: [],
			mounts: [],
		});
		for (const plugin of items) {
			if (!plugin?.name || plugin.missing) continue;
			edges.push({ from: "Host", to: plugin.name, type: "mount" });
		}
	}
	edges.push(...dependencyEdges);

	return { nodes, edges, width, height };
}

export function dependencyGraphModel(plugins) {
	return buildRelationshipGraphModel(plugins, false);
}

export function relationshipGraphModel(plugins) {
	return buildRelationshipGraphModel(plugins, true);
}

function svgElement(document, tag, attributes = {}) {
	const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
	for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, String(value));
	return node;
}

function edgeGeometry(edge) {
	const nodeWidthHalf = nodeWidth / 2;
	if (edge.type === "mount") {
		const startX = edge.from.x + nodeWidthHalf;
		const endX = edge.to.x - nodeWidthHalf;
		return {
			startX,
			endX,
			path: `M ${startX} ${edge.from.y} C ${startX + 44} ${edge.from.y}, ${endX - 44} ${edge.to.y}, ${endX} ${edge.to.y}`,
			labelX: (startX + endX) / 2,
			labelY: (edge.from.y + edge.to.y) / 2 - 7,
		};
	}
	const startX = edge.from.x - nodeWidthHalf;
	const endX = edge.to.x + nodeWidthHalf;
	return {
		startX,
		endX,
		path: `M ${startX} ${edge.from.y} C ${startX - 44} ${edge.from.y}, ${endX + 44} ${edge.to.y}, ${endX} ${edge.to.y}`,
		labelX: (startX + endX) / 2,
		labelY: (edge.from.y + edge.to.y) / 2 - 7,
	};
}

function nodeDescription(node) {
	if (node.kind === "host") return "Host 平台内核";
	const state = node.missing ? "缺失" : (stateLabels[node.state] || node.state);
	const capabilities = node.provides.length > 0 ? node.provides.join("、") : "无 Capability";
	return `${node.id}，${state}，Capability ${capabilities}`;
}

export function renderDependencyGraph(document, root, plugins, options = {}) {
	const model = relationshipGraphModel(plugins);
	const selectedId = options.selectedId || "";
	const onSelect = typeof options.onSelect === "function" ? options.onSelect : null;
	const scrollLeft = root.scrollLeft || 0;
	const scrollTop = root.scrollTop || 0;
	root.replaceChildren();
	if (model.nodes.length === 0) {
		const empty = document.createElement("p");
		empty.className = "plugin-empty";
		empty.textContent = "暂无可视化的插件依赖";
		root.append(empty);
		root.scrollLeft = scrollLeft;
		root.scrollTop = scrollTop;
		return model;
	}

	const svg = svgElement(document, "svg", {
		viewBox: `0 0 ${model.width} ${model.height}`,
		role: "group",
		"aria-label": "Host 与插件关系图",
		preserveAspectRatio: "xMidYMid meet",
	});
	const title = svgElement(document, "title");
	title.textContent = "Host 与插件关系图";
	svg.append(title);
	const desc = svgElement(document, "desc");
	desc.textContent = "实线箭头表示硬依赖，虚线箭头表示 Host 装载。点击插件查看详情，点击空白清除选择。";
	svg.append(desc);
	svg.addEventListener("click", (event) => {
		if (event.target === svg) onSelect?.("");
	});

	const defs = svgElement(document, "defs");
	for (const type of ["depends", "mount"]) {
		const marker = svgElement(document, "marker", {
			id: `plugin-arrow-${type}`,
			viewBox: "0 0 10 10",
			refX: "9",
			refY: "5",
			markerWidth: "7",
			markerHeight: "7",
			orient: "auto-start-reverse",
		});
		marker.append(svgElement(document, "path", {
			d: "M 0 0 L 10 5 L 0 10 z",
			class: `graph-arrow graph-arrow-${type}`,
		}));
		defs.append(marker);
	}
	svg.append(defs);

	const nodeIndex = new Map(model.nodes.map((node) => [node.id, node]));
	const related = new Set();
	if (selectedId) {
		related.add(selectedId);
		for (const edge of model.edges) {
			if (edge.from === selectedId) related.add(edge.to);
			if (edge.to === selectedId) related.add(edge.from);
		}
	}

	for (const edge of model.edges) {
		const from = nodeIndex.get(edge.from);
		const to = nodeIndex.get(edge.to);
		if (!from || !to) continue;
		const geometry = edgeGeometry({ ...edge, from, to });
		const active = selectedId && (edge.from === selectedId || edge.to === selectedId);
		const path = svgElement(document, "path", {
			d: geometry.path,
			fill: "none",
			class: `graph-edge graph-edge-${edge.type}${active ? " is-active" : ""}${selectedId && !active ? " is-muted" : ""}`,
			"marker-end": `url(#plugin-arrow-${edge.type})`,
			"data-from": edge.from,
			"data-to": edge.to,
		});
		svg.append(path);
		const label = svgElement(document, "text", {
			x: geometry.labelX,
			y: geometry.labelY,
			"text-anchor": "middle",
			class: `graph-edge-label${active ? " is-active" : ""}${selectedId && !active ? " is-muted" : ""}`,
		});
		label.textContent = edgeLabels[edge.type] || edge.type;
		svg.append(label);
	}

	for (const node of model.nodes) {
		const isSelected = node.id === selectedId;
		const isRelated = related.has(node.id);
		const group = svgElement(document, "g", {
			transform: `translate(${node.x} ${node.y})`,
			"data-plugin": node.id,
			class: `graph-node-group${isSelected ? " is-selected" : ""}${selectedId && !isRelated ? " is-muted" : ""}`,
			role: node.kind === "plugin" ? "button" : "img",
			"aria-label": nodeDescription(node),
			"aria-current": isSelected ? "true" : "false",
		});
		if (node.kind === "plugin") {
			group.setAttribute("tabindex", "0");
			group.addEventListener("click", (event) => {
				event.stopPropagation();
				onSelect?.(node.id);
			});
			group.addEventListener("keydown", (event) => {
				if (event.key === "Enter" || event.key === " ") {
					event.preventDefault();
					onSelect?.(node.id);
				}
			});
		}
		group.append(svgElement(document, "rect", {
			x: String(-nodeWidth / 2),
			y: String(-nodeHeight / 2),
			width: String(nodeWidth),
			height: String(nodeHeight),
			rx: "8",
			class: `graph-node graph-${node.kind === "host" ? "host" : node.state}`,
		}));
		const name = svgElement(document, "text", {
			x: "0", y: "-18", "text-anchor": "middle", class: "graph-title",
		});
		name.textContent = node.id;
		group.append(name);
		const state = svgElement(document, "text", {
			x: "0", y: "3", "text-anchor": "middle", class: "graph-state",
		});
		state.textContent = node.kind === "host"
			? "平台内核"
			: node.missing ? "缺失" : (stateLabels[node.state] || node.state);
		group.append(state);
		const detail = svgElement(document, "text", {
			x: "0", y: "24", "text-anchor": "middle", class: "graph-detail",
		});
		detail.textContent = node.kind === "host"
			? "装载、路由、UI Push"
			: node.provides.length > 0 ? node.provides.join(" · ") : `${node.mounts.length} 个 UI Mount`;
		group.append(detail);
		svg.append(group);
	}
	root.append(svg);
	root.scrollLeft = scrollLeft;
	root.scrollTop = scrollTop;
	return model;
}

function stateCounts(plugins) {
	const order = ["active", "loading", "inactive", "stopping", "failed", "missing"];
	const counts = new Map();
	for (const plugin of Array.isArray(plugins) ? plugins : []) {
		const state = plugin?.missing ? "missing" : (plugin?.state || "inactive");
		counts.set(state, (counts.get(state) || 0) + 1);
	}
	return order.filter((state) => counts.has(state)).map((state) => [state, counts.get(state)]);
}

function renderPluginOverview(document, root, plugins, handlers) {
	const panel = document.createElement("div");
	panel.className = "plugin-overview";
	const title = document.createElement("strong");
	title.className = "plugin-overview-title";
	title.textContent = "选择一个插件节点";
	panel.append(title);
	const hint = document.createElement("p");
	hint.className = "plugin-overview-hint";
	hint.textContent = "点击左侧图中的节点，查看状态、实例、依赖和设置。点击空白处清除选择。";
	panel.append(hint);

	const stats = document.createElement("div");
	stats.className = "plugin-overview-stats";
	const counts = stateCounts(plugins);
	if (counts.length === 0) {
		appendMuted(document, stats, "暂无插件");
	} else {
		for (const [state, count] of counts) {
			const badge = stateBadge(document, state);
			badge.textContent = `${count} ${stateLabels[state] || state}`;
			stats.append(badge);
		}
	}
	panel.append(stats);

	const configurable = (Array.isArray(plugins) ? plugins : [])
		.filter((plugin) => !plugin?.missing && Array.isArray(plugin?.settings?.schema) && plugin.settings.schema.length > 0);
	const section = appendDetailSection(document, panel, "可配置插件");
	if (configurable.length === 0) {
		appendMuted(document, section, "暂无插件声明设置。");
	} else {
		const links = document.createElement("div");
		links.className = "plugin-overview-links";
		for (const plugin of configurable) {
			const button = document.createElement("button");
			button.type = "button";
			button.className = "plugin-relation-link";
			button.textContent = plugin.name;
			button.title = `查看 ${plugin.name} 的设置`;
			button.addEventListener("click", () => handlers.onSelectPlugin?.(plugin.name));
			links.append(button);
		}
		section.append(links);
	}
	root.append(panel);
}

export function renderPluginDetail(document, root, model, plugins, selectedId, handlers = {}) {
	root.replaceChildren();
	const items = Array.isArray(plugins) ? plugins : [];
	const nodes = Array.isArray(model?.nodes) ? model.nodes : [];
	const selected = selectedId
		? nodes.find((node) => node.id === selectedId && node.kind === "plugin")
		: null;
	const plugin = selectedId ? items.find((item) => item?.name === selectedId) || null : null;

	if (!selectedId || !selected) {
		renderPluginOverview(document, root, items, handlers);
		return { plugin: null, settingsRoot: null };
	}

	const missing = !plugin || plugin.missing === true || selected.missing === true;
	const instance = missing ? null : selectedProviderInstance(plugin, handlers);
	const state = missing ? "missing" : displayState(plugin, instance);

	const header = document.createElement("header");
	header.className = "plugin-detail-header";
	const identity = document.createElement("div");
	identity.className = "plugin-detail-identity";
	const name = document.createElement("strong");
	name.className = "plugin-detail-name";
	name.textContent = selectedId;
	identity.append(name);
	if (plugin?.version) {
		const version = document.createElement("span");
		version.className = "plugin-version";
		version.textContent = plugin.version;
		identity.append(version);
	}
	const badges = document.createElement("div");
	badges.className = "plugin-detail-badges";
	badges.append(stateBadge(document, state, missing));
	if (!missing) {
		const enabled = document.createElement("span");
		enabled.className = plugin.enabled === false ? "plugin-disabled" : "plugin-enabled";
		enabled.textContent = plugin.enabled === false ? "禁用" : "启用";
		badges.append(enabled);
	}
	identity.append(badges);
	header.append(identity);

	const actions = document.createElement("div");
	actions.className = "plugin-detail-actions";
	const action = pluginAction(document, plugin, state, handlers);
	if (action) actions.append(action);
	const clear = document.createElement("button");
	clear.type = "button";
	clear.className = "plugin-detail-clear";
	clear.textContent = "清除选择";
	clear.addEventListener("click", () => handlers.onClear?.());
	actions.append(clear);
	header.append(actions);
	root.append(header);

	if (!missing && plugin?.description) {
		const description = document.createElement("p");
		description.className = "plugin-detail-description";
		description.textContent = plugin.description;
		root.append(description);
	}

	if (!missing && instance?.failure_message) {
		const failure = document.createElement("div");
		failure.className = "plugin-failure";
		failure.textContent = `${instance.failure_code || "failed"}: ${instance.failure_message}`;
		root.append(failure);
	}

	const instanceSection = appendDetailSection(document, root, "实例");
	if (missing || !plugin) {
		appendMuted(document, instanceSection, "未在插件目录中发现该依赖，没有可展示的实例。");
	} else {
		const instances = Array.isArray(plugin.instances) ? plugin.instances : [];
		if (instances.length === 0) {
			appendMuted(document, instanceSection, "暂无实例。");
		} else if (instances.length === 1) {
			const meta = document.createElement("span");
			meta.className = "plugin-instance-meta";
			meta.textContent = instanceDescription(instances[0]);
			instanceSection.append(meta);
		} else {
			const selector = instanceSelector(document, plugin, instance, handlers);
			if (selector) instanceSection.append(selector);
		}
		if (instance) {
			appendChipList(
				document,
				instanceSection,
				"plugin-registrations",
				"Registration",
				instance.registrations,
				"无 Registration",
				{ format: (item) => `${item?.capability || ""}.${item?.method || ""}` },
			);
		}
	}

	const relationSection = appendDetailSection(document, root, "关系");
	appendChipList(document, relationSection, "plugin-capabilities", "Capability", selected.provides, "无 Capability");
	appendChipList(
		document,
		relationSection,
		"plugin-dependencies",
		"硬依赖",
		selected.dependsOn,
		"无硬依赖",
		{ onSelect: (value) => handlers.onSelectPlugin?.(value) },
	);
	appendChipList(
		document,
		relationSection,
		"plugin-used-by",
		"被依赖",
		selected.usedBy,
		"无依赖方",
		{ onSelect: (value) => handlers.onSelectPlugin?.(value) },
	);
	appendChipList(
		document,
		relationSection,
		"plugin-mounts",
		"UI Mount",
		selected.mounts,
		"无 UI Mount",
		{ format: (mount) => `${mount?.plugin || selectedId}:${mount?.id || ""}` },
	);

	let settingsRoot = null;
	if (!missing && plugin) {
		const settingsSection = appendDetailSection(document, root, "设置");
		if (Array.isArray(plugin.instances) && plugin.instances.length > 1) {
			appendMuted(document, settingsSection, "设置作用于当前活跃实例，不随实例选择改变。");
		}
		const schema = Array.isArray(plugin.settings?.schema) ? plugin.settings.schema : [];
		if (schema.length === 0) {
			appendMuted(document, settingsSection, "该插件未提供可配置项。");
		} else {
			settingsRoot = document.createElement("div");
			settingsRoot.className = "plugin-settings";
			settingsRoot.dataset.pluginSettings = selectedId;
			settingsSection.append(settingsRoot);
		}
	}

	return { plugin: missing ? null : plugin, settingsRoot };
}

// renderDependencyEdgeList reuses the model renderDependencyGraph already built
// for the same plugin set, instead of resolving cycles and laying out twice.
export function renderDependencyEdgeList(document, root, pluginsOrModel) {
	const model = Array.isArray(pluginsOrModel?.nodes) ? pluginsOrModel : relationshipGraphModel(pluginsOrModel);
	root.replaceChildren();
	if (model.edges.length === 0) {
		const item = document.createElement("li");
		item.className = "plugin-muted";
		item.textContent = "暂无关系";
		root.append(item);
		return;
	}
	const order = { mount: 0, depends: 1 };
	for (const edge of [...model.edges].sort((a, b) =>
		(order[a.type] - order[b.type]) || `${a.from}:${a.to}`.localeCompare(`${b.from}:${b.to}`))) {
		const item = document.createElement("li");
		item.dataset.relation = edge.type;
		item.textContent = `${edge.from} —${edgeLabels[edge.type] || edge.type}→ ${edge.to}`;
		root.append(item);
	}
}
