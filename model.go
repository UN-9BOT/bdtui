package bdtui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	Cfg Config

	Width  int
	Height int

	BeadsDir string
	RepoDir  string
	Client   *BdClient

	Issues []Issue
	ByID   map[string]*Issue

	Columns      map[Status][]Issue
	ColumnDepths map[Status]map[string]int
	SelectedCol  int
	SelectedIdx  map[Status]int
	ScrollOffset map[Status]int

	ShowDetails    bool
	DetailsScroll  int
	DetailsIssueID string
	HelpScroll     int
	HelpQuery      string
	SortMode       SortMode
	Mode           Mode
	Leader         bool

	SearchQuery    string
	SearchPrev     string
	SearchInput    textinput.Model
	SearchExpanded bool

	Filter     Filter
	FilterForm *FilterForm

	Form         *IssueForm
	Prompt       *PromptState
	ParentPicker *ParentPickerState
	TmuxPicker   *TmuxPickerState

	DepList *DepListState

	ConfirmDelete             *ConfirmDelete
	ConfirmClosedParentCreate *ConfirmClosedParentCreate

	Toast      string
	ToastKind  string
	ToastUntil time.Time

	LastHash string
	Loading  bool

	Keymap Keymap
	Styles Styles

	Plugins                  PluginRegistry
	OpenFormInEditorOverride func(Model) (tea.Cmd, error)
	ResumeDetailsAfterEditor bool
	TmuxMark                 struct {
		PaneID string
		Token  int
	}
	UIFocused bool

	Now time.Time
}

type model = Model

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
		Cfg:      cfg,
		BeadsDir: beadsDir,
		RepoDir:  repoDir,
		Client:   NewBdClient(repoDir),

		Columns: map[Status][]Issue{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		ColumnDepths: map[Status]map[string]int{
			StatusOpen:       {},
			StatusInProgress: {},
			StatusBlocked:    {},
			StatusClosed:     {},
		},
		SelectedCol: 0,
		SelectedIdx: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},
		ScrollOffset: map[Status]int{
			StatusOpen:       0,
			StatusInProgress: 0,
			StatusBlocked:    0,
			StatusClosed:     0,
		},

		DetailsScroll:  0,
		DetailsIssueID: "",
		SortMode:       SortModeStatusDateOnly,

		Mode:        ModeBoard,
		SearchInput: sInput,
		Filter: Filter{
			Status:   "any",
			Priority: "any",
			Type:     "any",
		},
		Loading:   true,
		Now:       time.Now(),
		Keymap:    defaultKeymap(),
		Styles:    newStyles(),
		Plugins:   newPluginRegistry(cfg),
		UIFocused: true,
	}

	if mode, err := m.Client.GetSortMode(); err == nil {
		m.SortMode = mode
	}

	return m, nil
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadCmd("init")}
	if !m.Cfg.NoWatch {
		cmds = append(cmds, tickCmd())
		cmds = append(cmds, watchBeadsChangesCmd(m.BeadsDir))
	}
	return tea.Batch(cmds...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) setToast(kind, msg string) {
	m.ToastKind = kind
	m.Toast = msg
	m.ToastUntil = time.Now().Add(3500 * time.Millisecond)
}

func (m *model) clearTransientUI() {
	m.Leader = false
	m.Prompt = nil
	m.ParentPicker = nil
	m.TmuxPicker = nil
	m.DepList = nil
	m.ConfirmDelete = nil
	m.ConfirmClosedParentCreate = nil
	m.Form = nil
	m.FilterForm = nil
}

func (m *model) applyLoadedIssues(issues []Issue, hash string) {
	selectedID := m.currentIssueID()

	m.Issues = issues
	m.ByID = make(map[string]*Issue, len(issues))
	for i := range m.Issues {
		m.ByID[m.Issues[i].ID] = &m.Issues[i]
	}
	m.LastHash = hash

	m.computeColumns()
	m.normalizeSelectionBounds()

	if selectedID != "" {
		m.selectIssueByID(selectedID)
	}

	m.clampDetailsScroll()
	m.Loading = false
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

	for _, issue := range m.Issues {
		if !m.matchesFilter(issue) {
			continue
		}
		if !m.matchesSearch(issue) {
			continue
		}
		next[issue.Display] = append(next[issue.Display], issue)
	}

	for _, status := range statusOrder {
		sortIssuesByMode(next[status], m.SortMode)
		ordered, depthMap := orderColumnAsTree(next[status])
		next[status] = ordered
		depths[status] = depthMap
	}

	m.Columns = next
	m.ColumnDepths = depths
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
		col := m.Columns[status]
		idx := m.SelectedIdx[status]
		if idx >= len(col) {
			idx = len(col) - 1
		}
		if idx < 0 {
			idx = 0
		}
		m.SelectedIdx[status] = idx

		maxOffset := m.selectedVisibleRowIndex(status)
		if maxOffset < 0 {
			maxOffset = 0
		}
		if m.ScrollOffset[status] > maxOffset {
			m.ScrollOffset[status] = maxOffset
		}
	}
}

