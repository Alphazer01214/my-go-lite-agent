export const DEFAULT_WHEN_DEFINED_TIMEOUT_MS = 10_000;

// Dispatched on a member element each time it becomes the visible tab, so a
// lazily refreshing panel (e.g. git) can eagerly reload on activation.
export const TAB_SHOW_EVENT = "liteagent:tab-show";

const TAB_STORAGE_PREFIX = "liteagent.tab.";

function defaultTabStorage() {
	try {
		if (typeof localStorage !== "undefined") return localStorage;
	} catch {
		// localStorage access can throw in exotic embeds; tabs then simply do
		// not persist across reloads.
	}
	return null;
}

// mountKeyFor is the single place a Mount identity is spelled out. It matches the
// key the loader stores elements under, so callers do not rebuild it by hand.
export function mountKeyFor(mount) {
	return `${mount?.plugin ?? ""}:${mount?.id ?? ""}`;
}

function diagnosticFor(mount, code, error) {
	return {
		plugin: mount.plugin,
		path: mount.entry,
		code,
		message: error instanceof Error ? error.message : String(error),
	};
}

export function createMountLoader({
	importModule,
	customElements,
	document,
	slots,
	onDiagnostic,
	whenDefinedTimeoutMs = DEFAULT_WHEN_DEFINED_TIMEOUT_MS,
	resyncRetryDelaysMs = [0, 250, 500, 1000, 2000],
	tabStorage = defaultTabStorage(),
	// Host-side exclusion list for mounts that must never join a tab group.
	// Covers components that cannot declare `static overlay` themselves.
	overlayTags = [],
}) {
	const entryLoads = new Map();
	const mounted = new Map();
	// Bootstrap order of the most recent load()/resync() call; the tie-breaker
	// after a component's static tabOrder.
	const mountOrder = new Map();
	const tabState = new Map();

	function ensureEntry(entry) {
		let load = entryLoads.get(entry);
		if (!load) {
			// Cache only on success: a rejected import (half-written package,
			// 404 during a rescan) must stay retryable instead of poisoning the
			// mount forever.
			load = Promise.resolve().then(() => importModule(entry));
			entryLoads.set(entry, load);
			load.catch(() => {
				if (entryLoads.get(entry) === load) {
					entryLoads.delete(entry);
				}
			});
		}
		return load;
	}

	function waitForDefinition(tag) {
		if (customElements.get(tag)) {
			return Promise.resolve();
		}

		let timer;
		const timeout = new Promise((_, reject) => {
			timer = setTimeout(() => {
				reject(new Error(`custom element ${tag} definition timed out`));
			}, whenDefinedTimeoutMs);
		});

		return Promise.race([customElements.whenDefined(tag), timeout]).finally(() => {
			clearTimeout(timer);
		});
	}

	async function loadMount(spec) {
		const key = mountKeyFor(spec.mount);
		if (mounted.has(key)) {
			return;
		}

		try {
			await ensureEntry(spec.entry);
			await waitForDefinition(spec.tag);

			if (mounted.has(key)) {
				return;
			}
			const slot = slots.get(spec.slot);
			if (!slot) {
				throw new Error(`unknown slot ${spec.slot}`);
			}

			const element = document.createElement(spec.tag);
			element.dataset.mountKey = key;
			slot.append(element);
			mounted.set(key, { spec, element, ...tabMeta(spec) });
		} catch (error) {
			onDiagnostic(diagnosticFor(spec.mount, "ui_mount_failed", error));
		}
	}

	// Tab metadata comes from static fields on the component class, read after
	// the definition is guaranteed to exist. overlayTags is the host-side
	// fallback for components that cannot declare `static overlay`.
	function tabMeta(spec) {
		const ctor = customElements.get?.(spec.tag);
		const overlay = ctor?.overlay === true || overlayTags.includes(spec.tag);
		const title = typeof ctor?.tabTitle === "string" && ctor.tabTitle
			? ctor.tabTitle
			: spec.mount.plugin;
		const order = Number.isFinite(ctor?.tabOrder) ? ctor.tabOrder : 0;
		return { overlay, title, order };
	}

	function readStoredTab(slotName) {
		try {
			return tabStorage?.getItem(TAB_STORAGE_PREFIX + slotName) ?? "";
		} catch {
			return "";
		}
	}

	function writeStoredTab(slotName, key) {
		try {
			tabStorage?.setItem(TAB_STORAGE_PREFIX + slotName, key);
		} catch {
			// Persistence is best effort; switching still works this session.
		}
	}

	function setEntryVisibility(entry, visible) {
		entry.element.hidden = !visible;
		// Inline display defends against shadow styles whose :host display rule
		// would otherwise override the hidden attribute's UA rule.
		if (entry.element.style) entry.element.style.display = visible ? "" : "none";
	}

	// Rebuild one slot's tab group: order members, resolve the active tab,
	// toggle visibility, and show the bar once a second member exists. Overlay
	// elements never join the group and are never hidden.
	function reconcileTab(slotName) {
		const slot = slots.get(slotName);
		if (!slot) return;
		const members = [];
		for (const entry of mounted.values()) {
			if (entry.spec.slot !== slotName || entry.overlay) continue;
			members.push(entry);
		}
		members.sort((a, b) =>
			(a.order - b.order) ||
			((mountOrder.get(mountKeyFor(a.spec.mount)) ?? Number.MAX_SAFE_INTEGER) -
				(mountOrder.get(mountKeyFor(b.spec.mount)) ?? Number.MAX_SAFE_INTEGER)));

		const state = tabState.get(slotName) ?? { bar: null, signature: "", activeKey: "" };
		tabState.set(slotName, state);

		const stored = readStoredTab(slotName);
		const activeKey = members.some((entry) => mountKeyFor(entry.spec.mount) === stored)
			? stored
			: members[0]
				? mountKeyFor(members[0].spec.mount)
				: "";
		const changed = activeKey !== state.activeKey;
		state.activeKey = activeKey;
		if (activeKey) writeStoredTab(slotName, activeKey);

		let activeEntry = null;
		for (const entry of members) {
			const visible = mountKeyFor(entry.spec.mount) === activeKey;
			setEntryVisibility(entry, visible);
			if (visible) activeEntry = entry;
		}
		// Hidden members keep receiving pushes; activation is their moment to
		// refresh eagerly if they listen for the event.
		if (changed && activeEntry) {
			activeEntry.element.dispatchEvent?.(new Event(TAB_SHOW_EVENT));
		}

		if (members.length > 1) {
			if (!state.bar) {
				state.bar = document.createElement("div");
				state.bar.className = "slot-tabs";
				state.bar.setAttribute("role", "tablist");
				slot.prepend ? slot.prepend(state.bar) : slot.append(state.bar);
			}
			renderTabBar(state, members, activeKey, slotName);
		} else if (state.bar) {
			state.bar.remove();
			state.bar = null;
			state.signature = "";
		}
	}

	function renderTabBar(state, members, activeKey, slotName) {
		const signature = JSON.stringify(members.map((entry) => [
			mountKeyFor(entry.spec.mount),
			entry.title,
			mountKeyFor(entry.spec.mount) === activeKey,
		]));
		if (signature === state.signature) return;
		state.signature = signature;
		state.bar.replaceChildren?.();
		for (const entry of members) {
			const key = mountKeyFor(entry.spec.mount);
			const button = document.createElement("button");
			button.type = "button";
			button.textContent = entry.title;
			button.className = key === activeKey ? "active" : "";
			button.setAttribute("aria-selected", String(key === activeKey));
			button.addEventListener?.("click", () => {
				writeStoredTab(slotName, key);
				reconcileTab(slotName);
			});
			state.bar.append(button);
		}
	}

	// Tabs are generic: any slot that ends up with two or more non-overlay
	// mounts is grouped, not just the right rail.
	function reconcileTabs() {
		for (const slotName of slots.keys()) reconcileTab(slotName);
	}

	// A reconnect can land before the Runtime plugin is ready. Retry a failed
	// component resync for a few seconds before reporting it.
	async function resyncElement(spec, entry) {
		let lastError;
		for (const delay of resyncRetryDelaysMs) {
			if (delay > 0) {
				await new Promise((resolve) => setTimeout(resolve, delay));
			}
			try {
				await entry.element.resync();
				return;
			} catch (error) {
				lastError = error;
			}
		}
		onDiagnostic(diagnosticFor(spec.mount, "ui_resync_failed", lastError));
	}

	function trackMountOrder(mounts) {
		mountOrder.clear();
		mounts.forEach((spec, index) => mountOrder.set(mountKeyFor(spec.mount), index));
	}

	return {
		async load(mounts) {
			// Reconcile, not just append: a rescan may drop or rename a mount, and
			// the element of a mount that no longer exists must leave the slot.
			const wanted = new Set(mounts.map((spec) => mountKeyFor(spec.mount)));
			for (const [key, entry] of [...mounted]) {
				if (wanted.has(key)) continue;
				mounted.delete(key);
				entry.element.remove();
			}
			trackMountOrder(mounts);
			await Promise.all(mounts.map(loadMount));
			reconcileTabs();
			return [...mounted.keys()];
		},
		async resync(mounts) {
			// Re-read Runtime state after a reconnect. Components may implement
			// resync(); otherwise a remount re-runs connectedCallback.
			const wanted = new Set(mounts.map((spec) => mountKeyFor(spec.mount)));
			for (const [key, entry] of [...mounted]) {
				if (wanted.has(key)) continue;
				mounted.delete(key);
				entry.element.remove();
			}
			trackMountOrder(mounts);
			await Promise.all(mounts.map(async (spec) => {
				const key = mountKeyFor(spec.mount);
				const entry = mounted.get(key);
				if (!entry) {
					await loadMount(spec);
					return;
				}
				if (typeof entry.element.resync === "function") {
					await resyncElement(spec, entry);
					return;
				}
				mounted.delete(key);
				entry.element.remove();
				await loadMount(spec);
			}));
			reconcileTabs();
			return [...mounted.keys()];
		},
		get(key) {
			return mounted.get(key)?.element ?? null;
		},
		has(key) {
			return mounted.has(key);
		},
	};
}
