function isoTimestamp(value) {
	const date = value instanceof Date ? value : new Date(value);
	return Number.isNaN(date.getTime()) ? new Date(0).toISOString() : date.toISOString();
}

export function buildFactsExport(sessionId, facts, exportedAt = new Date()) {
	const items = Array.isArray(facts) ? facts : [];
	return {
		session_id: String(sessionId || ""),
		exported_at: isoTimestamp(exportedAt),
		fact_count: items.length,
		facts: items,
	};
}

export function serializeFactsExport(sessionId, facts, exportedAt = new Date()) {
	return `${JSON.stringify(buildFactsExport(sessionId, facts, exportedAt), null, 2)}\n`;
}

export function factsExportFilename(sessionId, factCount, exportedAt = new Date()) {
	const date = exportedAt instanceof Date ? exportedAt : new Date(exportedAt);
	const safeDate = Number.isNaN(date.getTime()) ? new Date(0) : date;
	const stamp = safeDate.toISOString()
		.replace(/[-:]/g, "")
		.replace(/\.\d{3}Z$/, "Z");
	const safeSessionId = String(sessionId || "session").replace(/[^a-zA-Z0-9._-]+/g, "-");
	const count = Number.isFinite(Number(factCount)) ? Math.max(0, Number(factCount)) : 0;
	return `${safeSessionId}-facts-${count}-${stamp}.json`;
}
