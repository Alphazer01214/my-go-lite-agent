// Sandbox confirmation panel. The Runtime pushes the complete pending list;
// this component only renders it and submits the user's decision.
const styles = `
*, *::before, *::after { box-sizing: border-box; }
:host {
  position: fixed;
  z-index: 30;
  right: 16px;
  bottom: 16px;
  display: block;
  width: min(420px, calc(100vw - 32px));
  max-height: min(60vh, 520px);
  overflow: auto;
  color: #202124;
  font: 13px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif;
}
:host([hidden]) { display: none; }
.panel {
  border: 1px solid #e2b77f;
  border-radius: 8px;
  background: #fffdf8;
  box-shadow: 0 12px 30px rgb(24 33 43 / 20%);
}
.head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid #f0dcc0;
}
.head strong { flex: 1; }
.count { color: #8a5a12; font-variant-numeric: tabular-nums; }
.hint, .error { padding: 0 12px; color: #6b7280; font-size: 12px; }
.error { color: #b91c1c; }
.list { display: grid; gap: 8px; padding: 10px 12px 12px; }
.empty { padding: 4px 0; color: #6b7280; font-size: 12px; }
.item {
  padding: 10px;
  border: 1px solid #ead7bd;
  border-radius: 7px;
  background: #ffffff;
}
.item-head { display: flex; align-items: baseline; gap: 6px; }
.tool { font-weight: 600; overflow-wrap: anywhere; }
.risk { margin-left: auto; color: #8a5a12; font-size: 11px; white-space: nowrap; }
.session { margin-top: 2px; color: #6b7280; font-size: 12px; overflow-wrap: anywhere; }
.reason { margin-top: 6px; color: #374151; font-size: 12px; overflow-wrap: anywhere; }
.preview {
  margin-top: 6px;
  padding: 6px 8px;
  border-radius: 5px;
  background: #f6f2ea;
  color: #4b5563;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 11px;
  overflow-wrap: anywhere;
}
.meta { margin-top: 4px; color: #9ca3af; font-size: 11px; }
.actions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 9px; }
button {
  padding: 5px 10px;
  border: 1px solid #d5dbe1;
  border-radius: 5px;
  background: #fff;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
button:hover:not(:disabled) { border-color: #147d64; }
button:disabled { cursor: not-allowed; opacity: .55; }
.allow { border-color: #147d64; color: #0c604c; }
.deny { border-color: #b91c1c; color: #b91c1c; }
.view { margin-left: auto; }
`;

function call(method, payload) {
	return window.LiteAgent.call("sandbox", method, payload);
}

function describeError(error) {
	return error instanceof Error ? error.message : String(error);
}

function formatRemaining(expiresAt, now) {
	const ms = Number(expiresAt) - now;
	if (!Number.isFinite(ms) || ms <= 0) return "已超时";
	const seconds = Math.ceil(ms / 1000);
	if (seconds < 60) return `${seconds}s 后超时`;
	const minutes = Math.floor(seconds / 60);
	return `${minutes}m${String(seconds % 60).padStart(2, "0")}s 后超时`;
}

class SandboxConfirm extends HTMLElement {
	connectedCallback() {
		this.attachShadow({ mode: "open" });
		this.shadowRoot.innerHTML = `
      <style>${styles}</style>
      <section class="panel" aria-label="待确认的工具调用">
        <div class="head">
          <strong>工具调用确认</strong>
          <span class="count"></span>
          <button type="button" class="refresh" title="刷新待确认列表">刷新</button>
        </div>
        <div class="error" aria-live="polite"></div>
        <div class="hint" hidden>这些调用会一直等待，直到你允许、拒绝或令牌超时。</div>
        <div class="list"></div>
      </section>
    `;
		this.count = this.shadowRoot.querySelector(".count");
		this.list = this.shadowRoot.querySelector(".list");
		this.error = this.shadowRoot.querySelector(".error");
		this.refreshButton = this.shadowRoot.querySelector(".refresh");
		this.state = { revision: 0, pending: [] };
		this.sessionNames = new Map();
		this.busy = new Set();
		this.refreshButton.addEventListener("click", () => this.refresh());
		this.tick = window.setInterval(() => this.updateCountdowns(), 1000);
		this.nameTimer = window.setInterval(() => this.refreshNames(), 15000);
		this.refresh();
	}

	disconnectedCallback() {
		if (this.tick) window.clearInterval(this.tick);
		if (this.nameTimer) window.clearInterval(this.nameTimer);
		this.tick = 0;
		this.nameTimer = 0;
	}

	async refresh() {
		try {
			const out = await call("pending.list", {});
			this.error.textContent = "";
			this.setData({ revision: this.state.revision, pending: out?.pending || [] });
			await this.refreshNames();
		} catch (error) {
			this.error.textContent = describeError(error);
			this.render();
		}
	}

	async resync() {
		await this.refresh();
	}

