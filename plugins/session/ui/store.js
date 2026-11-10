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
	tabs: [],
	meta: null,
	messages: [],
	messageCount: 0,
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

const ERROR_MESSAGES = {
	llm_upstream_error: "模型服务请求失败，请检查模型端点或网络",
	llm_timeout: "模型响应超时，请重试或换用更快的模型",
	llm_output_limit_exceeded: "模型输出超过上限，请缩小任务后重试",
	call_deadline_exceeded: "操作超时，请重试",
	turn_busy: "该会话已有一轮正在运行",
	max_steps_exceeded: "达到最大步数限制，请缩小任务后重试",
	tool_call_invalid: "模型工具调用格式无效",
	plugin_down: "所需插件未运行，请在插件面板检查",
	plugin_not_mounted: "所需插件未挂载",
	method_not_found: "插件能力不可用",
	bad_arguments: "请求参数无效",
	bad_payload: "请求内容无效",
	handler_error: "执行失败",
};

export function errorCode(error) {
	return String(error?.code || error?.response?.error_code || "");
}

export function friendlyErrorMessage(code, fallback = "执行失败") {
	return ERROR_MESSAGES[String(code || "")] || fallback;
}

export function reportError(scope, error) {
	const code = errorCode(error);
	const detail = errorMessage(error);
	const text = code && ERROR_MESSAGES[code] ? `${friendlyErrorMessage(code)}（${code}）` : detail;
	state.error = `${scope}：${text}`;
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

const TAB_STORAGE_KEY = "liteagent.session.tabs.v1";

function tabStorage() {
	try {
		return globalThis.sessionStorage || null;
	} catch {
		return null;
	}
}

function readTabStore() {
	const storage = tabStorage();
	if (!storage) return {};
	try {
		const parsed = JSON.parse(storage.getItem(TAB_STORAGE_KEY) || "{}");
		return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
	} catch {
		return {};
	}
}

function writeTabStore(store) {
	const storage = tabStorage();
	if (!storage) return;
	try {
		storage.setItem(TAB_STORAGE_KEY, JSON.stringify(store));
	} catch {
		// 存储不可用时退化为仅内存状态。
	}
}

function normalizeTabs(tabs) {
	const seen = new Set();
	const result = [];
	for (const key of Array.isArray(tabs) ? tabs : []) {
		if (typeof key !== "string") continue;
		if (seen.has(key)) continue;
		seen.add(key);
		result.push(key);
	}
	return result;
}

function saveTabState(workspace = state.workspace) {
	if (!workspace) return;
	const store = readTabStore();
	store[workspace] = {
		tabs: normalizeTabs(state.tabs),
		active: state.sessionId,
	};
	writeTabStore(store);
}

function loadTabState(workspace) {
	if (!workspace) return null;
	const value = readTabStore()[workspace];
	if (!value || !Array.isArray(value.tabs)) return null;
	return {
		tabs: normalizeTabs(value.tabs),
		active: typeof value.active === "string" ? value.active : "",
	};
}

function rememberTab(key) {
	const tabs = normalizeTabs([...state.tabs, key]);
	if (tabs.length === state.tabs.length) return;
	state.tabs = tabs;
	saveTabState();
}

function resetCurrent() {
	selectionSeq += 1;
	state.sessionId = "";
	state.meta = null;
	state.messages = [];
	state.messageCount = 0;
	state.facts = [];
	state.factsTotal = 0;
	state.selectedModel = state.defaultModel;
	state.error = "";
}

let initPromise = null;

export function init() {
	if (!initPromise) initPromise = bootstrap();
	return initPromise;
}

async function bootstrap() {
	try {
		bindCallLifecycle();
		await Promise.all([loadWorkspaces(), loadModels()]);
		state.ready = true;
		emit("state");
		await refreshSessions();
		await restoreTabState();
		await recoverCalls();
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
			await recoverCalls();
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
		await syncSessions();
	} catch (error) {
		reportError("会话列表", error);
	} finally {
		state.loading.sessions = false;
		emit("state");
	}
}

async function syncSessions() {
	const payload = await call("session", "list", sessionListRequest(state.workspace));
	state.sessions = Array.isArray(payload?.sessions) ? payload.sessions : [];
	await pruneTabs();
}

export function sessionListRequest(workspace) {
	const payload = { include_subagents: true };
	if (workspace) payload.workspace = workspace;
	return payload;
}

async function pruneTabs() {
	if (!state.tabs.length) return;
	const previous = state.tabs;
	const activeMissing = Boolean(
		state.sessionId
		&& !findSession(state.sessionId)
		&& !isSessionRunning(state.sessionId));
	let next = previous.filter((key) =>
		key === "" || isSessionRunning(key) || Boolean(findSession(key)) || (key === state.sessionId && !activeMissing));
	if (activeMissing) {
		const index = previous.indexOf(state.sessionId);
		next = next.filter((key) => key !== state.sessionId);
		state.tabs = normalizeTabs(next);
		saveTabState();
		const fallback = state.tabs[index] ?? state.tabs[index - 1] ?? state.tabs[0] ?? "";
		if (fallback === "") newChat();
		else await selectSession(fallback);
		emit("state");
		return;
	}
	if (next.length !== previous.length) {
		state.tabs = normalizeTabs(next);
		saveTabState();
		emit("state");
	}
}

async function restoreTabState() {
	const saved = loadTabState(state.workspace);
	if (!saved) return;
	const tabs = saved.tabs.filter((key) => key === "" || Boolean(findSession(key)));
	let active = saved.active;
	if (active && active !== "" && !findSession(active)) active = "";
	if (active && !tabs.includes(active)) tabs.push(active);
	if (!tabs.length) return;
	state.tabs = normalizeTabs(tabs);
	saveTabState();
	emit("state");
	if (active && active !== "") {
		await selectSession(active);
		return;
	}
	if (active === "" && state.tabs.includes("")) {
		resetCurrent();
		emit("state");
		saveTabState();
		return;
	}
	const first = state.tabs[0];
	if (first) await activateTab(first);
}

export function tabTitle(key) {
	if (!key) return "新会话";
	if (state.sessionId === key && state.meta?.title) return String(state.meta.title);
	const session = findSession(key);
	return String(session?.title || "会话").trim() || "会话";
}

export async function activateTab(key) {
	const target = String(key ?? "");
	if (!target) {
		newChat();
		return;
	}
	await selectSession(target);
}

export function closeTab(key) {
	const target = String(key ?? "");
	const running = target === "" ? state.sessionId === "" && isBusy() : isSessionRunning(target);
	if (running) return false;
	const index = state.tabs.indexOf(target);
	if (index < 0) return false;
	const wasActive = target === state.sessionId;
	state.tabs = state.tabs.filter((_, itemIndex) => itemIndex !== index);
	saveTabState();
	if (!wasActive) {
		emit("state");
		return true;
	}
	const fallback = state.tabs[index] ?? state.tabs[index - 1] ?? "";
	if (fallback === "") newChat();
	else selectSession(fallback);
	return true;
}

let selectionSeq = 0;

export function newChat() {
	rememberTab("");
	resetCurrent();
	emit("state");
	saveTabState();
}

export async function selectSession(id) {
	const sessionId = String(id || "");
	if (!sessionId) return;
	rememberTab(sessionId);
	if (sessionId === state.sessionId) {
		saveTabState();
		return;
	}
	selectionSeq += 1;
	const seq = selectionSeq;
	state.sessionId = sessionId;
	state.meta = findSession(sessionId);
	state.selectedModel = state.meta?.model || state.defaultModel;
	state.messages = [];
	state.messageCount = 0;
	state.facts = [];
	state.factsTotal = 0;
	state.error = "";
	saveTabState();
	emit("state");
	await Promise.all([loadMeta(sessionId, seq), loadMessages(sessionId, seq), loadFacts(sessionId, seq)]);
	const turn = state.turns.get(`s:${sessionId}`);
	if (turn) {
		turn.userPersisted = state.messages.some((message) =>
			message?.role === "user" && String(message.text || "") === turn.userText);
		if (turn.status === "error" || turn.status === "stopped") {
			const lastUser = [...state.messages].reverse().find((message) => message?.role === "user");
			const terminal = terminalTurnEnd();
			if (turn.userPersisted && lastUser && terminal
				&& Number(terminal.seq) > Number(lastUser.seq)
				&& String(lastUser.text || "") === turn.userText) {
				state.turns.delete(turn.key);
			}
		}
		emit("state");
	}
}

function findSession(id) {
	return state.sessions.find((session) => session?.session_id === id) || null;
}

let callLifecycleBound = false;

function findTurnByCall(callId) {
	for (const turn of state.turns.values()) {
		if (turn?.callId === callId) return turn;
	}
	return null;
}

function latestAssistantMessage() {
	return [...state.messages].reverse().find((message) => message?.role === "assistant") || null;
}

// A persisted assistant message supersedes the matching prefix of the live
// draft. Stripping only the prefix keeps chunks from a newer step attached.
function stripPersistedDraft(turn) {
	if (!turn) return;
	const last = latestAssistantMessage();
	if (!last) return;
	turn.lastAssistantSeq = Math.max(Number(turn.lastAssistantSeq) || 0, Number(last.seq) || 0);
	const text = String(last.text || "");
	const reasoning = String(last.reasoning || "");
	const draftText = String(turn.assistantText || "");
	const draftReasoning = String(turn.assistantReasoning || "");
	if (text && draftText.startsWith(text)) turn.assistantText = draftText.slice(text.length);
	if (reasoning && draftReasoning.startsWith(reasoning)) {
		turn.assistantReasoning = draftReasoning.slice(reasoning.length);
	}
}

function bindCallLifecycle() {
	if (callLifecycleBound) return;
	const api = bridge();
	if (typeof api.onCall !== "function") return;
	callLifecycleBound = true;
	api.onCall((frame) => {
		const payload = frame?.payload && typeof frame.payload === "object" ? frame.payload : {};
		if (payload.phase !== "finished") return;
		const callId = String(payload.call_id || "");
		if (!callId) return;
		const turn = findTurnByCall(callId);
		if (!turn || !turn.recovered || turn.finished) return;
		void finishRecoveredTurn(turn, "done");
	});
}

// Rebuild page-local running turns from the host call registry. This is the
// only path that recovers a turn whose original HTTP stream belonged to a
// previous page instance.
async function recoverCalls() {
	const api = bridge();
	if (typeof api.listCalls !== "function") return;
	let payload;
	try {
		payload = await api.listCalls();
	} catch {
		return;
	}
	const calls = Array.isArray(payload?.calls) ? payload.calls : [];
	const statusByCall = new Map();
	for (const call of calls) {
		const callId = String(call?.call_id || "");
		if (callId) statusByCall.set(callId, String(call.status || ""));
	}
	for (const call of calls) {
		if (call?.capability !== "loop" || call?.method !== "turn") continue;
		if (String(call.status || "") !== "running") continue;
		const callId = String(call.call_id || "");
		if (!callId) continue;
		const request = call.request && typeof call.request === "object" ? call.request : {};
		const sessionId = String(request.session_id || "");
		const existing = findTurnByCall(callId) || (sessionId ? state.turns.get(`s:${sessionId}`) : null);
		if (existing) {
			if (existing.watchFailed && !existing.finished && !existing.watchActive) {
				existing.watchFailed = false;
				void watchRecoveredCall(existing);
			}
			continue;
		}
		recoverCall(call);
	}
	for (const turn of [...state.turns.values()]) {
		if (!turn?.recovered || turn.finished || !turn.callId) continue;
		const status = statusByCall.get(turn.callId);
		if (status === "running") continue;
		// Terminal or vanished host record: stop presenting a live draft. Its
		// durable messages (or turn_end) are the source of truth.
		void finishRecoveredTurn(turn, status ? "done" : "lost");
	}
	startTurnPoll();
}

function recoverCall(call) {
	const callId = String(call.call_id || "");
	const request = call.request && typeof call.request === "object" ? call.request : {};
	const sessionId = String(request.session_id || "");
	const key = sessionId ? `s:${sessionId}` : `new:${callId}`;
	if (state.turns.has(key) || findTurnByCall(callId)) return;
	const turn = {
		key,
		sessionId,
		callId,
		userText: String(request.user_input || ""),
		assistantText: "",
		assistantReasoning: "",
		status: "streaming",
		error: "",
		errorCode: "",
		startedAt: Number(call.started_at) || Date.now(),
		userPersisted: false,
		lastAssistantSeq: 0,
		lastCallSeq: 0,
		recovered: true,
		finished: false,
		watchActive: false,
		watchFailed: false,
	};
	state.turns.set(key, turn);
	startTurnPoll();
	emit("state");
	void watchRecoveredCall(turn);
	if (sessionId && state.sessionId === sessionId) {
		void refreshRecoveredSession(turn);
	}
}

async function watchRecoveredCall(turn) {
	const api = bridge();
	if (typeof api.watchCall !== "function" || turn.finished) return;
	if (turn.watchActive) return;
	turn.watchActive = true;
	const controller = new AbortController();
	turn.watchAbort = controller;
	try {
		await api.watchCall(
			turn.callId,
			(frame) => handleRecoveredCallFrame(turn, frame),
			{ after: Number(turn.lastCallSeq) || 0, signal: controller.signal },
		);
		if (!turn.finished) await finishRecoveredTurn(turn, "closed");
	} catch (error) {
		if (turn.finished || controller.signal.aborted) return;
		turn.watchFailed = true;
		emit("state");
	} finally {
		turn.watchActive = false;
		if (turn.watchAbort === controller) turn.watchAbort = null;
	}
}

function handleRecoveredCallFrame(turn, frame) {
	if (turn.finished) return;
	const type = String(frame?.type || "");
	if (type === "replay") {
		const startedAt = Number(frame.started_at);
		if (startedAt) turn.startedAt = startedAt;
		if (frame.truncated) turn.draftTruncated = true;
		return;
	}
	if (type === "replay_end") {
		if (!turn.userPersisted) {
			turn.userPersisted = state.messages.some((message) =>
				message?.role === "user" && String(message.text || "") === turn.userText);
		}
		stripPersistedDraft(turn);
		emit("state");
		return;
	}
	if (type === "ping") return;
	if (type === "result") {
		void finishRecoveredTurn(turn, frame.status === "failed" ? "error" : "done");
		return;
	}
	if (type !== "event") return;
	const seq = Number(frame.seq) || 0;
	if (seq > 0 && seq <= (turn.lastCallSeq || 0)) return;
	if (seq > 0) turn.lastCallSeq = seq;
	const event = frame.event;
	if (!event || event.type !== "evt") return;
	let payload = event.payload;
	if (typeof payload === "string") {
		try {
			payload = JSON.parse(payload);
		} catch {
			payload = {};
		}
	}
	if (!payload || typeof payload !== "object") payload = {};
	if (event.method === "started") {
		const sessionId = String(payload.session_id || "");
		if (sessionId) void bindRecoveredSession(turn, sessionId);
		return;
	}
	if (event.method !== "chunk") return;
	const channel = payload.channel === "thinking" ? "thinking" : payload.channel === "content" ? "content" : "";
	const delta = typeof payload.delta === "string" ? payload.delta : "";
	if (!channel || !delta) return;
	if (channel === "thinking") turn.assistantReasoning += delta;
	else turn.assistantText += delta;
	if (currentTurn() === turn) emit("chunk", { key: turn.key, channel, delta });
}

async function bindRecoveredSession(turn, sessionId) {
	if (turn.finished || !sessionId || turn.sessionId === sessionId) return;
	adopt(turn, sessionId);
	await refreshSessions();
	if (state.sessionId === sessionId) await refreshRecoveredSession(turn);
	else emit("state");
}

async function refreshRecoveredSession(turn) {
	const sessionId = turn.sessionId;
	if (!sessionId) return;
	const seq = selectionSeq;
	await Promise.all([
		loadMeta(sessionId, seq),
		loadMessages(sessionId, seq),
		loadFacts(sessionId, seq),
	]);
	if (seq !== selectionSeq || state.sessionId !== sessionId) return;
	turn.userPersisted = state.messages.some((message) =>
		message?.role === "user" && String(message.text || "") === turn.userText);
	stripPersistedDraft(turn);
	emit("state");
}

async function finishRecoveredTurn(turn, reason) {
	if (!turn || turn.finished) return;
	turn.finished = true;
	turn.watchAbort?.abort();
	turn.watchAbort = null;
	const sessionId = turn.sessionId;
	await refreshSessions();
	if (sessionId && state.sessionId === sessionId) {
		await refreshCurrent();
	}
	if (state.turns.get(turn.key) === turn) state.turns.delete(turn.key);
	stopTurnPollIfIdle();
	emit("state");
	if (reason === "error" && sessionId && state.sessionId === sessionId) {
		// The terminal turn_end fact carries the user-visible error; nothing
		// page-local remains to render.
		emit("progress");
	}
}

async function loadMeta(id, seq) {
	try {
		const info = await call("session", "info", { session_id: id });
		if (seq !== selectionSeq) return;
		state.meta = info;
		if (info?.message_count !== undefined) {
			state.messageCount = Number(info.message_count) || 0;
		}
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
		state.messageCount = state.messages.length;
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
	const previous = state.workspace;
	if (previous) saveTabState(previous);
	try {
		await call("workspace", "select", { path: next });
		state.workspace = next;
		state.tabs = [];
		resetCurrent();
		emit("state");
		await refreshSessions();
		await restoreTabState();
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
		const previous = state.workspace;
		if (previous) saveTabState(previous);
		state.workspace = selected;
		state.tabs = [];
		resetCurrent();
		emit("state");
		await loadWorkspaces();
		await refreshSessions();
		await restoreTabState();
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
	const target = String(id || "");
	const index = state.tabs.indexOf(target);
	const wasActive = state.sessionId === target;
	try {
		await call("session", "delete", { session_id: target });
		state.tabs = state.tabs.filter((key) => key !== target);
		saveTabState();
		if (wasActive) {
			const fallback = state.tabs[index] ?? state.tabs[index - 1] ?? "";
			if (fallback === "") newChat();
			else await selectSession(fallback);
		}
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
	if (wasNew) {
		const newIndex = state.tabs.indexOf("");
		if (newIndex >= 0) {
			state.tabs.splice(newIndex, 1, sessionId);
		} else if (!state.tabs.includes(sessionId)) {
			state.tabs.push(sessionId);
		}
		state.tabs = normalizeTabs(state.tabs);
		if (!state.sessionId) state.sessionId = sessionId;
		saveTabState();
	}
}

function terminalTurnEnd() {
	let latest = null;
	for (const fact of state.facts) {
		if (fact?.type !== "turn_end") continue;
		const status = String(fact.meta?.status || "");
		if (status !== "error" && status !== "cancelled") continue;
		if (!latest || Number(fact.seq) > Number(latest.seq)) latest = fact;
	}
	return latest;
}

let turnPollTimer = 0;
let turnPollBusy = false;

function runningTurns() {
	return [...state.turns.values()].filter((turn) =>
		turn.sessionId && (turn.status === "streaming" || turn.status === "stopping"));
}

function startTurnPoll() {
	if (turnPollTimer) return;
	turnPollTimer = setInterval(pollRunningTurns, 2000);
}

function stopTurnPollIfIdle() {
	if (runningTurns().length > 0) return;
	if (!turnPollTimer) return;
	clearInterval(turnPollTimer);
	turnPollTimer = 0;
}

async function pollRunningTurns() {
	if (turnPollBusy) return;
	const running = runningTurns();
	if (running.length === 0) {
		stopTurnPollIfIdle();
		return;
	}
	turnPollBusy = true;
	let changed = false;
	try {
		for (const turn of running) {
			const id = turn.sessionId;
			let info = null;
			try {
				info = await call("session", "info", { session_id: id });
			} catch {
				continue;
			}
			if (!info || typeof info !== "object") continue;
			const factCount = Number(info.fact_count) || 0;
			const messageCount = Number(info.message_count) || 0;
			const session = findSession(id);
			if (session && (Number(session.fact_count) || 0) !== factCount) {
				session.fact_count = factCount;
				changed = true;
			}
			if (session && (Number(session.message_count) || 0) !== messageCount) {
				session.message_count = messageCount;
				changed = true;
			}
			if (state.sessionId !== id) continue;
			const messagesIncreased = messageCount > state.messageCount;
			if (state.meta?.title !== info.title
				|| state.factsTotal !== factCount
				|| state.messageCount !== messageCount) {
				state.meta = { ...(state.meta || {}), ...info };
				state.factsTotal = factCount;
				state.messageCount = messageCount;
				changed = true;
			}
			// Steps completed while the model is still streaming (assistant tool
			// calls and tool results) are already durable. Pull them in so the
			// process stays visible before the turn ends.
			if (messagesIncreased && !state.loading.messages) {
				await loadMessages(id, selectionSeq);
				if (state.sessionId !== id) continue;
				turn.userPersisted = state.messages.some((message) =>
					message?.role === "user" && String(message.text || "") === turn.userText);
				stripPersistedDraft(turn);
				changed = true;
				emit("state");
			}
		}
	} finally {
		try {
			await syncSessions();
			changed = true;
		} catch {
			// A transient list failure must not interrupt turn progress polling.
		}
		turnPollBusy = false;
		if (changed) emit("progress");
		stopTurnPollIfIdle();
	}
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
		callId: "",
		userText: input,
		assistantText: "",
		assistantReasoning: "",
		status: "streaming",
		error: "",
		errorCode: "",
		startedAt: Date.now(),
		userPersisted: false,
		lastAssistantSeq: 0,
		lastCallSeq: 0,
		recovered: false,
		finished: false,
	};
	state.turns.set(key, turn);
	startTurnPoll();
	emit("state");

	try {
		const result = await bridge().call(
			"loop",
			"turn",
			{ workspace, session_id: sessionId, user_input: input, model: selectedModel },
			(event) => {
				const payload = event?.payload || {};
				if (event?.id && !turn.callId) turn.callId = String(event.id);
				if (event?.method === "started" && payload.session_id) {
					adopt(turn, String(payload.session_id));
					emit("state");
					refreshSessions();
					pollRunningTurns();
					return;
				}
				if (event?.method !== "chunk") return;
				const channel = payload.channel === "thinking" ? "thinking" : payload.channel === "content" ? "content" : "";
				if (!channel) return;
				const delta = typeof payload.delta === "string" ? payload.delta : "";
				if (!delta) return;
				if (channel === "thinking") turn.assistantReasoning += delta;
				else turn.assistantText += delta;
				emit("chunk", { key: turn.key, channel, delta });
			},
		);

		const resultId = typeof result?.session_id === "string" && result.session_id
			? result.session_id
			: turn.sessionId;
		if (resultId) {
			adopt(turn, resultId);
			if (!state.sessionId) state.sessionId = resultId;
		}
		const assistantMessages = Array.isArray(result?.messages)
			? result.messages.filter((message) => message?.role === "assistant")
			: [];
		const finalText = assistantMessages
			.map((message) => String(message.text || "").trim())
			.filter(Boolean)
			.join("\n\n");
		const finalReasoning = assistantMessages
			.map((message) => String(message.reasoning || "").trim())
			.filter(Boolean)
			.join("\n\n");
		if (finalText) turn.assistantText = finalText;
		if (finalReasoning) turn.assistantReasoning = finalReasoning;
		turn.status = "done";
		turn.userPersisted = true;
		emit("state");

		await refreshSessions();
		if (resultId && state.sessionId === resultId) await refreshCurrent();
		state.turns.delete(turn.key);
		stopTurnPollIfIdle();
		emit("state");
	} catch (error) {
		const cancelled = error?.code === "call_cancelled" || turn.status === "stopping";
		turn.status = cancelled ? "stopped" : "error";
		turn.error = cancelled ? "" : errorMessage(error);
		turn.errorCode = cancelled ? "" : errorCode(error);
		emit("state");
		await refreshSessions();
		if (turn.sessionId && state.sessionId === turn.sessionId) {
			await refreshCurrent();
		}
		turn.userPersisted = state.messages.some((message) =>
			message?.role === "user" && String(message.text || "") === turn.userText);
		const lastUser = [...state.messages].reverse().find((message) => message?.role === "user");
		const terminal = terminalTurnEnd();
		const persisted = Boolean(
			turn.userPersisted
			&& lastUser
			&& terminal
			&& Number(terminal.seq) > Number(lastUser.seq)
			&& String(lastUser.text || "") === turn.userText);
		const visible = turn.sessionId ? state.sessionId === turn.sessionId : state.sessionId === "";
		if (persisted) {
			state.turns.delete(turn.key);
			stopTurnPollIfIdle();
			emit("state");
		} else {
			stopTurnPollIfIdle();
			emit("state");
			if (visible && cancelled) notify("已停止生成");
		}
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
