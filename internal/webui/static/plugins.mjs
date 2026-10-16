const stateLabels = Object.freeze({
	mounted: "已挂载",
	available: "可用",
	degraded: "降级",
	disabled: "已禁用",
	missing: "缺失",
});

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
	if (plugin.state === "missing") return null;
	const stopping = plugin.state === "mounted";
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

		const status = document.createElement("div");
		status.className = "plugin-status";
		const state = document.createElement("span");
		const stateName = typeof plugin.state === "string" ? plugin.state : "";
		state.className = `state-badge state-${stateName.replace(/[^a-z0-9_-]/g, "-") || "unknown"}`;
		state.textContent = stateLabels[stateName] || stateName || "未知";
		status.append(state);
		const enabled = document.createElement("span");
		enabled.className = plugin.enabled ? "plugin-enabled" : "plugin-disabled";
		enabled.textContent = plugin.enabled ? "启用" : "禁用";
		status.append(enabled);
		row.append(status);

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

export function dependencyGraphModel(plugins) {
	const items = Array.isArray(plugins) ? plugins : [];
	const names = [...new Set(items.map((plugin) => plugin?.name).filter(Boolean))].sort();
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

	const maxRows = Math.max(1, ...[...groups.values()].map((group) => group.length));
	const maxDepth = Math.max(0, ...names.map((name) => depths.get(name) || 0));
	const width = Math.max(560, 120 + maxDepth * 230);
	const height = Math.max(260, 80 + maxRows * 92);
	const nodes = names.map((name) => {
		const plugin = byName.get(name);
		const depth = depths.get(name) || 0;
		const group = groups.get(depth) || [name];
		const index = group.indexOf(name);
		const groupHeight = group.length * 92;
		return {
			id: name,
			state: plugin?.state || "missing",
			depth,
			x: 70 + depth * 230,
			y: 46 + (height - 80 - groupHeight) / 2 + index * 92,
			missing: !plugin,
		};
	});
	const edges = [];
	for (const plugin of items) {
		for (const dependency of plugin?.depends_on || []) {
			edges.push({ from: plugin.name, to: dependency });
		}
	}
	return { nodes, edges, width, height };
}

function svgElement(document, tag, attributes = {}) {
	const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
	for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, String(value));
	return node;
}

export function renderDependencyGraph(document, root, plugins) {
	const model = dependencyGraphModel(plugins);
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
		"aria-label": "插件依赖关系图",
	});
	const defs = svgElement(document, "defs");
	const marker = svgElement(document, "marker", {
		id: "plugin-arrow",
		viewBox: "0 0 10 10",
		refX: "9",
		refY: "5",
		markerWidth: "7",
		markerHeight: "7",
		orient: "auto-start-reverse",
	});
	marker.append(svgElement(document, "path", { d: "M 0 0 L 10 5 L 0 10 z", fill: "#8090a0" }));
	defs.append(marker);
	svg.append(defs);

	const nodeIndex = new Map(model.nodes.map((node) => [node.id, node]));
	for (const edge of model.edges) {
		const from = nodeIndex.get(edge.from);
		const to = nodeIndex.get(edge.to);
		if (!from || !to) continue;
		const startX = from.x - 82;
		const endX = to.x + 82;
		const path = svgElement(document, "path", {
			d: `M ${startX} ${from.y} C ${startX - 45} ${from.y}, ${endX + 45} ${to.y}, ${endX} ${to.y}`,
			fill: "none",
			stroke: "#8090a0",
			"stroke-width": "1.5",
			"marker-end": "url(#plugin-arrow)",
		});
		svg.append(path);
	}

	for (const node of model.nodes) {
		const group = svgElement(document, "g", {
			transform: `translate(${node.x} ${node.y})`,
			"data-plugin": node.id,
		});
		group.append(svgElement(document, "rect", {
			x: "-82", y: "-28", width: "164", height: "56", rx: "8",
			class: `graph-node graph-${node.state}`,
		}));
		const title = svgElement(document, "text", {
			x: "0", y: "-3", "text-anchor": "middle", class: "graph-title",
		});
		title.textContent = node.id;
		group.append(title);
		const state = svgElement(document, "text", {
			x: "0", y: "17", "text-anchor": "middle", class: "graph-state",
		});
		state.textContent = node.missing ? "缺失" : (stateLabels[node.state] || node.state);
		group.append(state);
		svg.append(group);
	}
	root.append(svg);
	return model;
}

export function renderDependencyEdgeList(document, root, plugins) {
	const model = dependencyGraphModel(plugins);
	root.replaceChildren();
	if (model.edges.length === 0) {
		const item = document.createElement("li");
		item.className = "plugin-muted";
		item.textContent = "暂无硬依赖关系";
		root.append(item);
		return;
	}
	for (const edge of [...model.edges].sort((a, b) => `${a.from}:${a.to}`.localeCompare(`${b.from}:${b.to}`))) {
		const item = document.createElement("li");
		item.textContent = `${edge.from} → ${edge.to}`;
		root.append(item);
	}
}
