const bridge = () => {
	const api = globalThis.LiteAgent;
	if (!api) throw new Error("LiteAgent bridge is unavailable");
	return api;
};

async function call(capability, method, payload) {
	return bridge().call(capability, method, payload ?? {});
}

export const state = {
	ready: false,
	workspaces: [],
	workspace: "",
	sessions: [],
	sessionId: "",
	meta: null,
	messages: [],
	facts: [],
	factsTotal: 0,
	factsFilter: "all",
	models: [],
	defaultModel: "",
	selectedModel: "",
	turns: new Map(),
	loading: { sessions: false, messages: false, facts: false, models: false },
	error: "",
	notice: "",
};

const bus = new Map();

export function on(type, handler) {
	let handlers = bus.get(type);
	if (!handlers) {
		handlers = new Set();
		bus.set(type, handlers);
	}
	handlers.add(handler);
	return () => handlers.delete(handler);
}

function emit(type, detail) {
	const handlers = bus.get(type);
	if (!handlers) return;
	for (const handler of [...handlers]) handler(detail);
}

function errorMessage(error) {
	return error instanceof Error ? error.message : String(error ?? "未知错误");
}

export function reportError(scope, error) {
	state.error = `${scope}：${errorMessage(error)}`;
	emit("error", { scope, error });
	emit("state");
}

export function clearError() {
	if (!state.error) return;
	state.error = "";
	emit("state");
}

let noticeTimer = 0;

export function notify(text) {
	state.notice = String(text || "");
	clearTimeout(noticeTimer);
	if (state.notice) {
		noticeTimer = setTimeout(() => {
			state.notice = "";
			emit("state");
		}, 4000);
	}
	emit("state");
}

let initPromise = null;

export function init() {
	if (!initPromise) initPromise = bootstrap();
	return initPromise;
}

async function bootstrap() {
	try {
		await Promise.all([loadWorkspaces(), loadModels()]);
		state.ready = true;
		emit("state");
		await refreshSessions();
	} catch (error) {
		reportError("初始化", error);
	}
}

let resyncPromise = null;

export function resync() {
	if (resyncPromise) return resyncPromise;
	resyncPromise = (async () => {
		try {
			await Promise.all([loadWorkspaces(), loadModels()]);
			await refreshSessions();
			if (state.sessionId) await refreshCurrent();
		} catch (error) {
			reportError("同步", error);
		} finally {
			resyncPromise = null;
		}
	})();
	return resyncPromise;
}

async function loadModels() {
	state.loading.models = true;
	emit("state");
	try {
		const payload = await call("llm", "models.list", {});
		state.models = Array.isArray(payload?.models) ? payload.models : [];
		state.defaultModel = String(payload?.default_model || state.models[0]?.id || "");
		state.selectedModel = state.meta?.model || state.selectedModel || state.defaultModel;
	} catch (error) {
		reportError("模型列表", error);
	} finally {
		state.loading.models = false;
		emit("state");
	}
}

async function loadWorkspaces() {
	const payload = await call("workspace", "list", {});
	state.workspaces = Array.isArray(payload?.workspaces) ? payload.workspaces : [];
	const selected = typeof payload?.selected === "string" ? payload.selected : "";
	if (selected) state.workspace = selected;
	if (!state.workspace) {
		const current = await call("workspace", "current", {});
		state.workspace = current?.workspace?.path || "";
	}
	emit("state");
}

export async function refreshSessions() {
	state.loading.sessions = true;
	emit("state");
	try {
		const payload = await call("session", "list", state.workspace ? { workspace: state.workspace } : {});
		state.sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
	} catch (error) {
		reportError("会话列表", error);
	} finally {
		state.loading.sessions = false;
		emit("state");
	}
}

let selectionSeq = 0;

export function newChat() {
	selectionSeq += 1;
	state.sessionId = "";
	state.meta = null;
	state.messages = [];
	state.facts = [];
	state.factsTotal = 0;
	state.selectedModel = state.defaultModel;
	state.error = "";
	emit("state");
}

export async function selectSession(id) {
	const sessionId = String(id || "");
	if (!sessionId || sessionId === state.sessionId) return;
	selectionSeq += 1;
	const seq = selectionSeq;
	state.sessionId = sessionId;
	state.meta = findSession(sessionId);
	state.selectedModel = state.meta?.model || state.defaultModel;
	state.messages = [];
	state.facts = [];
	state.factsTotal = 0;
	state.error = "";
	emit("state");
	await Promise.all([loadMeta(sessionId, seq), loadMessages(sessionId, seq), loadFacts(sessionId, seq)]);
}

