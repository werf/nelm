package track

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/chanced/caps"
	"github.com/gookit/color"
	prtable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/samber/lo"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
)

type TablesBuilder struct {
	taskStore *statestore.TaskStore
	logStore  *kdutil.Concurrent[*logstore.LogStore]

	defaultNamespace      string
	maxProgressTableWidth int
	maxLogEventTableWidth int
	colorize              bool

	nextLogPointers    map[string]int
	nextEventPointers  map[string]int
	hideReadinessTasks map[string]bool
	hidePresenceTasks  map[string]bool
	hideAbsenceTasks   map[string]bool
}

func NewTablesBuilder(taskStore *statestore.TaskStore, logStore *kdutil.Concurrent[*logstore.LogStore], opts TablesBuilderOptions) *TablesBuilder {
	defaultNamespace := lo.WithoutEmpty([]string{opts.DefaultNamespace, v1.NamespaceDefault})[0]

	builder := &TablesBuilder{
		taskStore:          taskStore,
		logStore:           logStore,
		defaultNamespace:   defaultNamespace,
		colorize:           opts.Colorize,
		nextLogPointers:    make(map[string]int),
		nextEventPointers:  make(map[string]int),
		hideReadinessTasks: make(map[string]bool),
		hidePresenceTasks:  make(map[string]bool),
		hideAbsenceTasks:   make(map[string]bool),
	}

	builder.SetMaxTableWidth(opts.MaxTableWidth)

	return builder
}

type TablesBuilderOptions struct {
	DefaultNamespace string
	Colorize         bool
	MaxTableWidth    int
}

func (b *TablesBuilder) BuildProgressTable() (table prtable.Writer, notEmpty bool) {
	table = prtable.NewWriter()
	setProgressTableStyle(table, b.maxProgressTableWidth)

	rowsGrouped := [][]prtable.Row{}

	if progressRows := b.buildReadinessProgressRows(); len(progressRows) != 0 {
		rowsGrouped = append(rowsGrouped, progressRows)
	}

	if presenceRows := b.buildPresenceProgressRows(); len(presenceRows) != 0 {
		rowsGrouped = append(rowsGrouped, presenceRows)
	}

	if absenceRows := b.buildAbsenceProgressRows(); len(absenceRows) != 0 {
		rowsGrouped = append(rowsGrouped, absenceRows)
	}

	if len(rowsGrouped) == 0 {
		return nil, false
	}

	for i, rowGroup := range rowsGrouped {
		if i != 0 {
			table.AppendRow(prtable.Row{"", "", ""})
		}

		table.AppendRows(rowGroup)
	}

	return table, true
}

func (b *TablesBuilder) BuildLogTables() (tables map[string]prtable.Writer, nonEmpty bool) {
	tables = make(map[string]prtable.Writer)

	b.logStore.RTransaction(func(ls *logstore.LogStore) {
		for _, crl := range ls.ResourcesLogs() {
			crl.RTransaction(func(rl *logstore.ResourceLogs) {
				for source, logLines := range rl.LogLines() {
					table := prtable.NewWriter()
					setLogTableStyle(table, b.maxLogEventTableWidth)

					header := buildLogsHeader(rl, source, b.defaultNamespace, b.colorize)

					nextLogPointer, found := b.nextLogPointers[header]
					if !found {
						nextLogPointer = 0
					}

					for i, logLine := range logLines {
						if i < nextLogPointer {
							continue
						}

						table.AppendRow(prtable.Row{logLine.Line})

						nextLogPointer++
					}

					b.nextLogPointers[header] = nextLogPointer

					if table.Length() != 0 {
						tables[header] = table
					}
				}
			})
		}
	})

	if len(tables) == 0 {
		return nil, false
	}

	return tables, true
}