	async refreshNames() {
		const ids = new Set((this.state.pending || []).map((item) => item.session_id).filter(Boolean));
		if (ids.size === 0) return;
		try {
			const out = await window.LiteAgent.call("session", "list", { include_subagents: true });
			let changed = false;
			for (const session of out?.sessions || []) {
				const label = session.name || session.session_id;
				if (this.sessionNames.get(session.session_id) !== label) {
					this.sessionNames.set(session.session_id, label);
					changed = true;
				}
			}
			for (const id of ids) {
				if (!this.sessionNames.has(id)) {
					this.sessionNames.set(id, id);
					changed = true;
				}
			}
			if (changed) this.render();
		} catch {
			// Without session the panel still shows raw ids.
		}
	}

	setData(state) {
		const next = state || { revision: 0, pending: [] };
		if (Number.isFinite(next.revision) && Number.isFinite(this.state.revision) &&
			next.revision < this.state.revision) return;
		this.state = next;
		this.render();
		this.refreshNames();
	}

	render() {
		if (!this.list) return;
		const pending = Array.isArray(this.state.pending) ? this.state.pending : [];
		const now = Date.now();
		const live = pending.filter((item) => Number(item.expires_at) > now);
		this.hidden = live.length === 0;
		this.count.textContent = live.length > 0 ? `${live.length} 项` : "";
		this.list.replaceChildren();
		if (live.length === 0) return;
		for (const item of live) this.list.append(this.itemNode(item, now));
	}

	itemNode(item, now) {
		const node = document.createElement("article");
		node.className = "item";
		const head = document.createElement("div");
		head.className = "item-head";
		const tool = document.createElement("span");
		tool.className = "tool";
		tool.textContent = item.tool || [item.capability, item.method].filter(Boolean).join(".") || "未知工具";
		const risk = document.createElement("span");
		risk.className = "risk";
		risk.textContent = `风险 ${item.risk ?? "?"}`;
		head.append(tool, risk);
		node.append(head);

		if (item.session_id) {
			const session = document.createElement("div");
			session.className = "session";
			session.textContent = `会话：${this.sessionNames.get(item.session_id) || item.session_id}`;
			node.append(session);
		}
		const reason = document.createElement("div");
		reason.className = "reason";
		reason.textContent = item.reason || "命中确认策略";
		node.append(reason);
		if (item.arguments_preview) {
			const preview = document.createElement("div");
			preview.className = "preview";
			preview.textContent = item.arguments_preview;
			node.append(preview);
		}
		const meta = document.createElement("div");
		meta.className = "meta";
		node.dataset.expires = String(item.expires_at);
		node.dataset.rule = item.rule_id || "";
		meta.textContent = this.metaText(node.dataset.rule, item.expires_at, now);
		node.append(meta);

		const actions = document.createElement("div");
		actions.className = "actions";
		const allow = document.createElement("button");
		allow.type = "button";
		allow.className = "allow";
		allow.textContent = "允许一次";
		allow.disabled = this.busy.has(item.token);
		allow.addEventListener("click", () => this.decide(item, "allow"));
		const deny = document.createElement("button");
		deny.type = "button";
		deny.className = "deny";
		deny.textContent = "拒绝";
		deny.disabled = this.busy.has(item.token);
		deny.addEventListener("click", () => this.decide(item, "deny"));
		actions.append(allow, deny);
		if (item.session_id) {
			const view = document.createElement("button");
			view.type = "button";
			view.className = "view";
			view.textContent = "查看会话";
			view.addEventListener("click", () => this.viewSession(item.session_id));
			actions.append(view);
		}
		node.append(actions);
		return node;
	}

	metaText(ruleID, expiresAt, now) {
		const parts = [];
		if (ruleID) parts.push(`规则 ${ruleID}`);
		parts.push(formatRemaining(expiresAt, now));
		return parts.join(" · ");
	}

	updateCountdowns() {
		if (!this.list) return;
		const now = Date.now();
		for (const node of this.list.querySelectorAll(".item")) {
			const meta = node.querySelector(".meta");
			if (!meta) continue;
			meta.textContent = this.metaText(node.dataset.rule || "", Number(node.dataset.expires), now);
		}
	}

	async decide(item, decision) {
		if (this.busy.has(item.token)) return;
		this.busy.add(item.token);
		this.error.textContent = "";
		this.render();
		try {
			await call("decide", { token: item.token, decision });
			this.busy.delete(item.token);
			this.state.pending = (this.state.pending || []).filter((entry) => entry.token !== item.token);
			this.render();
			await this.refreshNames();
		} catch (error) {
			this.busy.delete(item.token);
			this.error.textContent = describeError(error);
			this.render();
			await this.refresh();
		}
	}

	async viewSession(sessionId) {
		try {
			await window.LiteAgent.call("session", "select", { session_id: sessionId });
		} catch (error) {
			this.error.textContent = describeError(error);
			this.render();
		}
	}
}

customElements.define("sandbox-confirm", SandboxConfirm);
