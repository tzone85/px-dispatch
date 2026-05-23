package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements ProjectionStore by materializing events into
// denormalized SQLite tables for efficient querying.
type SQLiteStore struct {
	db *sql.DB
}

// Compile-time interface check.
var _ ProjectionStore = (*SQLiteStore)(nil)

// NewSQLiteStore opens (or creates) a SQLite database and runs migrations.
// WAL mode is enabled for concurrent readers.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if _, err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use by packages that share the same database.
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// Project dispatches an event to the appropriate projection handler.
// Unknown event types are silently ignored for forward compatibility.
func (s *SQLiteStore) Project(evt Event) error {
	switch evt.Type {
	case EventReqSubmitted:
		return s.projectReqSubmitted(evt)
	case EventReqAnalyzed:
		return s.updateReqStatus(evt, "analyzed")
	case EventReqPlanned:
		return s.updateReqStatus(evt, "planned")
	case EventReqPaused:
		return s.updateReqStatus(evt, "paused")
	case EventReqResumed:
		return s.updateReqStatus(evt, "resumed")
	case EventReqCompleted:
		return s.updateReqStatus(evt, "completed")
	case EventStoryCreated:
		return s.projectStoryCreated(evt)
	case EventStoryAssigned:
		return s.projectStoryAssigned(evt)
	case EventStoryStarted:
		return s.updateStoryStatus(evt.StoryID, "in_progress")
	case EventStoryCompleted:
		return s.updateStoryStatus(evt.StoryID, "review")
	case EventStoryReviewPassed:
		return s.updateStoryStatus(evt.StoryID, "qa")
	case EventStoryReviewFailed:
		return s.updateStoryStatus(evt.StoryID, "draft")
	case EventStoryQAPassed:
		return s.updateStoryStatus(evt.StoryID, "pr_submitted")
	case EventStoryQAFailed:
		return s.updateStoryStatus(evt.StoryID, "qa_failed")
	case EventStoryPRCreated:
		return s.projectStoryPRCreated(evt)
	case EventStoryMerged:
		return s.updateStoryStatus(evt.StoryID, "merged")
	case EventAgentSpawned:
		return s.projectAgentSpawned(evt)
	case EventAgentStuck:
		return s.updateAgentStatusByPayload(evt, "stuck")
	case EventAgentDied:
		return s.updateAgentStatusByPayload(evt, "dead")
	case EventAgentStale:
		return s.updateAgentStatusByPayload(evt, "stale")
	case EventAgentLost:
		return s.updateAgentStatusByPayload(evt, "lost")
	case EventStoryReviewRequested:
		return s.updateStoryStatus(evt.StoryID, "review")
	case EventStoryQAStarted:
		return s.updateStoryStatus(evt.StoryID, "qa")
	case EventStoryEstimated, EventStoryProgress:
		// Read-only metadata events — no projection update needed but they
		// are explicitly enumerated here so the wiring test can confirm
		// every declared EventType has a known disposition.
		return nil
	case EventBudgetWarning, EventBudgetExhausted:
		// Cost-side events; projected via the cost ledger, not the
		// projection store. Enumerated so wiring stays exhaustive.
		return nil
	case EventEscalationCreated:
		return s.projectEscalationCreated(evt)
	default:
		return nil
	}
}

// updateAgentStatusByPayload sets the agents row's status when an agent
// lifecycle event fires (stuck/dead/stale/lost). The payload may carry the
// agent id under either `agent_id` (when emitted by the poller) or as the
// event's outer AgentID (when emitted by the executor). We accept either.
func (s *SQLiteStore) updateAgentStatusByPayload(evt Event, status string) error {
	agentID := evt.AgentID
	if agentID == "" {
		var p struct {
			AgentID string `json:"agent_id"`
		}
		_ = json.Unmarshal(evt.Payload, &p)
		agentID = p.AgentID
	}
	if agentID == "" {
		return nil
	}
	_, err := s.db.Exec(
		`UPDATE agents SET status = ? WHERE id = ?`,
		status, agentID,
	)
	return err
}

// --- Requirement projections ---

func (s *SQLiteStore) projectReqSubmitted(evt Event) error {
	var p ReqSubmittedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode req submitted: %w", err)
	}

	_, err := s.db.Exec(
		`INSERT INTO requirements (id, title, description, status, repo_path, created_at)
		 VALUES (?, ?, ?, 'pending', ?, ?)`,
		p.ID, p.Title, p.Description, p.RepoPath, evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert requirement: %w", err)
	}
	return nil
}

func (s *SQLiteStore) updateReqStatus(evt Event, status string) error {
	var p ReqStatusPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode req status payload: %w", err)
	}

	_, err := s.db.Exec(
		`UPDATE requirements SET status = ?, updated_at = ? WHERE id = ?`,
		status, evt.Timestamp, p.ReqID,
	)
	if err != nil {
		return fmt.Errorf("update requirement status: %w", err)
	}
	return nil
}