func (b *TablesBuilder) BuildEventTables() (tables map[string]prtable.Writer, nonEmpty bool) {
	tables = make(map[string]prtable.Writer)

	for _, crts := range b.taskStore.ReadinessTasksStates() {
		crts.RTransaction(func(rts *statestore.ReadinessTaskState) {
			for _, crs := range rts.ResourceStates() {
				crs.RTransaction(func(rs *statestore.ResourceState) {
					events := rs.Events()
					if len(events) == 0 {
						return
					}

					table := prtable.NewWriter()
					setEventTableStyle(table, b.maxLogEventTableWidth)

					header := buildEventsHeader(rs, b.defaultNamespace, b.colorize)

					nextEventPointer, found := b.nextEventPointers[header]
					if !found {
						nextEventPointer = 0
					}

					for i, event := range events {
						if i < nextEventPointer {
							continue
						}

						table.AppendRow(prtable.Row{event.Message})

						nextEventPointer++
					}

					b.nextEventPointers[header] = nextEventPointer

					if table.Length() != 0 {
						tables[header] = table
					}
				})
			}
		})
	}

	if len(tables) == 0 {
		return nil, false
	}

	return tables, true
}

func (b *TablesBuilder) SetMaxTableWidth(maxTableWidth int) {
	var maxProgressTableWidth int
	if maxTableWidth > 0 {
		maxProgressTableWidth = maxTableWidth
	} else {
		maxProgressTableWidth = 140
	}
	b.maxProgressTableWidth = lo.Min([]int{maxProgressTableWidth, 200})

	var maxLogEventTableWidth int
	if maxTableWidth > 0 {
		maxLogEventTableWidth = maxTableWidth
	} else {
		maxLogEventTableWidth = 140
	}
	b.maxLogEventTableWidth = lo.Min([]int{maxLogEventTableWidth, 250})
}

func (b *TablesBuilder) buildReadinessProgressRows() (rows []prtable.Row) {
	crtss := b.taskStore.ReadinessTasksStates()
	sortReadinessTaskStates(crtss)

	for _, crts := range crtss {
		crts.RTransaction(func(rts *statestore.ReadinessTaskState) {
			if hide, ok := b.hideReadinessTasks[rts.UUID()]; ok && hide {
				return
			}

			readyPods := calculateReadyPods(rts)

			for _, crs := range rts.ResourceStates() {
				crs.RTransaction(func(rs *statestore.ResourceState) {
					var (
						stateCell    string
						resourceCell string
						infoCell     []string
					)

					isRootResource := rts.Name() == rs.Name() && rts.Namespace() == rs.Namespace() && rts.GroupVersionKind() == rs.GroupVersionKind()

					if isRootResource {
						stateCell = buildReadinessRootResourceStateCell(rts, b.colorize)
						resourceCell = buildRootResourceCell(rs, b.colorize)
					} else {
						stateCell = buildReadinessChildResourceStateCell(rs, b.colorize)
						resourceCell = buildChildResourceCell(rs, b.colorize)
					}

					if rs.Namespace() != "" && rs.Namespace() != b.defaultNamespace {
						infoCell = append(infoCell, buildNamespaceInfo(rs))
					}

					if statusInfo := buildStatusInfo(rs); statusInfo != "" {
						infoCell = append(infoCell, statusInfo)
					}

					if isRootResource && readyPods != nil {
						if readyPodsInfo := buildReadyPodsInfo(rs, *readyPods); readyPodsInfo != "" {
							infoCell = append(infoCell, readyPodsInfo)
						}
					}

					if genericConditionInfo := buildGenericConditionInfo(rs); genericConditionInfo != "" {
						infoCell = append(infoCell, genericConditionInfo)
					}

					if len(rs.Errors()) != 0 {
						infoCell = append(
							infoCell,
							buildErrorsInfo(rs),
							buildLastErrInfo(rs, b.colorize),
						)
					}

					rows = append(rows, prtable.Row{resourceCell, stateCell, strings.Join(infoCell, "  ")})

					if rts.Status() == statestore.ReadinessTaskStatusReady {
						b.hideReadinessTasks[rts.UUID()] = true
					}
				})
			}
		})
	}

	if len(rows) > 0 {
		headerRow := buildReadinessHeaderRow(b.colorize)
		rows = append([]prtable.Row{headerRow}, rows...)
	}

	return rows
}

