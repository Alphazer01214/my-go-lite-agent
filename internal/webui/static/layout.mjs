// Column resizing for the workspace grid. The shell owns only the two side
// widths; the center column stays fluid (minmax(320px, 1fr)). Widths persist
// per browser and restore on load.

export const DEFAULT_COLUMN_WIDTHS = Object.freeze({
	left: 232,
	right: 280,
});

export const COLUMN_WIDTH_LIMITS = Object.freeze({
	left: Object.freeze([180, 480]),
	right: Object.freeze([200, 640]),
});

const STORAGE_PREFIX = "liteagent.layout.";
const KEYBOARD_STEP_PX = 16;

function defaultStorage() {
	try {
		if (typeof localStorage !== "undefined") return localStorage;
	} catch {
		// Same best-effort rule as tab persistence.
	}
	return null;
}

export function clampColumnWidth(side, width) {
	const limits = COLUMN_WIDTH_LIMITS[side];
	if (!limits) return Math.round(Number(width) || 0);
	const value = Math.round(Number(width));
	if (!Number.isFinite(value)) return limits[0];
	return Math.min(limits[1], Math.max(limits[0], value));
}

function readStoredWidth(storage, side) {
	const raw = storage?.getItem(STORAGE_PREFIX + side);
	const value = Number(raw);
	if (raw == null || raw === "" || !Number.isFinite(value)) {
		return DEFAULT_COLUMN_WIDTHS[side];
	}
	return clampColumnWidth(side, value);
}

export function readColumnWidths(storage) {
	return {
		left: readStoredWidth(storage, "left"),
		right: readStoredWidth(storage, "right"),
	};
}

function writeStoredWidth(storage, side, width) {
	try {
		storage?.setItem(STORAGE_PREFIX + side, String(width));
	} catch {
		// Resizing still works this session without persistence.
	}
}

export function applyColumnWidths(workspace, widths) {
	if (!workspace?.style?.setProperty) return;
	workspace.style.setProperty("--left-width", `${widths.left}px`);
	workspace.style.setProperty("--right-width", `${widths.right}px`);
}

function beginDrag(document, handle) {
	if (document.body?.classList) document.body.classList.add("resizing");
	if (handle.setPointerCapture && document.activeElement !== handle) handle.focus?.();
}

function endDrag(document) {
	if (document.body?.classList) document.body.classList.remove("resizing");
}

// One resizer owns one side: data-resize="left" grows left by the drag delta;
// data-resize="right" grows right by minus the delta.
export function createColumnResizer({ side, storage, onWidths, widths }) {
	let startX = 0;
	let startWidth = 0;
	let dragging = false;

	function nextWidth(deltaX, direction) {
		return clampColumnWidth(side, startWidth + direction * deltaX);
	}

	return {
		handleEvent(event) {
			if (event.type === "pointerdown") {
				dragging = true;
				startX = event.clientX ?? 0;
				startWidth = widths()[side];
				event.currentTarget?.setPointerCapture?.(event.pointerId);
				return;
			}
			if (!dragging) return;
			if (event.type === "pointermove") {
				const direction = side === "left" ? 1 : -1;
				const next = { ...widths(), [side]: nextWidth((event.clientX ?? startX) - startX, direction) };
				onWidths(next, false);
				return;
			}
			// pointerup / pointercancel end the drag and persist.
			dragging = false;
			const direction = side === "left" ? 1 : -1;
			const next = { ...widths(), [side]: nextWidth((event.clientX ?? startX) - startX, direction) };
			onWidths(next, true);
		},
		handleKey(event) {
			const map = {
				ArrowLeft: -1,
				ArrowRight: 1,
				ArrowDown: -1,
				ArrowUp: 1,
			};
			const directionKey = map[event.key];
			if (directionKey === undefined) return false;
			// Right-side arrows are mirrored: ArrowRight widens left but
			// narrows right, matching the visual direction of the handle.
			const sign = side === "left" ? directionKey : -directionKey;
			const next = { ...widths(), [side]: clampColumnWidth(side, widths()[side] + sign * KEYBOARD_STEP_PX) };
			onWidths(next, true);
			event.preventDefault?.();
			return true;
		},
	};
}

export function initColumnLayout({ document, storage = defaultStorage() }) {
	const workspace = document.querySelector?.(".workspace");
	if (!workspace) return () => {};

	const state = { widths: readColumnWidths(storage) };
	const apply = (widths, persist) => {
		state.widths = {
			left: clampColumnWidth("left", widths.left),
			right: clampColumnWidth("right", widths.right),
		};
		applyColumnWidths(workspace, state.widths);
		if (persist) {
			writeStoredWidth(storage, "left", state.widths.left);
			writeStoredWidth(storage, "right", state.widths.right);
		}
	};
	apply(state.widths, false);

	const cleanups = [];
	for (const handle of workspace.querySelectorAll?.(".col-resizer") ?? []) {
		const side = handle.dataset?.resize;
		if (side !== "left" && side !== "right") continue;
		const resizer = createColumnResizer({
			side,
			storage,
			widths: () => state.widths,
			onWidths: apply,
		});
		const onPointer = (event) => {
			if (event.type === "pointerdown") beginDrag(document, handle);
			resizer.handleEvent(event);
			if (event.type === "pointerup" || event.type === "pointercancel") endDrag(document);
		};
		const onKey = (event) => resizer.handleKey(event);
		handle.addEventListener?.("pointerdown", onPointer);
		handle.addEventListener?.("pointermove", onPointer);
		handle.addEventListener?.("pointerup", onPointer);
		handle.addEventListener?.("pointercancel", onPointer);
		handle.addEventListener?.("keydown", onKey);
		cleanups.push(() => {
			handle.removeEventListener?.("pointerdown", onPointer);
			handle.removeEventListener?.("pointermove", onPointer);
			handle.removeEventListener?.("pointerup", onPointer);
			handle.removeEventListener?.("pointercancel", onPointer);
			handle.removeEventListener?.("keydown", onKey);
		});
	}
	return () => cleanups.forEach((fn) => fn());
}
