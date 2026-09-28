// Package tui provides QIX's full-screen terminal interface.
package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mrbooshehri/qix-go/internal/models"
	"github.com/mrbooshehri/qix-go/internal/storage"
)

const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	cyan   = "\x1b[38;5;44m"
	green  = "\x1b[38;5;42m"
	red    = "\x1b[38;5;203m"
	yellow = "\x1b[38;5;220m"
)

type keyEvent struct {
	name string
	r    rune
	text []rune
}

type taskItem struct {
	task     models.Task
	location string
}

type inputField struct {
	label    string
	value    []rune
	required bool
}

type inputForm struct {
	kind   string
	title  string
	fields []inputField
	index  int
}

type confirmation struct {
	kind     string
	prompt   string
	expected string
	input    []rune
	taskIDs  []string
}

type app struct {
	store            *storage.Storage
	projects         []string
	projectIndex     int
	project          *models.Project
	moduleIndex      int
	tasks            []taskItem
	taskIndex        int
	focus            int
	form             *inputForm
	confirmation     *confirmation
	message          string
	isError          bool
	showHelp         bool
	width            int
	height           int
	section          int
	sprintIndex      int
	reportIndex      int
	backupIndex      int
	detailScroll     int
	sectionScroll    int
	selectedTasks    map[string]bool
	selectionProject string
}

type tickMsg time.Time

const (
	sectionWorkspace = iota
	sectionTracking
	sectionSprints
	sectionReports
	sectionHealth
	sectionSettings
)

// Run starts the full-screen QIX interface. initialProject may be empty.
func Run(store *storage.Storage, initialProject string) error {
	a := &app{store: store, moduleIndex: -1, width: 100, height: 30, selectedTasks: make(map[string]bool)}
	if err := a.loadProjects(initialProject); err != nil {
		return err
	}
	_, err := tea.NewProgram(a, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func (a *app) Init() tea.Cmd { return tickCmd() }

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return tickMsg(now) })
}

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return a, tickCmd()
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return a, nil
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress || (msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown) {
			return a, nil
		}
		if a.form == nil && a.confirmation == nil && a.section == sectionWorkspace {
			a.focus = a.focusAt(msg.X, msg.Y)
		}
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		key := keyEvent{name: "down"}
		if delta < 0 {
			key.name = "up"
		}
		steps := delta
		if steps < 0 {
			steps = -steps
		}
		for i := 0; i < max(1, steps); i++ {
			_, err := a.updateKey(key)
			if err != nil {
				a.message, a.isError = err.Error(), true
				break
			}
		}
		return a, nil
	case tea.KeyMsg:
		key := bubbleKey(msg)
		quit, err := a.updateKey(key)
		if err != nil {
			a.message, a.isError = err.Error(), true
		}
		if quit {
			return a, tea.Quit
		}
	}
	return a, nil
}

func (a *app) View() string { return a.view(a.width, a.height) }

func bubbleKey(msg tea.KeyMsg) keyEvent {
	switch msg.Type {
	case tea.KeyCtrlC:
		return keyEvent{name: "ctrl-c"}
	case tea.KeyCtrlG:
		return keyEvent{name: "cancel"}
	case tea.KeyCtrlS:
		return keyEvent{name: "save"}
	case tea.KeyEsc:
		return keyEvent{name: "esc"}
	case tea.KeyTab:
		return keyEvent{name: "tab"}
	case tea.KeyShiftTab:
		return keyEvent{name: "shift-tab"}
	case tea.KeyEnter:
		return keyEvent{name: "enter"}
	case tea.KeyBackspace, tea.KeyDelete:
		return keyEvent{name: "backspace"}
	case tea.KeyUp:
		return keyEvent{name: "up"}
	case tea.KeyDown:
		return keyEvent{name: "down"}
	case tea.KeyLeft:
		return keyEvent{name: "left"}
	case tea.KeyRight:
		return keyEvent{name: "right"}
	case tea.KeyPgUp:
		return keyEvent{name: "page-up"}
	case tea.KeyPgDown:
		return keyEvent{name: "page-down"}
	case tea.KeyHome:
		return keyEvent{name: "home"}
	case tea.KeyEnd:
		return keyEvent{name: "end"}
	case tea.KeySpace:
		return keyEvent{name: "space", r: ' '}
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			return keyEvent{r: msg.Runes[0], text: msg.Runes}
		}
	}
	return keyEvent{}
}

func (a *app) focusAt(x, y int) int {
	contentHeight := max(1, a.height-4)
	projectWidth := clamp(a.width/4, 24, 34)
	projectHeight := max(7, contentHeight/2)
	taskHeight := max(9, contentHeight*3/5)
	if x < projectWidth {
		if y < 2+projectHeight {
			return 0
		}
		return 1
	}
	if y < 2+taskHeight {
		return 2
	}
	return 3
}

func (a *app) loadProjects(selectName string) error {
	projects, err := a.store.ListProjects()
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	sort.Strings(projects)
	a.projects = projects
	if len(projects) == 0 {
		a.project, a.tasks, a.projectIndex, a.taskIndex, a.moduleIndex = nil, nil, 0, 0, -1
		return nil
	}
	if a.projectIndex >= len(projects) {
		a.projectIndex = len(projects) - 1
	}
	for i, name := range projects {
		if name == selectName {
			a.projectIndex = i
			break
		}
	}
	return a.loadProject()
}

func (a *app) loadProject() error {
	if len(a.projects) == 0 {
		return nil
	}
	project, err := a.store.LoadProject(a.projects[a.projectIndex])
	if err != nil {
		return fmt.Errorf("load project %q: %w", a.projects[a.projectIndex], err)
	}
	a.project = project
	if a.selectionProject != project.Name {
		a.selectedTasks = make(map[string]bool)
		a.selectionProject = project.Name
	} else {
		a.pruneTaskSelection()
	}
	if a.moduleIndex >= len(project.Modules) {
		a.moduleIndex = len(project.Modules) - 1
	}
	return a.loadTasks()
}

func (a *app) loadTasks() error {
	a.tasks = a.tasks[:0]
	if a.project == nil {
		return nil
	}
	if a.moduleIndex < 0 {
		for _, task := range a.project.Tasks {
			a.tasks = append(a.tasks, taskItem{task: task, location: "project"})
		}
	} else {
		module := a.project.Modules[a.moduleIndex]
		for _, task := range module.Tasks {
			a.tasks = append(a.tasks, taskItem{task: task, location: module.Name})
		}
	}
	if a.taskIndex >= len(a.tasks) {
		a.taskIndex = max(0, len(a.tasks)-1)
	}
	return nil
}

