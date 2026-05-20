package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"

	"orchkit"
)

// Dashboard is a lightweight web UI for orchkit.
// It lets you trigger flows, watch tool calls live, and inspect run history.
//
// Usage:
//
//	d := dashboard.New(":9090")
//	d.Register("fetch-and-parse", flow, store)
//	d.Serve(ctx)
type Dashboard struct {
	addr  string
	flows map[string]*registration
	runs  []RunRecord
	mu    sync.RWMutex
}

type registration struct {
	name  string
	flow  *orchkit.Flow
	store orchkit.Store
}

// RunRecord captures one flow execution for the history view.
type RunRecord struct {
	ID       string
	Flow     string
	Status   string // running | success | error
	Started  time.Time
	Elapsed  time.Duration
	Steps    []StepRecord
	Error    string
}

type StepRecord struct {
	ID      string
	Status  string
	Elapsed time.Duration
	Error   string
}

func New(addr string) *Dashboard {
	return &Dashboard{
		addr:  addr,
		flows: map[string]*registration{},
	}
}

// Register makes a flow available in the dashboard.
func (d *Dashboard) Register(name string, flow *orchkit.Flow, store orchkit.Store) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.flows[name] = &registration{name: name, flow: flow, store: store}
}

// Serve starts the dashboard HTTP server. Blocks until ctx is cancelled.
func (d *Dashboard) Serve(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/run", d.handleRun)
	mux.HandleFunc("/runs", d.handleRuns)
	mux.HandleFunc("/api/runs", d.handleAPIRuns)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	srv := &http.Server{Addr: d.addr, Handler: mux}
	log.Printf("orchkit dashboard: http://%s", d.addr)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	flows := make([]string, 0, len(d.flows))
	for name := range d.flows {
		flows = append(flows, name)
	}
	runs := append([]RunRecord{}, d.runs...)
	d.mu.RUnlock()

	w.Header().Set("content-type", "text/html")
	indexTmpl.Execute(w, map[string]any{
		"Flows": flows,
		"Runs":  runs,
	})
}

