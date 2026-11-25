package report

const htmlPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Laws · {{.Product}}</title>
<link rel="icon" href="data:,">
<style>
/*css*/</style>
</head>
<body>
<div class="shell">
	<header class="page-header">
		<div class="page-header__row">
			<div class="page-header__main">
				<p class="eyebrow">{{.Product}} · proof gate</p>
				<h1 class="page-header__title">Laws</h1>
				<p class="page-header__meta">{{.Machines}} machines · {{.Laws}} laws{{if .Checks}} · {{.Checks}} gate checks{{end}}{{if .Generated}} · generated {{.Generated}}{{end}}{{if .Commit}} · <span class="cell-mono">{{.Commit}}</span>{{end}}</p>
			</div>
			<div class="page-header__actions">
				<button class="btn btn-outline-secondary btn-sm" id="theme" type="button">Switch theme</button>
			</div>
		</div>
	</header>

	<section class="stats" aria-label="Summary">
		<div class="card stat-card">
			<p class="stat-card__label">Machines</p>
			<p class="stat-card__value">{{.Machines}}</p>
			{{if .Failed}}<p class="stat-card__hint is-danger">{{.Failed}} could not be checked</p>{{end}}
		</div>
		<div class="card stat-card">
			<p class="stat-card__label">Laws</p>
			<p class="stat-card__value">{{.Laws}}</p>
			<p class="stat-card__hint">checked on every state and path</p>
		</div>
		<div class="card stat-card">
			<p class="stat-card__label">Broken</p>
			<p class="stat-card__value {{if .Broken}}is-danger{{else}}is-success{{end}}">{{.Broken}}</p>
		</div>
		<div class="card stat-card">
			<p class="stat-card__label">Pending</p>
			<p class="stat-card__value {{if .Pending}}is-warning{{else}}is-muted{{end}}">{{.Pending}}</p>
			<p class="stat-card__hint">{{if .Pending}}waiting on a decision{{else}}nothing waits on a decision{{end}}</p>
		</div>
	</section>

	<div class="verdict verdict--{{.Tone}}" role="status">
		<span class="status-pill status-pill--{{.Tone}}">{{.Verdict}}</span>
		<p>{{.Headline}}</p>
	</div>

	<div class="toolbar">
		<label class="search">
			<span class="visually-hidden">Filter machines or laws</span>
			<input id="filter" class="form-control" type="search" placeholder="Filter machines or laws…" autocomplete="off">
		</label>
		<div class="filters" role="group" aria-label="Show">
			<button class="chip is-active" type="button" data-show="all" aria-pressed="true">All</button>
			<button class="chip" type="button" data-show="broken" aria-pressed="false">Broken</button>
			<button class="chip" type="button" data-show="pending" aria-pressed="false">Pending</button>
		</div>
	</div>

	{{if .Attention}}
	<section class="section" id="attention" aria-label="Needs attention">
		<h2 class="section__title">Needs attention <span class="count">{{len .Attention}}</span></h2>
		<div class="card attention">
		{{range .Reasons}}
			<details class="reason reason--{{.Tone}}"{{if .Open}} open{{end}}>
				<summary><span class="status-pill status-pill--{{.Tone}}">{{len .Items}}</span><span class="reason__title">{{.Title}}</span></summary>
				<ul class="laws">
				{{range .Items}}
					<li class="law law--{{.Tone}}">
						<div class="law__row">
							<span class="law__name">{{.Name}}</span>
							<span class="law__where">{{.Where}}</span>
						</div>
						{{if .Detail}}<p class="law__doc">{{.Detail}}</p>{{end}}
						{{if .Violations}}<pre class="violations">{{join .Violations "\n"}}</pre>{{end}}
					</li>
				{{end}}
				</ul>
			</details>
		{{end}}
		</div>
	</section>
	{{end}}

	{{if .MachineCards}}
	<section class="section" aria-label="Machines">
		<h2 class="section__title">Machines <span class="count">{{len .MachineCards}}</span></h2>
		<main class="grid" id="machines">
		{{range .MachineCards}}
			<article class="card machine" data-verdict="{{template "verdict" .}}" data-name="{{.Name}}">
				{{template "head" .}}
				{{if .Attention}}<ul class="laws">{{range .Attention}}{{template "law" .}}{{end}}</ul>{{end}}
				{{if .Holding}}<details class="holding"><summary>{{len .Holding}} {{if eq (len .Holding) 1}}law holds{{else}}laws hold{{end}}</summary><ul class="laws">{{range .Holding}}{{template "law" .}}{{end}}</ul></details>{{end}}
			</article>
		{{end}}
		</main>
	</section>
	{{end}}

	{{if .Gates}}
	<section class="section" id="gates" aria-label="Gate checks">
		<h2 class="section__title">Gate checks</h2>
		<div class="toolbar">
			<label class="search">
				<span class="visually-hidden">Filter gate checks</span>
				<input id="gate-filter" class="form-control" type="search" placeholder="Filter gate checks or packages…" autocomplete="off">
			</label>
			<div class="filters" role="group" aria-label="Show gate checks">
				<button class="chip is-active" type="button" data-gate-show="all" aria-pressed="true">All</button>
				<button class="chip" type="button" data-gate-show="broken" aria-pressed="false">Broken</button>
				<button class="chip" type="button" data-gate-show="pending" aria-pressed="false">Pending</button>
				<button class="chip" type="button" data-gate-show="holds" aria-pressed="false">Holds</button>
			</div>
		</div>
		{{range .Gates}}
			<article class="card machine machine--wide" data-name="{{.Name}}">
				{{template "head" .}}
				{{if .Prefix}}<p class="machine__prefix">Names drop the shared prefix <code>{{.Prefix}}</code></p>{{end}}
				{{if .Groups}}<div class="groups">
				{{range .Groups}}
					<details class="group" data-group{{if .Open}} data-open open{{end}}>
						<summary>
							<span class="group__name">{{.Name}}</span>
							{{if .Broken}}<span class="status-pill status-pill--danger">{{.Broken}} broken</span>{{end}}
							{{if .Pending}}<span class="status-pill status-pill--info">{{.Pending}} pending</span>{{end}}
							{{if .Holds}}<span class="count">{{.Holds}} hold</span>{{end}}
						</summary>
						<ul class="checks">
						{{range .Checks}}
							<li class="check" data-state="{{lower .State}}">
								<div class="check__row">
									<span class="check__name">{{.Name}}</span>
									{{if ne .State "Holds"}}<span class="status-pill status-pill--{{.Tone}}">{{.State}}</span>{{end}}
								</div>
								{{if .Pending}}<p class="law__doc">{{.Pending}}</p>{{end}}
								{{if .Violations}}<pre class="violations">{{join .Violations "\n"}}</pre>{{end}}
							</li>
						{{end}}
						</ul>
					</details>
				{{end}}
				</div>{{end}}
			</article>
		{{end}}
		<p class="empty" id="gate-empty" hidden>No gate check matches the filter.</p>
	</section>
	{{end}}

	<p class="empty" id="empty" hidden>No machine or law matches the filter.</p>

	<footer class="footer">
		{{if .Reproduce}}<span>Run <code>{{.Reproduce}}</code> to reproduce this page.</span>{{end}}
		<span>Laws are rules; only the owner changes them.</span>
	</footer>
