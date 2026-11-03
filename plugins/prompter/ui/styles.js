export const panelStyles = `
:host {
	display: flex;
	flex-direction: column;
	min-height: 0;
	height: 100%;
	overflow: auto;
	color: inherit;
	font: 12px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
}
.panel {
	display: flex;
	flex-direction: column;
	gap: 10px;
	padding: 10px 12px;
	min-height: 0;
}
.head {
	display: flex;
	align-items: center;
	gap: 8px;
	flex-wrap: wrap;
}
.title {
	font-weight: 600;
	font-size: 13px;
}
.spacer { flex: 1; }
.muted { opacity: .65; }
.badge {
	display: inline-flex;
	align-items: center;
	padding: 1px 6px;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 999px;
	font-size: 11px;
	text-transform: uppercase;
	letter-spacing: .04em;
	opacity: .85;
}
.badge[data-kind="live"] { border-color: rgba(52, 168, 83, .6); }
.badge[data-kind="derived"] { border-color: rgba(66, 133, 244, .6); }
.badge[data-kind="none"] { opacity: .55; }
button {
	font: inherit;
	color: inherit;
	background: transparent;
	border: 1px solid rgba(127, 127, 127, .35);
	border-radius: 5px;
	padding: 2px 8px;
	cursor: pointer;
}
button:hover { background: rgba(127, 127, 127, .12); }
button[disabled] { opacity: .5; cursor: default; }
.meta, .dirs {
	display: flex;
	flex-wrap: wrap;
	gap: 6px;
}
.kv {
	display: inline-flex;
	gap: 4px;
	align-items: baseline;
	padding: 1px 6px;
	border-radius: 4px;
	background: rgba(127, 127, 127, .1);
	max-width: 100%;
}
.kv .k { opacity: .6; }
.kv .v { overflow-wrap: anywhere; }
.usage {
	display: flex;
	flex-direction: column;
	gap: 4px;
}
.usage-text { opacity: .8; }
.progress {
	height: 4px;
	border-radius: 999px;
	background: rgba(127, 127, 127, .2);
	overflow: hidden;
}
.progress-fill {
	height: 100%;
	background: currentColor;
	opacity: .55;
}
.body {
	display: flex;
	flex-direction: column;
	gap: 12px;
	min-height: 0;
}
.section-title {
	font-size: 11px;
	text-transform: uppercase;
	letter-spacing: .06em;
	opacity: .6;
	margin: 0 0 4px;
}
details {
	border: 1px solid rgba(127, 127, 127, .22);
	border-radius: 5px;
	padding: 4px 6px;
	margin: 4px 0;
}
summary {
	cursor: pointer;
	display: flex;
	gap: 6px;
	align-items: baseline;
	flex-wrap: wrap;
}
.role { font-weight: 600; }
.tag {
	font-size: 10px;
	padding: 0 4px;
	border-radius: 3px;
	background: rgba(127, 127, 127, .16);
	opacity: .8;
}
pre {
	margin: 6px 0 0;
	padding: 6px;
	border-radius: 4px;
	background: rgba(127, 127, 127, .08);
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	max-height: 320px;
	overflow: auto;
}
.empty {
	padding: 8px 0;
	opacity: .65;
}
.error {
	padding: 6px 8px;
	border: 1px solid rgba(220, 80, 80, .5);
	border-radius: 5px;
	color: #d36;
	overflow-wrap: anywhere;
}
.grid {
	display: grid;
	grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
	gap: 12px;
}
@media (max-width: 900px) {
	.grid { grid-template-columns: minmax(0, 1fr); }
}
.item {
	border: 1px solid rgba(127, 127, 127, .22);
	border-radius: 5px;
	padding: 6px 8px;
	margin: 4px 0;
}
.item-head {
	display: flex;
	align-items: baseline;
	gap: 6px;
	flex-wrap: wrap;
}
.item-name { font-weight: 600; }
.item-desc { opacity: .8; margin-top: 2px; }
.item-path { opacity: .55; margin-top: 2px; overflow-wrap: anywhere; }
.tools {
	margin: 4px 0 0;
	padding-left: 16px;
	opacity: .85;
}
.hint { opacity: .6; margin-top: 6px; }
.config {
	border: 1px solid rgba(127, 127, 127, .22);
	border-radius: 5px;
	padding: 4px 6px;
	margin-top: 4px;
}
.config textarea {
	display: block;
	width: 100%;
	box-sizing: border-box;
	min-height: 160px;
	margin-top: 6px;
	padding: 6px;
	border: 1px solid rgba(127, 127, 127, .3);
	border-radius: 4px;
	background: rgba(127, 127, 127, .08);
	color: inherit;
	font: 11px/1.45 ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
	resize: vertical;
}
.config-actions {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-top: 6px;
}
.config-status { opacity: .7; overflow-wrap: anywhere; }
.config-status.ok { color: #2f9e44; }
.config-status.error { color: #d36; }
`;