func (b *TablesBuilder) buildPresenceProgressRows() (rows []prtable.Row) {
	cptss := b.taskStore.PresenceTasksStates()
	sortPresenceTaskStates(cptss)

	for _, cpts := range cptss {
		cpts.RTransaction(func(pts *statestore.PresenceTaskState) {
			if hide, ok := b.hidePresenceTasks[pts.UUID()]; ok && hide {
				return
			}

			pts.ResourceState().RTransaction(func(rs *statestore.ResourceState) {
				stateCell := buildPresenceRootResourceStateCell(pts, b.colorize)
				resourceCell := buildRootResourceCell(rs, b.colorize)

				var infoCell []string

				if rs.Namespace() != "" && rs.Namespace() != b.defaultNamespace {
					infoCell = append(infoCell, buildNamespaceInfo(rs))
				}

				if len(rs.Errors()) != 0 {
					infoCell = append(
						infoCell,
						buildErrorsInfo(rs),
						buildLastErrInfo(rs, b.colorize),
					)
				}

				rows = append(rows, prtable.Row{resourceCell, stateCell, strings.Join(infoCell, "  ")})

				if pts.Status() == statestore.PresenceTaskStatusPresent {
					b.hidePresenceTasks[pts.UUID()] = true
				}
			})
		})
	}

	if len(rows) > 0 {
		headerRow := buildPresenceHeaderRow(b.colorize)
		rows = append([]prtable.Row{headerRow}, rows...)
	}

	return rows
}

func (b *TablesBuilder) buildAbsenceProgressRows() (rows []prtable.Row) {
	catss := b.taskStore.AbsenceTasksStates()
	sortAbsenceTaskStates(catss)

	for _, cats := range catss {
		cats.RTransaction(func(ats *statestore.AbsenceTaskState) {
			if hide, ok := b.hideAbsenceTasks[ats.UUID()]; ok && hide {
				return
			}

			ats.ResourceState().RTransaction(func(rs *statestore.ResourceState) {
				stateCell := buildAbsenceRootResourceStateCell(ats, b.colorize)
				resourceCell := buildRootResourceCell(rs, b.colorize)

				var infoCell []string

				if rs.Namespace() != "" && rs.Namespace() != b.defaultNamespace {
					infoCell = append(infoCell, buildNamespaceInfo(rs))
				}

				if len(rs.Errors()) != 0 {
					infoCell = append(
						infoCell,
						buildErrorsInfo(rs),
						buildLastErrInfo(rs, b.colorize),
					)
				}

				rows = append(rows, prtable.Row{resourceCell, stateCell, strings.Join(infoCell, "  ")})

				if ats.Status() == statestore.AbsenceTaskStatusAbsent {
					b.hideAbsenceTasks[ats.UUID()] = true
				}
			})
		})
	}

	if len(rows) > 0 {
		headerRow := buildAbsenceHeaderRow(b.colorize)
		rows = append([]prtable.Row{headerRow}, rows...)
	}

	return rows
}

func buildReadinessHeaderRow(colorize bool) prtable.Row {
	resourceColumn := "RESOURCE (→READY)"
	if colorize {
		resourceColumn = color.New(color.Bold).Sprintf(resourceColumn)
	}

	stateColumn := "STATE"
	if colorize {
		stateColumn = color.New(color.Bold).Sprintf(stateColumn)
	}

	infoColumn := "INFO"
	if colorize {
		infoColumn = color.New(color.Bold).Sprintf(infoColumn)
	}

	headerRow := prtable.Row{
		resourceColumn,
		stateColumn,
		infoColumn,
	}

	return headerRow
}

func buildPresenceHeaderRow(colorize bool) prtable.Row {
	resourceColumn := "RESOURCE (→PRESENT)"
	if colorize {
		resourceColumn = color.New(color.Bold).Sprintf(resourceColumn)
	}

	stateColumn := "STATE"
	if colorize {
		stateColumn = color.New(color.Bold).Sprintf(stateColumn)
	}

	infoColumn := "INFO"
	if colorize {
		infoColumn = color.New(color.Bold).Sprintf(infoColumn)
	}

	headerRow := prtable.Row{
		resourceColumn,
		stateColumn,
		infoColumn,
	}

	return headerRow
}

func buildAbsenceHeaderRow(colorize bool) prtable.Row {
	resourceColumn := "RESOURCE (→ABSENT)"
	if colorize {
		resourceColumn = color.New(color.Bold).Sprintf(resourceColumn)
	}

	stateColumn := "STATE"
	if colorize {
		stateColumn = color.New(color.Bold).Sprintf(stateColumn)
	}

	infoColumn := "INFO"
	if colorize {
		infoColumn = color.New(color.Bold).Sprintf(infoColumn)
	}

	headerRow := prtable.Row{
		resourceColumn,
		stateColumn,
		infoColumn,
	}

	return headerRow
}

