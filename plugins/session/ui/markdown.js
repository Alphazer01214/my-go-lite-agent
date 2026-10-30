import { el } from "./dom.js";

const INLINE_PATTERN = /(\*\*[^*]+\*\*|`[^`\n]+`|\[([^\]]+)\]\((https?:\/\/[^\s)]+)\))/g;

function addText(parent, text) {
	if (text) parent.append(document.createTextNode(text));
}

function appendInline(parent, text) {
	let last = 0;
	for (const match of text.matchAll(INLINE_PATTERN)) {
		addText(parent, text.slice(last, match.index));
		const token = match[0];
		if (token.startsWith("**")) {
			parent.append(el("strong", { text: token.slice(2, -2) }));
		} else if (token.startsWith("`")) {
			parent.append(el("code", { class: "md-inline", text: token.slice(1, -1) }));
		} else {
			parent.append(el("a", {
				href: match[3],
				target: "_blank",
				rel: "noreferrer noopener",
				text: match[2],
			}));
		}
		last = match.index + token.length;
	}
	addText(parent, text.slice(last));
}

function isBlockStart(line) {
	return /^```/.test(line)
		|| /^#{1,4}\s+/.test(line)
		|| /^\s*[-*]\s+/.test(line)
		|| /^\s*\d+[.)]\s+/.test(line)
		|| /^\s*>\s?/.test(line);
}

const LIST_ITEM = /^(\s*)([-*]|\d+[.)])\s+(.*)$/;

function parseList(lines, start, ordered) {
	const list = el(ordered ? "ol" : "ul", { class: "md-list" });
	let index = start;
	let baseIndent = -1;
	let item = null;

	const readItem = (line) => {
		const match = line.match(LIST_ITEM);
		if (!match) return null;
		return { indent: match[1].length, ordered: /^\d/.test(match[2]), text: match[3] };
	};

	while (index < lines.length) {
		const line = lines[index];
		const entry = readItem(line);
		if (entry && entry.ordered === ordered) {
			if (baseIndent < 0) baseIndent = entry.indent;
			if (entry.indent <= baseIndent) {
				item = el("li");
				appendInline(item, entry.text);
				list.append(item);
				index += 1;
				continue;
			}
		}
		if (!item) break;
		if (!line.trim()) {
			let look = index;
			while (look < lines.length && !lines[look].trim()) look += 1;
			if (look >= lines.length) break;
			const nextEntry = readItem(lines[look]);
			if (nextEntry && nextEntry.ordered === ordered && nextEntry.indent <= baseIndent) {
				index = look;
				continue;
			}
			if (/^\s{2,}\S/.test(lines[look])) {
				item.append(document.createTextNode("\n"));
				index += 1;
				continue;
			}
			break;
		}
		if (/^\s{2,}\S/.test(line)) {
			item.append(document.createTextNode("\n"));
			appendInline(item, line.trim());
			index += 1;
			continue;
		}
		break;
	}
	return { node: list, next: index };
}

export function renderMarkdown(text) {
	const fragment = document.createDocumentFragment();
	const lines = String(text || "").replace(/\r\n?/g, "\n").split("\n");
	let index = 0;

	while (index < lines.length) {
		const line = lines[index];
		if (!line.trim()) {
			index += 1;
			continue;
		}

		const fence = line.match(/^```(.*)$/);
		if (fence) {
			const language = fence[1].trim();
			const code = [];
			index += 1;
			while (index < lines.length && !/^```/.test(lines[index])) {
				code.push(lines[index]);
				index += 1;
			}
			if (index < lines.length) index += 1;
			const pre = el("pre", { class: "md-code" },
				language ? el("div", { class: "md-code-lang", text: language }) : null,
				el("code", { text: code.join("\n") }),
			);
			fragment.append(pre);
			continue;
		}

		const heading = line.match(/^(#{1,4})\s+(.*)$/);
		if (heading) {
			const level = Math.min(heading[1].length + 2, 6);
			const node = el(`h${level}`, { class: "md-heading" });
			appendInline(node, heading[2]);
			fragment.append(node);
			index += 1;
			continue;
		}

		if (/^\s*[-*]\s+/.test(line)) {
			const { node, next } = parseList(lines, index, false);
			fragment.append(node);
			index = next;
			continue;
		}

		if (/^\s*\d+[.)]\s+/.test(line)) {
			const { node, next } = parseList(lines, index, true);
			fragment.append(node);
			index = next;
			continue;
		}

		if (/^\s*>\s?/.test(line)) {
			const quote = el("blockquote", { class: "md-quote" });
			const buffer = [];
			while (index < lines.length && /^\s*>\s?/.test(lines[index])) {
				buffer.push(lines[index].replace(/^\s*>\s?/, ""));
				index += 1;
			}
			appendInline(quote, buffer.join("\n"));
			fragment.append(quote);
			continue;
		}

		const paragraph = [];
		while (index < lines.length) {
			const next = lines[index];
			if (!next.trim() || isBlockStart(next)) break;
			paragraph.push(next);
			index += 1;
		}
		const node = el("p", { class: "md-p" });
		appendInline(node, paragraph.join("\n"));
		fragment.append(node);
	}

	return fragment;
}