func (a *app) updateKey(key keyEvent) (bool, error) {
	if key.name == "ctrl-c" {
		return true, nil
	}
	if a.form != nil {
		a.message, a.isError = "", false
		return false, a.updateForm(key)
	}
	if a.confirmation != nil {
		a.message, a.isError = "", false
		return false, a.updateConfirmation(key)
	}
	switch key.r {
	case 'W':
		a.section = sectionWorkspace
		a.sectionScroll = 0
		return false, nil
	case 'T':
		a.section = sectionTracking
		a.sectionScroll = 0
		return false, nil
	case 'S':
		a.section = sectionSprints
		a.sectionScroll = 0
		return false, nil
	case 'R':
		a.section = sectionReports
		a.sectionScroll = 0
		return false, nil
	case 'H':
		a.section = sectionHealth
		a.sectionScroll = 0
		return false, nil
	case 'G':
		a.section = sectionSettings
		a.sectionScroll = 0
		return false, nil
	}
	if key.name == "esc" && a.showHelp {
		a.showHelp = false
		return false, nil
	}
	if key.name == "esc" && a.selectedTaskCount() > 0 {
		a.clearTaskSelection()
		a.message = "Task selection cleared"
		return false, nil
	}
	if key.r == 'q' {
		return true, nil
	}
	if key.r == '?' {
		a.showHelp = !a.showHelp
		return false, nil
	}
	if a.section != sectionWorkspace {
		a.message, a.isError = "", false
		return false, a.updateSection(key)
	}
	a.message, a.isError = "", false
	switch {
	case key.name == "tab":
		a.focus = (a.focus + 1) % 4
		a.detailScroll = 0
	case key.name == "left":
		a.focus = max(0, a.focus-1)
	case key.name == "right":
		a.focus = min(3, a.focus+1)
	case key.name == "enter" && a.focus < 3:
		a.focus++
	case key.name == "up" || key.r == 'k':
		return false, a.move(-1)
	case key.name == "down" || key.r == 'j':
		return false, a.move(1)
	case key.name == "page-up":
		return false, a.move(-a.pageSize())
	case key.name == "page-down":
		return false, a.move(a.pageSize())
	case key.name == "home":
		return false, a.moveToBoundary(false)
	case key.name == "end":
		return false, a.moveToBoundary(true)
	case key.r == 'r':
		name := ""
		if len(a.projects) > 0 {
			name = a.projects[a.projectIndex]
			a.store.InvalidateCache(name)
		}
		if err := a.loadProjects(name); err != nil {
			return false, err
		}
		a.message = "Data refreshed"
	case key.r == 'p':
		a.startProjectForm()
	case key.r == 'm':
		if a.project == nil {
			return false, fmt.Errorf("create a project first with p")
		}
		a.startModuleForm()
	case key.r == 'e' && a.focus == 1:
		if a.project == nil || a.moduleIndex < 0 {
			return false, fmt.Errorf("select a module to edit")
		}
		a.startModuleEditForm()
	case key.r == 'e' && a.focus == 0:
		if a.project == nil {
			return false, fmt.Errorf("select a project to edit")
		}
		a.startProjectEditForm()
	case key.r == 'e' && a.focus == 2:
		if len(a.tasks) == 0 {
			return false, fmt.Errorf("select a task to edit")
		}
		a.startTaskEditForm()
	case key.r == 'n':
		if a.project == nil {
			return false, fmt.Errorf("create a project first with p")
		}
		a.startTaskForm()
	case key.r == 'd' && a.focus == 0:
		if a.project == nil {
			return false, nil
		}
		a.confirmation = &confirmation{
			kind:     "delete-project",
			prompt:   fmt.Sprintf("Delete %s and all its data? Type the project name", a.project.Name),
			expected: a.project.Name,
		}
	case key.r == 'd' && a.focus == 1:
		if a.project == nil || a.moduleIndex < 0 {
			return false, fmt.Errorf("select a module to remove")
		}
		module := a.project.Modules[a.moduleIndex]
		a.confirmation = &confirmation{
			kind:     "delete-module",
			prompt:   fmt.Sprintf("Remove %s and its %d task(s)? Type the module name", module.Name, len(module.Tasks)),
			expected: module.Name,
		}
	case key.r == 'd' && a.focus == 2:
		if len(a.tasks) == 0 {
			return false, nil
		}
		taskIDs := a.actionTaskIDs()
		prompt := fmt.Sprintf("Delete %s [%s]?", a.tasks[a.taskIndex].task.Title, a.tasks[a.taskIndex].task.ID)
		if len(taskIDs) > 1 || a.selectedTaskCount() > 0 {
			prompt = fmt.Sprintf("Delete %d selected task(s)?", len(taskIDs))
		}
		a.confirmation = &confirmation{
			kind:    "delete-task",
			prompt:  prompt + " Parent, dependency, and sprint references will be cleaned.",
			taskIDs: taskIDs,
		}
	case key.r == 'l' && a.focus == 2:
		return false, a.startTaskRelationForm("parent")
	case key.r == 'y' && a.focus == 2:
		return false, a.startTaskRelationForm("dependency")
	case key.r == 'c' && a.focus == 2:
		return false, a.startRecurrenceForm()
	case key.r == 'u' && a.focus == 2:
		return false, a.removeRecurrence()
	case key.r == 'C' && a.focus == 2:
		return false, a.completeTask()
	case key.r == 't' && a.focus == 2:
		return false, a.startTimeLogForm()
	case key.r == 'o' && a.focus == 2:
		return false, a.openSelectedJira()
	case key.name == "space" && a.focus == 2:
		return false, a.toggleTaskSelection()
	case key.r == 'a' && a.focus == 2:
		return false, a.toggleAllVisibleTasks()
	case key.r == 'x':
		return false, a.cycleStatus()
	case key.r >= '1' && key.r <= '4':
		statuses := []models.TaskStatus{models.StatusTodo, models.StatusDoing, models.StatusDone, models.StatusBlocked}
		return false, a.setStatus(statuses[int(key.r-'1')])
	}
	return false, nil
}

func (a *app) startProjectForm() {
	a.form = &inputForm{
		kind:  "project",
		title: "Create project",
		fields: []inputField{
			{label: "Name", required: true},
			{label: "Description"},
			{label: "Tags (comma-separated)"},
		},
	}
}

func (a *app) startProjectEditForm() {
	a.form = &inputForm{
		kind:  "edit-project",
		title: "Edit project " + a.project.Name,
		fields: []inputField{
			{label: "Name", value: []rune(a.project.Name), required: true},
			{label: "Description", value: []rune(a.project.Description)},
			{label: "Tags (comma-separated)", value: []rune(strings.Join(a.project.Tags, ", "))},
		},
	}
}

func (a *app) startModuleForm() {
	a.form = &inputForm{
		kind:  "module",
		title: "Create module in " + a.project.Name,
		fields: []inputField{
			{label: "Name", required: true},
			{label: "Description"},
			{label: "Tags (comma-separated)"},
		},
	}
}

func (a *app) startModuleEditForm() {
	module := a.project.Modules[a.moduleIndex]
	a.form = &inputForm{
		kind:  "edit-module",
		title: "Edit module " + module.Name,
		fields: []inputField{
			{label: "Name", value: []rune(module.Name), required: true},
			{label: "Description", value: []rune(module.Description)},
			{label: "Tags (comma-separated)", value: []rune(strings.Join(module.Tags, ", "))},
		},
	}
}