</div>
<script>
(function () {
	var root = document.documentElement;
	var saved = null;
	try { saved = localStorage.getItem("laws-theme"); } catch (e) {}
	if (saved) { root.setAttribute("data-theme", saved); }
	var toggle = document.getElementById("theme");
	toggle.addEventListener("click", function () {
		var dark = root.getAttribute("data-theme") === "dark" ||
			(!root.getAttribute("data-theme") && matchMedia("(prefers-color-scheme: dark)").matches);
		var next = dark ? "light" : "dark";
		root.setAttribute("data-theme", next);
		try { localStorage.setItem("laws-theme", next); } catch (e) {}
	});

	var input = document.getElementById("filter");
	var chips = document.querySelectorAll("[data-show]");
	var cards = document.querySelectorAll("[data-verdict]");
	var empty = document.getElementById("empty");
	var show = "all";

	function apply() {
		var q = input.value.trim().toLowerCase();
		var visible = 0;
		cards.forEach(function (card) {
			var text = card.textContent.toLowerCase();
			var verdict = card.getAttribute("data-verdict");
			var ok = (show === "all" || verdict === show) && (!q || text.indexOf(q) !== -1);
			card.hidden = !ok;
			if (ok) { visible++; }
			card.querySelectorAll("details").forEach(function (part) {
				part.open = !!q && part.textContent.toLowerCase().indexOf(q) !== -1;
			});
		});
		empty.hidden = visible !== 0;
	}
	input.addEventListener("input", apply);

	// Gate checks have their own filter: it shows single checks, hides the
	// packages left empty and opens the ones that match.
	var gateInput = document.getElementById("gate-filter");
	var gateChips = document.querySelectorAll("[data-gate-show]");
	var gateGroups = document.querySelectorAll("[data-group]");
	var gateEmpty = document.getElementById("gate-empty");
	var gateShow = "all";
	function applyGates() {
		var q = gateInput.value.trim().toLowerCase();
		var filtering = !!q || gateShow !== "all";
		var total = 0;
		gateGroups.forEach(function (group) {
			var name = group.querySelector(".group__name").textContent.toLowerCase();
			var whole = !!q && name.indexOf(q) !== -1;
			var shown = 0;
			group.querySelectorAll("[data-state]").forEach(function (check) {
				var state = gateShow === "all" || check.getAttribute("data-state") === gateShow;
				var text = !q || whole || check.textContent.toLowerCase().indexOf(q) !== -1;
				check.hidden = !(state && text);
				if (!check.hidden) { shown++; }
			});
			group.hidden = shown === 0;
			group.open = filtering ? shown > 0 : group.hasAttribute("data-open");
			total += shown;
		});
		if (gateEmpty) { gateEmpty.hidden = total !== 0; }
	}
	if (gateInput) {
		gateInput.addEventListener("input", applyGates);
		gateChips.forEach(function (chip) {
			chip.addEventListener("click", function () {
				gateShow = chip.getAttribute("data-gate-show");
				gateChips.forEach(function (c) {
					var on = c === chip;
					c.classList.toggle("is-active", on);
					c.setAttribute("aria-pressed", on ? "true" : "false");
				});
				applyGates();
			});
		});
	}
	chips.forEach(function (chip) {
		chip.addEventListener("click", function () {
			show = chip.getAttribute("data-show");
			chips.forEach(function (c) {
				var on = c === chip;
				c.classList.toggle("is-active", on);
				c.setAttribute("aria-pressed", on ? "true" : "false");
			});
			apply();
		});
	});
})();
</script>
</body>
</html>
{{define "verdict"}}{{if or .Failure .Broken}}broken{{else if .Pending}}pending{{else}}ok{{end}}{{end}}
{{define "head"}}<header class="machine__head">
	<div class="machine__title">
		{{if .Module}}<p class="eyebrow">{{.Module}}</p>{{end}}
		<h2>{{.Name}}</h2>
		{{if .Source}}<code class="machine__source">{{.Source}}</code>{{end}}
	</div>
	<div class="machine__meta">
		<span class="count">{{.Count}} {{if .GateChecks}}checks{{else if eq .Count 1}}law{{else}}laws{{end}}</span>
		<span class="status-pill status-pill--{{.Tone}}">{{.Verdict}}</span>
	</div>
