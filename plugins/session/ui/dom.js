export function el(tag, props = {}, ...children) {
	const node = document.createElement(tag);
	for (const [key, value] of Object.entries(props || {})) {
		if (value === null || value === undefined || value === false) continue;
		if (key === "class") node.className = value;
		else if (key === "text") node.textContent = String(value);
		else if (key === "dataset") Object.assign(node.dataset, value);
		else if (key === "attrs") {
			for (const [name, attrValue] of Object.entries(value)) {
				if (attrValue !== null && attrValue !== undefined && attrValue !== false) {
					node.setAttribute(name, String(attrValue));
				}
			}
		} else if (key.startsWith("on") && typeof value === "function") {
			node.addEventListener(key.slice(2).toLowerCase(), value);
		} else if (key in node) {
			node[key] = value;
		} else {
			node.setAttribute(key, String(value));
		}
	}
	for (const child of children.flat(Infinity)) {
		if (child === null || child === undefined || child === false) continue;
		node.append(child instanceof Node ? child : document.createTextNode(String(child)));
	}
	return node;
}

export function timeOfDay(timestamp) {
	const value = Number(timestamp);
	if (!value) return "";
	const date = new Date(value);
	return date.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

export function relativeTime(timestamp) {
	const value = Number(timestamp);
	if (!value) return "";
	const diff = Date.now() - value;
	if (diff < 60_000) return "刚刚";
	if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
	const date = new Date(value);
	const today = new Date();
	const startToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
	if (value >= startToday) return timeOfDay(value);
	if (value >= startToday - 86_400_000) return "昨天";
	if (value >= startToday - 7 * 86_400_000) return `${Math.floor((startToday - value) / 86_400_000)} 天前`;
	return `${date.getMonth() + 1}/${date.getDate()}`;
}

export function dayBucket(timestamp) {
	const value = Number(timestamp);
	const today = new Date();
	const startToday = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime();
	if (value >= startToday) return "今天";
	if (value >= startToday - 86_400_000) return "昨天";
	if (value >= startToday - 7 * 86_400_000) return "7 天内";
	return "更早";
}

export function shortPath(path) {
	const parts = String(path || "").split(/[\\/]+/).filter(Boolean);
	return parts.length ? parts[parts.length - 1] : "";
}