func (a *app) startTaskForm() {
	location := "project level"
	if a.moduleIndex >= 0 {
		location = a.project.Modules[a.moduleIndex].Name
	}
	a.form = &inputForm{
		kind:  "task",
		title: "Create task in " + location,
		fields: []inputField{
			{label: "Title", required: true},
			{label: "Description"},
			{label: "Status (todo/doing/done/blocked)", value: []rune("todo"), required: true},
			{label: "Priority (low/medium/high)", value: []rune("medium"), required: true},
			{label: "Estimated hours", value: []rune("0"), required: true},
			{label: "Tags (comma-separated)"},
			{label: "Jira issue"},
		},
	}
}

func (a *app) startTaskEditForm() {
	task := a.tasks[a.taskIndex].task
	a.form = &inputForm{
		kind:  "edit-task",
		title: "Edit task " + task.ID,
		fields: []inputField{
			{label: "Title", value: []rune(task.Title), required: true},
			{label: "Description", value: []rune(task.Description)},
			{label: "Status (todo/doing/done/blocked)", value: []rune(task.Status), required: true},
			{label: "Priority (low/medium/high)", value: []rune(task.Priority), required: true},
			{label: "Estimated hours", value: []rune(strconv.FormatFloat(task.EstimatedHours, 'f', -1, 64)), required: true},
			{label: "Tags (comma-separated)", value: []rune(strings.Join(task.Tags, ", "))},
			{label: "Jira issue", value: []rune(task.JiraIssue)},
		},
	}
}

func (a *app) startTaskRelationForm(relation string) error {
	if len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	label := "Parent task ID"
	kind := "task-parent"
	if relation == "dependency" {
		label, kind = "Dependency task ID", "task-dependency"
	}
	a.form = &inputForm{kind: kind, title: "Set " + relation + " for " + a.tasks[a.taskIndex].task.ID, fields: []inputField{{label: label, required: true}}}
	return nil
}

func (a *app) startRecurrenceForm() error {
	if len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	a.form = &inputForm{kind: "task-recurrence", title: "Schedule recurring task", fields: []inputField{{label: "Pattern (daily, weekly:day, monthly:day, interval:days)", required: true}}}
	return nil
}

func (a *app) startTimeLogForm() error {
	if len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	a.form = &inputForm{kind: "task-time", title: "Log time for " + a.tasks[a.taskIndex].task.ID, fields: []inputField{{label: "Hours", required: true}, {label: "Date (YYYY-MM-DD)", value: []rune(time.Now().Format("2006-01-02")), required: true}}}
	return nil
}

func (a *app) removeRecurrence() error {
	if len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	task := a.tasks[a.taskIndex].task
	if err := a.store.RemoveTaskRecurrence(a.project.Name, task.ID); err != nil {
		return err
	}
	if err := a.loadProject(); err != nil {
		return err
	}
	a.message = "Removed recurrence from " + task.ID
	return nil
}