</header>
{{if .Failure}}<p class="machine__failure">Could not be checked: {{.Failure}}</p>{{end}}{{end}}
{{define "law"}}<li class="law law--{{.Tone}}">
	<div class="law__row">
		<span class="law__name">{{.Name}}</span>
		<span class="status-pill status-pill--{{.Tone}}">{{.State}}</span>
	</div>
	{{if .Doc}}<p class="law__doc">{{.Doc}}</p>{{end}}
	{{if .Pending}}<p class="law__doc">{{.Pending}}</p>{{end}}
	{{if .Violations}}<pre class="violations">{{join .Violations "\n"}}</pre>{{end}}
</li>{{end}}
`

// htmlCSS carries the Pantacor design language: one purple accent, slate
// neutrals, cards on a darker canvas, soft status pills, Inter. Both themes
// come from custom properties, so nothing hard-codes a light surface.
const htmlCSS = `
:root {
	--font-sans: "Inter", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
	--font-mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
	--accent: #6c63ff;
	--accent-text: #4b43c9;
	--accent-soft-bg: rgba(108, 99, 255, 0.1);
	--canvas: #e9ecef;
	--surface: #fff;
	--surface-tertiary: #f8fafc;
	--border: #e2e8f0;
	--page-divider: #cbd5e1;
	--text: #334155;
	--text-strong: #0f172a;
	--text-muted: #64748b;
	--link: #4b43c9;
	--input-bg: #fff;
	--input-fg: #221f48;
	--input-border: #cbd5e1;
	--code-bg: #f8fafc;
	--code-fg: #334155;
	--badge-success-bg: #d1fae5; --badge-success-fg: #047857;
	--badge-info-bg: #dbeafe;    --badge-info-fg: #1d4ed8;
	--badge-warning-bg: #fef3c7; --badge-warning-fg: #92400e;
	--badge-danger-bg: #fee2e2;  --badge-danger-fg: #b91c1c;
	--badge-neutral-bg: #e2e8f0; --badge-neutral-fg: #334155;
	--radius: 6px;
	--radius-lg: 8px;
	--shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
	--shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.1);
	--focus-ring: 0 0 0 0.2rem rgba(108, 99, 255, 0.3);
	color-scheme: light;
}
:root[data-theme="dark"] {
	--canvas: #262a2f;
	--surface: #2f353c;
	--surface-tertiary: #343a40;
	--border: #495057;
	--page-divider: rgba(255, 255, 255, 0.14);
	--text: #dee2e6;
	--text-strong: #f8f9fa;
	--text-muted: #94a3b8;
	--accent-text: #a5a0ff;
	--accent-soft-bg: rgba(108, 99, 255, 0.2);
	--link: #a5a0ff;
	--input-bg: #343a40;
	--input-fg: #dee2e6;
	--input-border: #495057;
	--code-bg: rgba(255, 255, 255, 0.06);
	--code-fg: #cbd5e1;
	--badge-success-bg: rgba(16, 185, 129, 0.18); --badge-success-fg: #6ee7b7;
	--badge-info-bg: rgba(59, 130, 246, 0.22);    --badge-info-fg: #93c5fd;
	--badge-warning-bg: rgba(245, 158, 11, 0.2);  --badge-warning-fg: #fcd34d;
	--badge-danger-bg: rgba(239, 68, 68, 0.2);    --badge-danger-fg: #fca5a5;
	--badge-neutral-bg: rgba(148, 163, 184, 0.22); --badge-neutral-fg: #e2e8f0;
	--shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.3);
	--shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.4);
	color-scheme: dark;
}
@media (prefers-color-scheme: dark) {
	:root:not([data-theme="light"]) {
		--canvas: #262a2f;
		--surface: #2f353c;
		--surface-tertiary: #343a40;
		--border: #495057;
		--page-divider: rgba(255, 255, 255, 0.14);
		--text: #dee2e6;
		--text-strong: #f8f9fa;
		--text-muted: #94a3b8;
		--accent-text: #a5a0ff;
		--accent-soft-bg: rgba(108, 99, 255, 0.2);
		--link: #a5a0ff;
		--input-bg: #343a40;
		--input-fg: #dee2e6;
		--input-border: #495057;
		--code-bg: rgba(255, 255, 255, 0.06);
		--code-fg: #cbd5e1;
		--badge-success-bg: rgba(16, 185, 129, 0.18); --badge-success-fg: #6ee7b7;
		--badge-info-bg: rgba(59, 130, 246, 0.22);    --badge-info-fg: #93c5fd;
		--badge-warning-bg: rgba(245, 158, 11, 0.2);  --badge-warning-fg: #fcd34d;
		--badge-danger-bg: rgba(239, 68, 68, 0.2);    --badge-danger-fg: #fca5a5;
		--badge-neutral-bg: rgba(148, 163, 184, 0.22); --badge-neutral-fg: #e2e8f0;
		--shadow-sm: 0 1px 2px 0 rgba(0, 0, 0, 0.3);
		--shadow-md: 0 4px 6px -1px rgba(0, 0, 0, 0.4);
		color-scheme: dark;
	}
}
*, *::before, *::after { box-sizing: border-box; }
html { -webkit-text-size-adjust: 100%; }
body {
	margin: 0;
	font-family: var(--font-sans);
	font-size: 14px;
	line-height: 1.5;
	color: var(--text);
	background: var(--canvas);
	transition: background-color 0.15s, color 0.15s;
}
a { color: var(--link); }
code { font-family: var(--font-mono); font-size: 12px; background: var(--code-bg); color: var(--code-fg); border-radius: 4px; padding: 0.1em 0.35em; }
h1, h2 { margin: 0; }
[hidden] { display: none !important; }
.visually-hidden { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }

.shell { max-width: 1280px; margin: 0 auto; padding: 2.5rem 2rem 3rem; }

.page-header { margin-bottom: 1.5rem; }
.eyebrow { margin: 0 0 0.25rem; font-size: 11px; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--text-muted); }
.page-header__row { display: flex; flex-wrap: wrap; gap: 1rem; align-items: flex-end; justify-content: space-between; }
.page-header__title { font-size: 30px; font-weight: 700; letter-spacing: -0.01em; line-height: 1.2; color: var(--text-strong); }
.page-header__meta { margin: 0.375rem 0 0; font-size: 14px; color: var(--text-muted); }
.page-header__actions { display: flex; gap: 0.5rem; }

.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); box-shadow: var(--shadow-sm); }

.stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1rem; margin-bottom: 1.25rem; }
.stat-card { padding: 1rem 1.25rem; }
.stat-card__label { margin: 0; font-size: 13px; font-weight: 500; color: var(--text-muted); }
.stat-card__value { margin: 0.125rem 0 0; font-size: 28px; font-weight: 700; color: var(--text-strong); }
.stat-card__hint { margin: 0.125rem 0 0; font-size: 12px; color: var(--text-muted); }
.is-success { color: var(--badge-success-fg); }
.is-danger { color: var(--badge-danger-fg); }
.is-warning { color: var(--badge-warning-fg); }
.is-muted { color: var(--text-muted); }