func setProgressTableStyle(table prtable.Writer, tableWidth int) {
	style := prtable.StyleBoxDefault
	style.PaddingLeft = ""
	style.PaddingRight = "  "

	columnConfigs := []prtable.ColumnConfig{
		{
			Number: 1,
		},
		{
			Number: 2,
		},
		{
			Number: 3,
		},
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))
	columnsWidth := tableWidth - paddingsWidth

	columnConfigs[1].WidthMax = 7
	columnConfigs[0].WidthMax = int(math.Floor(float64(columnsWidth-columnConfigs[1].WidthMax)) * 0.6)
	columnConfigs[2].WidthMax = int(math.Floor(float64(columnsWidth-columnConfigs[1].WidthMax)) * 0.4)

	table.SetColumnConfigs(columnConfigs)
	table.SetStyle(prtable.Style{
		Box:     style,
		Color:   prtable.ColorOptionsDefault,
		Format:  prtable.FormatOptions{},
		HTML:    prtable.DefaultHTMLOptions,
		Options: prtable.OptionsNoBordersAndSeparators,
		Title:   prtable.TitleOptionsDefault,
	})
}

func setLogTableStyle(table prtable.Writer, tableWidth int) {
	style := prtable.StyleBoxDefault
	style.PaddingLeft = ""
	style.PaddingRight = ""

	columnConfigs := []prtable.ColumnConfig{
		{
			Number: 1,
		},
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))
	columnsWidth := tableWidth - paddingsWidth

	columnConfigs[0].WidthMax = columnsWidth

	table.SetColumnConfigs(columnConfigs)
	table.SetStyle(prtable.Style{
		Box:     style,
		Color:   prtable.ColorOptionsDefault,
		Format:  prtable.FormatOptions{},
		HTML:    prtable.DefaultHTMLOptions,
		Options: prtable.OptionsNoBordersAndSeparators,
		Title:   prtable.TitleOptionsDefault,
	})
}

func setEventTableStyle(table prtable.Writer, tableWidth int) {
	style := prtable.StyleBoxDefault
	style.PaddingLeft = ""
	style.PaddingRight = ""

	columnConfigs := []prtable.ColumnConfig{
		{
			Number: 1,
		},
	}

	paddingsWidth := len(columnConfigs) * (len(style.PaddingLeft) + len(style.PaddingRight))
	columnsWidth := tableWidth - paddingsWidth

	columnConfigs[0].WidthMax = columnsWidth

	table.SetColumnConfigs(columnConfigs)
	table.SetStyle(prtable.Style{
		Box:     style,
		Color:   prtable.ColorOptionsDefault,
		Format:  prtable.FormatOptions{},
		HTML:    prtable.DefaultHTMLOptions,
		Options: prtable.OptionsNoBordersAndSeparators,
		Title:   prtable.TitleOptionsDefault,
	})
}

func buildLogsHeader(resourceLogs *logstore.ResourceLogs, source, defaultNamespace string, colorize bool) string {
	result := "Logs for " + resourceLogs.GroupVersionKind().Kind + "/" + resourceLogs.Name() + ", " + source

	if resourceLogs.Namespace() != defaultNamespace {
		result += ", namespace: " + resourceLogs.Namespace()
	}

	if colorize {
		result = color.New(color.Bold, color.Blue).Sprintf(result)
	}

	return result
}

func buildEventsHeader(resourceState *statestore.ResourceState, defaultNamespace string, colorize bool) string {
	result := "Events for " + resourceState.GroupVersionKind().Kind + "/" + resourceState.Name()

	if resourceState.Namespace() != defaultNamespace {
		result += ", namespace: " + resourceState.Namespace()
	}

	if colorize {
		result = color.New(color.Bold, color.Blue).Sprintf(result)
	}

	return result
}

func buildReadinessRootResourceStateCell(taskState *statestore.ReadinessTaskState, colorize bool) string {
	var stateCell string

	switch status := taskState.Status(); status {
	case statestore.ReadinessTaskStatusReady:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Green).Sprintf(stateCell)
		}
	case statestore.ReadinessTaskStatusProgressing:
		stateCell = "WAITING"
		if colorize {
			stateCell = color.New(color.Yellow).Sprintf(stateCell)
		}
	case statestore.ReadinessTaskStatusFailed:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Red).Sprintf(stateCell)
		}
	default:
		panic("unexpected task status")
	}

	return stateCell
}