func (a *app) completeTask() error {
	if len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	task := a.tasks[a.taskIndex].task
	err := a.store.UpdateTask(a.project.Name, task.ID, func(current *models.Task) error {
		current.Status = models.StatusDone
		if current.Recurrence != nil && current.Recurrence.Enabled {
			pattern := string(current.Recurrence.Type)
			if current.Recurrence.Value != "" {
				pattern += ":" + current.Recurrence.Value
			}
			next, err := parseRecurrence(pattern, time.Now())
			if err != nil {
				return err
			}
			next.LastCompleted = time.Now().Format("2006-01-02")
			current.Recurrence = &next
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := a.loadProject(); err != nil {
		return err
	}
	a.message = "Completed task " + task.ID
	return nil
}

func (a *app) updateForm(key keyEvent) error {
	field := &a.form.fields[a.form.index]
	switch key.name {
	case "esc", "cancel":
		a.form = nil
	case "tab", "down":
		a.form.index = min(len(a.form.fields)-1, a.form.index+1)
	case "shift-tab", "up":
		a.form.index = max(0, a.form.index-1)
	case "backspace":
		if len(field.value) > 0 {
			field.value = field.value[:len(field.value)-1]
		}
	case "enter":
		if field.required && strings.TrimSpace(string(field.value)) == "" {
			return fmt.Errorf("%s cannot be empty", strings.ToLower(field.label))
		}
		if a.form.index < len(a.form.fields)-1 {
			a.form.index++
			return nil
		}
		return a.submitForm()
	case "save":
		for _, candidate := range a.form.fields {
			if candidate.required && strings.TrimSpace(string(candidate.value)) == "" {
				return fmt.Errorf("%s cannot be empty", strings.ToLower(candidate.label))
			}
		}
		return a.submitForm()
	default:
		if len(key.text) > 0 {
			field.value = append(field.value, key.text...)
		} else if key.r >= 32 && utf8.RuneLen(key.r) > 0 {
			field.value = append(field.value, key.r)
		}
	}
	return nil
}

func (a *app) submitForm() error {
	form := a.form
	if form.kind == "settings" {
		return a.submitSettings(form)
	}
	if form.kind == "project" {
		name := strings.TrimSpace(string(form.fields[0].value))
		description := strings.TrimSpace(string(form.fields[1].value))
		tags := splitCommaSeparated(string(form.fields[2].value))
		if _, err := a.store.CreateProject(name, description, tags); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProjects(name); err != nil {
			return err
		}
		a.focus = 0
		a.message = "Created project " + name
		return nil
	}
	if form.kind == "edit-project" {
		oldName := a.project.Name
		newName := strings.TrimSpace(string(form.fields[0].value))
		description := strings.TrimSpace(string(form.fields[1].value))
		tags := splitCommaSeparated(string(form.fields[2].value))
		if newName != oldName {
			if err := a.store.RenameProject(oldName, newName); err != nil {
				return err
			}
		}
		if err := a.store.UpdateProject(newName, func(project *models.Project) error {
			project.Description = description
			project.Tags = tags
			return nil
		}); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProjects(newName); err != nil {
			return err
		}
		a.message = "Updated project " + newName
		return nil
	}
	if form.kind == "module" {
		name := strings.TrimSpace(string(form.fields[0].value))
		description := strings.TrimSpace(string(form.fields[1].value))
		tags := splitCommaSeparated(string(form.fields[2].value))
		module := models.Module{Name: name, Description: description, Tags: tags}
		if err := a.store.AddModule(a.project.Name, module); err != nil {
			return err
		}
		a.form = nil
		a.moduleIndex = len(a.project.Modules)
		if err := a.loadProject(); err != nil {
			return err
		}
		a.focus = 1
		a.message = "Created module " + name
		return nil
	}
	if form.kind == "edit-module" {
		oldName := a.project.Modules[a.moduleIndex].Name
		newName := strings.TrimSpace(string(form.fields[0].value))
		description := strings.TrimSpace(string(form.fields[1].value))
		tags := splitCommaSeparated(string(form.fields[2].value))
		if newName != oldName {
			for _, module := range a.project.Modules {
				if module.Name == newName {
					return fmt.Errorf("module '%s' already exists", newName)
				}
			}
		}
		if err := a.store.UpdateModule(a.project.Name, oldName, func(module *models.Module) error {
			module.Name = newName
			module.Description = description
			module.Tags = tags
			return nil
		}); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		a.focus = 1
		a.message = "Updated module " + oldName
		if newName != oldName {
			a.message = fmt.Sprintf("Renamed module %s to %s", oldName, newName)
		}
		return nil
	}
	if form.kind == "sprint" || form.kind == "edit-sprint" {
		name := strings.TrimSpace(string(form.fields[0].value))
		startDate := strings.TrimSpace(string(form.fields[1].value))
		endDate := strings.TrimSpace(string(form.fields[2].value))
		start, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			return fmt.Errorf("start date must use YYYY-MM-DD")
		}
		end, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			return fmt.Errorf("end date must use YYYY-MM-DD")
		}
		if end.Before(start) {
			return fmt.Errorf("end date must not be before start date")
		}
		if form.kind == "sprint" {
			if err := a.store.AddSprint(a.project.Name, models.Sprint{Name: name, StartDate: startDate, EndDate: endDate}); err != nil {
				return err
			}
		} else {
			oldName := a.project.Sprints[a.sprintIndex].Name
			if err := a.store.UpdateProject(a.project.Name, func(project *models.Project) error {
				for _, candidate := range project.Sprints {
					if candidate.Name == name && candidate.Name != oldName {
						return fmt.Errorf("sprint '%s' already exists", name)
					}
				}
				for i := range project.Sprints {
					if project.Sprints[i].Name == oldName {
						project.Sprints[i].Name = name
						project.Sprints[i].StartDate = startDate
						project.Sprints[i].EndDate = endDate
						return nil
					}
				}
				return fmt.Errorf("sprint '%s' not found", oldName)
			}); err != nil {
				return err
			}
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		if form.kind == "sprint" {
			a.sprintIndex = len(a.project.Sprints) - 1
			a.message = "Created sprint " + name
		} else {
			a.message = "Updated sprint " + name
		}
		return nil
	}
	if form.kind == "backup-export" {
		path := strings.TrimSpace(string(form.fields[0].value))
		if path == "" {
			return fmt.Errorf("export path cannot be empty")
		}
		if err := a.store.ExportBackup(path); err != nil {
			return err
		}
		a.form = nil
		a.message = "Exported backup to " + path
		return nil
	}

	if form.kind == "task-parent" || form.kind == "task-dependency" {
		task := a.tasks[a.taskIndex].task
		otherID := strings.TrimSpace(string(form.fields[0].value))
		var err error
		if form.kind == "task-parent" {
			err = a.store.LinkTaskAsChild(a.project.Name, task.ID, otherID)
		} else {
			err = a.store.AddTaskDependency(a.project.Name, task.ID, otherID)
		}
		if err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = "Updated relationships for " + task.ID
		return nil
	}
	if form.kind == "task-recurrence" {
		task := a.tasks[a.taskIndex].task
		recurrence, err := parseRecurrence(strings.TrimSpace(string(form.fields[0].value)), time.Now())
		if err != nil {
			return err
		}
		if err := a.store.SetTaskRecurrence(a.project.Name, task.ID, recurrence); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = fmt.Sprintf("Scheduled %s; next due %s", task.ID, recurrence.NextDue)
		return nil
	}
	if form.kind == "task-time" {
		task := a.tasks[a.taskIndex].task
		hours, err := strconv.ParseFloat(strings.TrimSpace(string(form.fields[0].value)), 64)
		if err != nil || hours <= 0 {
			return fmt.Errorf("hours must be a positive number")
		}
		date := strings.TrimSpace(string(form.fields[1].value))
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return fmt.Errorf("date must use YYYY-MM-DD")
		}
		if err := a.store.AddTimeEntry(a.project.Name, task.ID, models.TimeEntry{Date: date, Hours: hours}); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = fmt.Sprintf("Logged %.2fh to %s", hours, task.ID)
		return nil
	}

	title := strings.TrimSpace(string(form.fields[0].value))
	description := strings.TrimSpace(string(form.fields[1].value))
	status, err := parseTaskStatus(string(form.fields[2].value))
	if err != nil {
		return err
	}
	priority, err := parsePriority(string(form.fields[3].value))
	if err != nil {
		return err
	}
	estimated, err := strconv.ParseFloat(strings.TrimSpace(string(form.fields[4].value)), 64)
	if err != nil || estimated < 0 {
		return fmt.Errorf("estimated hours must be zero or a positive number")
	}
	tags := splitCommaSeparated(string(form.fields[5].value))
	jira := strings.TrimSpace(string(form.fields[6].value))
	if form.kind == "edit-task" {
		taskID := a.tasks[a.taskIndex].task.ID
		if err := a.store.UpdateTask(a.project.Name, taskID, func(task *models.Task) error {
			task.Title = title
			task.Description = description
			task.Status = status
			task.Priority = priority
			task.EstimatedHours = estimated
			task.Tags = tags
			task.JiraIssue = jira
			return nil
		}); err != nil {
			return err
		}
		a.form = nil
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = "Updated task " + taskID
		return nil
	}
	moduleName := ""
	if a.moduleIndex >= 0 {
		moduleName = a.project.Modules[a.moduleIndex].Name
	}
	if err := a.store.AddTask(a.projects[a.projectIndex], moduleName, models.Task{
		Title: title, Description: description, Status: status, Priority: priority,
		EstimatedHours: estimated, Tags: tags, JiraIssue: jira,
	}); err != nil {
		return err
	}
	a.form = nil
	if err := a.loadProject(); err != nil {
		return err
	}
	a.taskIndex = len(a.tasks) - 1
	a.focus = 2
	a.message = "Created task " + title
	return nil
}

func (a *app) updateConfirmation(key keyEvent) error {
	confirm := a.confirmation
	switch key.name {
	case "esc", "cancel":
		a.confirmation = nil
		a.message = "Deletion cancelled"
	case "backspace":
		if len(confirm.input) > 0 {
			confirm.input = confirm.input[:len(confirm.input)-1]
		}
	case "enter":
		if confirm.expected != "" && strings.TrimSpace(string(confirm.input)) != confirm.expected {
			return fmt.Errorf("confirmation does not match %q", confirm.expected)
		}
		if confirm.kind == "delete-project" {
			name := confirm.expected
			if err := a.store.DeleteProject(name); err != nil {
				return err
			}
			a.confirmation = nil
			if err := a.loadProjects(""); err != nil {
				return err
			}
			a.focus = 0
			a.message = "Deleted project " + name
		}
		if confirm.kind == "delete-module" {
			projectName := a.project.Name
			moduleName := confirm.expected
			if err := a.store.RemoveModule(projectName, moduleName); err != nil {
				return err
			}
			a.confirmation = nil
			a.moduleIndex = -1
			a.taskIndex = 0
			if err := a.loadProject(); err != nil {
				return err
			}
			a.focus = 1
			a.message = "Removed module " + moduleName
		}
		if confirm.kind == "delete-task" {
			count, err := a.store.RemoveTasks(a.project.Name, confirm.taskIDs)
			if err != nil {
				return err
			}
			a.confirmation = nil
			a.clearTaskSelection()
			if err := a.loadProject(); err != nil {
				return err
			}
			a.focus = 2
			a.message = fmt.Sprintf("Removed %d task(s)", count)
		}
		if confirm.kind == "delete-sprint" {
			name := confirm.expected
			if err := a.store.UpdateProject(a.project.Name, func(project *models.Project) error {
				for i := range project.Sprints {
					if project.Sprints[i].Name == name {
						project.Sprints = append(project.Sprints[:i], project.Sprints[i+1:]...)
						return nil
					}
				}
				return fmt.Errorf("sprint %s not found", name)
			}); err != nil {
				return err
			}
			a.confirmation = nil
			if err := a.loadProject(); err != nil {
				return err
			}
			a.sprintIndex = min(a.sprintIndex, max(0, len(a.project.Sprints)-1))
			a.message = "Removed sprint " + name
		}
		if confirm.kind == "restore-backup" {
			backups, err := a.store.ListBackups()
			if err != nil {
				return err
			}
			var path string
			for _, backup := range backups {
				if backup.Name == confirm.expected {
					path = backup.Path
					break
				}
			}
			if path == "" {
				return fmt.Errorf("backup %s not found", confirm.expected)
			}
			safety, err := a.store.RestoreBackup(path)
			if err != nil {
				return fmt.Errorf("restore failed (safety backup: %s): %w", safety, err)
			}
			a.confirmation = nil
			if err := a.loadProjects(""); err != nil {
				return err
			}
			a.message = "Restored " + confirm.expected + " (safety backup: " + safety + ")"
		}
	default:
		if len(key.text) > 0 {
			confirm.input = append(confirm.input, key.text...)
		} else if key.r >= 32 && utf8.RuneLen(key.r) > 0 {
			confirm.input = append(confirm.input, key.r)
		}
	}
	return nil
}

func (a *app) move(delta int) error {
	if a.focus == 0 {
		if len(a.projects) == 0 {
			return nil
		}
		a.projectIndex = clamp(a.projectIndex+delta, 0, len(a.projects)-1)
		a.moduleIndex = -1
		a.taskIndex = 0
		a.detailScroll = 0
		return a.loadProject()
	}
	if a.focus == 1 {
		if a.project == nil {
			return nil
		}
		a.moduleIndex = clamp(a.moduleIndex+delta, -1, len(a.project.Modules)-1)
		a.taskIndex = 0
		a.detailScroll = 0
		return a.loadTasks()
	}
	if a.focus == 2 && len(a.tasks) > 0 {
		a.taskIndex = clamp(a.taskIndex+delta, 0, len(a.tasks)-1)
		a.detailScroll = 0
	}
	if a.focus == 3 {
		visible := max(1, a.detailPaneHeight()-2)
		a.detailScroll = clamp(a.detailScroll+delta, 0, max(0, len(a.detailLines())-visible))
	}
	return nil
}

func (a *app) moveToBoundary(end bool) error {
	if a.focus == 0 && len(a.projects) > 0 {
		a.projectIndex = 0
		if end {
			a.projectIndex = len(a.projects) - 1
		}
		a.moduleIndex, a.taskIndex, a.detailScroll = -1, 0, 0
		return a.loadProject()
	}
	if a.focus == 1 && a.project != nil {
		a.moduleIndex = -1
		if end && len(a.project.Modules) > 0 {
			a.moduleIndex = len(a.project.Modules) - 1
		}
		a.taskIndex, a.detailScroll = 0, 0
		return a.loadTasks()
	}
	if a.focus == 2 && len(a.tasks) > 0 {
		a.taskIndex = 0
		if end {
			a.taskIndex = len(a.tasks) - 1
		}
		a.detailScroll = 0
	}
	if a.focus == 3 {
		a.detailScroll = 0
		if end {
			a.detailScroll = max(0, len(a.detailLines())-(a.detailPaneHeight()-2))
		}
	}
	return nil
}

func (a *app) pageSize() int {
	contentHeight := max(1, a.height-4)
	switch a.focus {
	case 0:
		return max(1, max(7, contentHeight/2)-3)
	case 1:
		return max(1, contentHeight-max(7, contentHeight/2)-3)
	case 2:
		return max(1, max(9, contentHeight*3/5)-3)
	default:
		return max(1, a.detailPaneHeight()-3)
	}
}

func (a *app) detailPaneHeight() int {
	contentHeight := max(1, a.height-4)
	return contentHeight - max(9, contentHeight*3/5)
}

func (a *app) cycleStatus() error {
	if len(a.tasks) == 0 || a.focus != 2 {
		return nil
	}
	current := a.tasks[a.taskIndex].task.Status
	next := models.StatusTodo
	switch current {
	case models.StatusTodo:
		next = models.StatusDoing
	case models.StatusDoing:
		next = models.StatusDone
	case models.StatusDone:
		next = models.StatusBlocked
	}
	return a.setStatus(next)
}

func (a *app) setStatus(status models.TaskStatus) error {
	if len(a.tasks) == 0 || a.focus != 2 {
		return nil
	}
	ids := a.actionTaskIDs()
	count, err := a.store.UpdateTasksStatus(a.projects[a.projectIndex], ids, status)
	if err != nil {
		return err
	}
	wasBulk := a.selectedTaskCount() > 0
	a.clearTaskSelection()
	if err := a.loadProject(); err != nil {
		return err
	}
	if wasBulk {
		a.message = fmt.Sprintf("Moved %d selected task(s) to %s", count, status)
	} else {
		a.message = fmt.Sprintf("Task %s moved to %s", ids[0], status)
	}
	return nil
}

func (a *app) toggleTaskSelection() error {
	if len(a.tasks) == 0 {
		return nil
	}
	a.ensureTaskSelection()
	id := a.tasks[a.taskIndex].task.ID
	if a.selectedTasks[id] {
		delete(a.selectedTasks, id)
	} else {
		a.selectedTasks[id] = true
	}
	a.message = fmt.Sprintf("%d task(s) selected", a.selectedTaskCount())
	return nil
}

func (a *app) toggleAllVisibleTasks() error {
	if len(a.tasks) == 0 {
		return nil
	}
	a.ensureTaskSelection()
	allSelected := true
	for _, item := range a.tasks {
		if !a.selectedTasks[item.task.ID] {
			allSelected = false
			break
		}
	}
	for _, item := range a.tasks {
		if allSelected {
			delete(a.selectedTasks, item.task.ID)
		} else {
			a.selectedTasks[item.task.ID] = true
		}
	}
	a.message = fmt.Sprintf("%d task(s) selected", a.selectedTaskCount())
	return nil
}

func (a *app) actionTaskIDs() []string {
	if a.selectedTaskCount() == 0 {
		if len(a.tasks) == 0 {
			return nil
		}
		return []string{a.tasks[a.taskIndex].task.ID}
	}
	ids := make([]string, 0, len(a.selectedTasks))
	for _, task := range a.project.GetAllTasks() {
		if a.selectedTasks[task.ID] {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func (a *app) ensureTaskSelection() {
	if a.selectedTasks == nil {
		a.selectedTasks = make(map[string]bool)
	}
}

func (a *app) selectedTaskCount() int {
	return len(a.selectedTasks)
}

func (a *app) clearTaskSelection() {
	a.selectedTasks = make(map[string]bool)
}

func (a *app) pruneTaskSelection() {
	if len(a.selectedTasks) == 0 || a.project == nil {
		return
	}
	valid := make(map[string]bool)
	for _, task := range a.project.GetAllTasks() {
		valid[task.ID] = true
	}
	for id := range a.selectedTasks {
		if !valid[id] {
			delete(a.selectedTasks, id)
		}
	}
}

func (a *app) view(width, height int) string {
	if width < 72 || height < 22 {
		return red + bold + "QIX needs a terminal at least 72x22. Resize the window or press q to quit." + reset
	}
	if a.section != sectionWorkspace {
		return a.sectionView(width, height)
	}
	contentHeight := height - 4
	projectWidth := clamp(width/4, 24, 34)
	mainWidth := width - projectWidth - 1
	projectHeight := max(7, contentHeight/2)
	moduleHeight := contentHeight - projectHeight
	taskHeight := max(9, contentHeight*3/5)
	detailHeight := contentHeight - taskHeight

	projectLines := make([]string, 0, len(a.projects)+1)
	if len(a.projects) == 0 {
		projectLines = append(projectLines, "  No projects", "", "  Press p to create one")
	} else {
		start, end := visibleRange(len(a.projects), a.projectIndex, projectHeight-2)
		for i := start; i < end; i++ {
			projectLines = append(projectLines, tableRow("  "+a.projects[i], projectWidth-2, i, i == a.projectIndex, ""))
		}
	}

	moduleLines := make([]string, 0, 1)
	if a.project == nil {
		moduleLines = append(moduleLines, "  Select a project")
	} else {
		count := len(a.project.Modules) + 1
		selected := a.moduleIndex + 1
		start, end := visibleRange(count, selected, moduleHeight-2)
		for row := start; row < end; row++ {
			text := "  (project tasks)"
			if row > 0 {
				module := a.project.Modules[row-1]
				text = fmt.Sprintf("  %s (%d)", module.Name, len(module.Tasks))
			}
			moduleLines = append(moduleLines, tableRow(text, projectWidth-2, row, row == selected, ""))
		}
		if len(a.project.Modules) == 0 {
			moduleLines = append(moduleLines, "", "  Press m to create one")
		}
	}

	taskLines := make([]string, 0, len(a.tasks)+1)
	if a.project != nil && len(a.tasks) == 0 {
		taskLines = append(taskLines, "  No tasks", "", "  Press n to create one")
	}
	if len(a.tasks) > 0 {
		header := fmt.Sprintf("  %-3s %-7s %-8s %s", "SEL", "STATUS", "ID", "TITLE")
		taskLines = append(taskLines, tableHeader(header, mainWidth-2))
		start, end := visibleRange(len(a.tasks), a.taskIndex, taskHeight-3)
		for i := start; i < end; i++ {
			item := a.tasks[i]
			mark := "[ ]"
			if a.selectedTasks[item.task.ID] {
				mark = "[x]"
			}
			content := fmt.Sprintf("  %-3s %-7s %-8s %s", mark, item.task.Status, item.task.ID, item.task.Title)
			taskLines = append(taskLines, tableRow(content, mainWidth-2, i, i == a.taskIndex, statusColor(item.task.Status)))
		}
	}

	allDetails := a.detailLines()
	details := scrollWindow(allDetails, a.detailScroll, detailHeight-2)
	projectTitle := scrollTitle("Projects", a.projectIndex, len(a.projects))
	moduleCount := 0
	if a.project != nil {
		moduleCount = len(a.project.Modules) + 1
	}
	moduleTitle := scrollTitle("Modules", a.moduleIndex+1, moduleCount)
	projectBox := box(projectTitle, projectLines, projectWidth, projectHeight, a.focus == 0)
	moduleBox := box(moduleTitle, moduleLines, projectWidth, moduleHeight, a.focus == 1)
	left := append(projectBox, moduleBox...)
	taskTitle := "Project Tasks"
	if a.project != nil && a.moduleIndex >= 0 {
		taskTitle = "Tasks • " + a.project.Modules[a.moduleIndex].Name
	}
	taskTitle = scrollTitle(taskTitle, a.taskIndex, len(a.tasks))
	if count := a.selectedTaskCount(); count > 0 {
		taskTitle += fmt.Sprintf(" • %d selected", count)
	}
	detailTitle := scrollOffsetTitle("Details", a.detailScroll, len(allDetails), detailHeight-2)
	topRight := box(taskTitle, taskLines, mainWidth, taskHeight, a.focus == 2)
	bottomRight := box(detailTitle, details, mainWidth, detailHeight, a.focus == 3)
	right := append(topRight, bottomRight...)

	var b strings.Builder
	b.WriteString(a.navigation(width, "WORKSPACE") + "\n")
	summary := " No project selected"
	if a.project != nil {
		counts := a.project.CountByStatus()
		summary = fmt.Sprintf(" %s  •  %d modules  •  %d tasks  •  %d doing  •  %.0f%% complete", a.project.Name, len(a.project.Modules), len(a.project.GetAllTasks()), counts[models.StatusDoing], a.project.GetCompletionPercentage())
	}
	b.WriteString(dim + fit(summary, width) + reset + "\n")
	for i := 0; i < contentHeight; i++ {
		b.WriteString(left[i] + " " + right[i] + "\n")
	}
	b.WriteString(dim + fit(" ↑/↓ move  space mark  a all  1-4 status  d delete  esc clear  tab focus  ? help", width) + reset + "\n")
	footer := a.message
	color := green
	if a.isError {
		color = red
	}
	b.WriteString(color + fit(" "+footer, width) + reset)
	base := b.String()
	if a.form != nil || a.confirmation != nil {
		return a.modalView(base, width, height)
	}
	return base
}

func (a *app) modalView(base string, width, height int) string {
	modalWidth := clamp(width-18, 46, 72)
	var title, content, hint string
	if a.form != nil {
		title = a.form.title
		visibleFields := clamp((height-10)/2, 1, len(a.form.fields))
		start, end := visibleRange(len(a.form.fields), a.form.index, visibleFields)
		rows := make([]string, 0, visibleFields*2+1)
		for i := start; i < end; i++ {
			field := a.form.fields[i]
			label := field.label
			if field.required {
				label += " *"
			}
			labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
			inputStyle := lipgloss.NewStyle().Width(modalWidth-4).Padding(0, 1).Background(lipgloss.Color("238")).Foreground(lipgloss.Color("252"))
			value := string(field.value)
			if i == a.form.index {
				labelStyle = labelStyle.Bold(true).Foreground(lipgloss.Color("44"))
				inputStyle = inputStyle.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("229"))
				value += "█"
			}
			rows = append(rows, labelStyle.Render(label), inputStyle.Render(value))
		}
		if len(a.form.fields) > visibleFields {
			rows = append(rows, lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("Fields %d–%d of %d", start+1, end, len(a.form.fields))))
		}
		content = strings.Join(rows, "\n")
		hint = "↑/↓ or tab select  •  enter next/save  •  ctrl-s save  •  esc cancel"
	} else {
		title = "Confirm destructive action"
		content = a.confirmation.prompt
		if a.confirmation.expected != "" {
			content += "\n" + lipgloss.NewStyle().Width(modalWidth-4).Padding(0, 1).MarginTop(1).
				Background(lipgloss.Color("238")).Foreground(lipgloss.Color("229")).Render(string(a.confirmation.input)+"█")
			hint = "type the exact value, then enter  •  esc cancels"
		} else {
			hint = "enter confirms  •  esc cancels"
		}
	}
	if a.isError && a.message != "" {
		hint = red + a.message + reset + "\n" + hint
	}
	card := lipgloss.NewStyle().
		Width(modalWidth).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("44")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252")).
		Render(
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("44")).Render(title) + "\n\n" +
				content + "\n\n" +
				lipgloss.NewStyle().Faint(true).Render(hint),
		)
	return overlayView(base, card, width, height)
}

func (a *app) detailLines() []string {
	if a.showHelp {
		return []string{
			"Navigation: arrows or j/k; tab/left/right changes pane",
			"p: new project    m: new module    n: new task",
			"e: edit selected item    d: remove selected item",
			"r: refresh from disk",
			"space: mark task    a: mark/unmark visible tasks    esc: clear marks",
			"x: cycle status    1-4: set status for marked/current task",
			"l: set parent    y: add dependency    c/u: set/remove recurrence",
			"C: complete task    t: log time    o: open Jira issue",
			"1 todo  2 doing  3 done  4 blocked",
			"?: close help    q: quit",
		}
	}
	if a.focus == 0 {
		return a.projectDetailLines()
	}
	if a.focus == 1 {
		return a.moduleDetailLines()
	}
	if len(a.tasks) == 0 {
		return []string{"Select a task to see its details."}
	}
	item := a.tasks[a.taskIndex]
	task := item.task
	lines := []string{
		bold + task.Title + reset,
		fmt.Sprintf("ID: %s    Status: %s    Priority: %s", task.ID, task.Status, task.Priority),
		fmt.Sprintf("Location: %s    Estimate: %.2fh    Actual: %.2fh", item.location, task.EstimatedHours, task.CalculateActualHours()),
	}
	if task.Description != "" {
		lines = append(lines, "Description: "+task.Description)
	}
	if len(task.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(task.Tags, ", "))
	}
	if task.ParentID != "" {
		lines = append(lines, "Parent: "+task.ParentID)
	}
	if len(task.Dependencies) > 0 {
		lines = append(lines, "Depends on: "+strings.Join(task.Dependencies, ", "))
	}
	if task.Recurrence != nil && task.Recurrence.Enabled {
		pattern := string(task.Recurrence.Type)
		if task.Recurrence.Value != "" {
			pattern += ":" + task.Recurrence.Value
		}
		lines = append(lines, fmt.Sprintf("Recurrence: %s    Next due: %s", pattern, task.Recurrence.NextDue))
	}
	if task.JiraIssue != "" {
		lines = append(lines, "Jira: "+task.JiraIssue)
	}
	return lines
}

