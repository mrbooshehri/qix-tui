// Package tui provides QIX's full-screen terminal interface.
package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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

type terminal struct {
	state string
}

func openTerminal() (*terminal, error) {
	in, err := os.Stdin.Stat()
	if err != nil || in.Mode()&os.ModeCharDevice == 0 {
		return nil, fmt.Errorf("the TUI requires an interactive terminal")
	}
	out, err := os.Stdout.Stat()
	if err != nil || out.Mode()&os.ModeCharDevice == 0 {
		return nil, fmt.Errorf("the TUI requires an interactive terminal")
	}

	stateCmd := exec.Command("stty", "-g")
	stateCmd.Stdin = os.Stdin
	state, err := stateCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read terminal state: %w", err)
	}
	rawCmd := exec.Command("stty", "raw", "-echo")
	rawCmd.Stdin = os.Stdin
	rawCmd.Stdout = io.Discard
	rawCmd.Stderr = io.Discard
	if err := rawCmd.Run(); err != nil {
		return nil, fmt.Errorf("enable raw terminal mode: %w", err)
	}

	t := &terminal{state: strings.TrimSpace(string(state))}
	fmt.Print("\x1b[?1049h\x1b[?25l")
	return t, nil
}

func (t *terminal) close() {
	fmt.Print("\x1b[0m\x1b[?25h\x1b[?1049l")
	cmd := exec.Command("stty", t.state)
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}

func (t *terminal) size() (int, int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	b, err := cmd.Output()
	if err != nil {
		return 100, 30
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 {
		return 100, 30
	}
	rows, rowErr := strconv.Atoi(fields[0])
	cols, colErr := strconv.Atoi(fields[1])
	if rowErr != nil || colErr != nil || rows < 1 || cols < 1 {
		return 100, 30
	}
	return cols, rows
}

type keyEvent struct {
	name string
	r    rune
}

func readKey(r *bufio.Reader) (keyEvent, error) {
	b, err := r.ReadByte()
	if err != nil {
		return keyEvent{}, err
	}
	switch b {
	case 3:
		return keyEvent{name: "ctrl-c"}, nil
	case 7:
		return keyEvent{name: "cancel"}, nil
	case 9:
		return keyEvent{name: "tab"}, nil
	case 13, 10:
		return keyEvent{name: "enter"}, nil
	case 127, 8:
		return keyEvent{name: "backspace"}, nil
	case 27:
		next, err := r.ReadByte()
		if err != nil {
			return keyEvent{name: "esc"}, nil
		}
		if next != '[' {
			return keyEvent{name: "esc"}, nil
		}
		direction, err := r.ReadByte()
		if err != nil {
			return keyEvent{name: "esc"}, nil
		}
		switch direction {
		case 'A':
			return keyEvent{name: "up"}, nil
		case 'B':
			return keyEvent{name: "down"}, nil
		case 'C':
			return keyEvent{name: "right"}, nil
		case 'D':
			return keyEvent{name: "left"}, nil
		}
		return keyEvent{name: "esc"}, nil
	case ' ':
		return keyEvent{name: "space", r: ' '}, nil
	}
	if b >= utf8.RuneSelf {
		if err := r.UnreadByte(); err != nil {
			return keyEvent{}, err
		}
		char, _, err := r.ReadRune()
		if err != nil {
			return keyEvent{}, err
		}
		return keyEvent{r: char}, nil
	}
	if b >= 32 {
		return keyEvent{r: rune(b)}, nil
	}
	return keyEvent{}, nil
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
}

type app struct {
	store        *storage.Storage
	projects     []string
	projectIndex int
	project      *models.Project
	moduleIndex  int
	tasks        []taskItem
	taskIndex    int
	focus        int
	form         *inputForm
	confirmation *confirmation
	message      string
	isError      bool
	showHelp     bool
}

// Run starts the full-screen QIX interface. initialProject may be empty.
func Run(store *storage.Storage, initialProject string) error {
	term, err := openTerminal()
	if err != nil {
		return err
	}
	defer term.close()

	a := &app{store: store, moduleIndex: -1}
	if err := a.loadProjects(initialProject); err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		width, height := term.size()
		fmt.Print(a.view(width, height))
		key, err := readKey(reader)
		if err != nil {
			return err
		}
		quit, err := a.update(key)
		if err != nil {
			a.message, a.isError = err.Error(), true
		}
		if quit {
			return nil
		}
	}
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

func (a *app) update(key keyEvent) (bool, error) {
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
	if key.name == "esc" && a.showHelp {
		a.showHelp = false
		return false, nil
	}
	if key.r == 'q' {
		return true, nil
	}
	if key.r == '?' {
		a.showHelp = !a.showHelp
		return false, nil
	}
	a.message, a.isError = "", false
	switch {
	case key.name == "tab":
		a.focus = (a.focus + 1) % 3
	case key.name == "left":
		a.focus = max(0, a.focus-1)
	case key.name == "right":
		a.focus = min(2, a.focus+1)
	case key.name == "enter" && a.focus < 2:
		a.focus++
	case key.name == "up" || key.r == 'k':
		return false, a.move(-1)
	case key.name == "down" || key.r == 'j':
		return false, a.move(1)
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
	case key.name == "space" || key.r == 'x':
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
		},
	}
}

