package web

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tzone85/project-x/internal/state"
)

// startTime records when the server process started, for the health endpoint.
var startTime = time.Now()

// Handlers provides HTTP handler methods for the web dashboard API.
// All handlers return JSON responses with proper Content-Type headers.
type Handlers struct {
	eventStore    state.EventStore
	projStore     *state.SQLiteStore
	db            *sql.DB
	version       string
	dailyLimitUSD float64
	logPath       string
}

// maxLogLines caps the number of trailing log lines served by GetLogs to
// keep per-request memory bounded regardless of the requested ?limit=.
const maxLogLines = 5000

// aboutResponse is the JSON shape returned by /api/about.
// Serves as the canonical project description preview consumed by the dashboard.
type aboutResponse struct {
	Name           string   `json:"name"`
	Tagline        string   `json:"tagline"`
	Version        string   `json:"version"`
	Description    string   `json:"description"`
	Features       []string `json:"features"`
	PipelineStages []string `json:"pipeline_stages"`
	Runtimes       []string `json:"runtimes"`
	Repo           string   `json:"repo"`
	Docs           string   `json:"docs"`
	License        string   `json:"license"`
}

// projectAbout is the immutable canonical description served by /api/about.
// Mirrors README.md and is verified by tests so the two cannot drift silently.
var projectAbout = aboutResponse{
	Name:    "Project X",
	Tagline: "Autonomous AI agent orchestration for the full software development lifecycle.",
	Description: "px decomposes natural-language requirements into atomic stories, " +
		"dispatches AI coding agents across parallel waves, and drives each story through " +
		"code review, QA, rebase with LLM-powered conflict resolution, and auto-merge — " +
		"all while enforcing cost budgets and monitoring session health.",
	Features: []string{
		"Multi-Agent Orchestration (parallel waves, DAG-based dependency resolution)",
		"Cost Protection (per-story, per-requirement, daily budget breakers)",
		"Multi-Runtime (Claude Code, Codex, Gemini)",
		"Two-Pass Planning (decompose + validate before dispatch)",
		"7-Stage Pipeline (autocommit, diff, review, QA, rebase, merge, cleanup)",
		"LLM Conflict Resolution (rebase conflicts auto-resolved up to 10 rounds)",
		"Session Health Watchdog (stale/dead/missing detection + recovery)",
		"TUI + Web Dashboards (6 panels, real-time SSE)",
		"Event-Sourced State (append-only JSONL + SQLite projections)",
	},
	PipelineStages: []string{
		"autocommit",
		"diffcheck",
		"review",
		"qa",
		"rebase",
		"merge",
		"cleanup",
	},
	Runtimes: []string{"claude-code", "codex", "gemini"},
	Repo:     "https://github.com/tzone85/project-x",
	Docs:     "/docs/architecture.md",
	License:  "Apache-2.0",
}

// GetAbout returns project metadata so dashboards and external tools can
// preview what px does without parsing the README.
func (h *Handlers) GetAbout(w http.ResponseWriter, r *http.Request) {
	resp := projectAbout
	resp.Version = h.version
	if resp.Version == "" {
		resp.Version = "dev"
	}
	writeJSON(w, resp)
}

// ListEscalations returns all recorded escalations, newest first.
func (h *Handlers) ListEscalations(w http.ResponseWriter, r *http.Request) {
	escs, err := h.projStore.ListEscalations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ensureSlice(escs))
}

// ListRequirements returns all non-archived requirements as JSON.
func (h *Handlers) ListRequirements(w http.ResponseWriter, r *http.Request) {
	reqs, err := h.projStore.ListRequirements(state.ReqFilter{ExcludeArchived: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ensureSlice(reqs))
}

// ListStories returns stories filtered by optional query parameters:
// req_id, status, limit, offset.
func (h *Handlers) ListStories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := state.StoryFilter{
		ReqID:  q.Get("req_id"),
		Status: q.Get("status"),
		Limit:  parseIntParam(q.Get("limit"), 0),
		Offset: parseIntParam(q.Get("offset"), 0),
	}

	stories, err := h.projStore.ListStories(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ensureSlice(stories))
}

