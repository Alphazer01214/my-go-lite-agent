import test from "node:test";
import assert from "node:assert/strict";

import { createLiteAgent } from "./static/lite-agent.js";

function ndjsonResponse(frames, status = 200) {
	const body = new ReadableStream({
		start(controller) {
			const encoder = new TextEncoder();
			for (const frame of frames) {
				controller.enqueue(encoder.encode(`${JSON.stringify(frame)}\n`));
			}
			controller.close();
		},
	});
	return new Response(body, {
		status,
		headers: { "Content-Type": "application/x-ndjson" },
	});
}

test("LiteAgent.call streams events then resolves response payload", async () => {
	let request;
	const agent = createLiteAgent({
		newRequestID: () => "ui-call-fixed",
		fetchImpl: async (url, options) => {
			request = { url, options };
			return ndjsonResponse([
				{
					version: 1,
					id: "ui-call-fixed",
					type: "evt",
					from: "session",
					capability: "session",
					method: "progress",
					payload: { step: 1 },
				},
				{
					version: 1,
					id: "ui-call-fixed",
					type: "res",
					from: "session",
					capability: "session",
					method: "query",
					payload: { ok: true },
					error_code: "",
					error_msg: "",
				},
			]);
		},
	});

	const events = [];
	const payload = await agent.call("session", "query", { session_id: "s1" }, (event) => {
		events.push(event);
	});

	assert.equal(request.url, "/bridge/call");
	assert.equal(request.options.method, "POST");
	assert.deepEqual(JSON.parse(request.options.body), {
		version: 1,
		id: "ui-call-fixed",
		type: "req",
		capability: "session",
		method: "query",
		payload: { session_id: "s1" },
	});
	assert.equal(events.length, 1);
	assert.equal(events[0].type, "evt");
	assert.deepEqual(payload, { ok: true });
});

test("LiteAgent.call rejects error and malformed streams", async () => {
	const errorAgent = createLiteAgent({
		newRequestID: () => "ui-call-error",
		fetchImpl: async () => ndjsonResponse([{
			version: 1,
			id: "ui-call-error",
			type: "res",
			capability: "ghost",
			method: "run",
			error_code: "method_not_found",
			error_msg: "missing",
		}]),
	});
	await assert.rejects(
		errorAgent.call("ghost", "run", {}),
		(error) => error.code === "method_not_found",
	);

	const malformedAgent = createLiteAgent({
		newRequestID: () => "ui-call-bad-id",
		fetchImpl: async () => ndjsonResponse([{
			version: 1,
			id: "wrong",
			type: "res",
			capability: "demo",
			method: "run",
			payload: {},
		}]),
	});
	await assert.rejects(malformedAgent.call("demo", "run", {}), /unexpected Browser Call id/);
});

test("LiteAgent reports connection status and resyncs after reconnect", async () => {
	let calls = 0;
	let resyncs = 0;
	const statuses = [];
	let resolveResync;
	const resynced = new Promise((resolve) => {
		resolveResync = resolve;
	});
	const agent = createLiteAgent({
		reconnectDelayMs: 1,
		maxReconnectDelayMs: 1,
		fetchImpl: async () => {
			calls += 1;
			if (calls === 1) {
				return ndjsonResponse([
					{ version: 1, id: "hello-1", type: "hello" },
				]);
			}
			return ndjsonResponse([
				{ version: 1, id: "hello-2", type: "hello" },
				{
					version: 1,
					id: "ui-push-1",
					type: "ui",
					mount: { plugin: "demo", id: "main" },
					kind: "state",
					payload: {},
				},
			]);
		},
	});
	agent.onStatus((status) => statuses.push(status));
	agent.onResync(() => {
		resyncs += 1;
		resolveResync();
		agent.closeUI();
	});

	await resynced;
	assert.equal(resyncs, 1);
	assert.ok(statuses.includes("connecting"));
	assert.ok(statuses.includes("connected"));
	assert.ok(statuses.includes("reconnecting"));
	assert.ok(calls >= 2);
});

test("LiteAgent dispatches UI and diagnostic pushes", async () => {
	const seen = new Promise((resolve) => {
		const uiFrames = [];
		const diagnostics = [];
		const agent = createLiteAgent({
			reconnectDelayMs: 1,
			fetchImpl: async () => ndjsonResponse([
				{
					version: 1,
					id: "ui-push-1",
					type: "ui",
					mount: { plugin: "demo", id: "main" },
					kind: "state",
					payload: { revision: 1 },
				},
				{
					version: 1,
					id: "diag-1",
					type: "diagnostic",
					diagnostic: { plugin: "demo", code: "warning", message: "hello" },
				},
			]),
		});
		agent.onUI((frame) => {
			uiFrames.push(frame);
			if (uiFrames.length === 1 && diagnostics.length === 1) {
				agent.closeUI();
				resolve({ uiFrames, diagnostics });
			}
		});
		agent.onDiagnostic((diagnostic) => {
			diagnostics.push(diagnostic);
			if (uiFrames.length === 1 && diagnostics.length === 1) {
				agent.closeUI();
				resolve({ uiFrames, diagnostics });
			}
		});
	});

	const result = await seen;
	assert.equal(result.uiFrames.length, 1);
	assert.equal(result.uiFrames[0].kind, "state");
	assert.equal(result.diagnostics[0].code, "warning");
});
