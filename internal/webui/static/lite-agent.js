let fallbackID = 0;

function nextRequestID() {
	if (globalThis.crypto?.randomUUID) {
		return `ui-call-${globalThis.crypto.randomUUID()}`;
	}
	fallbackID += 1;
	return `ui-call-${Date.now()}-${fallbackID}`;
}

async function readNDJSON(response, onFrame) {
	const reader = response.body?.getReader();
	if (!reader) {
		const text = await response.text();
		for (const line of text.split(/\r?\n/)) {
			if (line.trim()) onFrame(JSON.parse(line));
		}
		return;
	}

	const decoder = new TextDecoder();
	let buffer = "";
	for (;;) {
		const { value, done } = await reader.read();
		buffer += decoder.decode(value ?? new Uint8Array(), { stream: !done });
		const lines = buffer.split(/\r?\n/);
		buffer = lines.pop() ?? "";
		for (const line of lines) {
			if (line.trim()) onFrame(JSON.parse(line));
		}
		if (done) break;
	}
	if (buffer.trim()) onFrame(JSON.parse(buffer));
}

function frameError(frame) {
	const error = new Error(frame.error_msg || "Browser Call failed");
	error.code = frame.error_code || "handler_error";
	error.response = frame;
	return error;
}

export function createLiteAgent({
	fetchImpl = globalThis.fetch?.bind(globalThis),
	newRequestID = nextRequestID,
	reconnectDelayMs = 1000,
} = {}) {
	const uiHandlers = new Set();
	const diagnosticHandlers = new Set();
	const evictedHandlers = new Set();
	let uiStarted = false;
	let uiClosed = false;

	async function call(capability, method, payload = {}, onEvent) {
		if (!capability || !method) {
			throw new TypeError("capability and method are required");
		}
		if (onEvent !== undefined && typeof onEvent !== "function") {
			throw new TypeError("onEvent must be a function");
		}

		const id = newRequestID();
		const response = await fetchImpl("/bridge/call", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				Accept: "application/x-ndjson",
			},
			body: JSON.stringify({
				version: 1,
				id,
				type: "req",
				capability,
				method,
				payload: payload ?? {},
			}),
		});
		if (!response.ok) {
			throw new Error(`Browser Call failed with HTTP ${response.status}`);
		}

		let result;
		let sawResponse = false;
		await readNDJSON(response, (frame) => {
			if (frame.id !== id) {
				throw new Error(`unexpected Browser Call id ${frame.id}`);
			}
			if (sawResponse) {
				throw new Error("Browser Call received data after response");
			}
			if (frame.type === "evt") {
				onEvent?.(frame);
				return;
			}
			if (frame.type !== "res") {
				throw new Error(`unexpected Browser Call frame type ${frame.type}`);
			}
			sawResponse = true;
			if (frame.error_code) {
				throw frameError(frame);
			}
			result = frame.payload;
		});
		if (!sawResponse) {
			throw new Error("Browser Call completed without a response");
		}
		return result;
	}

	function dispatchUI(frame) {
		if (frame.type === "evicted") {
			// A newer page took the single v1 UI connection. Stop reconnecting so
			// the two pages do not take turns stealing the stream.
			uiClosed = true;
			for (const handler of evictedHandlers) handler(frame);
			return;
		}
		if (frame.type === "ui") {
			for (const handler of uiHandlers) handler(frame);
			return;
		}
		if (frame.type === "diagnostic") {
			for (const handler of diagnosticHandlers) handler(frame.diagnostic);
		}
	}

	async function runUI() {
		while (!uiClosed) {
			try {
				const response = await fetchImpl("/bridge/ui", {
					headers: { Accept: "application/x-ndjson" },
				});
				if (!response.ok) {
					throw new Error(`UI stream failed with HTTP ${response.status}`);
				}
				await readNDJSON(response, dispatchUI);
			} catch {
				// The Shell reconnects after transport failure.
			}
			if (!uiClosed) {
				await new Promise((resolve) => setTimeout(resolve, reconnectDelayMs));
			}
		}
	}

	function onUI(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		uiHandlers.add(handler);
		if (!uiStarted) {
			uiStarted = true;
			runUI();
		}
	}

	function onDiagnostic(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		diagnosticHandlers.add(handler);
		if (!uiStarted) {
			uiStarted = true;
			runUI();
		}
	}

	function onEvicted(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		evictedHandlers.add(handler);
		if (!uiStarted) {
			uiStarted = true;
			runUI();
		}
	}

	function mountKey(element) {
		return element?.dataset?.mountKey ?? null;
	}

	function closeUI() {
		uiClosed = true;
		uiHandlers.clear();
		diagnosticHandlers.clear();
		evictedHandlers.clear();
	}

	return { call, onUI, onDiagnostic, onEvicted, mountKey, closeUI };
}

export const LiteAgent = createLiteAgent();

if (typeof window !== "undefined") {
	window.LiteAgent = LiteAgent;
}