func (m model) matchesSearch(issue Issue) bool {
	q := strings.TrimSpace(strings.ToLower(m.SearchQuery))
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
	if m.Filter.Assignee != "" && !strings.EqualFold(issue.Assignee, m.Filter.Assignee) {
		return false
	}

	if m.Filter.Label != "" {
		found := false
		for _, label := range issue.Labels {
			if strings.EqualFold(label, m.Filter.Label) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if m.Filter.Status != "" && m.Filter.Status != "any" && string(issue.Display) != m.Filter.Status {
		return false
	}

	if m.Filter.Priority != "" && m.Filter.Priority != "any" {
		p, err := parsePriority(m.Filter.Priority)
		if err == nil && issue.Priority != p {
			return false
		}
	}

	if m.Filter.Type != "" && m.Filter.Type != "any" && !strings.EqualFold(issue.IssueType, m.Filter.Type) {
		return false
	}

	return true
}

func (m model) currentStatus() Status {
	return statusOrder[m.SelectedCol]
}

func (m model) currentColumn() []Issue {
	return m.Columns[m.currentStatus()]
}

func (m model) currentIssue() *Issue {
	col := m.currentColumn()
	if len(col) == 0 {
		return nil
	}
	idx := m.SelectedIdx[m.currentStatus()]
	if idx < 0 || idx >= len(col) {
		return nil
	}
	issue := col[idx]
	base := m.ByID[issue.ID]
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
		col := m.Columns[status]
		for idx, issue := range col {
			if strings.EqualFold(issue.ID, id) {
				m.SelectedCol = colIdx
				m.SelectedIdx[status] = idx
				m.ensureSelectionVisible(status)
				return true
			}
		}
	}
	return false
}

func (m *model) moveSelection(delta int) {
	status := m.currentStatus()
	col := m.Columns[status]
	if len(col) == 0 {
		m.SelectedIdx[status] = 0
		m.ScrollOffset[status] = 0
		return
	}

	idx := m.SelectedIdx[status] + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(col) {
		idx = len(col) - 1
	}
	m.SelectedIdx[status] = idx
	m.ensureSelectionVisible(status)
}

func (m *model) moveColumn(delta int) {
	next := m.SelectedCol + delta
	if next < 0 {
		next = 0
	}
	if next >= len(statusOrder) {
		next = len(statusOrder) - 1
	}
	m.SelectedCol = next
	m.ensureSelectionVisible(m.currentStatus())
}

func (m model) boardInnerHeight() int {
	h := m.Height
	if h <= 0 {
		return 10
	}

	h -= 1 // title
	h -= 1 // footer
	h -= m.inspectorOuterHeight()
	h -= m.inlineSearchBlockHeight()

	// Golden Rule: account for borders
	h -= 2

	if h < 6 {
		h = 6
	}
	return h
}

func (m model) inspectorInnerWidth() int {
	w := max(20, m.Width-4)
	return max(4, w-4)
}

func (m model) inspectorInnerHeight() int {
	const (
		collapsedInner   = 3
		maxPercentNum    = 2 // 2/5 = 40%
		maxPercentDen    = 5
		minOuter         = 5
		minBoardInner    = 6
		baseLayoutChrome = 4 // title + footer + board border
	)
	if !m.ShowDetails {
		return collapsedInner
	}

	targetOuter := (m.Height * maxPercentNum) / maxPercentDen
	if targetOuter < minOuter {
		targetOuter = minOuter
	}

	layoutChrome := baseLayoutChrome + m.inlineSearchBlockHeight()
	maxOuter := m.Height - (layoutChrome + minBoardInner)
	if maxOuter < minOuter {
		maxOuter = minOuter
	}
	if targetOuter > maxOuter {
		targetOuter = maxOuter
	}

	inner := targetOuter - 2 // inspector border
	if inner < collapsedInner {
		return collapsedInner
	}
	return inner
}

func (m model) inlineSearchBlockHeight() int {
	return 1 + m.inlineFiltersBlockHeight()
}

func (m model) inlineFiltersBlockHeight() int {
	if !m.inlineFiltersVisible() {
		return 0
	}
	if m.Mode == ModeSearch && m.SearchExpanded {
		return 6 // header + 5 filter keys
	}
	return 1
}

func (m model) inlineFiltersVisible() bool {
	if m.Mode == ModeSearch && m.SearchExpanded {
		return true
	}
	return m.Mode != ModeSearch && !m.Filter.IsEmpty()
}

func (m model) detailsViewportHeight() int {
	if !m.ShowDetails {
		return 0
	}
	h := m.inspectorInnerHeight() - 3
	if h < 0 {
		return 0
	}
	return h
}

func (m model) inspectorOuterHeight() int {
	return m.inspectorInnerHeight() + 2
}

func (m model) detailsMaxScroll(issue *Issue) int {
	if issue == nil {
		return 0
	}
	height := m.detailsViewportHeight()
	if height <= 0 {
		return 0
	}
	lines := detailLines(issue, m.inspectorInnerWidth())
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m *model) clampDetailsScroll() {
	if !m.ShowDetails {
		m.DetailsScroll = 0
		m.DetailsIssueID = ""
		return
	}
	issue := m.currentIssue()
	if issue == nil {
		m.DetailsScroll = 0
		m.DetailsIssueID = ""
		return
	}
	if m.DetailsIssueID != issue.ID {
		m.DetailsIssueID = issue.ID
		m.DetailsScroll = 0
		return
	}
	maxOffset := m.detailsMaxScroll(issue)
	if m.DetailsScroll > maxOffset {
		m.DetailsScroll = maxOffset
	}
	if m.DetailsScroll < 0 {
		m.DetailsScroll = 0
	}
}

func (m *model) ensureSelectionVisible(status Status) {
	itemsPerPage := m.boardInnerHeight() - 3
	if itemsPerPage < 1 {
		itemsPerPage = 1
	}

	idx := m.selectedVisibleRowIndex(status)
	off := m.ScrollOffset[status]
	if idx < off {
		off = idx
	}
	if idx >= off+itemsPerPage {
		off = idx - itemsPerPage + 1
	}
	if off < 0 {
		off = 0
	}
	m.ScrollOffset[status] = off
}

func (m model) selectedVisibleRowIndex(status Status) int {
	col := m.Columns[status]
	if len(col) == 0 {
		return 0
	}

	idx := m.SelectedIdx[status]
	if idx < 0 {
		idx = 0
	}
	if idx >= len(col) {
		idx = len(col) - 1
	}

	rows, issueRowIndex := m.buildColumnRows(status)
	if len(rows) == 0 {
		return 0
	}
	issueID := col[idx].ID
	if rowIdx, ok := issueRowIndex[issueID]; ok {
		return rowIdx
	}
	return idx
}

func (m model) loadCmd(source string) tea.Cmd {
	return func() tea.Msg {
		issues, hash, err := m.Client.ListIssues()
		return loadedMsg{Issues: issues, hash: hash, err: err, source: source}
	}
}

func opCmd(info string, fn func() error) tea.Cmd {
	return func() tea.Msg {
		err := fn()
		return opMsg{info: info, err: err}
	}
}

func pluginCmd(info string, fn func() error) tea.Cmd {
	return func() tea.Msg {
		err := fn()
		return pluginMsg{info: info, err: err}
	}
}

func (m *model) scheduleTmuxMarkCleanup(delay time.Duration) tea.Cmd {
	paneID := strings.TrimSpace(m.TmuxMark.PaneID)
	if paneID == "" {
		return nil
	}
	m.TmuxMark.Token++
	token := m.TmuxMark.Token
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return tmuxMarkCleanupMsg{PaneID: paneID, Token: token}
	})
}

func (m *model) cancelTmuxMarkCleanup() {
	m.TmuxMark.Token++
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
	return fmt.Sprintf("BDTUI | %s | .beads: %s", strings.ToUpper(string(m.Mode)), m.BeadsDir)
}

func sortIssuesByMode(items []Issue, mode SortMode) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		if mode == SortModePriorityThenStatusDate && left.Priority != right.Priority {
			return left.Priority < right.Priority
		}

		if left.UpdatedAt != right.UpdatedAt {
			return left.UpdatedAt > right.UpdatedAt
		}
		return left.ID < right.ID
	})
}

func persistSortModeCmd(client *BdClient, mode SortMode) tea.Cmd {
	if client == nil {
		return nil
	}

	return func() tea.Msg {
		err := client.SetSortMode(mode)
		return sortModePersistMsg{Mode: mode, err: err}
	}
}

func (m *model) setIssueStatusLocal(id string, status Status) {
	if strings.TrimSpace(id) == "" {
		return
	}
	for i := range m.Issues {
		if m.Issues[i].ID != id {
			continue
		}
		m.Issues[i].Status = status
		m.Issues[i].Display = status
		break
	}
	if issue := m.ByID[id]; issue != nil {
		issue.Status = status
		issue.Display = status
	}
}