func buildReadinessChildResourceStateCell(resourceState *statestore.ResourceState, colorize bool) string {
	var stateCell string

	switch status := resourceState.Status(); status {
	case statestore.ResourceStatusReady:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Green).Sprintf(stateCell)
		}
	case statestore.ResourceStatusCreated, statestore.ResourceStatusDeleted:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Yellow).Sprintf(stateCell)
		}
	case statestore.ResourceStatusFailed:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Red).Sprintf(stateCell)
		}
	default:
		panic("unexpected resource status")
	}

	return stateCell
}

func buildRootResourceCell(resourceState *statestore.ResourceState, colorize bool) string {
	var resourceCell string

	kind := resourceState.GroupVersionKind().Kind
	if colorize {
		kind = color.New(color.Cyan).Sprintf(kind)
	}

	resourceCell = fmt.Sprintf("%s/%s", kind, resourceState.Name())

	return resourceCell
}

func buildChildResourceCell(resourceState *statestore.ResourceState, colorize bool) string {
	return " • " + buildRootResourceCell(resourceState, colorize)
}

func buildReadyPodsInfo(resourceState *statestore.ResourceState, readyPods int) string {
	var info string
	if attr, found := lo.Find(resourceState.Attributes(), func(attr statestore.Attributer) bool {
		return attr.Name() == statestore.AttributeNameRequiredReplicas
	}); found {
		requiredReadyPods := attr.(*statestore.Attribute[int]).Value
		info = fmt.Sprintf("Ready:%d/%d", readyPods, requiredReadyPods)
	}

	return info
}

func buildStatusInfo(resourceState *statestore.ResourceState) string {
	var info string
	if attr, found := lo.Find(resourceState.Attributes(), func(attr statestore.Attributer) bool {
		return attr.Name() == statestore.AttributeNameStatus
	}); found {
		status := attr.(*statestore.Attribute[string]).Value
		info = fmt.Sprintf("Status:%s", status)
	}

	return info
}

func buildNamespaceInfo(resourceState *statestore.ResourceState) string {
	return fmt.Sprintf("Namespace:%s", resourceState.Namespace())
}

func buildGenericConditionInfo(resourceState *statestore.ResourceState) string {
	var condition, current string
	if attr, found := lo.Find(resourceState.Attributes(), func(attr statestore.Attributer) bool {
		return attr.Name() == statestore.AttributeNameConditionTarget
	}); found {
		condition = attr.(*statestore.Attribute[string]).Value

		if attr, found := lo.Find(resourceState.Attributes(), func(attr statestore.Attributer) bool {
			return attr.Name() == statestore.AttributeNameConditionCurrentValue
		}); found {
			current = attr.(*statestore.Attribute[string]).Value
		}
	}

	if condition == "" {
		return ""
	} else if current == "" {
		return fmt.Sprintf("Tracking:%q", condition)
	} else {
		return fmt.Sprintf(`Tracking:"%s=%s"`, condition, current)
	}
}

func buildErrorsInfo(resourceState *statestore.ResourceState) string {
	var errsCount int
	for _, errs := range resourceState.Errors() {
		errsCount += len(errs)
	}

	return fmt.Sprintf("Errors:%d", errsCount)
}

func buildLastErrInfo(resourceState *statestore.ResourceState, colorize bool) string {
	var lastErr *statestore.Error
	for _, errs := range resourceState.Errors() {
		for _, err := range errs {
			if lastErr == nil {
				lastErr = err
				continue
			}

			if err.Time.After(lastErr.Time) {
				lastErr = err
			}
		}
	}

	errInfo := fmt.Sprintf("LastError:%q", lastErr.Err.Error())
	if colorize {
		errInfo = color.New(color.Red).Sprintf(errInfo)
	}

	return errInfo
}

func buildPresenceRootResourceStateCell(taskState *statestore.PresenceTaskState, colorize bool) string {
	var stateCell string

	switch status := taskState.Status(); status {
	case statestore.PresenceTaskStatusPresent:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Green).Sprintf(stateCell)
		}
	case statestore.PresenceTaskStatusProgressing:
		stateCell = "WAITING"
		if colorize {
			stateCell = color.New(color.Yellow).Sprintf(stateCell)
		}
	case statestore.PresenceTaskStatusFailed:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Red).Sprintf(stateCell)
		}
	default:
		panic("unexpected task status")
	}

	return stateCell
}

