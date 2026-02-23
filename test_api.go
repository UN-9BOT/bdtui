package bdtui

import (
	"regexp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
)

type BoardRow struct {
	Issue Issue
	Depth int
	Ghost bool
}

type TmuxRunner interface {
	Run(args ...string) (string, error)
}

type FormEditorPayload = formEditorPayload

type FormEditorMsg struct {
	Payload FormEditorPayload
	Err     error
}

func (m FormEditorMsg) toInternal() formEditorMsg {
	return formEditorMsg{
		payload: m.Payload,
		err:     m.Err,
	}
}

type ReopenParentForCreateMsg struct {
	ParentID string
	Err      error
}

func NewModel(cfg Config) (Model, error) {
	return newModel(cfg)
}

func ParseConfig(args []string) (Config, error) {
	return parseConfig(args)
}

func ParsePluginToggles(raw string) (PluginToggles, error) {
	return parsePluginToggles(raw)
}

func (t PluginToggles) Enabled(name string) bool {
	return t.enabled(name)
}

func NewIssueFormCreate(issues []Issue) *IssueForm {
	return newIssueFormCreate(issues)
}

func NewIssueFormEdit(issue *Issue, issues []Issue) *IssueForm {
	return newIssueFormEdit(issue, issues)
}

func (f *IssueForm) Fields() []string {
	return f.fields()
}

func (f *IssueForm) IsTextField(field string) bool {
	return f.isTextField(field)
}

func (f *IssueForm) LoadInputFromField() {
	f.loadInputFromField()
}

func (f *IssueForm) SaveInputToField() {
	f.saveInputToField()
}

func BuildParentOptions(issues []Issue, selfID string, selectedParent string) ([]ParentOption, int) {
	return buildParentOptions(issues, selfID, selectedParent)
}

func NewFilterForm(base Filter) *FilterForm {
	return newFilterForm(base)
}

func ParseSortMode(raw string) (SortMode, bool) {
	return parseSortMode(raw)
}

func NewStyles() Styles {
	return newStyles()
}

func DefaultKeymap() Keymap {
	return defaultKeymap()
}

func CycleStatus(current Status) Status {
	return cycleStatus(current)
}

func CycleStatusBackward(current Status) Status {
	return cycleStatusBackward(current)
}

func CyclePriority(current int) int {
	return cyclePriority(current)
}

func CyclePriorityBackward(current int) int {
	return cyclePriorityBackward(current)
}

func SortIssuesByMode(items []Issue, mode SortMode) {
	sortIssuesByMode(items, mode)
}

func DetailLines(issue *Issue, inner int) []string {
	return detailLines(issue, inner)
}

func FirstNDescriptionLines(description string, maxSourceLines int, width int) ([]string, bool) {
	return firstNDescriptionLines(description, maxSourceLines, width)
}

func ParseEditorContent(raw []byte) (FormEditorPayload, error) {
	return parseEditorContent(raw)
}

func MarshalEditorContent(payload FormEditorPayload) ([]byte, error) {
	return marshalEditorContent(payload)
}

func BeadsWatchTargets(root string) []string {
	return beadsWatchTargets(root)
}

func IsBeadsWatchEventRelevant(ev fsnotify.Event) bool {
	return isBeadsWatchEventRelevant(ev)
}

func NewTmuxPlugin(enabled bool, runner TmuxRunner) *TmuxPlugin {
	return newTmuxPlugin(enabled, runner)
}

func ParseTmuxClientSessions(raw string) map[string]bool {
	return parseTmuxClientSessions(raw)
}

func ParseTmuxTargets(raw string) []TmuxTarget {
	return parseTmuxTargets(raw)
}

func ShortType(issueType string) string {
	return shortType(issueType)
}

func ShortTypeDashboard(issueType string) string {
	return shortTypeDashboard(issueType)
}

func DashboardEpicAccentStyle(issueType string) (lipgloss.Style, bool) {
	return dashboardEpicAccentStyle(issueType)
}

func DashboardDimmedRowStyle(issueType string, foreground lipgloss.Color, faint bool) lipgloss.Style {
	return dashboardDimmedRowStyle(issueType, foreground, faint)
}

func RenderIssueRow(item Issue, maxTextWidth int, depth int) string {
	return renderIssueRow(item, maxTextWidth, depth)
}

