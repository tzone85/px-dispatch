package dashboard

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tzone85/px-dispatch/internal/state"
)

const (
	refreshInterval = 2 * time.Second
	panelCount      = 6
)

// Panel indices.
const (
	panelPipeline    = 0
	panelAgents      = 1
	panelActivity    = 2
	panelEscalations = 3
	panelCost        = 4
	panelLogs        = 5
)

var panelNames = []string{"Pipeline", "Agents", "Activity", "Escalations", "Cost", "Logs"}

// tickMsg signals a periodic data refresh.
type tickMsg time.Time

// dataMsg carries refreshed data from the stores.
type dataMsg struct {
	requirements []state.Requirement
	stories      []state.Story
	agents       []state.Agent
	events       []state.Event
	escalations  []state.Escalation
	costData     costData
	err          error
}

// Model is the top-level Bubbletea model for the dashboard.
type Model struct {
	eventStore state.EventStore
	projStore  state.ProjectionStore
	db         *sql.DB
	version    string
	reqFilter  state.ReqFilter
	logPath    string
	dailyLimit float64

	activePanel int
	width       int
	height      int

	// Scrollable viewports per panel.
	viewports [panelCount]*ScrollableViewport

	// Cached data.
	requirements []state.Requirement
	stories      []state.Story
	agents       []state.Agent
	events       []state.Event
	escalations  []state.Escalation
	costInfo     costData
	lastRefresh  time.Time
	err          error
}

// Config holds the dependencies needed to create a dashboard Model.
type Config struct {
	EventStore state.EventStore
	ProjStore  state.ProjectionStore
	DB         *sql.DB
	Version    string
	ReqFilter  state.ReqFilter
	LogPath    string
	DailyLimit float64
}

// New creates a new dashboard Model with the given configuration.
func New(cfg Config) Model {
	var viewports [panelCount]*ScrollableViewport
	for i := range viewports {
		viewports[i] = NewScrollableViewport()
	}

	return Model{
		eventStore:  cfg.EventStore,
		projStore:   cfg.ProjStore,
		db:          cfg.DB,
		version:     cfg.Version,
		reqFilter:   cfg.ReqFilter,
		logPath:     cfg.LogPath,
		dailyLimit:  cfg.DailyLimit,
		activePanel: panelPipeline,
		viewports:   viewports,
	}
}

// Init starts the first tick and data fetch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		fetchDataCmd(m),
	)
}

// Update handles messages and returns a new model (immutable pattern).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case tickMsg:
		return m, tea.Batch(
			tickCmd(),
			fetchDataCmd(m),
		)

	case dataMsg:
		return m.handleData(msg)
	}

	return m, nil
}

// View renders the entire dashboard UI.
func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	// Title bar.
	sections = append(sections, m.renderTabs())

	// Error banner if present.
	if m.err != nil {
		sections = append(sections, errorStyle.Render(fmt.Sprintf("  Error: %s", m.err)))
	}

	// Active panel content.
	sections = append(sections, m.viewports[m.activePanel].View())

	// Status bar.
	sections = append(sections, m.renderStatusBar())

	return strings.Join(sections, "\n")
}

// handleKey processes keyboard input and returns a new model.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		updated := m
		updated.activePanel = (m.activePanel + 1) % panelCount
		return updated, nil

	case "shift+tab":
		updated := m
		updated.activePanel = (m.activePanel - 1 + panelCount) % panelCount
		return updated, nil

	case "1", "2", "3", "4", "5", "6":
		updated := m
		updated.activePanel = int(msg.Runes[0] - '1')
		return updated, nil

	case "j", "down":
		m.viewports[m.activePanel].ScrollDown()
		return m, nil

	case "k", "up":
		m.viewports[m.activePanel].ScrollUp()
		return m, nil

	case "g":
		m.viewports[m.activePanel].GotoTop()
		return m, nil

	case "G":
		m.viewports[m.activePanel].GotoBottom()
		return m, nil

	case "pgup":
		m.viewports[m.activePanel].PageUp()
		return m, nil

	case "pgdown":
		m.viewports[m.activePanel].PageDown()
		return m, nil
	}

	return m, nil
}

// handleResize responds to terminal size changes.
func (m Model) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	updated := m
	updated.width = msg.Width
	updated.height = msg.Height

	// Reserve space for tabs (2 lines) + status bar (1 line) + margins.
	panelHeight := msg.Height - 4
	if panelHeight < 1 {
		panelHeight = 1
	}

	for i := range updated.viewports {
		updated.viewports[i].SetHeight(panelHeight)
	}

	// Re-render panels with new dimensions.
	updated.refreshPanelContent()

	return updated, nil
}

