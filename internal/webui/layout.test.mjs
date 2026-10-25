import test from "node:test";
import assert from "node:assert/strict";

import {
	applyColumnWidths,
	clampColumnWidth,
	createColumnResizer,
	initColumnLayout,
	DEFAULT_COLUMN_WIDTHS,
} from "./static/layout.mjs";

function createStorage(initial = {}) {
	return {
		store: new Map(Object.entries(initial)),
		getItem(key) {
			return this.store.has(key) ? this.store.get(key) : null;
		},
		setItem(key, value) {
			this.store.set(key, String(value));
		},
	};
}

function fakeHandle(side) {
	const listeners = new Map();
	return {
		dataset: { resize: side },
		listeners,
		addEventListener(type, handler) {
			const list = listeners.get(type) ?? [];
			list.push(handler);
			listeners.set(type, list);
		},
		removeEventListener(type, handler) {
			const list = (listeners.get(type) ?? []).filter((item) => item !== handler);
			listeners.set(type, list);
		},
		dispatch(type, event = {}) {
			for (const handler of listeners.get(type) ?? []) {
				handler({ type, currentTarget: this, preventDefault() {}, ...event });
			}
		},
	};
}

function fakeDocument(handles) {
	const classes = new Set();
	return {
		body: {
			classList: {
				add(name) {
					classes.add(name);
				},
				remove(name) {
					classes.delete(name);
				},
				has(name) {
					return classes.has(name);
				},
			},
		},
		querySelector(selector) {
			return selector === ".workspace" ? this.workspace : null;
		},
		workspace: {
			style: {
				vars: {},
				setProperty(name, value) {
					this.vars[name] = value;
				},
			},
			_handles: handles,
			querySelectorAll(selector) {
				return selector === ".col-resizer" ? handles : [];
			},
		},
	};
}

test("clampColumnWidth enforces per-side limits", () => {
	assert.equal(clampColumnWidth("left", 100), 180);
	assert.equal(clampColumnWidth("left", 9999), 480);
	assert.equal(clampColumnWidth("left", 300), 300);
	assert.equal(clampColumnWidth("right", 100), 200);
	assert.equal(clampColumnWidth("right", 9999), 640);
	assert.equal(clampColumnWidth("left", Number.NaN), 180);
});

test("initColumnLayout applies stored widths and persists after a drag", () => {
	const storage = createStorage({ "liteagent.layout.left": "300" });
	const left = fakeHandle("left");
	const right = fakeHandle("right");
	const doc = fakeDocument([left, right]);

	initColumnLayout({ document: doc, storage });
	assert.equal(doc.workspace.style.vars["--left-width"], "300px");
	assert.equal(doc.workspace.style.vars["--right-width"], `${DEFAULT_COLUMN_WIDTHS.right}px`);
	left.dispatch("pointerdown", { clientX: 100, pointerId: 1 });
	assert.equal(doc.body.classList.has("resizing"), true);
	left.dispatch("pointermove", { clientX: 160, pointerId: 1 });
	assert.equal(doc.workspace.style.vars["--left-width"], "360px");
	// Dragging has not persisted yet.
	assert.equal(storage.getItem("liteagent.layout.left"), "300");

	left.dispatch("pointerup", { clientX: 160, pointerId: 1 });
	assert.equal(doc.body.classList.has("resizing"), false);
	assert.equal(storage.getItem("liteagent.layout.left"), "360");
	assert.equal(storage.getItem("liteagent.layout.right"), "280");
});

test("dragging the right handle grows the right column on leftward movement", () => {
	const storage = createStorage();
	const right = fakeHandle("right");
	const doc = fakeDocument([right]);

	initColumnLayout({ document: doc, storage });
	right.dispatch("pointerdown", { clientX: 500, pointerId: 1 });
	right.dispatch("pointermove", { clientX: 420, pointerId: 1 });
	assert.equal(doc.workspace.style.vars["--right-width"], "360px");
	// Far past the lower limit: dragging right narrows the column to 200px.
	right.dispatch("pointermove", { clientX: 9999, pointerId: 1 });
	assert.equal(doc.workspace.style.vars["--right-width"], "200px");
	// Far past the upper limit: dragging far left grows it to 640px.
	right.dispatch("pointermove", { clientX: -9999, pointerId: 1 });
	assert.equal(doc.workspace.style.vars["--right-width"], "640px");
	right.dispatch("pointerup", { clientX: -9999, pointerId: 1 });
	assert.equal(storage.getItem("liteagent.layout.right"), "640");
});

test("keyboard arrows adjust the widths in 16px steps and persist", () => {
	const storage = createStorage();
	const left = fakeHandle("left");
	const right = fakeHandle("right");
	const doc = fakeDocument([left, right]);

	initColumnLayout({ document: doc, storage });
	left.dispatch("keydown", { key: "ArrowRight" });
	assert.equal(doc.workspace.style.vars["--left-width"], "248px");
	assert.equal(storage.getItem("liteagent.layout.left"), "248");

	// The right handle mirrors the arrows: ArrowRight narrows the right column.
	right.dispatch("keydown", { key: "ArrowRight" });
	assert.equal(doc.workspace.style.vars["--right-width"], "264px");
	right.dispatch("keydown", { key: "ArrowLeft" });
	assert.equal(doc.workspace.style.vars["--right-width"], "280px");

	left.dispatch("keydown", { key: "Enter" });
	assert.equal(doc.workspace.style.vars["--left-width"], "248px");
});

test("createColumnResizer clamps a drag that overshoots the limits", () => {
	const widths = () => ({ left: 232, right: 280 });
	const applied = [];
	const resizer = createColumnResizer({
		side: "left",
		storage: null,
		widths,
		onWidths: (next, persist) => applied.push({ ...next, persist }),
	});

	resizer.handleEvent({ type: "pointerdown", clientX: 0 });
	resizer.handleEvent({ type: "pointermove", clientX: 10_000 });
	resizer.handleEvent({ type: "pointerup", clientX: 10_000 });
	assert.deepEqual(applied[applied.length - 1], { left: 480, right: 280, persist: true });
});

test("applyColumnWidths tolerates a missing workspace", () => {
	assert.doesNotThrow(() => applyColumnWidths(null, { left: 200, right: 200 }));
});
