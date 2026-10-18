const stateLabels = Object.freeze({
	inactive: "未激活",
	loading: "加载中",
	active: "运行中",
	stopping: "停止中",
	failed: "失败",
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

function instanceSelector(document, plugin, instance, handlers) {
	const instances = Array.isArray(plugin?.instances) ? plugin.instances : [];
	if (instances.length === 0) return null;
	const wrap = document.createElement("div");
	wrap.className = "plugin-instances";
	wrap.dataset.label = "实例";
	const label = document.createElement("span");
	label.className = "plugin-detail-label";
	label.textContent = "实例";
	const select = document.createElement("select");
	select.className = "plugin-instance-select";
	select.dataset.plugin = plugin.name || "";
	for (const item of instances) {
		const option = document.createElement("option");
		option.value = item.instance_id || "";
		option.textContent = `${item.instance_id || "实例"} · G${item.generation || 0} · ${stateLabels[item.state] || item.state || "未知"}`;
		select.append(option);
	}
	select.value = instance?.instance_id || "";
	select.addEventListener("change", () => {
		handlers?.onSelectInstance?.(plugin.name, select.value);
	});
	wrap.append(label, select);
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

function appendTextList(document, parent, className, label, values, emptyText, format = (value) => value) {
	const list = document.createElement("div");
	list.className = className;
	list.dataset.label = label;
	const heading = document.createElement("span");
	heading.className = "plugin-detail-label";
	heading.textContent = label;
	list.append(heading);
	const entries = Array.isArray(values) ? values.filter(Boolean) : [];
	if (entries.length === 0) {
		const empty = document.createElement("span");
		empty.className = "plugin-muted";
		empty.textContent = emptyText;
		list.append(empty);
		parent.append(list);
		return;
	}
	for (const value of entries) {
		const item = document.createElement("span");
		item.className = `${className}-item`;
		item.textContent = format(value);
		list.append(item);
	}
	parent.append(list);
}

function pluginAction(document, plugin, handlers) {
	if (plugin.missing) return null;
	const state = displayState(plugin, selectedProviderInstance(plugin, handlers));
	const stopping = state === "active" || state === "loading" || state === "stopping";
	const button = document.createElement("button");
	button.type = "button";
	button.className = stopping ? "plugin-action plugin-stop" : "plugin-action plugin-start";
	button.textContent = stopping ? "停止" : "启动";
	button.dataset.action = stopping ? "stop" : "start";
	button.dataset.plugin = plugin.name || "";
	button.title = stopping ? `停止 ${plugin.name || "插件"}` : `启动 ${plugin.name || "插件"}`;
	button.setAttribute("aria-label", button.title);
	button.addEventListener("click", () => {
		const action = stopping ? handlers.onStop : handlers.onStart;
		action?.(plugin, button);
	});
	return button;
}

export function renderPluginList(document, list, plugins, handlers = {}) {
	const entries = Array.isArray(plugins) ? plugins : [];
	list.replaceChildren();

	if (entries.length === 0) {
		const empty = document.createElement("li");
		empty.className = "plugin-empty";
		empty.textContent = "暂无插件";
		list.append(empty);
		return;
	}

	for (const plugin of entries) {
		const row = document.createElement("li");
		row.className = "plugin-row";
		row.dataset.plugin = plugin.name || "";

		const primary = document.createElement("div");
		primary.className = "plugin-primary";
		const name = document.createElement("strong");
		name.textContent = plugin.name || "未命名插件";
		primary.append(name);
		if (plugin.version) {
			const version = document.createElement("span");
			version.className = "plugin-version";
			version.textContent = plugin.version;
			primary.append(version);
		}
		const action = pluginAction(document, plugin, handlers);
		if (action) primary.append(action);
		row.append(primary);

		const selectedInstance = selectedProviderInstance(plugin, handlers);
		const status = document.createElement("div");
		status.className = "plugin-status";
		const state = document.createElement("span");
		const stateName = displayState(plugin, selectedInstance);
		state.className = `state-badge state-${stateName.replace(/[^a-z0-9_-]/g, "-") || "unknown"}`;
		state.textContent = stateLabels[stateName] || stateName || "未知";
		status.append(state);
		if (plugin.missing) {
			const missing = document.createElement("span");
			missing.className = "plugin-missing state-badge state-missing";
			missing.textContent = "缺失";
			status.append(missing);
		}
		const selector = instanceSelector(document, plugin, selectedInstance, handlers);
		if (selector) status.append(selector);
		const enabled = document.createElement("span");
		enabled.className = plugin.enabled ? "plugin-enabled" : "plugin-disabled";
		enabled.textContent = plugin.enabled ? "启用" : "禁用";
		status.append(enabled);
		row.append(status);

		appendTextList(
			document,
			row,
			"plugin-registrations",
			"Registration",
			selectedInstance?.registrations,
			"无 Registration",
			(item) => `${item?.capability || ""}.${item?.method || ""}`,
		);
		if (selectedInstance?.failure_message) {
			const failure = document.createElement("div");
			failure.className = "plugin-failure";
			failure.textContent = `${selectedInstance.failure_code || "failed"}: ${selectedInstance.failure_message}`;
			row.append(failure);
		}
		appendTextList(document, row, "plugin-capabilities", "Capability", plugin.provides, "无 Capability");
		appendTextList(document, row, "plugin-dependencies", "依赖", plugin.depends_on, "无硬依赖");
		appendTextList(
			document,
			row,
			"plugin-mounts",
			"UI Mount",
			plugin.mounts,
			"无 UI Mount",
			(mount) => `${mount?.plugin || plugin.name || ""}:${mount?.id || ""}`,
		);

		list.append(row);
	}
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
	root.replaceChildren();
	if (model.nodes.length === 0) {
		const empty = document.createElement("p");
		empty.className = "plugin-empty";
		empty.textContent = "暂无可视化的插件依赖";
		root.append(empty);
		return model;
	}

	const svg = svgElement(document, "svg", {
		viewBox: `0 0 ${model.width} ${model.height}`,
		role: "img",
		"aria-label": "Host 与插件关系图",
		preserveAspectRatio: "xMidYMid meet",
	});
	const title = svgElement(document, "title");
	title.textContent = "Host 与插件关系图";
	svg.append(title);
	const desc = svgElement(document, "desc");
	desc.textContent = "实线箭头表示硬依赖，虚线箭头表示 Host 装载。点击插件可查看关系详情。";
	svg.append(desc);

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
		});
		if (node.kind === "plugin") {
			group.setAttribute("tabindex", "0");
			group.addEventListener("click", (event) => {
				event.stopPropagation();
				onSelect?.(isSelected ? "" : node.id);
			});
			group.addEventListener("keydown", (event) => {
				if (event.key === "Enter" || event.key === " ") {
					event.preventDefault();
					onSelect?.(isSelected ? "" : node.id);
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
	return model;
}

function detailRow(document, root, label, values, emptyText) {
	const row = document.createElement("div");
	row.className = "graph-detail-row";
	const term = document.createElement("span");
	term.className = "plugin-detail-label";
	term.textContent = label;
	row.append(term);
	const entries = Array.isArray(values) ? values.filter(Boolean) : [];
	const value = document.createElement("span");
	value.className = entries.length > 0 ? "graph-detail-value" : "plugin-muted";
	value.textContent = entries.length > 0 ? entries.join("、") : emptyText;
	row.append(value);
	root.append(row);
}

export function renderDependencyDetails(document, root, model, selectedId) {
	root.replaceChildren();
	const node = model.nodes.find((item) => item.id === selectedId && item.kind === "plugin");
	if (!node) {
		const empty = document.createElement("p");
		empty.className = "graph-detail-empty";
		empty.textContent = "选择插件节点，查看它的 Capability、依赖和 Mount。";
		root.append(empty);
		return;
	}

	const heading = document.createElement("div");
	heading.className = "graph-detail-heading";
	const name = document.createElement("strong");
	name.textContent = node.id;
	heading.append(name);
	const state = document.createElement("span");
	state.className = `state-badge state-${String(node.state || "unknown").replace(/[^a-z0-9_-]/g, "-")}`;
	state.textContent = node.missing ? "缺失" : (stateLabels[node.state] || node.state);
	heading.append(state);
	root.append(heading);

	detailRow(document, root, "Capability", node.provides, "无 Capability");
	detailRow(document, root, "硬依赖", node.dependsOn, "无硬依赖");
	detailRow(document, root, "被依赖", node.usedBy, "无依赖方");
	detailRow(
		document,
		root,
		"UI Mount",
		node.mounts.map((mount) => `${mount?.plugin || node.id}:${mount?.id || ""}`),
		"无 UI Mount",
	);
}

export function renderDependencyEdgeList(document, root, plugins) {
	const model = relationshipGraphModel(plugins);
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
