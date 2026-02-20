package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	cfg Config

	width  int
	height int

	beadsDir string
	repoDir  string
	client   *BdClient

	issues []Issue
	byID   map[string]*Issue

	columns      map[Status][]Issue
	columnDepths map[Status]map[string]int
	selectedCol  int
	selectedIdx  map[Status]int
	scrollOffset map[Status]int

	showDetails bool
	mode        Mode
	leader      bool

	searchQuery string
	searchInput textinput.Model

	filter     Filter
	filterForm *FilterForm

	form         *IssueForm
	prompt       *PromptState
	parentPicker *ParentPickerState

	depList *DepListState

	confirmDelete *ConfirmDelete

	toast      string
	toastKind  string
	toastUntil time.Time

	lastHash string
	loading  bool

	keymap Keymap
	styles Styles

	now time.Time
}

func newModel(cfg Config) (model, error) {
	beadsDir, repoDir, err := findBeadsDir(cfg.BeadsDir)
	if err != nil {
		return model{}, err
	}

	sInput := textinput.New()
	sInput.Prompt = "search> "
	sInput.CharLimit = 256
	sInput.Focus()

	m := model{
		cfg:      cfg,
		beadsDir: beadsDir,
		repoDir:  repoDir,
		client:   NewBdClient(repoDir),

		columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		columnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		selectedCol: 0,
		selectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		scrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},

		mode:        ModeBoard,
		searchInput: sInput,
		filter: Filter{
			Status:   "any",
			Priority: "any",
		},
		loading: true,
		now:     time.Now(),
		keymap:  defaultKeymap(),
		styles:  newStyles(),
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadCmd("init")}
	if !m.cfg.NoWatch {
		cmds = append(cmds, tickCmd())
	}
	return tea.Batch(cmds...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) setToast(kind, msg string) {
	m.toastKind = kind
	m.toast = msg
	m.toastUntil = time.Now().Add(3500 * time.Millisecond)
}

func (m *model) clearTransientUI() {
	m.leader = false
	m.prompt = nil
	m.parentPicker = nil
	m.depList = nil
	m.confirmDelete = nil
	m.form = nil
	m.filterForm = nil
}

func (m *model) applyLoadedIssues(issues []Issue, hash string) {
	selectedID := m.currentIssueID()

	m.issues = issues
	m.byID = make(map[string]*Issue, len(issues))
	for i := range m.issues {
		m.byID[m.issues[i].ID] = &m.issues[i]
	}
	m.lastHash = hash

	m.computeColumns()
	m.normalizeSelectionBounds()

	if selectedID != "" {
		m.selectIssueByID(selectedID)
	}

	m.loading = false
}

func (m *model) computeColumns() {
	next := map[Status][]Issue{
		StatusOpen:       {},
		StatusInProgress: {},
		StatusBlocked:    {},
		StatusClosed:     {},
	}
	depths := map[Status]map[string]int{
		StatusOpen:       {},
		StatusInProgress: {},
		StatusBlocked:    {},
		StatusClosed:     {},
	}

	for _, issue := range m.issues {
		if !m.matchesSearch(issue) {
			continue
		}
		if !m.matchesFilter(issue) {
			continue
		}
		next[issue.Display] = append(next[issue.Display], issue)
	}

	for _, status := range statusOrder {
		ordered, depthMap := orderColumnAsTree(next[status])
		next[status] = ordered
		depths[status] = depthMap
	}

	m.columns = next
	m.columnDepths = depths
}

func orderColumnAsTree(input []Issue) ([]Issue, map[string]int) {
	depth := make(map[string]int, len(input))
	if len(input) == 0 {
		return input, depth
	}

	indexByID := make(map[string]int, len(input))
	issueByID := make(map[string]Issue, len(input))
	childrenByParent := make(map[string][]string, len(input))

	for i, issue := range input {
		indexByID[issue.ID] = i
		issueByID[issue.ID] = issue
	}

	roots := make([]string, 0, len(input))
	for _, issue := range input {
		parentID := strings.TrimSpace(issue.Parent)
		if parentID == "" || parentID == issue.ID {
			roots = append(roots, issue.ID)
			continue
		}
		if _, ok := indexByID[parentID]; !ok {
			roots = append(roots, issue.ID)
			continue
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], issue.ID)
	}

	sortIDs := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool {
			return indexByID[ids[i]] < indexByID[ids[j]]
		})
	}

	sortIDs(roots)
	for parent := range childrenByParent {
		sortIDs(childrenByParent[parent])
	}

	ordered := make([]Issue, 0, len(input))
	visited := make(map[string]bool, len(input))

	var dfs func(id string, d int)
	dfs = func(id string, d int) {
		if visited[id] {
			return
		}
		issue, ok := issueByID[id]
		if !ok {
			return
		}
		visited[id] = true
		depth[id] = d
		ordered = append(ordered, issue)
		for _, childID := range childrenByParent[id] {
			dfs(childID, d+1)
		}
	}

	for _, rootID := range roots {
		dfs(rootID, 0)
	}

	// Fallback for cycles or disconnected nodes.
	for _, issue := range input {
		if !visited[issue.ID] {
			dfs(issue.ID, 0)
		}
	}

	return ordered, depth
}