func buildAbsenceRootResourceStateCell(taskState *statestore.AbsenceTaskState, colorize bool) string {
	var stateCell string

	switch status := taskState.Status(); status {
	case statestore.AbsenceTaskStatusAbsent:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Green).Sprintf(stateCell)
		}
	case statestore.AbsenceTaskStatusProgressing:
		stateCell = "WAITING"
		if colorize {
			stateCell = color.New(color.Yellow).Sprintf(stateCell)
		}
	case statestore.AbsenceTaskStatusFailed:
		stateCell = caps.ToUpper(string(status))
		if colorize {
			stateCell = color.New(color.Red).Sprintf(stateCell)
		}
	default:
		panic("unexpected task status")
	}

	return stateCell
}

func calculateReadyPods(rts *statestore.ReadinessTaskState) *int {
	var readyPods *int
	for _, crs := range rts.ResourceStates() {
		crs.RTransaction(func(rs *statestore.ResourceState) {
			if rs.GroupVersionKind().GroupKind() == (schema.GroupKind{Group: "", Kind: "Pod"}) {
				if readyPods == nil {
					readyPods = new(int)
				}

				if rs.Status() == statestore.ResourceStatusReady {
					*readyPods++
				}
			}
		})
	}

	return readyPods
}

func sortReadinessTaskStates(taskStates []*kdutil.Concurrent[*statestore.ReadinessTaskState]) {
	sort.Slice(taskStates, func(i, j int) bool {
		var less bool

		taskStates[i].RTransaction(func(irts *statestore.ReadinessTaskState) {
			taskStates[j].RTransaction(func(jrts *statestore.ReadinessTaskState) {
				iResourceStatesLen := len(irts.ResourceStates())
				jResourceStatesLen := len(jrts.ResourceStates())
				if iResourceStatesLen > jResourceStatesLen {
					less = true
					return
				} else if iResourceStatesLen < jResourceStatesLen {
					return
				}

				less = compareKindNameNamespace(
					irts.Name(),
					irts.Namespace(),
					irts.GroupVersionKind().Kind,
					jrts.Name(),
					jrts.Namespace(),
					jrts.GroupVersionKind().Kind,
				)
			})
		})

		return less
	})
}

func sortPresenceTaskStates(taskStates []*kdutil.Concurrent[*statestore.PresenceTaskState]) {
	sort.Slice(taskStates, func(i, j int) bool {
		var less bool

		taskStates[i].RTransaction(func(ipts *statestore.PresenceTaskState) {
			taskStates[j].RTransaction(func(jpts *statestore.PresenceTaskState) {
				less = compareKindNameNamespace(
					ipts.Name(),
					ipts.Namespace(),
					ipts.GroupVersionKind().Kind,
					jpts.Name(),
					jpts.Namespace(),
					jpts.GroupVersionKind().Kind,
				)
			})
		})

		return less
	})
}

func sortAbsenceTaskStates(taskStates []*kdutil.Concurrent[*statestore.AbsenceTaskState]) {
	sort.Slice(taskStates, func(i, j int) bool {
		var less bool

		taskStates[i].RTransaction(func(iats *statestore.AbsenceTaskState) {
			taskStates[j].RTransaction(func(jats *statestore.AbsenceTaskState) {
				less = compareKindNameNamespace(
					iats.Name(),
					iats.Namespace(),
					iats.GroupVersionKind().Kind,
					jats.Name(),
					jats.Namespace(),
					jats.GroupVersionKind().Kind,
				)
			})
		})

		return less
	})
}

func compareKindNameNamespace(iName, iNamespace, iKind, jName, jNamespace, jKind string) bool {
	if iKind < jKind {
		return true
	} else if iKind > jKind {
		return false
	}

	if iNamespace < jNamespace {
		return true
	} else if iNamespace > jNamespace {
		return false
	}

	if iName < jName {
		return true
	} else if iName > jName {
		return false
	}

	if iNamespace < jNamespace {
		return true
	} else if iNamespace > jNamespace {
		return false
	}

	return false
}
