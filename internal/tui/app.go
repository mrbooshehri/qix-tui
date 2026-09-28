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

type app struct {
	store        *storage.Storage
	projects     []string
	projectIndex int
	project      *models.Project
	tasks        []taskItem
	taskIndex    int
	focus        int
	mode         string
	input        []rune
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

	a := &app{store: store}
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
		a.project, a.tasks, a.projectIndex, a.taskIndex = nil, nil, 0, 0
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
	a.tasks = a.tasks[:0]
	for _, task := range project.Tasks {
		a.tasks = append(a.tasks, taskItem{task: task, location: "project"})
	}
	for _, module := range project.Modules {
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
	if a.mode != "" {
		return false, a.updateInput(key)
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
		a.focus = (a.focus + 1) % 2
	case key.name == "left":
		a.focus = 0
	case key.name == "right" || key.name == "enter" && a.focus == 0:
		a.focus = 1
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
		a.mode, a.input = "project", nil
	case key.r == 'n':
		if a.project == nil {
			return false, fmt.Errorf("create a project first with p")
		}
		a.mode, a.input = "task", nil
	case key.name == "space" || key.r == 'x':
		return false, a.cycleStatus()
	case key.r >= '1' && key.r <= '4':
		statuses := []models.TaskStatus{models.StatusTodo, models.StatusDoing, models.StatusDone, models.StatusBlocked}
		return false, a.setStatus(statuses[int(key.r-'1')])
	}
	return false, nil
}

func (a *app) updateInput(key keyEvent) error {
	switch key.name {
	case "esc", "cancel":
		a.mode, a.input = "", nil
	case "backspace":
		if len(a.input) > 0 {
			a.input = a.input[:len(a.input)-1]
		}
	case "enter":
		value := strings.TrimSpace(string(a.input))
		if value == "" {
			return fmt.Errorf("name cannot be empty")
		}
		mode := a.mode
		a.mode, a.input = "", nil
		if mode == "project" {
			if _, err := a.store.CreateProject(value, "", nil); err != nil {
				return err
			}
			if err := a.loadProjects(value); err != nil {
				return err
			}
			a.message = "Created project " + value
			return nil
		}
		if err := a.store.AddTask(a.projects[a.projectIndex], "", models.Task{Title: value}); err != nil {
			return err
		}
		if err := a.loadProject(); err != nil {
			return err
		}
		a.taskIndex = len(a.tasks) - 1
		a.focus = 1
		a.message = "Created task " + value
	default:
		if key.r >= 32 && utf8.RuneLen(key.r) > 0 {
			a.input = append(a.input, key.r)
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
		a.taskIndex = 0
		return a.loadProject()
	}
	if len(a.tasks) > 0 {
		a.taskIndex = clamp(a.taskIndex+delta, 0, len(a.tasks)-1)
	}
	return nil
}

func (a *app) cycleStatus() error {
	if len(a.tasks) == 0 || a.focus != 1 {
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
	if len(a.tasks) == 0 || a.focus != 1 {
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
		projectLines = visibleWindow(projectLines, a.projectIndex, contentHeight-2)
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
	left := box("Projects", projectLines, projectWidth, contentHeight, a.focus == 0)
	topRight := box("Tasks", taskLines, mainWidth, taskHeight, a.focus == 1)
	bottomRight := box("Details", details, mainWidth, detailHeight, false)
	right := append(topRight, bottomRight...)

	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	b.WriteString(cyan + bold + fit(" QIX / PROJECT WORKSPACE", width) + reset + "\r\n")
	summary := " No project selected"
	if a.project != nil {
		counts := a.project.CountByStatus()
		summary = fmt.Sprintf(" %s  •  %d tasks  •  %d doing  •  %.0f%% complete", a.project.Name, len(a.tasks), counts[models.StatusDoing], a.project.GetCompletionPercentage())
	}
	b.WriteString(dim + fit(summary, width) + reset + "\r\n")
	for i := 0; i < contentHeight; i++ {
		b.WriteString(left[i] + " " + right[i] + "\r\n")
	}
	b.WriteString(dim + fit(" ↑/↓ or j/k move  tab/←/→ focus  n new task  p new project  space status  r refresh  ? help  q quit", width) + reset + "\r\n")
	footer := a.message
	color := green
	if a.isError {
		color = red
	}
	if a.mode != "" {
		label := "New task title"
		if a.mode == "project" {
			label = "New project name"
		}
		footer = label + ": " + string(a.input) + "_  (enter save, ctrl-g cancel)"
		color = yellow
	}
	b.WriteString(color + fit(" "+footer, width) + reset)
	return b.String()
}

func (a *app) detailLines() []string {
	if a.showHelp {
		return []string{
			"Navigation: arrows or j/k; tab changes pane",
			"n: new task    p: new project    r: refresh",
			"space/x: cycle status    1-4: set status",
			"1 todo  2 doing  3 done  4 blocked",
			"?: close help    q: quit",
		}
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
