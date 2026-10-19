export const DEFAULT_WHEN_DEFINED_TIMEOUT_MS = 10_000;

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
}) {
	const entryLoads = new Map();
	const mounted = new Map();

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
			mounted.set(key, { spec, element });
		} catch (error) {
			onDiagnostic(diagnosticFor(spec.mount, "ui_mount_failed", error));
		}
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
			await Promise.all(mounts.map(loadMount));
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

