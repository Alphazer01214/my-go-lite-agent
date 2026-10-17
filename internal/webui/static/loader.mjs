export const DEFAULT_WHEN_DEFINED_TIMEOUT_MS = 10_000;

function mountKey(mount) {
	return `${mount.plugin}:${mount.id}`;
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
}) {
	const entryLoads = new Map();
	const mounted = new Map();

	function ensureEntry(entry) {
		let load = entryLoads.get(entry);
		if (!load) {
			load = Promise.resolve().then(() => importModule(entry));
			entryLoads.set(entry, load);
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
		const key = mountKey(spec.mount);
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
			mounted.set(key, element);
		} catch (error) {
			onDiagnostic(diagnosticFor(spec.mount, "ui_mount_failed", error));
		}
	}

	return {
		async load(mounts) {
			await Promise.all(mounts.map(loadMount));
			return [...mounted.keys()];
		},
		get(key) {
			return mounted.get(key) ?? null;
		},
		has(key) {
			return mounted.has(key);
		},
	};
}

