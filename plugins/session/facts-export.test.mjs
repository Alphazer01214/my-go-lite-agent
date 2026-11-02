import assert from "node:assert/strict";
import test from "node:test";

import {
	buildFactsExport,
	factsExportFilename,
	serializeFactsExport,
} from "./ui/facts-export.js";

test("buildFactsExport keeps selected fact order and adds export metadata", () => {
	const facts = [
		{ seq: 2, type: "message", text_content: "hello" },
		{ seq: 5, type: "turn_end" },
	];
	const exportedAt = new Date("2026-11-02T05:06:07.000Z");

	const payload = buildFactsExport("s-123", facts, exportedAt);

	assert.deepEqual(payload, {
		session_id: "s-123",
		exported_at: "2026-11-02T05:06:07.000Z",
		fact_count: 2,
		facts,
	});
});

test("factsExportFilename sanitizes session id and includes count and time", () => {
	const filename = factsExportFilename(
		"s/123:demo",
		2,
		new Date("2026-11-02T05:06:07.000Z"),
	);

	assert.equal(filename, "s-123-demo-facts-2-20261102T050607Z.json");
});

test("serializeFactsExport matches copy and download JSON content", () => {
	const facts = [{ seq: 3, type: "message", text_content: "hello" }];
	const serialized = serializeFactsExport(
		"s-123",
		facts,
		new Date("2026-11-02T05:06:07.000Z"),
	);

	assert.equal(
		serialized,
		`${JSON.stringify(buildFactsExport(
			"s-123",
			facts,
			new Date("2026-11-02T05:06:07.000Z"),
		), null, 2)}\n`,
	);
});