func (a *app) moduleDetailLines() []string {
	if a.project == nil {
		return []string{"Select a project first."}
	}
	if a.moduleIndex < 0 {
		return []string{
			bold + "Project-level tasks" + reset,
			"Tasks here belong directly to " + a.project.Name + ".",
			"Select a module below, or press m to create one.",
		}
	}
	module := a.project.Modules[a.moduleIndex]
	counts := map[models.TaskStatus]int{}
	estimated, actual := 0.0, 0.0
	for _, task := range module.Tasks {
		counts[task.Status]++
		estimated += task.EstimatedHours
		actual += task.CalculateActualHours()
	}
	completion := 0.0
	if len(module.Tasks) > 0 {
		completion = float64(counts[models.StatusDone]) / float64(len(module.Tasks)) * 100
	}
	lines := []string{
		bold + module.Name + reset,
		fmt.Sprintf("Tasks: %d    Todo: %d    Doing: %d    Done: %d    Blocked: %d", len(module.Tasks), counts[models.StatusTodo], counts[models.StatusDoing], counts[models.StatusDone], counts[models.StatusBlocked]),
		fmt.Sprintf("Estimated: %.2fh    Actual: %.2fh", estimated, actual),
		fmt.Sprintf("Completion: %s %.1f%%", progressBar(completion, 20), completion),
	}
	if module.Description != "" {
		lines = append(lines, "Description: "+module.Description)
	}
	if len(module.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(module.Tags, ", "))
	}
	if !module.CreatedAt.IsZero() {
		lines = append(lines, "Created: "+module.CreatedAt.Format("2006-01-02 15:04"))
	}
	return lines
}

