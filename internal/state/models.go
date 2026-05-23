// Package state defines the core domain models, event types, and store
// interfaces for the event-sourced architecture of px-dispatch.
package state

// Requirement represents a user-submitted requirement.
type Requirement struct {
	ID          string
	Title       string
	Description string
	Status      string
	RepoPath    string
	CreatedAt   string
}

// Story represents an atomic unit of work decomposed from a requirement.
type Story struct {
	ID                 string
	ReqID              string
	Title              string
	Description        string
	AcceptanceCriteria string
	Complexity         int
	Status             string
	AgentID            string
	Branch             string
	PRUrl              string
	PRNumber           int
	OwnedFiles         []string
	WaveHint           string
	Wave               int
	CreatedAt          string
}

// Agent represents an AI agent assigned to work on stories.
type Agent struct {
	ID             string
	Type           string
	Model          string
	Runtime        string
	Status         string
	CurrentStoryID string
	SessionName    string
	CreatedAt      string
}

// Escalation represents a recorded escalation between agent roles.
type Escalation struct {
	ID         string
	StoryID    string
	FromAgent  string
	Reason     string
	Status     string
	Resolution string
	CreatedAt  string
}

// StoryDep represents a dependency edge between stories.
type StoryDep struct {
	StoryID     string
	DependsOnID string
}

// StoryFilter for querying stories.
type StoryFilter struct {
	Status string
	ReqID  string
	Limit  int
	Offset int
}

// ReqFilter for querying requirements.
type ReqFilter struct {
	RepoPath        string
	ExcludeArchived bool
	Limit           int
	Offset          int
}

// AgentFilter for querying agents.
type AgentFilter struct {
	Status string
}

// EventFilter for querying events.
type EventFilter struct {
	Type    EventType
	AgentID string
	StoryID string
	Limit   int
	After   string // cursor-based: events after this timestamp
}