func (a *app) startTaskForm() {
	a.form = &inputForm{
		kind:  "task",
		title: "Create project-level task",
		fields: []inputField{
			{label: "Title", required: true},
		},
	}
}

func (a *app) updateForm(key keyEvent) error {
	field := &a.form.fields[a.form.index]
	switch key.name {
	case "esc", "cancel":
		a.form = nil
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
	default:
		if key.r >= 32 && utf8.RuneLen(key.r) > 0 {
			field.value = append(field.value, key.r)
		}
	}
	return nil
}

func (a *app) submitForm() error {
	form := a.form
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
		if err := a.store.UpdateModule(a.project.Name, oldName, func(module *models.Module) error {
			module.Name = newName
			module.Description = description
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

	title := strings.TrimSpace(string(form.fields[0].value))
	moduleName := ""
	if a.moduleIndex >= 0 {
		moduleName = a.project.Modules[a.moduleIndex].Name
	}
	if err := a.store.AddTask(a.projects[a.projectIndex], moduleName, models.Task{Title: title}); err != nil {
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
		if strings.TrimSpace(string(confirm.input)) != confirm.expected {
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
	default:
		if key.r >= 32 && utf8.RuneLen(key.r) > 0 {
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
		return a.loadProject()
	}
	if a.focus == 1 {
		if a.project == nil {
			return nil
		}
		a.moduleIndex = clamp(a.moduleIndex+delta, -1, len(a.project.Modules)-1)
		a.taskIndex = 0
		return a.loadTasks()
	}
	if a.focus == 2 && len(a.tasks) > 0 {
		a.taskIndex = clamp(a.taskIndex+delta, 0, len(a.tasks)-1)
	}
	return nil
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
	item := a.tasks[a.taskIndex]
	if err := a.store.UpdateTaskStatus(a.projects[a.projectIndex], item.task.ID, status); err != nil {
		return err
	}
	if err := a.loadProject(); err != nil {
		return err
	}
	a.message = fmt.Sprintf("Task %s moved to %s", item.task.ID, status)
	return nil
}

func (a *app) view(width, height int) string {
	if width < 72 || height < 22 {
		return "\x1b[H\x1b[2J" + red + bold + "QIX needs a terminal at least 72x22. Resize the window or press q to quit." + reset
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
	}
	for i, name := range a.projects {
		prefix := "  "
		if i == a.projectIndex {
			prefix = "> "
		}
		projectLines = append(projectLines, prefix+name)
	}
	if len(a.projects) > 0 {
		projectLines = visibleWindow(projectLines, a.projectIndex, projectHeight-2)
	}

	moduleLines := make([]string, 0, 1)
	if a.project == nil {
		moduleLines = append(moduleLines, "  Select a project")
	} else {
		rootPrefix := "  "
		if a.moduleIndex < 0 {
			rootPrefix = "> "
		}
		moduleLines = append(moduleLines, rootPrefix+"(project tasks)")
		for i, module := range a.project.Modules {
			prefix := "  "
			if i == a.moduleIndex {
				prefix = "> "
			}
			moduleLines = append(moduleLines, fmt.Sprintf("%s%s (%d)", prefix, module.Name, len(module.Tasks)))
		}
		if len(a.project.Modules) == 0 {
			moduleLines = append(moduleLines, "", "  Press m to create one")
		}
		moduleLines = visibleWindow(moduleLines, a.moduleIndex+1, moduleHeight-2)
	}

	taskLines := make([]string, 0, len(a.tasks)+1)
	if a.project != nil && len(a.tasks) == 0 {
		taskLines = append(taskLines, "  No tasks", "", "  Press n to create one")
	}
	for i, item := range a.tasks {
		prefix := "  "
		if i == a.taskIndex {
			prefix = "> "
		}
		taskLines = append(taskLines, fmt.Sprintf("%s%-7s %-8s %s", prefix, item.task.Status, item.task.ID, item.task.Title))
	}
	if len(a.tasks) > 0 {
		taskLines = visibleWindow(taskLines, a.taskIndex, taskHeight-2)
	}

	details := a.detailLines()
	projectBox := box("Projects", projectLines, projectWidth, projectHeight, a.focus == 0)
	moduleBox := box("Modules", moduleLines, projectWidth, moduleHeight, a.focus == 1)
	left := append(projectBox, moduleBox...)
	taskTitle := "Project Tasks"
	if a.project != nil && a.moduleIndex >= 0 {
		taskTitle = "Tasks • " + a.project.Modules[a.moduleIndex].Name
	}
	topRight := box(taskTitle, taskLines, mainWidth, taskHeight, a.focus == 2)
	bottomRight := box("Details", details, mainWidth, detailHeight, false)
	right := append(topRight, bottomRight...)

	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(cyan + bold + fit(" QIX / PROJECT WORKSPACE", width) + reset + "\r\n")
	summary := " No project selected"
	if a.project != nil {
		counts := a.project.CountByStatus()
		summary = fmt.Sprintf(" %s  •  %d modules  •  %d tasks  •  %d doing  •  %.0f%% complete", a.project.Name, len(a.project.Modules), len(a.project.GetAllTasks()), counts[models.StatusDoing], a.project.GetCompletionPercentage())
	}
	b.WriteString(dim + fit(summary, width) + reset + "\r\n")
	for i := 0; i < contentHeight; i++ {
		b.WriteString(left[i] + " " + right[i] + "\r\n")
	}
	b.WriteString(dim + fit(" ↑/↓ move  tab focus  p project  m module  e edit module  d remove selected  n task  space status  ? help  q quit", width) + reset + "\r\n")
	footer := a.message
	color := green
	if a.isError {
		color = red
	}
	if a.form != nil {
		field := a.form.fields[a.form.index]
		footer = fmt.Sprintf("%s [%d/%d] • %s: %s_  (enter next/save, ctrl-g cancel)", a.form.title, a.form.index+1, len(a.form.fields), field.label, string(field.value))
		color = yellow
		if a.isError {
			footer = a.message + " • " + footer
			color = red
		}
	}
	if a.confirmation != nil {
		footer = a.confirmation.prompt + ": " + string(a.confirmation.input) + "_  (enter confirm, ctrl-g cancel)"
		color = yellow
		if a.isError {
			footer = a.message + " • " + footer
			color = red
		}
	}
	b.WriteString(color + fit(" "+footer, width) + reset)
	return b.String()
}

func (a *app) detailLines() []string {
	if a.showHelp {
		return []string{
			"Navigation: arrows or j/k; tab/left/right changes pane",
			"p: new project    m: new module    n: new task",
			"e: edit module    d: remove focused project/module",
			"r: refresh from disk",
			"space/x: cycle status    1-4: set status",
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

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	// ANSI sequences are only used in the details title. Preserve them while
	// padding based on visible content; truncation strips styling safely.
	plain := stripANSI(s)
	if runeCount(plain) > width {
		runes := []rune(plain)
		if width == 1 {
			return string(runes[:1])
		}
		return string(runes[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-runeCount(plain))
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

func progressBar(percent float64, width int) string {
	percent = float64(clamp(int(percent+0.5), 0, 100))
	filled := int(percent / 100 * float64(width))
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

func visibleWindow(lines []string, selected, height int) []string {
	if height <= 0 || len(lines) <= height {
		return lines
	}
	start := selected - height/2
	if start < 0 {
		start = 0
	}
	if start+height > len(lines) {
		start = len(lines) - height
	}
	return lines[start : start+height]
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
