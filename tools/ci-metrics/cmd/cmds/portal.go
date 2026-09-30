// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

const defaultPortalCacheTTL = 15 * time.Minute

// PortalCommand serves a tiny, dependency-free web UI for CI metrics.
type PortalCommand struct {
	cmd *kingpin.CmdClause

	athenaConfig athena.Config
	tables       report.Tables
	branches     []string
	listen       string
	cacheTTL     time.Duration
	minExecs     int
}

// NewPortalCommand registers the portal subcommand on app.
func NewPortalCommand(app *kingpin.Application) *PortalCommand {
	c := &PortalCommand{cacheTTL: defaultPortalCacheTTL}
	c.cmd = app.Command("portal", "Serve a simple web portal for CI metrics")
	c.cmd.Flag("listen", "Address to listen on").Default(":8080").StringVar(&c.listen)
	c.cmd.Flag("cache-ttl", "How long to cache Athena query results").Default(defaultPortalCacheTTL.String()).DurationVar(&c.cacheTTL)
	c.cmd.Flag("branches", "Comma-separated full refs whose runs count, merge-queue runs targeting them included").
		Default(strings.Join(report.DefaultFlakyBranches, ",")).
		PlaceHolder("REF,...").
		SetValue(&csvValue{target: &c.branches})
	c.cmd.Flag("min-execs", "Minimum executions before a test is considered flaky").
		Default(strconv.Itoa(report.DefaultFlakyMinExecs)).
		IntVar(&c.minExecs)
	c.cmd.Flag("meta-table", "Parquet meta table to read").Default(report.DefaultMetaTable).StringVar(&c.tables.Meta)
	c.cmd.Flag("testcases-table", "Parquet testcases table to read").Default(report.DefaultTestcasesTable).StringVar(&c.tables.Testcases)
	registerAthenaConfigFlags(c.cmd, &c.athenaConfig)
	return c
}

func (c *PortalCommand) FullCommand() string { return c.cmd.FullCommand() }

// Run starts the HTTP server.
func (c *PortalCommand) Run(ctx context.Context) error {
	exec, err := athena.NewFromConfig(ctx, c.athenaConfig)
	if err != nil {
		return trace.Wrap(err, "creating athena client")
	}

	p := &portalServer{
		exec:     exec,
		database: c.athenaConfig.Database,
		tables:   c.tables,
		branches: c.branches,
		cacheTTL: c.cacheTTL,
		minExecs: c.minExecs,
		cache:    make(map[string]cacheEntry),
	}

	srv := &http.Server{Addr: c.listen, Handler: p.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("serving ci metrics portal on http://%s\n", c.listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return trace.Wrap(err, "serving portal")
	}
	return nil
}

type portalServer struct {
	exec athena.Executor

	database string
	tables   report.Tables
	branches []string
	cacheTTL time.Duration
	minExecs int

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	expires time.Time
	value   any
}

func (p *portalServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.index)
	mux.HandleFunc("/api/config", p.config)
	mux.HandleFunc("/api/flaky", p.flaky)
	mux.HandleFunc("/api/failures", p.failures)
	return mux
}

func (p *portalServer) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(portalHTML))
}

func (p *portalServer) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"branches": p.branches})
}

func (p *portalServer) flaky(w http.ResponseWriter, r *http.Request) {
	from, to, err := requestWindow(r, defaultFrom(), time.Now().UTC())
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	branches, err := p.selectedBranches(r)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	key := "flaky:" + report.Day(from) + ":" + report.Day(to) + ":" + strings.Join(branches, ",")
	value, err := p.cached(r.Context(), key, func(ctx context.Context) (any, error) {
		return report.ExecuteFlakyRows(ctx, p.exec, p.scope(from, to), report.FlakyParams{
			MinExecs: p.minExecs,
			Top:      report.DefaultFlakyTop,
			Branches: branches,
		})
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"from": report.Day(from), "to": report.Day(to), "rows": value})
}