function findSession(id) {
	return state.sessions.find((session) => session?.session_id === id) || null;
}

async function loadMeta(id, seq) {
	try {
		const info = await call("session", "info", { session_id: id });
		if (seq !== selectionSeq) return;
		state.meta = info;
		state.selectedModel = info.model || state.selectedModel || state.defaultModel;
		emit("state");
	} catch (error) {
		if (seq === selectionSeq) reportError("会话信息", error);
	}
}

async function loadMessages(id, seq = selectionSeq) {
	state.loading.messages = true;
	emit("state");
	try {
		const payload = await call("session", "messages", { session_id: id });
		if (seq !== selectionSeq) return;
		state.messages = Array.isArray(payload?.messages) ? payload.messages : [];
	} catch (error) {
		if (seq === selectionSeq) reportError("消息", error);
	} finally {
		if (seq === selectionSeq) {
			state.loading.messages = false;
			emit("state");
		}
	}
}

async function loadFacts(id, seq = selectionSeq) {
	state.loading.facts = true;
	emit("state");
	try {
		const payload = await call("session", "facts", { session_id: id });
		if (seq !== selectionSeq) return;
		state.facts = Array.isArray(payload?.facts) ? payload.facts : [];
		state.factsTotal = Number(payload?.total) || state.facts.length;
	} catch (error) {
		if (seq === selectionSeq) reportError("Facts", error);
	} finally {
		if (seq === selectionSeq) {
			state.loading.facts = false;
			emit("state");
		}
	}
}

export async function refreshCurrent() {
	if (!state.sessionId) return;
	const id = state.sessionId;
	const seq = selectionSeq;
	await Promise.all([loadMeta(id, seq), loadMessages(id, seq), loadFacts(id, seq)]);
}

export async function selectWorkspace(path) {
	const next = String(path || "");
	if (!next || next === state.workspace) return;
	clearError();
	try {
		await call("workspace", "select", { path: next });
		state.workspace = next;
		newChat();
		await refreshSessions();
	} catch (error) {
		reportError("切换工作区", error);
	}
}

export async function addWorkspace() {
	clearError();
	try {
		const picked = await bridge().pickDirectory();
		if (!picked || picked.cancelled || !picked.path) return;
		const payload = await call("workspace", "create", { path: picked.path });
		const selected = payload?.selected || payload?.workspace?.path || picked.path;
		state.workspace = selected;
		newChat();
		await loadWorkspaces();
		await refreshSessions();
		notify(`已添加工作区：${payload?.workspace?.name || selected}`);
	} catch (error) {
		reportError("添加工作区", error);
	}
}

export async function renameSession(id, title) {
	const value = String(title || "").trim();
	if (!value) return;
	clearError();
	try {
		await call("session", "update", { session_id: id, title: value });
		await refreshSessions();
		if (state.sessionId === id) await loadMeta(id, selectionSeq);
	} catch (error) {
		reportError("重命名", error);
	}
}

export async function togglePin(id) {
	const session = findSession(id);
	if (!session) return;
	clearError();
	try {
		await call("session", "update", { session_id: id, pinned: !session.pinned });
		await refreshSessions();
	} catch (error) {
		reportError("置顶", error);
	}
}

export async function deleteSession(id) {
	clearError();
	try {
		await call("session", "delete", { session_id: id });
		if (state.sessionId === id) newChat();
		await refreshSessions();
	} catch (error) {
		reportError("删除会话", error);
	}
}

export function setFactsFilter(filter) {
	const next = ["all", "message", "other"].includes(filter) ? filter : "all";
	if (next === state.factsFilter) return;
	state.factsFilter = next;
	emit("state");
}

export function setSelectedModel(model) {
	const id = String(model || "");
	if (!id || id === state.selectedModel) return;
	state.selectedModel = id;
	emit("state");
}

let newTurnSeq = 0;

export function currentTurn() {
	if (state.sessionId) return state.turns.get(`s:${state.sessionId}`) || null;
	let latest = null;
	for (const turn of state.turns.values()) {
		if (turn.key.startsWith("new:")) latest = turn;
	}
	return latest;
}