func (a *app) projectDetailLines() []string {
	if a.project == nil {
		return []string{"Create a project with p to get started."}
	}
	project := a.project
	counts := project.CountByStatus()
	lines := []string{
		bold + project.Name + reset,
		fmt.Sprintf("Tasks: %d    Modules: %d    Sprints: %d", len(project.GetAllTasks()), len(project.Modules), len(project.Sprints)),
		fmt.Sprintf("Todo: %d    Doing: %d    Done: %d    Blocked: %d", counts[models.StatusTodo], counts[models.StatusDoing], counts[models.StatusDone], counts[models.StatusBlocked]),
		fmt.Sprintf("Estimated: %.2fh    Actual: %.2fh", project.CalculateTotalEstimated(), project.CalculateTotalActual()),
		fmt.Sprintf("Completion: %s %.1f%%", progressBar(project.GetCompletionPercentage(), 20), project.GetCompletionPercentage()),
	}
	if project.Description != "" {
		lines = append(lines, "Description: "+project.Description)
	}
	if len(project.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(project.Tags, ", "))
	}
	if !project.CreatedAt.IsZero() {
		lines = append(lines, "Created: "+project.CreatedAt.Format("2006-01-02 15:04"))
	}
	if len(project.Modules) > 0 {
		names := make([]string, 0, len(project.Modules))
		for _, module := range project.Modules {
			names = append(names, module.Name)
		}
		lines = append(lines, "Modules: "+strings.Join(names, ", "))
	}
	return lines
}

