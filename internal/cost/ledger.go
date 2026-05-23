package cost

import (
	"database/sql"
	"math/rand"
	"time"

	"github.com/oklog/ulid/v2"
)

// TokenUsage represents a single LLM call's token usage.
type TokenUsage struct {
	ReqID        string
	StoryID      string
	AgentID      string
	Model        string
	InputTokens  int
	OutputTokens int
	Stage        string // planning, review, qa, conflict_resolution
}

// Ledger tracks token usage and cost.
type Ledger interface {
	Record(usage TokenUsage) error
	QueryByStory(storyID string) (float64, error)
	QueryByRequirement(reqID string) (float64, error)
	QueryByDay(date string) (float64, error) // date format: "2006-01-02"
}

// Compile-time check that SQLiteLedger implements Ledger.
var _ Ledger = (*SQLiteLedger)(nil)

// SQLiteLedger implements Ledger using the token_usage table.
type SQLiteLedger struct {
	db      *sql.DB
	pricing map[string]PricingEntry
}

// NewSQLiteLedger creates a new SQLiteLedger backed by the given database
// and pricing table.
func NewSQLiteLedger(db *sql.DB, pricing map[string]PricingEntry) *SQLiteLedger {
	return &SQLiteLedger{
		db:      db,
		pricing: pricing,
	}
}

// Record computes the cost for the usage and inserts a row into token_usage.
func (l *SQLiteLedger) Record(usage TokenUsage) error {
	costUSD := ComputeCost(usage.Model, usage.InputTokens, usage.OutputTokens, l.pricing)

	id := newULID()

	_, err := l.db.Exec(
		`INSERT INTO token_usage (id, req_id, story_id, agent_id, model, input_tokens, output_tokens, cost_usd, stage)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		usage.ReqID,
		usage.StoryID,
		usage.AgentID,
		usage.Model,
		usage.InputTokens,
		usage.OutputTokens,
		costUSD,
		usage.Stage,
	)
	return err
}

// QueryByStory returns the total cost in USD for the given story.
func (l *SQLiteLedger) QueryByStory(storyID string) (float64, error) {
	return l.sumCost("SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE story_id = ?", storyID)
}

// QueryByRequirement returns the total cost in USD for the given requirement.
func (l *SQLiteLedger) QueryByRequirement(reqID string) (float64, error) {
	return l.sumCost("SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE req_id = ?", reqID)
}

// QueryByDay returns the total cost in USD for the given date (format: "2006-01-02").
// `created_at` is stored as UTC; callers pass local-formatted dates (time.Now().Format(...)),
// so we convert stored timestamps to localtime before comparing to avoid a midnight-boundary
// off-by-one in non-UTC timezones (e.g. SAST=UTC+2: 00:30 local is still yesterday in UTC).
func (l *SQLiteLedger) QueryByDay(date string) (float64, error) {
	return l.sumCost("SELECT COALESCE(SUM(cost_usd), 0) FROM token_usage WHERE date(created_at, 'localtime') = ?", date)
}

// sumCost executes a query that returns a single float64 value.
func (l *SQLiteLedger) sumCost(query string, args ...any) (float64, error) {
	var total float64
	err := l.db.QueryRow(query, args...).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

// newULID generates a new ULID string.
func newULID() string {
	entropy := rand.New(rand.NewSource(time.Now().UnixNano()))
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}