func (p *portalServer) failures(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("test"))
	if q == "" {
		writeError(w, trace.BadParameter("test is required"), http.StatusBadRequest)
		return
	}
	classname := strings.TrimSpace(r.URL.Query().Get("package"))
	if classname == "" {
		// Backward compatible with the first portal draft and direct API callers.
		classname = strings.TrimSpace(r.URL.Query().Get("class"))
	}
	from, to, err := requestWindow(r, defaultFrom(), time.Now().UTC())
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	limit, err := report.AtoiDefault(r.URL.Query().Get("limit"), report.DefaultFailedRunsLimit)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	branches, err := p.selectedBranches(r)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	key := "failures:" + report.Day(from) + ":" + report.Day(to) + ":" + strings.Join(branches, ",") + ":" + classname + ":" + q + ":" + strconv.Itoa(limit)
	value, err := p.cached(r.Context(), key, func(ctx context.Context) (any, error) {
		return report.ExecuteFailedRuns(ctx, p.exec, p.scope(from, to), report.FailedRunParams{
			Classname: classname,
			TestName:  q,
			Branches:  branches,
			Limit:     limit,
		})
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"from": report.Day(from), "to": report.Day(to), "rows": value})
}

func (p *portalServer) cached(ctx context.Context, key string, load func(context.Context) (any, error)) (any, error) {
	if p.cacheTTL > 0 {
		p.mu.Lock()
		if ent, ok := p.cache[key]; ok && time.Now().Before(ent.expires) {
			p.mu.Unlock()
			return ent.value, nil
		}
		p.mu.Unlock()
	}

	value, err := load(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if p.cacheTTL > 0 {
		p.mu.Lock()
		p.cache[key] = cacheEntry{expires: time.Now().Add(p.cacheTTL), value: value}
		p.mu.Unlock()
	}
	return value, nil
}

func (p *portalServer) scope(from, to time.Time) report.Scope {
	return report.Scope{Database: p.database, Tables: p.tables, From: from, To: to}
}

func (p *portalServer) selectedBranches(r *http.Request) ([]string, error) {
	requested := strings.TrimSpace(r.URL.Query().Get("branches"))
	if requested == "" {
		return p.branches, nil
	}

	allowed := make(map[string]struct{}, len(p.branches))
	for _, branch := range p.branches {
		allowed[branch] = struct{}{}
	}

	seen := make(map[string]struct{})
	var selected []string
	for part := range strings.SplitSeq(requested, ",") {
		branch := strings.TrimSpace(part)
		if branch == "" {
			continue
		}
		if _, ok := allowed[branch]; !ok {
			return nil, trace.BadParameter("branch %q is not configured for this portal", branch)
		}
		if _, ok := seen[branch]; ok {
			continue
		}
		seen[branch] = struct{}{}
		selected = append(selected, branch)
	}
	if len(selected) == 0 {
		return nil, trace.BadParameter("select at least one branch")
	}
	return selected, nil
}

func requestWindow(r *http.Request, defaultFrom, defaultTo time.Time) (time.Time, time.Time, error) {
	from, to := defaultFrom, defaultTo
	var err error
	if s := r.URL.Query().Get("from"); s != "" {
		from, err = report.ParseDay(s)
		if err != nil {
			return time.Time{}, time.Time{}, trace.Wrap(err)
		}
	}
	if s := r.URL.Query().Get("to"); s != "" {
		to, err = report.ParseDay(s)
		if err != nil {
			return time.Time{}, time.Time{}, trace.Wrap(err)
		}
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, trace.BadParameter("from must be before or equal to to")
	}
	return from.UTC(), to.UTC(), nil
}

func defaultFrom() time.Time { return time.Now().UTC().AddDate(0, 0, -1) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		writeError(w, err, http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

const portalHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>CI Metrics</title>
  <style>
    :root { color-scheme: light; font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f6f8fb; color: #172033; }
    main { width: min(1760px, calc(100vw - 48px)); margin: 0 auto; padding: 32px 0 56px; }
    header { margin-bottom: 24px; }
    h1 { margin: 0 0 6px; font-size: 2.2rem; letter-spacing: -0.03em; }
    h2 { margin-top: 0; }
    .subtle, .status, .hint { color: #667085; }
    .card { background: white; border: 1px solid #e4e7ec; border-radius: 14px; box-shadow: 0 10px 25px rgba(16,24,40,.06); padding: 20px; margin-bottom: 20px; }
    .filters-card { padding: 12px 16px; box-shadow: none; }
    .filters-card form { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: 10px 32px; margin: 0; }
    .filters-card fieldset { flex: 0 1 auto; max-width: 720px; margin-left: auto; }
    .filters-card input { min-height: 30px; }
    .filters-card button { padding: 6px 12px; }
    form { display: grid; gap: 14px; margin-bottom: 16px; }
    .filters { display: flex; flex-wrap: wrap; gap: 10px 14px; align-items: end; }
    label { font-weight: 600; color: #344054; }
    input { display: block; margin-top: 4px; min-height: 34px; border: 1px solid #d0d5dd; border-radius: 8px; padding: 5px 10px; font: inherit; }
    input[type="text"], input:not([type]) { width: min(42rem, 80vw); }
    button { border: 1px solid #175cd3; border-radius: 8px; background: #175cd3; color: white; font: inherit; font-weight: 700; padding: 8px 14px; cursor: pointer; }
    button.secondary { border-color: #d0d5dd; background: white; color: #344054; }
    button.link { border: 0; background: transparent; color: #175cd3; padding: 0; text-align: left; font-weight: 700; }
    fieldset { border: 1px solid #e4e7ec; border-radius: 10px; padding: 8px 10px; }
    legend { color: #667085; font-size: .9rem; font-weight: 700; padding: 0 6px; }
    #branches { display: flex; flex-wrap: wrap; gap: 6px 14px; }
    #branches label { font-size: .9rem; font-weight: 500; white-space: nowrap; }
    #branches input { display: inline; min-height: auto; margin: 0 4px 0 0; padding: 0; }
    .tabs { display: flex; gap: 8px; margin-bottom: 0; }
    .tab { border-color: #d0d5dd; background: white; color: #344054; border-bottom-left-radius: 0; border-bottom-right-radius: 0; }
    .tab[aria-selected="true"] { background: #175cd3; border-color: #175cd3; color: white; }
    .panel { border-top-left-radius: 0; }
    [hidden] { display: none !important; }
    .table-wrap { overflow-x: auto; border: 1px solid #e4e7ec; border-radius: 10px; }
    table { width: 100%; border-collapse: collapse; background: white; }
    th, td { padding: 10px 12px; border-bottom: 1px solid #eaecf0; vertical-align: top; }
    th { background: #f9fafb; color: #475467; text-align: left; font-size: .85rem; text-transform: uppercase; letter-spacing: .03em; white-space: nowrap; }
    td.number { text-align: right; font-variant-numeric: tabular-nums; }
    td.wrap { min-width: 14rem; overflow-wrap: anywhere; }
    td.nowrap { white-space: nowrap; }
    tr:last-child td { border-bottom: 0; }
    a { color: #175cd3; font-weight: 700; }
    .empty { padding: 18px; text-align: center; color: #667085; }
  </style>
</head>
<body>
  <main>
    <header>
      <h1>CI Metrics</h1>
      <p class="subtle">Find flaky tests and the GitHub Actions runs where they failed.</p>
    </header>

    <section class="card filters-card">
      <form id="range-form">
        <div class="filters">
          <label>From <input id="from" name="from" type="date"></label>
          <label>To <input id="to" name="to" type="date"></label>
          <button type="submit">Refresh</button>
        </div>
        <fieldset>
          <legend>Target branches</legend>
          <div id="branches"></div>
        </fieldset>
      </form>
    </section>

    <nav class="tabs" role="tablist" aria-label="CI metric views">
      <button id="top-tab" class="tab" type="button" role="tab" aria-controls="top-panel" aria-selected="true">Top flaky tests</button>
      <button id="search-tab" class="tab" type="button" role="tab" aria-controls="search-panel" aria-selected="false">Search failed runs</button>
    </nav>

    <section id="top-panel" class="card panel" role="tabpanel" aria-labelledby="top-tab">
      <h2>Top 20 flaky tests</h2>
      <p class="hint">Click a test name to switch to failed-run search for that test.</p>
      <p id="flaky-status" class="status"></p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr><th>#</th><th>Test</th><th>Package</th><th>Execs</th><th>Fails</th><th>Fail %</th></tr>
          </thead>
          <tbody id="flaky-body"></tbody>
        </table>
      </div>
    </section>

    <section id="search-panel" class="card panel" role="tabpanel" aria-labelledby="search-tab" hidden>
      <h2>Failed GitHub runs</h2>
      <p class="hint">Search by test name, or click a test from the top flaky list.</p>
      <form id="search-form">
        <div class="filters">
          <label>Test name <input id="test" name="test" placeholder="e.g. TestProxyJump" required></label>
          <button type="submit">Search failed runs</button>
          <button type="button" id="clear-search" class="secondary">Clear</button>
        </div>
      </form>
      <p id="failures-status" class="status">Select a flaky test or search by test name.</p>
      <div class="table-wrap">
        <table>
          <thead>
            <tr><th>Time</th><th>Branch</th><th>Run</th></tr>
          </thead>
          <tbody id="failures-body"></tbody>
        </table>
      </div>
    </section>
  </main>
<script>
const from = document.getElementById('from');
const to = document.getElementById('to');
const test = document.getElementById('test');
const branches = document.getElementById('branches');
const flaky_body = document.getElementById('flaky-body');
const failures_body = document.getElementById('failures-body');
const flaky_status = document.getElementById('flaky-status');
const failures_status = document.getElementById('failures-status');
const topTab = document.getElementById('top-tab');
const searchTab = document.getElementById('search-tab');
const topPanel = document.getElementById('top-panel');
const searchPanel = document.getElementById('search-panel');
const today = new Date();
let failuresRequest = 0;
const yesterday = new Date(Date.now() - 24*60*60*1000);
const day = d => d.toISOString().slice(0, 10);
from.value = day(yesterday);
to.value = day(today);

function showTab(name) {
  const search = name === 'search';
  topPanel.hidden = search;
  searchPanel.hidden = !search;
  topTab.setAttribute('aria-selected', String(!search));
  searchTab.setAttribute('aria-selected', String(search));
  location.hash = search ? 'search' : 'top';
  if (search) test.focus();
}

async function getJSON(url) {
  const r = await fetch(url);
  const data = await r.json();
  if (!r.ok) throw new Error(data.error || r.statusText);
  return data;
}

function selectedBranches() {
  return Array.from(document.querySelectorAll('input[name="branch"]:checked')).map(i => i.value);
}

function queryPrefix() {
  const selected = selectedBranches();
  if (!selected.length) throw new Error('Select at least one target branch.');
  return 'from=' + encodeURIComponent(from.value) + '&to=' + encodeURIComponent(to.value) + '&branches=' + encodeURIComponent(selected.join(','));
}

async function loadConfig() {
  const data = await getJSON('/api/config');
  branches.textContent = '';
  for (const branch of data.branches) {
    const label = document.createElement('label');
    const input = document.createElement('input');
    input.type = 'checkbox';
    input.name = 'branch';
    input.value = branch;
    input.checked = true;
    input.onchange = () => { loadFlaky(); if (test.value.trim()) loadFailures(); };
    label.appendChild(input);
    label.appendChild(document.createTextNode(branch));
    branches.appendChild(label);
  }
}

function emptyRow(body, columns, text) {
  const tr = document.createElement('tr');
  const td = document.createElement('td');
  td.className = 'empty';
  td.colSpan = columns;
  td.textContent = text;
  tr.appendChild(td);
  body.appendChild(tr);
}

async function loadFlaky() {
  flaky_status.textContent = 'Running Athena query for flaky tests...';
  flaky_body.textContent = '';
  emptyRow(flaky_body, 6, 'Loading flaky tests...');
  try {
    const data = await getJSON('/api/flaky?' + queryPrefix());
    flaky_body.textContent = '';
    for (const row of data.rows) {
      const tr = document.createElement('tr');
      tr.innerHTML = '<td class="number">' + row.rank + '</td><td class="wrap"><button class="link" type="button"></button></td><td class="wrap"></td><td class="number">' + row.execs + '</td><td class="number">' + row.fails + '</td><td class="number">' + row.fail_pct + '</td>';
      tr.children[1].firstChild.textContent = row.test_name;
      tr.children[1].firstChild.title = 'Show failed GitHub runs for this test';
      tr.children[1].firstChild.onclick = () => { test.value = row.test_name; showTab('search'); loadFailures(); };
      tr.children[2].textContent = row.package;
      flaky_body.appendChild(tr);
    }
    if (data.rows.length) {
      flaky_status.textContent = 'Showing ' + data.rows.length + ' tests for ' + data.from + ' through ' + data.to + '.';
    } else {
      flaky_status.textContent = 'No flaky tests found.';
      emptyRow(flaky_body, 6, 'No flaky tests found for this range and branch selection.');
    }
  } catch (e) {
    flaky_body.textContent = '';
    emptyRow(flaky_body, 6, 'Could not load flaky tests.');
    flaky_status.textContent = e.message;
  }
}

async function loadFailures() {
  const request = ++failuresRequest;
  failures_status.textContent = 'Running Athena query for failed runs...';
  failures_body.textContent = '';
  emptyRow(failures_body, 3, 'Loading failed runs...');
  try {
    if (!test.value.trim()) throw new Error('Enter a test name or click one of the flaky tests above.');
    const url = '/api/failures?' + queryPrefix() + '&test=' + encodeURIComponent(test.value.trim());
    const data = await getJSON(url);
    if (request !== failuresRequest) return;
    failures_body.textContent = '';
    for (const row of data.rows) {
      const tr = document.createElement('tr');
      const link = row.github_url ? '<a href="' + row.github_url + '">open</a>' : '';
      tr.innerHTML = '<td class="nowrap"></td><td class="wrap"></td><td class="nowrap">' + link + '</td>';
      tr.children[0].textContent = row.timestamp;
      tr.children[1].textContent = row.git_ref;
      failures_body.appendChild(tr);
    }
    if (data.rows.length) {
      failures_status.textContent = 'Showing ' + data.rows.length + ' failed ' + (data.rows.length === 1 ? 'run' : 'runs') + ' for ' + test.value.trim() + '.';
    } else {
      failures_status.textContent = 'No failed runs found.';
      emptyRow(failures_body, 3, 'No failed runs found for this search.');
    }
  } catch (e) {
    if (request !== failuresRequest) return;
    failures_body.textContent = '';
    emptyRow(failures_body, 3, 'Could not load failed runs.');
    failures_status.textContent = e.message;
  }
}

topTab.onclick = () => showTab('top');
searchTab.onclick = () => showTab('search');
document.getElementById('range-form').onsubmit = e => { e.preventDefault(); loadFlaky(); if (test.value.trim()) loadFailures(); };
document.getElementById('search-form').onsubmit = e => { e.preventDefault(); showTab('search'); loadFailures(); };
document.getElementById('clear-search').onclick = () => { test.value = ''; failures_body.textContent = ''; failures_status.textContent = 'Select a flaky test or search by test name.'; };
loadConfig().then(() => { if (location.hash === '#search') showTab('search'); return loadFlaky(); }).catch(e => { flaky_status.textContent = e.message; });
</script>
</body>
</html>`
