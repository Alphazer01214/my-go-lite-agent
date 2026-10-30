import test from "node:test";
import assert from "node:assert/strict";

import { collectSettingsValues } from "./settings.mjs";

test("collectSettingsValues keeps list booleans and blank secrets", () => {
	const schema = [{
		key: "models",
		type: "list",
		item_fields: [
			{ key: "id", type: "string" },
			{ key: "api_key", type: "secret" },
			{ key: "vision", type: "boolean" },
		],
	}];
	const values = collectSettingsValues(schema, {
		models: [{ id: "model-a", api_key: "", vision: false }],
	});
	assert.deepEqual(values.models, [{ id: "model-a", api_key: "", vision: false }]);
});
