package bdtui_test

import b "bdtui"

type model = b.Model

type Status = b.Status
type Issue = b.Issue
type Filter = b.Filter
type SortMode = b.SortMode
type Mode = b.Mode
type PromptAction = b.PromptAction
type PromptState = b.PromptState
type IssueForm = b.IssueForm
type ParentOption = b.ParentOption
type ParentPickerState = b.ParentPickerState
type TmuxPickerState = b.TmuxPickerState
type FilterForm = b.FilterForm
type DeleteMode = b.DeleteMode
type ConfirmDelete = b.ConfirmDelete
type ConfirmClosedParentCreate = b.ConfirmClosedParentCreate
type DepListState = b.DepListState
type PluginRegistry = b.PluginRegistry
type PluginToggles = b.PluginToggles
type TmuxTarget = b.TmuxTarget
type BoardRow = b.BoardRow
type formEditorPayload = b.FormEditorPayload
type formEditorMsg = b.FormEditorMsg
type reopenParentForCreateMsg = b.ReopenParentForCreateMsg

const (
	StatusOpen       = b.StatusOpen
	StatusInProgress = b.StatusInProgress
	StatusBlocked    = b.StatusBlocked
	StatusClosed     = b.StatusClosed

	SortModeStatusDateOnly         = b.SortModeStatusDateOnly
	SortModePriorityThenStatusDate = b.SortModePriorityThenStatusDate

	ModeBoard                     = b.ModeBoard
	ModeDetails                   = b.ModeDetails
	ModeHelp                      = b.ModeHelp
	ModeSearch                    = b.ModeSearch
	ModeFilter                    = b.ModeFilter
	ModeCreate                    = b.ModeCreate
	ModeEdit                      = b.ModeEdit
	ModePrompt                    = b.ModePrompt
	ModeParentPicker              = b.ModeParentPicker
	ModeTmuxPicker                = b.ModeTmuxPicker
	ModeDepList                   = b.ModeDepList
	ModeConfirmDelete             = b.ModeConfirmDelete
	ModeConfirmClosedParentCreate = b.ModeConfirmClosedParentCreate

	PromptAssignee  = b.PromptAssignee
	PromptLabels    = b.PromptLabels
	PromptDepAdd    = b.PromptDepAdd
	PromptDepRemove = b.PromptDepRemove
	PromptParentSet = b.PromptParentSet

	DeleteModeForce   = b.DeleteModeForce
	DeleteModeCascade = b.DeleteModeCascade
)

var (
	parseConfig        = b.ParseConfig
	parsePluginToggles = b.ParsePluginToggles
	parseSortMode      = b.ParseSortMode

	newIssueFormCreate = b.NewIssueFormCreate
	newIssueFormEdit   = b.NewIssueFormEdit
	buildParentOptions = b.BuildParentOptions
	newFilterForm      = b.NewFilterForm

	newStyles     = b.NewStyles
	defaultKeymap = b.DefaultKeymap

	cycleStatus           = b.CycleStatus
	cycleStatusBackward   = b.CycleStatusBackward
	cyclePriority         = b.CyclePriority
	cyclePriorityBackward = b.CyclePriorityBackward
	sortIssuesByMode      = b.SortIssuesByMode

	detailLines            = b.DetailLines
	firstNDescriptionLines = b.FirstNDescriptionLines

	parseEditorContent   = b.ParseEditorContent
	marshalEditorContent = b.MarshalEditorContent

	beadsWatchTargets         = b.BeadsWatchTargets
	isBeadsWatchEventRelevant = b.IsBeadsWatchEventRelevant

	parseTmuxClientSessions     = b.ParseTmuxClientSessions
	parseTmuxTargets            = b.ParseTmuxTargets
	shortType                   = b.ShortType
	shortTypeDashboard          = b.ShortTypeDashboard
	dashboardEpicAccentStyle    = b.DashboardEpicAccentStyle
	dashboardDimmedRowStyle     = b.DashboardDimmedRowStyle
	renderIssueRow              = b.RenderIssueRow
	renderIssueRowSelectedPlain = b.RenderIssueRowSelectedPlain
	renderIssueRowGhostPlain    = b.RenderIssueRowGhostPlain
	statusHeaderStyle           = b.StatusHeaderStyle
	max                         = b.Max
	NewBdClient                 = b.NewBdClient
	statusOrder                 = b.StatusOrder()
	ansiSGRRegexp               = b.AnsiSGRRegexp()
)

func newTmuxPlugin(enabled bool, runner interface {
	Run(args ...string) (string, error)
}) *b.TmuxPlugin {
	return b.NewTmuxPlugin(enabled, runner)
}