.verdict { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 1.25rem; padding: 0.875rem 1.125rem; border-radius: var(--radius-lg); border: 1px solid var(--border); background: var(--surface); box-shadow: var(--shadow-sm); }
.verdict p { margin: 0; font-weight: 500; color: var(--text-strong); }
.verdict--success { border-left: 3px solid var(--badge-success-fg); }
.verdict--danger { border-left: 3px solid var(--badge-danger-fg); }
.verdict--warning { border-left: 3px solid var(--badge-warning-fg); }

.toolbar { display: flex; flex-wrap: wrap; gap: 0.75rem; align-items: center; justify-content: space-between; margin-bottom: 1rem; }
.search { flex: 1 1 280px; max-width: 420px; }
.form-control {
	width: 100%; padding: 0.5rem 0.75rem; font: inherit; font-weight: 500;
	color: var(--input-fg); background: var(--input-bg);
	border: 1px solid var(--input-border); border-radius: var(--radius);
}
.form-control::placeholder { color: var(--text-muted); font-weight: 400; opacity: 1; }
.form-control:focus-visible { outline: none; border-color: var(--accent); box-shadow: var(--focus-ring); }
.filters { display: flex; gap: 0.5rem; }
.chip {
	padding: 0.3rem 0.75rem; font: inherit; font-size: 13px; font-weight: 500;
	color: var(--text); background: transparent; border: 1px solid var(--border);
	border-radius: 999px; cursor: pointer; transition: background-color 0.15s, border-color 0.15s, color 0.15s;
}
.chip:hover { background: var(--surface-tertiary); }
.chip.is-active { background: var(--accent-soft-bg); border-color: var(--accent); color: var(--accent-text); font-weight: 600; }
.chip:focus-visible { outline: none; box-shadow: var(--focus-ring); }

