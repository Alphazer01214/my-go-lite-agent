const NS = "http://www.w3.org/2000/svg";

const PATHS = {
	plus: ["M8 2.5v11", "M2.5 8h11"],
	search: ["M7 12.2a5.2 5.2 0 1 0 0-10.4 5.2 5.2 0 0 0 0 10.4Z", "M10.9 10.9l3.1 3.1"],
	bookmark: ["M4 2.5h8v11l-4-2.6-4 2.6Z"],
	edit: ["M10.6 2.6l2.8 2.8", "M12.2 4.2L5 11.4 2.5 13.5l2.1-2.5 7.2-7.2Z"],
	trash: ["M2.8 4.2h10.4", "M6 4.2V2.8h4v1.4", "M4.4 4.2l.7 9h5.8l.7-9", "M6.6 7v4.2", "M9.4 7v4.2"],
	send: ["M8 13V3", "M3.8 7.2L8 3l4.2 4.2"],
	stop: ["M4.6 4.6h6.8v6.8H4.6Z"],
	refresh: ["M13 8a5 5 0 1 1-1.6-3.7", "M13.2 2.8v3.1H10"],
	x: ["M3.5 3.5l9 9", "M12.5 3.5l-9 9"],
	chevron: ["M6 3.5L10.5 8 6 12.5"],
	arrowDown: ["M8 3v10", "M4 9l4 4 4-4"],
	folder: ["M2.5 4.2h4l1.2 1.5h5.8v7.1h-11Z"],
	sliders: ["M3 4h10", "M3 8h10", "M3 12h10", "M5.8 2.8v2.4", "M9.4 6.8v2.4", "M6.8 10.8v2.4"],
};

export function icon(name, size = 16) {
	const svg = document.createElementNS(NS, "svg");
	svg.setAttribute("viewBox", "0 0 16 16");
	svg.setAttribute("width", String(size));
	svg.setAttribute("height", String(size));
	svg.setAttribute("fill", "none");
	svg.setAttribute("stroke", "currentColor");
	svg.setAttribute("stroke-width", "1.4");
	svg.setAttribute("stroke-linecap", "round");
	svg.setAttribute("stroke-linejoin", "round");
	svg.setAttribute("aria-hidden", "true");
	for (const d of PATHS[name] || []) {
		const path = document.createElementNS(NS, "path");
		path.setAttribute("d", d);
		svg.append(path);
	}
	return svg;
}