// GetRequirement returns a single requirement by ID.
func (s *SQLiteStore) GetRequirement(id string) (Requirement, error) {
	var r Requirement
	err := s.db.QueryRow(
		`SELECT id, title, description, status, repo_path, created_at
		 FROM requirements WHERE id = ?`, id,
	).Scan(&r.ID, &r.Title, &r.Description, &r.Status, &r.RepoPath, &r.CreatedAt)
	if err != nil {
		return Requirement{}, fmt.Errorf("get requirement %s: %w", id, err)
	}
	return r, nil
}

// ListRequirements returns requirements matching the filter.
func (s *SQLiteStore) ListRequirements(filter ReqFilter) ([]Requirement, error) {
	var clauses []string
	var args []any

	if filter.RepoPath != "" {
		clauses = append(clauses, "repo_path = ?")
		args = append(args, filter.RepoPath)
	}
	if filter.ExcludeArchived {
		clauses = append(clauses, "status != 'archived'")
	}

	query := "SELECT id, title, description, status, repo_path, created_at FROM requirements"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at ASC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requirements: %w", err)
	}
	defer rows.Close()

	var reqs []Requirement
	for rows.Next() {
		var r Requirement
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Status, &r.RepoPath, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan requirement: %w", err)
		}
		reqs = append(reqs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requirements: %w", err)
	}

	return reqs, nil
}

// ArchiveRequirement sets a requirement's status to "archived".
func (s *SQLiteStore) ArchiveRequirement(reqID string) error {
	_, err := s.db.Exec(
		`UPDATE requirements SET status = 'archived', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		reqID,
	)
	if err != nil {
		return fmt.Errorf("archive requirement %s: %w", reqID, err)
	}
	return nil
}

// --- Story projections ---

func (s *SQLiteStore) projectStoryCreated(evt Event) error {
	var p StoryCreatedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode story created: %w", err)
	}

	ownedFilesJSON, err := json.Marshal(p.OwnedFiles)
	if err != nil {
		return fmt.Errorf("marshal owned_files: %w", err)
	}
	// Normalize nil slice to empty JSON array
	if p.OwnedFiles == nil {
		ownedFilesJSON = []byte("[]")
	}

	_, err = s.db.Exec(
		`INSERT INTO stories (id, req_id, title, description, acceptance_criteria, complexity,
		 status, owned_files, wave_hint, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?)`,
		p.ID, p.ReqID, p.Title, p.Description, p.AcceptanceCriteria, p.Complexity,
		string(ownedFilesJSON), p.WaveHint, evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert story: %w", err)
	}

	// Insert dependency edges
	for _, depID := range p.DependsOn {
		_, err := s.db.Exec(
			`INSERT INTO story_deps (story_id, depends_on_id) VALUES (?, ?)`,
			p.ID, depID,
		)
		if err != nil {
			return fmt.Errorf("insert story dep %s -> %s: %w", p.ID, depID, err)
		}
	}

	return nil
}

func (s *SQLiteStore) projectStoryAssigned(evt Event) error {
	var p StoryAssignedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode story assigned: %w", err)
	}

	_, err := s.db.Exec(
		`UPDATE stories SET status = 'assigned', agent_id = ?, wave = ?, updated_at = ?
		 WHERE id = ?`,
		p.AgentID, p.Wave, evt.Timestamp, evt.StoryID,
	)
	if err != nil {
		return fmt.Errorf("update story assigned: %w", err)
	}
	return nil
}

func (s *SQLiteStore) updateStoryStatus(storyID, status string) error {
	_, err := s.db.Exec(
		`UPDATE stories SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, storyID,
	)
	if err != nil {
		return fmt.Errorf("update story %s status to %s: %w", storyID, status, err)
	}
	return nil
}

func (s *SQLiteStore) projectStoryPRCreated(evt Event) error {
	var p StoryPRCreatedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode story pr created: %w", err)
	}

	_, err := s.db.Exec(
		`UPDATE stories SET pr_url = ?, pr_number = ?, status = 'pr_submitted', updated_at = ?
		 WHERE id = ?`,
		p.PRUrl, p.PRNumber, evt.Timestamp, evt.StoryID,
	)
	if err != nil {
		return fmt.Errorf("update story pr created: %w", err)
	}
	return nil
}

// GetStory returns a single story by ID, deserializing the owned_files JSON.
func (s *SQLiteStore) GetStory(id string) (Story, error) {
	var st Story
	var ownedFilesJSON string

	err := s.db.QueryRow(
		`SELECT id, req_id, title, description, acceptance_criteria, complexity,
		 status, agent_id, branch, pr_url, pr_number, owned_files, wave_hint, wave, created_at
		 FROM stories WHERE id = ?`, id,
	).Scan(
		&st.ID, &st.ReqID, &st.Title, &st.Description, &st.AcceptanceCriteria,
		&st.Complexity, &st.Status, &st.AgentID, &st.Branch, &st.PRUrl,
		&st.PRNumber, &ownedFilesJSON, &st.WaveHint, &st.Wave, &st.CreatedAt,
	)
	if err != nil {
		return Story{}, fmt.Errorf("get story %s: %w", id, err)
	}

	if err := json.Unmarshal([]byte(ownedFilesJSON), &st.OwnedFiles); err != nil {
		return Story{}, fmt.Errorf("unmarshal owned_files for story %s: %w", id, err)
	}

	return st, nil
}