export function isSessionRunning(id) {
	const turn = state.turns.get(`s:${id}`);
	return !!turn && (turn.status === "streaming" || turn.status === "stopping");
}

export function isBusy() {
	const turn = currentTurn();
	return !!turn && (turn.status === "streaming" || turn.status === "stopping");
}

function canSend() {
	return !isBusy();
}

function adopt(turn, sessionId) {
	if (!sessionId || turn.sessionId === sessionId) return;
	const wasNew = turn.key.startsWith("new:");
	const oldKey = turn.key;
	turn.sessionId = sessionId;
	turn.key = `s:${sessionId}`;
	state.turns.delete(oldKey);
	state.turns.set(turn.key, turn);
	if (wasNew && !state.sessionId) state.sessionId = sessionId;
}

export async function sendTurn(text, model = "") {
	const input = String(text || "").trim();
	if (!input || !canSend()) return false;
	const workspace = state.meta?.workspace || state.workspace;
	if (!workspace) {
		reportError("发送", new Error("请先选择工作区"));
		return false;
	}
	clearError();

	const previous = currentTurn();
	if (previous) state.turns.delete(previous.key);

	const sessionId = state.sessionId;
	const selectedModel = String(state.meta?.model || model || state.selectedModel || state.defaultModel || "");
	const key = sessionId ? `s:${sessionId}` : `new:${++newTurnSeq}`;
	const turn = {
		key,
		sessionId,
		userText: input,
		assistantText: "",
		status: "streaming",
		error: "",
		userPersisted: false,
	};
	state.turns.set(key, turn);
	emit("state");

	try {
		const result = await bridge().call(
			"loop",
			"turn",
			{ workspace, session_id: sessionId, user_input: input, model: selectedModel },
			(event) => {
				const payload = event?.payload || {};
				if (event?.method === "started" && payload.session_id) {
					adopt(turn, String(payload.session_id));
					emit("state");
					refreshSessions();
					return;
				}
				if (event?.method !== "chunk" || payload.channel !== "content") return;
				const delta = typeof payload.delta === "string" ? payload.delta : "";
				if (!delta) return;
				turn.assistantText += delta;
				emit("chunk", { key: turn.key, delta });
			},
		);

		const resultId = typeof result?.session_id === "string" && result.session_id
			? result.session_id
			: turn.sessionId;
		if (resultId) {
			adopt(turn, resultId);
			if (!state.sessionId) state.sessionId = resultId;
		}
		const finalText = Array.isArray(result?.messages)
			? result.messages
				.filter((message) => message?.role === "assistant")
				.map((message) => String(message.text || ""))
				.join("\n\n")
			: "";
		if (finalText) turn.assistantText = finalText;
		turn.status = "done";
		turn.userPersisted = true;
		emit("state");

		await refreshSessions();
		if (resultId && state.sessionId === resultId) await refreshCurrent();
		state.turns.delete(turn.key);
		emit("state");
	} catch (error) {
		const cancelled = error?.code === "call_cancelled" || turn.status === "stopping";
		turn.status = cancelled ? "stopped" : "error";
		turn.error = cancelled ? "" : errorMessage(error);
		emit("state");
		await refreshSessions();
		if (turn.sessionId && state.sessionId === turn.sessionId) {
			await refreshCurrent();
		}
		turn.userPersisted = state.messages.some((message) =>
			message?.role === "user" && String(message.text || "") === turn.userText);
		emit("state");
		if (cancelled) notify("已停止生成");
		else reportError("本轮", error);
	}
	return true;
}

export async function stopTurn() {
	const turn = currentTurn();
	if (!turn || turn.status !== "streaming") return;
	if (!turn.sessionId) {
		reportError("停止", new Error("会话尚未建立，无法停止"));
		return;
	}
	turn.status = "stopping";
	emit("state");
	try {
		await call("loop", "cancel", { session_id: turn.sessionId });
	} catch (error) {
		turn.status = "streaming";
		reportError("停止", error);
	}
}

export function workspaceLabel(path = state.workspace) {
	const workspace = state.workspaces.find((entry) => entry.path === path);
	if (workspace?.name) return workspace.name;
	const parts = String(path || "").split(/[\\/]+/).filter(Boolean);
	return parts.length ? parts[parts.length - 1] : "未选择工作区";
}