func box(title string, body []string, width, height int, active bool) []string {
	borderColor := dim
	if active {
		borderColor = cyan
	}
	inner := width - 2
	topLabel := "─ " + title + " "
	top := "┌" + topLabel + strings.Repeat("─", max(0, inner-runeCount(topLabel))) + "┐"
	lines := make([]string, height)
	lines[0] = borderColor + top + reset
	for i := 1; i < height-1; i++ {
		content := ""
		if i-1 < len(body) {
			content = body[i-1]
		}
		lines[i] = borderColor + "│" + reset + fit(content, inner) + borderColor + "│" + reset
	}
	lines[height-1] = borderColor + "└" + strings.Repeat("─", inner) + "┘" + reset
	return lines
}

func tableHeader(content string, width int) string {
	return lipgloss.NewStyle().
		Width(width).
		MaxWidth(width).
		Bold(true).
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("239")).
		Render(ansi.Truncate(content, width, "…"))
}

func tableRow(content string, width, row int, selected bool, foreground string) string {
	background := lipgloss.Color("235")
	if row%2 == 1 {
		background = lipgloss.Color("236")
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width).Background(background).Foreground(lipgloss.Color("252"))
	if foreground != "" {
		style = style.Foreground(lipgloss.Color(foreground))
	}
	if selected {
		if strings.HasPrefix(content, "  ") {
			content = "› " + strings.TrimPrefix(content, "  ")
		}
		style = style.Bold(true).Background(lipgloss.Color("24")).Foreground(lipgloss.Color("231"))
	}
	return style.Render(ansi.Truncate(content, width, "…"))
}