// handleData processes refreshed data from the stores.
func (m Model) handleData(msg dataMsg) (tea.Model, tea.Cmd) {
	updated := m
	updated.err = msg.err

	if msg.err == nil {
		updated.requirements = msg.requirements
		updated.stories = msg.stories
		updated.agents = msg.agents
		updated.events = msg.events
		updated.escalations = msg.escalations
		updated.costInfo = msg.costData
		updated.lastRefresh = time.Now()
	}

	updated.refreshPanelContent()
	return updated, nil
}

// refreshPanelContent updates the content of all viewports from cached data.
func (m *Model) refreshPanelContent() {
	m.viewports[panelPipeline].SetContent(renderPipeline(m.stories, m.requirements))
	m.viewports[panelAgents].SetContent(renderAgents(m.agents))
	m.viewports[panelActivity].SetContent(renderActivity(m.events))
	m.viewports[panelEscalations].SetContent(renderEscalations(m.escalations))
	m.viewports[panelCost].SetContent(renderCost(m.costInfo))
	m.viewports[panelLogs].SetContent(renderLogs(m.logPath))
}

// renderTabs renders the tab bar at the top.
func (m Model) renderTabs() string {
	var tabs []string
	for i, name := range panelNames {
		label := fmt.Sprintf("%d:%s", i+1, name)
		if i == m.activePanel {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// renderStatusBar renders the bottom status bar.
func (m Model) renderStatusBar() string {
	// Left: version.
	left := fmt.Sprintf(" PX v%s", m.version)

	// Center: wave progress.
	center := m.waveProgress()

	// Budget summary.
	budget := fmt.Sprintf("$%.2f / $%.2f", m.costInfo.dailyTotal, m.costInfo.dailyLimit)

	// Scroll position for active panel.
	scroll := m.viewports[m.activePanel].ScrollIndicator()

	// Key hints.
	hints := "1-6:panels j/k:scroll q:quit"

	bar := fmt.Sprintf("%s  |  %s  |  %s  |  %s  |  %s",
		left, center, budget, scroll, hints,
	)

	// Pad to full width.
	if m.width > 0 && len(bar) < m.width {
		bar += strings.Repeat(" ", m.width-len(bar))
	}

	return statusBarStyle.Render(bar)
}

// waveProgress computes the current wave progress summary.
func (m Model) waveProgress() string {
	if len(m.stories) == 0 {
		return "No stories"
	}

	// Find the highest wave number.
	maxWave := 0
	for _, s := range m.stories {
		if s.Wave > maxWave {
			maxWave = s.Wave
		}
	}

	if maxWave == 0 {
		return "Wave 0"
	}

	// Count merged stories in the current (highest) wave.
	total := 0
	merged := 0
	for _, s := range m.stories {
		if s.Wave == maxWave {
			total++
			if s.Status == "merged" {
				merged++
			}
		}
	}

	return fmt.Sprintf("Wave %d: %d/%d merged", maxWave, merged, total)
}

// tickCmd returns a command that sends a tickMsg after the refresh interval.
func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchDataCmd returns a command that queries all stores for fresh data.
func fetchDataCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		var msg dataMsg

		reqs, err := m.projStore.ListRequirements(m.reqFilter)
		if err != nil {
			msg.err = fmt.Errorf("list requirements: %w", err)
			return msg
		}
		msg.requirements = reqs

		stories, err := m.projStore.ListStories(state.StoryFilter{})
		if err != nil {
			msg.err = fmt.Errorf("list stories: %w", err)
			return msg
		}
		msg.stories = stories

		agents, err := m.projStore.ListAgents(state.AgentFilter{})
		if err != nil {
			msg.err = fmt.Errorf("list agents: %w", err)
			return msg
		}
		msg.agents = agents

		events, err := m.eventStore.List(state.EventFilter{Limit: activityEventLimit})
		if err != nil {
			msg.err = fmt.Errorf("list events: %w", err)
			return msg
		}
		msg.events = events

		escalations, err := m.projStore.ListEscalations()
		if err != nil {
			msg.err = fmt.Errorf("list escalations: %w", err)
			return msg
		}
		msg.escalations = escalations

		// Query cost data.
		if m.db != nil {
			msg.costData = queryCostData(m.db, reqs, m.dailyLimit)
		}

		return msg
	}
}
