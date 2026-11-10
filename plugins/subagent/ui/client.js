export async function callCapability(capability, method, payload = {}) {
	const api = globalThis.LiteAgent;
	if (!api || typeof api.call !== "function") {
		throw new Error("LiteAgent bridge is unavailable");
	}
	return api.call(capability, method, payload ?? {});
}

export async function callSubagent(method, payload = {}) {
	return callCapability("subagent", method, payload);
}

export function node(tag, className, text) {
	const element = document.createElement(tag);
	if (className) element.className = className;
	if (text !== undefined && text !== null) element.textContent = String(text);
	return element;
}

export function clear(target) {
	target?.replaceChildren?.();
}

export function formatNumber(value) {
	const number = Number(value);
	if (!Number.isFinite(number)) return "—";
	return number.toLocaleString();
}

export function formatTime(value) {
	if (!value) return "—";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return String(value);
	return date.toLocaleString();
}

export function stringify(value) {
	try {
		return JSON.stringify(value, null, 2);
	} catch {
		return String(value);
	}
}

export function errorMessage(error) {
	return error instanceof Error ? error.message : String(error ?? "unknown error");
}