// ListAgents returns agents filtered by optional status query parameter.
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	filter := state.AgentFilter{
		Status: r.URL.Query().Get("status"),
	}

	agents, err := h.projStore.ListAgents(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ensureSlice(agents))
}

// ListEvents returns events from the event store, filtered by optional
// query parameters: type, agent_id, story_id, limit.
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := state.EventFilter{
		Type:    state.EventType(q.Get("type")),
		AgentID: q.Get("agent_id"),
		StoryID: q.Get("story_id"),
		Limit:   parseIntParam(q.Get("limit"), 0),
		After:   q.Get("after"),
	}

	events, err := h.eventStore.List(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ensureSlice(events))
}

// costResponse is the JSON shape returned by the cost endpoint.
type costResponse struct {
	TodayUSD      float64 `json:"today_usd"`
	DailyLimitUSD float64 `json:"daily_limit_usd"`
	ReqUSD        float64 `json:"req_usd,omitempty"`
	StoryUSD      float64 `json:"story_usd,omitempty"`
}

// logsResponse is the JSON shape returned by /api/logs.
type logsResponse struct {
	Lines []string `json:"lines"`
	Path  string   `json:"path"`
}

// GetCost returns cost summary data. Supports optional query parameters:
// req_id (cost for a specific requirement), story_id (cost for a specific story).
func (h *Handlers) GetCost(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today := time.Now().Format("2006-01-02")

	resp := costResponse{}

	// Daily cost.
	dailyCost, err := queryCostByDay(h.db, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp.TodayUSD = dailyCost
	resp.DailyLimitUSD = h.dailyLimitUSD

	// Optional: cost by requirement.
	if reqID := q.Get("req_id"); reqID != "" {
		reqCost, err := queryCostByReq(h.db, reqID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.ReqUSD = reqCost
	}

	// Optional: cost by story.
	if storyID := q.Get("story_id"); storyID != "" {
		storyCost, err := queryCostByStory(h.db, storyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.StoryUSD = storyCost
	}

	writeJSON(w, resp)
}

// healthResponse is the JSON shape returned by the health endpoint.
type healthResponse struct {
	Status string `json:"status"`
	Uptime string `json:"uptime"`
}

// GetLogs returns the trailing log lines from the configured log file.
// Optional ?limit=N caps to the last N lines (default 200, max maxLogLines).
// If the log file does not exist (e.g. fresh install before any run), an
// empty array is returned with HTTP 200 rather than an error.
func (h *Handlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r.URL.Query().Get("limit"), 200)
	if limit <= 0 {
		limit = 200
	}
	if limit > maxLogLines {
		limit = maxLogLines
	}

	resp := logsResponse{Lines: []string{}, Path: h.logPath}

	if h.logPath == "" {
		writeJSON(w, resp)
		return
	}

	data, err := os.ReadFile(h.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, resp)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Split into lines, drop trailing empty line if file ends with newline.
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	resp.Lines = all
	writeJSON(w, resp)
}

// GetHealth returns the server health status.
func (h *Handlers) GetHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthResponse{
		Status: "ok",
		Uptime: time.Since(startTime).Round(time.Second).String(),
	})
}

// writeJSON encodes v as JSON and writes it to w with the correct Content-Type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// parseIntParam parses a string to int, returning defaultVal on error.
func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// ensureSlice returns an empty non-nil slice if the input is nil.
// This ensures JSON encoding produces [] instead of null.
func ensureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// queryCostByDay returns the total cost for a given date from the token_usage table.
func queryCostByDay(db *sql.DB, date string) (float64, error) {
	var total float64
	err := db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE date(created_at) = ?",
		date,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// queryCostByReq returns the total cost for a requirement.
func queryCostByReq(db *sql.DB, reqID string) (float64, error) {
	var total float64
	err := db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE req_id = ?",
		reqID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// queryCostByStory returns the total cost for a story.
func queryCostByStory(db *sql.DB, storyID string) (float64, error) {
	var total float64
	err := db.QueryRow(
		"SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE story_id = ?",
		storyID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}
