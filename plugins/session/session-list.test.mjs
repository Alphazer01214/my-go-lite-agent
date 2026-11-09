import assert from "node:assert/strict";
import test from "node:test";

import { sessionListRequest } from "./ui/store.js";

test("session list includes subagent sessions", () => {
	assert.deepEqual(sessionListRequest("/workspace"), {
		include_subagents: true,
		workspace: "/workspace",
	});
	assert.deepEqual(sessionListRequest(""), { include_subagents: true });
});