func (m *model) normalizeSelectionBounds() {
	for _, status := range statusOrder {
		col := m.columns[status]
		idx := m.selectedIdx[status]
		if idx >= len(col) {
			idx = len(col) - 1
		}
		if idx < 0 {
			idx = 0
		}
		m.selectedIdx[status] = idx

		maxOffset := idx
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.scrollOffset[status] > maxOffset {
			m.scrollOffset[status] = maxOffset
		}
	}
}

func (m model) matchesSearch(issue Issue) bool {
	q := strings.TrimSpace(strings.ToLower(m.searchQuery))
	if q == "" {
		return true
	}

	inLabels := false
	for _, label := range issue.Labels {
		if strings.Contains(strings.ToLower(label), q) {
			inLabels = true
			break
		}
	}

	return strings.Contains(strings.ToLower(issue.ID), q) ||
		strings.Contains(strings.ToLower(issue.Title), q) ||
		strings.Contains(strings.ToLower(issue.Description), q) ||
		strings.Contains(strings.ToLower(issue.Assignee), q) ||
		inLabels
}

func (m model) matchesFilter(issue Issue) bool {
	if m.filter.Assignee != "" && !strings.EqualFold(issue.Assignee, m.filter.Assignee) {
		return false
	}

	if m.filter.Label != "" {
		found := false
		for _, label := range issue.Labels {
			if strings.EqualFold(label, m.filter.Label) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if m.filter.Status != "" && m.filter.Status != "any" && string(issue.Display) != m.filter.Status {
		return false
	}

	if m.filter.Priority != "" && m.filter.Priority != "any" {
		p, err := parsePriority(m.filter.Priority)
		if err == nil && issue.Priority != p {
			return false
		}
	}

	return true
}

func (m model) currentStatus() Status {
	return statusOrder[m.selectedCol]
}

func (m model) currentColumn() []Issue {
	return m.columns[m.currentStatus()]
}

func (m model) currentIssue() *Issue {
	col := m.currentColumn()
	if len(col) == 0 {
		return nil
	}
	idx := m.selectedIdx[m.currentStatus()]
	if idx < 0 || idx >= len(col) {
		return nil
	}
	issue := col[idx]
	base := m.byID[issue.ID]
	if base != nil {
		return base
	}
	return &issue
}

func (m model) currentIssueID() string {
	issue := m.currentIssue()
	if issue == nil {
		return ""
	}
	return issue.ID
}

func (m *model) selectIssueByID(id string) bool {
	for colIdx, status := range statusOrder {
		col := m.columns[status]
		for idx, issue := range col {
			if strings.EqualFold(issue.ID, id) {
				m.selectedCol = colIdx
				m.selectedIdx[status] = idx
				m.ensureSelectionVisible(status)
				return true
			}
		}
	}
	return false
}

func (m *model) moveSelection(delta int) {
	status := m.currentStatus()
	col := m.columns[status]
	if len(col) == 0 {
		m.selectedIdx[status] = 0
		m.scrollOffset[status] = 0
		return
	}

	idx := m.selectedIdx[status] + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(col) {
		idx = len(col) - 1
	}
	m.selectedIdx[status] = idx
	m.ensureSelectionVisible(status)
}

func (m *model) moveColumn(delta int) {
	next := m.selectedCol + delta
	if next < 0 {
		next = 0
	}
	if next >= len(statusOrder) {
		next = len(statusOrder) - 1
	}
	m.selectedCol = next
	m.ensureSelectionVisible(m.currentStatus())
}

func (m model) boardInnerHeight() int {
	h := m.height
	if h <= 0 {
		return 10
	}

	h -= 1 // title
	h -= 1 // footer
	h -= m.inspectorOuterHeight()

	// Golden Rule: account for borders
	h -= 2

	if h < 6 {
		h = 6
	}
	return h
}

func (m model) inspectorInnerHeight() int {
	if m.showDetails {
		return 5
	}
	return 3
}

func (m model) inspectorOuterHeight() int {
	return m.inspectorInnerHeight() + 2
}

func (m *model) ensureSelectionVisible(status Status) {
	itemsPerPage := m.boardInnerHeight() - 2
	if itemsPerPage < 1 {
		itemsPerPage = 1
	}

	idx := m.selectedIdx[status]
	off := m.scrollOffset[status]
	if idx < off {
		off = idx
	}
	if idx >= off+itemsPerPage {
		off = idx - itemsPerPage + 1
	}
	if off < 0 {
		off = 0
	}
	m.scrollOffset[status] = off
}

func (m model) loadCmd(source string) tea.Cmd {
	return func() tea.Msg {
		issues, hash, err := m.client.ListIssues()
		return loadedMsg{issues: issues, hash: hash, err: err, source: source}
	}
}

func opCmd(info string, fn func() error) tea.Cmd {
	return func() tea.Msg {
		err := fn()
		return opMsg{info: info, err: err}
	}
}

func depListCmd(c *BdClient, issueID string) tea.Cmd {
	return func() tea.Msg {
		txt, err := c.DepList(issueID)
		return depListMsg{issueID: issueID, text: txt, err: err}
	}
}

func deletePreviewCmd(c *BdClient, issueID string) tea.Cmd {
	return func() tea.Msg {
		txt, err := c.DeletePreview(issueID)
		return deletePreviewMsg{issueID: issueID, text: txt, err: err}
	}
}

func buildTitle(m model) string {
	return fmt.Sprintf("BDTUI | %s | .beads: %s", strings.ToUpper(string(m.mode)), m.beadsDir)
}