.btn {
	display: inline-flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem;
	font: inherit; font-weight: 500; border-radius: var(--radius); cursor: pointer;
	border: 1px solid transparent; transition: background-color 0.15s, border-color 0.15s, color 0.15s;
}
.btn-sm { padding: 0.25rem 0.75rem; font-size: 13px; }
.btn-outline-secondary { background: transparent; border-color: var(--input-border); color: var(--text); }
.btn-outline-secondary:hover { background: var(--surface-tertiary); }
.btn:focus-visible { outline: none; box-shadow: var(--focus-ring); }

.status-pill {
	display: inline-flex; align-items: center; gap: 0.375rem;
	padding: 0.125rem 0.5rem; border-radius: var(--radius);
	font-size: 12px; font-weight: 600; line-height: 1.5; white-space: nowrap;
	background: var(--badge-neutral-bg); color: var(--badge-neutral-fg);
}
.status-pill--success { background: var(--badge-success-bg); color: var(--badge-success-fg); }
.status-pill--info { background: var(--badge-info-bg); color: var(--badge-info-fg); }
.status-pill--warning { background: var(--badge-warning-bg); color: var(--badge-warning-fg); }
.status-pill--danger { background: var(--badge-danger-bg); color: var(--badge-danger-fg); }

.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 1rem; }
.machine { display: flex; flex-direction: column; overflow: hidden; }
.machine--wide { overflow: hidden; }
.section { margin-top: 1.75rem; }
.section__title { display: flex; gap: 0.5rem; align-items: baseline; margin: 0 0 0.75rem; font-size: 16px; font-weight: 600; color: var(--text-strong); }
.attention { list-style: none; margin: 0; padding: 0; overflow: hidden; }
.law__where { margin: 0; flex-shrink: 0; max-width: 45%; text-align: right; font-family: var(--font-mono); font-size: 12px; color: var(--text-muted); overflow-wrap: anywhere; }
.machine__prefix { margin: 0; padding: 0.5rem 1.125rem; font-size: 12px; color: var(--text-muted); border-bottom: 1px solid var(--border); overflow-wrap: anywhere; }
.reason > summary, .holding > summary, .group > summary { display: flex; gap: 0.5rem; align-items: baseline; justify-content: space-between; padding: 0.625rem 1.125rem; cursor: pointer; list-style: none; font-size: 12.5px; color: var(--text-muted); }
.reason > summary::-webkit-details-marker, .holding > summary::-webkit-details-marker, .group > summary::-webkit-details-marker { display: none; }
.reason > summary::before, .holding > summary::before, .group > summary::before { content: "\25B8"; margin-right: 0.375rem; transition: transform 0.15s; }
.reason[open] > summary::before, .holding[open] > summary::before, .group[open] > summary::before { transform: rotate(90deg); }
.holding > summary, .reason > summary { justify-content: flex-start; }
.reason { border-bottom: 1px solid var(--border); }
.reason:last-child { border-bottom: 0; }
.reason[open] > summary { border-bottom: 1px solid var(--border); }
.reason__title { font-weight: 600; color: var(--text-strong); }
.holding[open] > summary { border-bottom: 1px solid var(--border); }
.laws--cols, .groups { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); }
.laws--cols .law, .group { border-bottom: 1px solid var(--border); border-right: 1px solid var(--border); }
.group > summary { justify-content: flex-start; }
.group__name { flex: 1; font-family: var(--font-mono); font-size: 12.5px; font-weight: 600; color: var(--text-strong); overflow-wrap: anywhere; }
.checks { list-style: none; margin: 0; padding: 0 1.125rem 0.75rem 2rem; }
.check { padding: 0.125rem 0; }
.check__row { display: flex; gap: 0.5rem; align-items: center; justify-content: space-between; }
.check__name { font-family: var(--font-mono); font-size: 12px; color: var(--text); overflow-wrap: anywhere; }
.check .violations { font-size: 11px; }
.group .status-pill { flex-shrink: 0; }
#gates .toolbar { margin-bottom: 0.75rem; }
.machine__head { display: flex; flex-wrap: wrap; gap: 0.75rem; align-items: flex-start; justify-content: space-between; padding: 1rem 1.125rem; border-bottom: 1px solid var(--border); }
.machine__title { min-width: 0; }
.machine__title h2 { font-size: 15px; font-weight: 600; color: var(--text-strong); overflow-wrap: anywhere; }
.machine__source { display: inline-block; margin-top: 0.25rem; overflow-wrap: anywhere; }
.machine__meta { display: flex; align-items: center; gap: 0.5rem; }
.count { font-size: 12px; color: var(--text-muted); }
.machine__failure { margin: 0; padding: 0.75rem 1.125rem; font-size: 12.5px; color: var(--badge-danger-fg); background: color-mix(in srgb, var(--badge-danger-bg) 45%, transparent); }