// ListStories returns stories matching the filter with pagination.
func (s *SQLiteStore) ListStories(filter StoryFilter) ([]Story, error) {
	var clauses []string
	var args []any

	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.ReqID != "" {
		clauses = append(clauses, "req_id = ?")
		args = append(args, filter.ReqID)
	}

	query := `SELECT id, req_id, title, description, acceptance_criteria, complexity,
		status, agent_id, branch, pr_url, pr_number, owned_files, wave_hint, wave, created_at
		FROM stories`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at ASC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stories: %w", err)
	}
	defer rows.Close()

	var stories []Story
	for rows.Next() {
		var st Story
		var ownedFilesJSON string
		if err := rows.Scan(
			&st.ID, &st.ReqID, &st.Title, &st.Description, &st.AcceptanceCriteria,
			&st.Complexity, &st.Status, &st.AgentID, &st.Branch, &st.PRUrl,
			&st.PRNumber, &ownedFilesJSON, &st.WaveHint, &st.Wave, &st.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan story: %w", err)
		}

		if err := json.Unmarshal([]byte(ownedFilesJSON), &st.OwnedFiles); err != nil {
			return nil, fmt.Errorf("unmarshal owned_files: %w", err)
		}

		stories = append(stories, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stories: %w", err)
	}

	return stories, nil
}

// ArchiveStoriesByReq sets the status of all stories for a requirement to "archived".
func (s *SQLiteStore) ArchiveStoriesByReq(reqID string) error {
	_, err := s.db.Exec(
		`UPDATE stories SET status = 'archived', updated_at = CURRENT_TIMESTAMP WHERE req_id = ?`,
		reqID,
	)
	if err != nil {
		return fmt.Errorf("archive stories for req %s: %w", reqID, err)
	}
	return nil
}

// ListStoryDeps returns all dependency edges for stories belonging to a requirement.
func (s *SQLiteStore) ListStoryDeps(reqID string) ([]StoryDep, error) {
	rows, err := s.db.Query(
		`SELECT sd.story_id, sd.depends_on_id
		 FROM story_deps sd
		 JOIN stories s ON sd.story_id = s.id
		 WHERE s.req_id = ?`,
		reqID,
	)
	if err != nil {
		return nil, fmt.Errorf("list story deps for req %s: %w", reqID, err)
	}
	defer rows.Close()

	var deps []StoryDep
	for rows.Next() {
		var d StoryDep
		if err := rows.Scan(&d.StoryID, &d.DependsOnID); err != nil {
			return nil, fmt.Errorf("scan story dep: %w", err)
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate story deps: %w", err)
	}

	return deps, nil
}

// --- Agent projections ---

func (s *SQLiteStore) projectAgentSpawned(evt Event) error {
	var p AgentSpawnedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode agent spawned: %w", err)
	}

	_, err := s.db.Exec(
		`INSERT INTO agents (id, type, model, runtime, status, current_story_id, session_name, created_at)
		 VALUES (?, ?, ?, ?, 'active', ?, ?, ?)`,
		p.ID, p.Type, p.Model, p.Runtime, p.StoryID, p.SessionName, evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert agent: %w", err)
	}
	return nil
}

// ListAgents returns agents matching the filter.
func (s *SQLiteStore) ListAgents(filter AgentFilter) ([]Agent, error) {
	var clauses []string
	var args []any

	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}

	query := "SELECT id, type, model, runtime, status, current_story_id, session_name, created_at FROM agents"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Type, &a.Model, &a.Runtime, &a.Status,
			&a.CurrentStoryID, &a.SessionName, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}

	return agents, nil
}

// --- Escalation projections ---

func (s *SQLiteStore) projectEscalationCreated(evt Event) error {
	var p EscalationCreatedPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode escalation created: %w", err)
	}

	_, err := s.db.Exec(
		`INSERT INTO escalations (id, story_id, from_agent, reason, status, created_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)`,
		p.ID, p.StoryID, p.FromAgent, p.Reason, evt.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert escalation: %w", err)
	}
	return nil
}

// ListEscalations returns all escalations ordered by created_at DESC.
func (s *SQLiteStore) ListEscalations() ([]Escalation, error) {
	rows, err := s.db.Query(
		`SELECT id, story_id, from_agent, reason, status, resolution, created_at
		 FROM escalations ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list escalations: %w", err)
	}
	defer rows.Close()

	var escs []Escalation
	for rows.Next() {
		var e Escalation
		if err := rows.Scan(&e.ID, &e.StoryID, &e.FromAgent, &e.Reason,
			&e.Status, &e.Resolution, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan escalation: %w", err)
		}
		escs = append(escs, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate escalations: %w", err)
	}

	return escs, nil
}