func statusColor(status models.TaskStatus) string {
	switch status {
	case models.StatusDoing:
		return "220"
	case models.StatusDone:
		return "42"
	case models.StatusBlocked:
		return "203"
	default:
		return "250"
	}
}

func visibleRange(total, selected, height int) (int, int) {
	if total <= 0 || height <= 0 {
		return 0, 0
	}
	height = min(height, total)
	selected = clamp(selected, 0, total-1)
	start := selected - height/2
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = total - height
	}
	return start, start + height
}

func scrollWindow(lines []string, offset, height int) []string {
	if height <= 0 || len(lines) == 0 {
		return nil
	}
	offset = clamp(offset, 0, max(0, len(lines)-height))
	end := min(len(lines), offset+height)
	return lines[offset:end]
}

func scrollTitle(title string, selected, total int) string {
	if total <= 0 {
		return title
	}
	return fmt.Sprintf("%s  %d/%d", title, clamp(selected+1, 1, total), total)
}

func scrollOffsetTitle(title string, offset, total, height int) string {
	if total <= height || total == 0 {
		return title
	}
	return fmt.Sprintf("%s  lines %d–%d/%d", title, offset+1, min(total, offset+height), total)
}

func overlayView(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	overlayLines := strings.Split(overlay, "\n")
	overlayWidth := 0
	for _, line := range overlayLines {
		overlayWidth = max(overlayWidth, ansi.StringWidth(line))
	}
	x := max(0, (width-overlayWidth)/2)
	y := max(0, (height-len(overlayLines))/2)
	dimmed := lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("243"))
	for row := 0; row < height && row < len(baseLines); row++ {
		plain := []rune(fit(stripANSI(baseLines[row]), width))
		if row < y || row >= y+len(overlayLines) {
			baseLines[row] = dimmed.Render(string(plain))
			continue
		}
		modalLine := overlayLines[row-y]
		lineWidth := ansi.StringWidth(modalLine)
		left := min(x, len(plain))
		right := min(len(plain), x+lineWidth)
		baseLines[row] = dimmed.Render(string(plain[:left])) + modalLine + dimmed.Render(string(plain[right:]))
	}
	return strings.Join(baseLines[:height], "\n")
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// ANSI sequences are only used in the details title. Preserve them while
	// padding based on visible content; truncation strips styling safely.
	visibleWidth := ansi.StringWidth(s)
	if visibleWidth > width {
		if width == 1 {
			return ansi.Truncate(s, 1, "")
		}
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-visibleWidth)
}

func stripANSI(s string) string {
	for {
		start := strings.IndexByte(s, '\x1b')
		if start < 0 {
			return s
		}
		end := strings.IndexByte(s[start:], 'm')
		if end < 0 {
			return s[:start]
		}
		s = s[:start] + s[start+end+1:]
	}
}

func runeCount(s string) int { return utf8.RuneCountInString(s) }

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseTaskStatus(value string) (models.TaskStatus, error) {
	status := models.TaskStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case models.StatusTodo, models.StatusDoing, models.StatusDone, models.StatusBlocked:
		return status, nil
	default:
		return "", fmt.Errorf("status must be todo, doing, done, or blocked")
	}
}

func parsePriority(value string) (models.Priority, error) {
	priority := models.Priority(strings.ToLower(strings.TrimSpace(value)))
	switch priority {
	case models.PriorityLow, models.PriorityMedium, models.PriorityHigh:
		return priority, nil
	default:
		return "", fmt.Errorf("priority must be low, medium, or high")
	}
}

func parseRecurrence(pattern string, now time.Time) (models.Recurrence, error) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(pattern)), ":", 2)
	value := ""
	if len(parts) == 2 {
		value = parts[1]
	}
	recurrence := models.Recurrence{Value: value, Enabled: true}
	switch parts[0] {
	case "daily":
		recurrence.Type = models.RecurDaily
		recurrence.NextDue = now.AddDate(0, 0, 1).Format("2006-01-02")
	case "weekly":
		days := map[string]time.Weekday{"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday, "thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday}
		target, ok := days[value]
		if !ok {
			return recurrence, fmt.Errorf("weekly recurrence needs a weekday, for example weekly:monday")
		}
		recurrence.Type = models.RecurWeekly
		delta := (int(target) - int(now.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7
		}
		recurrence.NextDue = now.AddDate(0, 0, delta).Format("2006-01-02")
	case "monthly":
		day, err := strconv.Atoi(value)
		if err != nil || day < 1 || day > 31 {
			return recurrence, fmt.Errorf("monthly recurrence needs a day from 1 to 31")
		}
		recurrence.Type = models.RecurMonthly
		next := now.AddDate(0, 1, 0)
		lastDay := time.Date(next.Year(), next.Month()+1, 0, 0, 0, 0, 0, next.Location()).Day()
		recurrence.NextDue = time.Date(next.Year(), next.Month(), min(day, lastDay), 0, 0, 0, 0, next.Location()).Format("2006-01-02")
	case "interval":
		days, err := strconv.Atoi(value)
		if err != nil || days < 1 {
			return recurrence, fmt.Errorf("interval recurrence needs a positive day count")
		}
		recurrence.Type = models.RecurInterval
		recurrence.NextDue = now.AddDate(0, 0, days).Format("2006-01-02")
	default:
		return recurrence, fmt.Errorf("recurrence must be daily, weekly:day, monthly:day, or interval:days")
	}
	return recurrence, nil
}

func progressBar(percent float64, width int) string {
	percent = float64(clamp(int(percent+0.5), 0, 100))
	filled := int(percent / 100 * float64(width))
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