func (d *Dashboard) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("flow")
	if name == "" {
		http.Error(w, "flow name required", http.StatusBadRequest)
		return
	}

	d.mu.RLock()
	reg, ok := d.flows[name]
	d.mu.RUnlock()
	if !ok {
		http.Error(w, "flow not found: "+name, http.StatusNotFound)
		return
	}

	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	record := RunRecord{
		ID:      runID,
		Flow:    name,
		Status:  "running",
		Started: time.Now(),
	}

	d.mu.Lock()
	d.runs = append([]RunRecord{record}, d.runs...)
	d.mu.Unlock()

	// Run in background and update record when done.
	go func() {
		hooks := &orchkit.Hooks{
			OnStepStart: func(id string, in orchkit.Input) {
				d.mu.Lock()
				defer d.mu.Unlock()
				for i, run := range d.runs {
					if run.ID == runID {
						d.runs[i].Steps = append(d.runs[i].Steps, StepRecord{
							ID:     id,
							Status: "running",
						})
						break
					}
				}
			},
			OnStepEnd: func(id string, out orchkit.Output, err error, elapsed time.Duration) {
				d.mu.Lock()
				defer d.mu.Unlock()
				for i, run := range d.runs {
					if run.ID == runID {
						for j, step := range run.Steps {
							if step.ID == id {
								d.runs[i].Steps[j].Status = "success"
								d.runs[i].Steps[j].Elapsed = elapsed
								if err != nil {
									d.runs[i].Steps[j].Status = "error"
									d.runs[i].Steps[j].Error = err.Error()
								}
								break
							}
						}
						break
					}
				}
			},
		}

		store := reg.store
		if store == nil {
			store = orchkit.NewMemStore()
		}

		start := time.Now()
		_, err := orchkit.Run(r.Context(), reg.flow, store, orchkit.RunOptions{Hooks: hooks})
		elapsed := time.Since(start)

		d.mu.Lock()
		for i, run := range d.runs {
			if run.ID == runID {
				d.runs[i].Elapsed = elapsed
				if err != nil {
					d.runs[i].Status = "error"
					d.runs[i].Error = err.Error()
				} else {
					d.runs[i].Status = "success"
				}
				break
			}
		}
		d.mu.Unlock()
	}()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) handleRuns(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	runs := append([]RunRecord{}, d.runs...)
	d.mu.RUnlock()
	w.Header().Set("content-type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// ----------------------------------------------------------------------------
// HTML template — dark IDE-style, clean, professional.
// ----------------------------------------------------------------------------

var indexTmpl = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>orchkit</title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  :root{
    --bg:#0f1117;--surface:#1a1d27;--surface2:#22263a;--surface3:#2a2f45;
    --border:#2e3347;--text:#e2e8f0;--text2:#94a3b8;--text3:#64748b;
    --accent:#6366f1;--accent2:#818cf8;--green:#22c55e;--red:#ef4444;--yellow:#f59e0b;
  }
  body{background:var(--bg);color:var(--text);font-family:'SF Mono',Monaco,monospace;font-size:13px;min-height:100vh}
  .topbar{background:var(--surface);border-bottom:1px solid var(--border);padding:0 24px;height:48px;display:flex;align-items:center;gap:16px}
  .topbar .logo{color:var(--accent2);font-weight:700;font-size:15px;letter-spacing:.5px}
  .topbar .sub{color:var(--text3);font-size:11px}
  .container{max-width:1100px;margin:0 auto;padding:24px}
  .grid{display:grid;grid-template-columns:280px 1fr;gap:20px}
  .panel{background:var(--surface);border:1px solid var(--border);border-radius:8px;overflow:hidden}
  .panel-header{padding:12px 16px;border-bottom:1px solid var(--border);color:var(--text2);font-size:11px;text-transform:uppercase;letter-spacing:.8px}
  .panel-body{padding:16px}
  .flow-item{display:flex;align-items:center;justify-content:space-between;padding:10px 12px;border:1px solid var(--border);border-radius:6px;margin-bottom:8px;background:var(--surface2)}
  .flow-name{color:var(--text);font-weight:500}
  .btn{padding:6px 14px;border:none;border-radius:5px;cursor:pointer;font-size:12px;font-family:inherit;font-weight:500;transition:opacity .15s}
  .btn-primary{background:var(--accent);color:#fff}
  .btn-primary:hover{opacity:.85}
  .run-item{padding:12px 16px;border-bottom:1px solid var(--border)}
  .run-item:last-child{border-bottom:none}
  .run-header{display:flex;align-items:center;gap:10px;margin-bottom:6px}
  .badge{padding:2px 8px;border-radius:10px;font-size:10px;font-weight:600;text-transform:uppercase}
  .badge-success{background:#14532d;color:var(--green)}
  .badge-error{background:#450a0a;color:var(--red)}
  .badge-running{background:#1e1b4b;color:var(--accent2)}
  .run-flow{color:var(--text);font-weight:500}
  .run-meta{color:var(--text3);font-size:11px}
  .steps{margin-top:8px;padding-left:12px;border-left:2px solid var(--border)}
  .step{display:flex;align-items:center;gap:8px;padding:3px 0;color:var(--text2);font-size:11px}
  .dot{width:6px;height:6px;border-radius:50%;flex-shrink:0}
  .dot-success{background:var(--green)}
  .dot-error{background:var(--red)}
  .dot-running{background:var(--accent2)}
  .empty{color:var(--text3);text-align:center;padding:32px;font-size:12px}
  .error-msg{color:var(--red);font-size:11px;margin-top:4px}
  .elapsed{color:var(--text3);margin-left:auto;font-size:11px}
</style>
</head>
<body>
<div class="topbar">
  <span class="logo">◆ orchkit</span>
  <span class="sub">orchestration kernel</span>
</div>
<div class="container">
  <div class="grid">
    <div>
      <div class="panel">
        <div class="panel-header">Registered Flows</div>
        <div class="panel-body">
          {{if .Flows}}
            {{range .Flows}}
            <div class="flow-item">
              <span class="flow-name">{{.}}</span>
              <form method="POST" action="/run" style="margin:0">
                <input type="hidden" name="flow" value="{{.}}">
                <button class="btn btn-primary" type="submit">▶ Run</button>
              </form>
            </div>
            {{end}}
          {{else}}
            <div class="empty">No flows registered yet.</div>
          {{end}}
        </div>
      </div>
    </div>
    <div>
      <div class="panel">
        <div class="panel-header">Run History</div>
        {{if .Runs}}
          {{range .Runs}}
          <div class="run-item">
            <div class="run-header">
              <span class="badge badge-{{.Status}}">{{.Status}}</span>
              <span class="run-flow">{{.Flow}}</span>
              <span class="elapsed">{{if .Elapsed}}{{.Elapsed.Round 1000000}}{{end}}</span>
            </div>
            <div class="run-meta">{{.Started.Format "2006-01-02 15:04:05"}}</div>
            {{if .Error}}<div class="error-msg">{{.Error}}</div>{{end}}
            {{if .Steps}}
            <div class="steps">
              {{range .Steps}}
              <div class="step">
                <span class="dot dot-{{.Status}}"></span>
                <span>{{.ID}}</span>
                {{if .Elapsed}}<span class="elapsed">{{.Elapsed.Round 1000000}}</span>{{end}}
                {{if .Error}}<span style="color:var(--red)">— {{.Error}}</span>{{end}}
              </div>
              {{end}}
            </div>
            {{end}}
          </div>
          {{end}}
        {{else}}
          <div class="empty">No runs yet. Trigger a flow to see history here.</div>
        {{end}}
      </div>
    </div>
  </div>
</div>
<script>
  // Auto-refresh while any run is in "running" state.
  function hasRunning() {
    return document.querySelector('.badge-running') !== null;
  }
  function scheduleRefresh() {
    if (hasRunning()) {
      setTimeout(() => window.location.reload(), 1500);
    }
  }
  scheduleRefresh();
</script>
</body>
</html>`))
