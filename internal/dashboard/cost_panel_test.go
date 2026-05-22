package dashboard

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tzone85/project-x/internal/state"
)

func TestRenderBar_NoLimit(t *testing.T) {
	out := renderBar(0, 0, 10)
	if !strings.Contains(out, strings.Repeat("-", 10)) {
		t.Errorf("zero-limit bar should be all dashes, got %q", out)
	}
}

func TestRenderBar_ColorThresholds(t *testing.T) {
	// We can't easily decode lipgloss colors, but we can verify the bar
	// shape (filled + empty) regardless of color.
	for _, tc := range []struct {
		used, limit float64
		wantFilled  int
	}{
		{0, 10, 0},
		{5, 10, 5}, // 50% = yellow
		{8, 10, 8}, // 80% = orange
		{10, 10, 10},
		{20, 10, 10}, // over -> clamps to width
	} {
		out := renderBar(tc.used, tc.limit, 10)
		// Count '=' chars (filled section).
		if filled := strings.Count(out, "="); filled != tc.wantFilled {
			t.Errorf("renderBar(%v, %v, 10) filled=%d, want %d", tc.used, tc.limit, filled, tc.wantFilled)
		}
	}
}

func TestRenderBar_NegativeUsedClampsToZero(t *testing.T) {
	out := renderBar(-5, 10, 10)
	if filled := strings.Count(out, "="); filled != 0 {
		t.Errorf("negative used should yield 0 filled, got %d", filled)
	}
}

func TestRenderBudgetLine_Thresholds(t *testing.T) {
	tests := []struct {
		name        string
		used, limit float64
		wantSubstr  string
	}{
		{"under 80%", 5, 10, "50%"},
		{"over 80%", 8.5, 10, "85%"},
		{"over 100%", 12, 10, "120%"},
		{"no limit", 5, 0, "0%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderBudgetLine("Daily", tt.used, tt.limit)
			if !strings.Contains(out, tt.wantSubstr) {
				t.Errorf("budget line %q missing %q", out, tt.wantSubstr)
			}
		})
	}
}

func TestCostStyled_Thresholds(t *testing.T) {
	tests := []struct {
		cost   float64
		expect string
	}{
		{0, "$0.00"},
		{3, "$3.00"},
		{5, "$5.00"},
		{10, "$10.00"},
		{99, "$99.00"},
	}
	for _, tt := range tests {
		if got := costStyled(tt.cost); !strings.Contains(got, tt.expect) {
			t.Errorf("costStyled(%v) = %q, want substring %q", tt.cost, got, tt.expect)
		}
	}
}

func TestRenderCost_NoRequirements(t *testing.T) {
	out := renderCost(costData{dailyTotal: 1, dailyLimit: 10})
	if !strings.Contains(out, "Cost Overview") {
		t.Errorf("expected header, got %q", out)
	}
	if !strings.Contains(out, "No requirements tracked") {
		t.Errorf("expected empty placeholder, got %q", out)
	}
}

func TestRenderCost_WithRequirements(t *testing.T) {
	data := costData{
		dailyTotal: 4,
		dailyLimit: 10,
		reqCosts: []reqCost{
			{req: state.Requirement{ID: "REQ12345EXTRA", Title: "demo req"}, cost: 2.5},
			{req: state.Requirement{ID: "REQ2", Title: strings.Repeat("X", 50)}, cost: 11},
		},
	}
	out := renderCost(data)
	if !strings.Contains(out, "REQ12345") {
		t.Errorf("expected truncated id, got %q", out)
	}
	if !strings.Contains(out, "demo req") {
		t.Errorf("expected req title, got %q", out)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("expected long title truncated with ellipsis, got %q", out)
	}
	if !strings.Contains(out, "$2.50") {
		t.Errorf("expected $2.50 cost, got %q", out)
	}
}

func TestQueryCostData_RealSQLite(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mustExec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE token_usage (req_id TEXT, cost_usd REAL, created_at TEXT)`)
	mustExec(`INSERT INTO token_usage VALUES ('r-1', 0.50, datetime('now'))`)
	mustExec(`INSERT INTO token_usage VALUES ('r-1', 1.25, datetime('now'))`)
	mustExec(`INSERT INTO token_usage VALUES ('r-2', 0.75, datetime('now'))`)

	reqs := []state.Requirement{
		{ID: "r-1", Title: "one"},
		{ID: "r-2", Title: "two"},
		{ID: "r-3", Title: "no costs"},
	}

	data := queryCostData(db, reqs, 5.0)
	if data.dailyLimit != 5.0 {
		t.Errorf("dailyLimit = %v, want 5.0", data.dailyLimit)
	}
	const wantTotal = 2.50
	if data.dailyTotal != wantTotal {
		t.Errorf("dailyTotal = %v, want %v", data.dailyTotal, wantTotal)
	}
	if len(data.reqCosts) != 3 {
		t.Fatalf("reqCosts size = %d, want 3", len(data.reqCosts))
	}
	if data.reqCosts[0].cost != 1.75 {
		t.Errorf("r-1 cost = %v, want 1.75", data.reqCosts[0].cost)
	}
	if data.reqCosts[2].cost != 0 {
		t.Errorf("r-3 cost = %v, want 0", data.reqCosts[2].cost)
	}
}

func TestQueryCostData_MissingTable(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// No token_usage table — every query errors → costs default to 0.
	data := queryCostData(db, []state.Requirement{{ID: "r-1"}}, 5.0)
	if data.dailyTotal != 0 {
		t.Errorf("dailyTotal should default to 0 on missing table, got %v", data.dailyTotal)
	}
	if data.reqCosts[0].cost != 0 {
		t.Errorf("reqCosts[0].cost should default to 0, got %v", data.reqCosts[0].cost)
	}
}
