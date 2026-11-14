export async function callCapability(capability, method, payload = {}) {
	const api = globalThis.LiteAgent;
	if (!api || typeof api.call !== "function") {
		throw new Error("LiteAgent bridge is unavailable");
	}
	return api.call(capability, method, payload ?? {});
}

export async function callGit(method, payload = {}) {
	return callCapability("git", method, payload);
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

export function errorCode(error) {
	return String(error?.code || error?.response?.error_code || "");
}

export function errorMessage(error) {
	const code = errorCode(error);
	const detail = error instanceof Error ? error.message : String(error ?? "未知错误");
	switch (code) {
		case "git_workspace_missing":
			return "还没有选择工作区";
		case "git_not_repo":
			return "当前工作区不是 Git 仓库";
		case "git_unavailable":
			return "找不到 git 命令";
		case "call_deadline_exceeded":
			return "Git 操作超时";
		case "call_cancelled":
			return "Git 操作已取消";
		case "plugin_down":
			return "Git 插件未运行";
		case "method_not_found":
			return "Git 插件能力不可用";
		case "git_bad_path":
			return `路径无效：${detail}`;
		case "git_bad_branch":
			return detail.includes("current branch") ? "不能删除当前分支" : `分支名无效：${detail}`;
		default:
			return detail;
	}
}