.laws { list-style: none; margin: 0; padding: 0; }
.law { padding: 0.75rem 1.125rem; border-bottom: 1px solid var(--border); }
.law:last-child { border-bottom: 0; }
.law--danger { background: color-mix(in srgb, var(--badge-danger-bg) 45%, transparent); }
.law--info { background: color-mix(in srgb, var(--badge-info-bg) 35%, transparent); }
.law__row { display: flex; gap: 0.5rem; align-items: center; justify-content: space-between; }
.law__name { font-family: var(--font-mono); font-size: 12.5px; font-weight: 600; color: var(--text-strong); overflow-wrap: anywhere; }
.law__doc { margin: 0.25rem 0 0; font-size: 12.5px; color: var(--text-muted); }
.violations {
	margin: 0.5rem 0 0; padding: 0.625rem 0.75rem; overflow-x: auto;
	font-family: var(--font-mono); font-size: 12px; line-height: 1.45;
	color: #e2e8f0; background: #0f141a; border-radius: var(--radius);
}

.empty { padding: 2rem; text-align: center; color: var(--text-muted); }
.footer { display: flex; flex-wrap: wrap; gap: 0.5rem 1.5rem; justify-content: space-between; margin-top: 2rem; padding-top: 1rem; border-top: 1px solid var(--page-divider); font-size: 12px; color: var(--text-muted); }

@media (max-width: 720px) {
	.shell { padding: 1.5rem 1rem 2rem; }
	.stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
	.grid { grid-template-columns: 1fr; }
	.page-header__title { font-size: 26px; }
}
@media (prefers-reduced-motion: reduce) {
	* { transition: none !important; }
}
@media print {
	body { background: #fff; }
	.toolbar, .page-header__actions, .footer { display: none; }
	.machine { break-inside: avoid; }
}
`
