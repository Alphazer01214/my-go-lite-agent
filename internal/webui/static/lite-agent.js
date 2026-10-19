let fallbackID = 0;

function nextRequestID() {
	if (globalThis.crypto?.randomUUID) {
		return `ui-call-${globalThis.crypto.randomUUID()}`;
	}
	fallbackID += 1;
	return `ui-call-${Date.now()}-${fallbackID}`;
}

// readNDJSON consumes one NDJSON response. The reader is always released, also
// when onFrame throws, so an aborted stream does not leak the connection.
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
	try {
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
	} finally {
		reader.cancel().catch(() => {});
		reader.releaseLock();
	}
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
	maxReconnectDelayMs = 10_000,
} = {}) {
	const uiHandlers = new Set();
	const diagnosticHandlers = new Set();
	const statusHandlers = new Set();
	const resyncHandlers = new Set();
	let uiStarted = false;
	let uiClosed = false;
	let connectedOnce = false;
	let reconnectAttempt = 0;
	let status = "connecting";

	function setStatus(next) {
		if (status === next) return;
		status = next;
		for (const handler of statusHandlers) handler(next);
	}

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
			// Drain and release the body so the connection can be reused.
			await response.body?.cancel().catch(() => {});
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
		if (frame.type === "hello") {
			const reconnected = connectedOnce;
			connectedOnce = true;
			reconnectAttempt = 0;
			setStatus("connected");
			if (reconnected) {
				for (const handler of resyncHandlers) handler(frame);
			}
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
			if (!connectedOnce) setStatus("connecting");
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
			if (uiClosed) return;
			setStatus(connectedOnce ? "reconnecting" : "connecting");
			const delay = Math.min(maxReconnectDelayMs, reconnectDelayMs * 2 ** reconnectAttempt);
			reconnectAttempt += 1;
			await new Promise((resolve) => setTimeout(resolve, delay));
		}
	}

	function onUI(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		uiHandlers.add(handler);
		startUI();
	}

	function onDiagnostic(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		diagnosticHandlers.add(handler);
		startUI();
	}

	function onStatus(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		statusHandlers.add(handler);
		startUI();
		handler(status);
	}

	function onResync(handler) {
		if (typeof handler !== "function") throw new TypeError("handler must be a function");
		resyncHandlers.add(handler);
		startUI();
	}

	// startUI opens the UI stream at most once. After closeUI the runtime is
	// shut down for good, so re-registering a handler must not leave it wired to
	// a loop that will never run again.
	function startUI() {
		if (uiStarted || uiClosed) return;
		uiStarted = true;
		runUI();
	}

	function closeUI() {
		uiClosed = true;
		uiStarted = false;
		uiHandlers.clear();
		diagnosticHandlers.clear();
		statusHandlers.clear();
		resyncHandlers.clear();
	}

	return { call, onUI, onDiagnostic, onStatus, onResync, closeUI };
}

export const LiteAgent = createLiteAgent();

if (typeof window !== "undefined") {
	window.LiteAgent = LiteAgent;
}