func RenderIssueRowSelectedPlain(item Issue, maxTextWidth int, depth int) string {
	return renderIssueRowSelectedPlain(item, maxTextWidth, depth)
}

func RenderIssueRowGhostPlain(item Issue, maxTextWidth int, depth int) string {
	return renderIssueRowGhostPlain(item, maxTextWidth, depth)
}

func StatusHeaderStyle(status Status) lipgloss.Style {
	return statusHeaderStyle(status)
}

func StatusIndex(status Status) int {
	for i, s := range statusOrder {
		if s == status {
			return i
		}
	}
	return 0
}

func Max(a, b int) int {
	return max(a, b)
}

func StatusOrder() []Status {
	return append([]Status(nil), statusOrder...)
}

func AnsiSGRRegexp() *regexp.Regexp {
	return ansiSGRRegexp
}

func (m Model) BuildColumnRows(status Status) ([]BoardRow, map[string]int) {
	rows, issueRowIndex := m.buildColumnRows(status)
	out := make([]BoardRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, BoardRow{
			Issue: row.issue,
			Depth: row.depth,
			Ghost: row.ghost,
		})
	}
	return out, issueRowIndex
}

func (m Model) SelectedVisibleRowIndex(status Status) int {
	return m.selectedVisibleRowIndex(status)
}

func (m *Model) EnsureSelectionVisible(status Status) {
	m.ensureSelectionVisible(status)
}

func (m *Model) ComputeColumns() {
	m.computeColumns()
}

func (m *Model) NormalizeSelectionBounds() {
	m.normalizeSelectionBounds()
}

func (m *Model) SelectIssueByID(id string) bool {
	return m.selectIssueByID(id)
}

func (m Model) CurrentIssue() *Issue {
	return m.currentIssue()
}

func (m Model) HandleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	return m.handleMouse(msg)
}

func (m Model) HandleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleKey(msg)
}

func (m Model) HandleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleHelpKey(msg)
}

func (m Model) HandleDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleDetailsKey(msg)
}

func (m Model) HandleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleSearchKey(msg)
}

func (m Model) HandleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleFormKey(msg)
}

func (m Model) HandleTmuxPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleTmuxPickerKey(msg)
}

func (m Model) HandleConfirmClosedParentCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleConfirmClosedParentCreateKey(msg)
}

func (m Model) HandleBoardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleBoardKey(msg)
}

func (m Model) HandleLeaderCombo(key string) (tea.Model, tea.Cmd) {
	return m.handleLeaderCombo(key)
}

func (m *Model) MarkTmuxPickerSelection() error {
	return m.markTmuxPickerSelection()
}

func (m Model) FormatBeadsStartTaskCommand(issueID string) string {
	return m.formatBeadsStartTaskCommand(issueID)
}

func (m Model) RenderInlineSearchBlock() string {
	return m.renderInlineSearchBlock()
}

func (m Model) RenderBoard() string {
	return m.renderBoard()
}

func (m Model) RenderInspector() string {
	return m.renderInspector()
}

func (m Model) RenderFooter() string {
	return m.renderFooter()
}

func (m Model) RenderHelpModal() string {
	return m.renderHelpModal()
}

func (m Model) RenderFormModal() string {
	return m.renderFormModal()
}

func (m Model) RenderDeleteModal() string {
	return m.renderDeleteModal()
}

func (m Model) RenderConfirmClosedParentCreateModal() string {
	return m.renderConfirmClosedParentCreateModal()
}

func (m Model) HelpContentLines() []string {
	return m.helpContentLines()
}

func (m Model) HelpViewportContentLines() int {
	return m.helpViewportContentLines()
}

func (m Model) HelpMaxScroll() int {
	return m.helpMaxScroll()
}

func (m Model) BoardInnerHeight() int {
	return m.boardInnerHeight()
}

func (m Model) InspectorInnerHeight() int {
	return m.inspectorInnerHeight()
}

func (m Model) InspectorOuterHeight() int {
	return m.inspectorOuterHeight()
}

func (m Model) DetailsViewportHeight() int {
	return m.detailsViewportHeight()
}

func (m Model) ApplyFocusDimming(out string) string {
	return m.applyFocusDimming(out)
}
