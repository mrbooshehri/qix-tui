package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/logging"
	"github.com/mrbooshehri/qix-go/internal/models"
)

func (a *app) openSelectedJira() error {
	if a.project == nil || len(a.tasks) == 0 {
		return fmt.Errorf("select a task first")
	}
	issue := strings.TrimSpace(a.tasks[a.taskIndex].task.JiraIssue)
	if issue == "" {
		return fmt.Errorf("selected task has no Jira issue; edit it with e")
	}
	base := strings.TrimSpace(config.Get().JiraBaseURL)
	if base == "" {
		return fmt.Errorf("configure jira_base_url or JIRA_BASE_URL first")
	}
	url := strings.TrimRight(base, "/") + "/" + issue
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open Jira issue %s: %w", url, err)
	}
	a.message = "Opened Jira issue " + issue
	return nil
}

func (a *app) navigation(width int, active string) string {
	type navigationItem struct{ key, label, short, section string }
	items := []navigationItem{
		{"W", "Workspace", "Work", "WORKSPACE"},
		{"T", "Tracking", "Track", "TRACKING"},
		{"S", "Sprints", "Sprint", "SPRINTS"},
		{"R", "Reports", "Report", "REPORTS"},
		{"H", "Health", "Health", "HEALTH"},
		{"G", "Settings", "Setup", "SETTINGS"},
	}
	labels := make([]string, len(items))
	for i, item := range items {
		label := item.label
		if width < 100 {
			label = item.short
		}
		text := item.key + " " + label
		if item.section == active {
			labels[i] = cyan + bold + "[" + text + "]" + reset
		} else {
			labels[i] = dim + " " + text + " " + reset
		}
	}
	return fit(" QIX / "+active+"  "+strings.Join(labels, " "), width)
}

func (a *app) updateSection(key keyEvent) error {
	visible := max(1, a.height-6)
	if key.name == "page-up" {
		a.sectionScroll = max(0, a.sectionScroll-visible)
		return nil
	}
	if key.name == "page-down" {
		a.sectionScroll += visible
		return nil
	}
	if key.name == "home" {
		a.sectionScroll = 0
		return nil
	}
	if key.name == "end" {
		a.sectionScroll = 1 << 20
		return nil
	}
	switch a.section {
	case sectionTracking:
		return a.updateTracking(key)
	case sectionSprints:
		return a.updateSprints(key)
	case sectionReports:
		if key.name == "up" || key.r == 'k' {
			a.sectionScroll = max(0, a.sectionScroll-1)
		}
		if key.name == "down" || key.r == 'j' {
			a.sectionScroll++
		}
		if key.name == "left" || key.r == 'h' {
			a.reportIndex = max(0, a.reportIndex-1)
			a.sectionScroll = 0
		}
		if key.name == "right" || key.r == 'l' {
			a.reportIndex = min(4, a.reportIndex+1)
			a.sectionScroll = 0
		}
	case sectionHealth:
		backups, err := a.store.ListBackups()
		if err != nil {
			return err
		}
		if key.name == "up" || key.r == 'k' {
			a.backupIndex = max(0, a.backupIndex-1)
			a.sectionScroll = max(0, a.sectionScroll-1)
		}
		if key.name == "down" || key.r == 'j' {
			a.backupIndex = min(max(0, len(backups)-1), a.backupIndex+1)
			a.sectionScroll++
		}
		if key.r == 'r' {
			a.message = "Health checks refreshed"
		}
		if key.r == 'b' {
			path, err := a.store.CreateArchive()
			if err != nil {
				return err
			}
			a.message = "Created backup " + path
		}
		if key.r == 'c' {
			count, err := a.store.CleanupBackups()
			if err != nil {
				return err
			}
			a.message = fmt.Sprintf("Removed %d expired backup(s)", count)
		}
		if key.r == 'e' {
			a.form = &inputForm{kind: "backup-export", title: "Export QIX backup", fields: []inputField{{label: "Output path", required: true}}}
		}
		if key.r == 'o' {
			if len(backups) == 0 {
				return fmt.Errorf("no backup selected")
			}
			name := backups[a.backupIndex].Name
			a.confirmation = &confirmation{kind: "restore-backup", prompt: "Restore " + name + " and overwrite current data? Type its filename", expected: name}
		}
	case sectionSettings:
		if key.name == "up" || key.r == 'k' {
			a.sectionScroll = max(0, a.sectionScroll-1)
		}
		if key.name == "down" || key.r == 'j' {
			a.sectionScroll++
		}
		if key.r == 'e' || key.name == "enter" {
			a.startSettingsForm()
		}
		if key.r == 'r' {
			if err := config.Init(); err != nil {
				return err
			}
			logging.SetLevel(config.Get().LogLevel)
			a.message = "Settings reloaded from disk"
		}
	}
	return nil
}

func (a *app) updateTracking(key keyEvent) error {
	if key.name == "up" || key.r == 'k' {
		a.sectionScroll = max(0, a.sectionScroll-1)
	}
	if key.name == "down" || key.r == 'j' {
		a.sectionScroll++
	}
	if key.r == 'x' {
		elapsed, path, taskID, err := a.store.StopTracking()
		if err != nil {
			return err
		}
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = fmt.Sprintf("Stopped %s/%s after %s", path, taskID, elapsed.Round(time.Second))
		return nil
	}
	if key.r == 's' {
		if a.project == nil || len(a.tasks) == 0 {
			return fmt.Errorf("select a task in Workspace first")
		}
		item := a.tasks[a.taskIndex]
		module := ""
		if item.location != "project" {
			module = item.location
		}
		active, err := a.store.GetActiveSession()
		if err != nil {
			return err
		}
		if active == nil {
			err = a.store.StartTracking(a.project.Name, module, item.task.ID)
		} else {
			err = a.store.SwitchTracking(a.project.Name, module, item.task.ID)
		}
		if err != nil {
			return err
		}
		a.message = "Tracking " + item.task.ID + " — " + item.task.Title
	}
	if key.r == 't' {
		return a.startTimeLogForm()
	}
	return nil
}

func (a *app) updateSprints(key keyEvent) error {
	if a.project == nil {
		return fmt.Errorf("select a project in Workspace first")
	}
	sprints := a.project.Sprints
	if key.name == "up" || key.r == 'k' {
		a.sprintIndex = max(0, a.sprintIndex-1)
		a.sectionScroll = max(0, a.sectionScroll-1)
	}
	if key.name == "down" || key.r == 'j' {
		a.sprintIndex = min(max(0, len(sprints)-1), a.sprintIndex+1)
		a.sectionScroll++
	}
	if key.r == 'n' {
		today := time.Now()
		a.form = &inputForm{kind: "sprint", title: "Create sprint in " + a.project.Name, fields: []inputField{
			{label: "Name", required: true},
			{label: "Start date (YYYY-MM-DD)", value: []rune(today.Format("2006-01-02")), required: true},
			{label: "End date (YYYY-MM-DD)", value: []rune(today.AddDate(0, 0, 13).Format("2006-01-02")), required: true},
		}}
	}
	if key.r == 'e' && len(sprints) > 0 {
		sprint := sprints[a.sprintIndex]
		a.form = &inputForm{kind: "edit-sprint", title: "Edit sprint " + sprint.Name, fields: []inputField{
			{label: "Name", value: []rune(sprint.Name), required: true},
			{label: "Start date (YYYY-MM-DD)", value: []rune(sprint.StartDate), required: true},
			{label: "End date (YYYY-MM-DD)", value: []rune(sprint.EndDate), required: true},
		}}
	}
	if (key.r == 'a' || key.r == 'u') && (len(sprints) == 0 || len(a.tasks) == 0) {
		return fmt.Errorf("select a sprint and a task in Workspace first")
	}
	if key.r == 'a' {
		sprint, task := sprints[a.sprintIndex], a.tasks[a.taskIndex].task
		if err := a.store.AssignTaskToSprint(a.project.Name, sprint.Name, task.ID); err != nil {
			return err
		}
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = fmt.Sprintf("Assigned %s to %s", task.ID, sprint.Name)
	}
	if key.r == 'u' {
		sprint, task := sprints[a.sprintIndex], a.tasks[a.taskIndex].task
		if err := a.store.UpdateProject(a.project.Name, func(project *models.Project) error {
			for i := range project.Sprints {
				if project.Sprints[i].Name != sprint.Name {
					continue
				}
				for j, id := range project.Sprints[i].TaskIDs {
					if id == task.ID {
						project.Sprints[i].TaskIDs = append(project.Sprints[i].TaskIDs[:j], project.Sprints[i].TaskIDs[j+1:]...)
						return nil
					}
				}
				return fmt.Errorf("task %s is not assigned to %s", task.ID, sprint.Name)
			}
			return fmt.Errorf("sprint %s not found", sprint.Name)
		}); err != nil {
			return err
		}
		if err := a.loadProject(); err != nil {
			return err
		}
		a.message = fmt.Sprintf("Unassigned %s from %s", task.ID, sprint.Name)
	}
	if key.r == 'd' && len(sprints) > 0 {
		sprint := sprints[a.sprintIndex]
		a.confirmation = &confirmation{kind: "delete-sprint", prompt: "Delete sprint " + sprint.Name + "? Type its name", expected: sprint.Name}
	}
	return nil
}

func (a *app) sectionView(width, height int) string {
	var title, footer string
	var body []string
	switch a.section {
	case sectionTracking:
		title, body, footer = "TRACKING", a.trackingLines(), " s start/switch selected task  x stop  t log time  W workspace  q quit"
	case sectionSprints:
		title, body, footer = "SPRINTS", a.sprintLines(), " ↑/↓ select  n new  e edit  a assign  u unassign  d remove  pgup/pgdn scroll"
	case sectionReports:
		title, body, footer = "REPORTS", a.reportLines(), " ←/→ report  ↑/↓ or pgup/pgdn scroll  W workspace  q quit"
	case sectionHealth:
		title, body, footer = "HEALTH", a.healthLines(), " ↑/↓ backup  pgup/pgdn scroll  b create  e export  o restore  c cleanup"
	default:
		title, body, footer = "SETTINGS", a.settingsLines(), " e/enter edit settings  r reload  ↑/↓ or pgup/pgdn scroll  q quit"
	}
	visibleHeight := max(1, height-6)
	a.sectionScroll = clamp(a.sectionScroll, 0, max(0, len(body)-visibleHeight))
	visibleBody := scrollWindow(body, a.sectionScroll, visibleHeight)
	lines := box(scrollOffsetTitle(title, a.sectionScroll, len(body), visibleHeight), visibleBody, width, height-4, true)
	var b strings.Builder
	b.WriteString(a.navigation(width, title) + "\n")
	b.WriteString(dim + fit(a.projectSummary(), width) + reset + "\n")
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	b.WriteString(dim + fit(footer, width) + reset + "\n")
	color := green
	if a.isError {
		color = red
	}
	b.WriteString(color + fit(" "+a.message, width) + reset)
	base := b.String()
	if a.form != nil || a.confirmation != nil {
		return a.modalView(base, width, height)
	}
	return base
}

func (a *app) projectSummary() string {
	if a.project == nil {
		return " No project selected — return to Workspace and select one"
	}
	return fmt.Sprintf(" %s  •  %d tasks  •  %.0f%% complete", a.project.Name, len(a.project.GetAllTasks()), a.project.GetCompletionPercentage())
}

func (a *app) trackingLines() []string {
	lines := []string{"ACTIVE TIMER"}
	active, err := a.store.GetActiveSession()
	if err != nil {
		return append(lines, "Error: "+err.Error())
	}
	if active == nil {
		lines = append(lines,
			tableRow("  ○ IDLE       No task is currently running", max(1, a.width-2), 0, false, "250"),
			"  Select a task in Workspace, then press s here to start tracking.",
		)
	} else {
		elapsed := time.Since(active.StartTime)
		projectName := strings.SplitN(active.Path, "/", 2)[0]
		title, status := active.TaskID, ""
		if task, _, findErr := a.store.FindTask(projectName, active.TaskID); findErr == nil {
			title, status = task.Title, string(task.Status)
		}
		lines = append(lines,
			tableHeader("  STATE    TASK                         LOCATION                 ELAPSED", max(1, a.width-2)),
			tableRow(fmt.Sprintf("  ● ACTIVE  %-28s %-24s %s", "["+active.TaskID+"] "+title, active.Path, elapsed.Round(time.Second)), max(1, a.width-2), 0, true, "42"),
			fmt.Sprintf("  Started %s  •  status %s  •  %.2fh running", active.StartTime.Format("2006-01-02 15:04"), emptyFallback(status, "unknown"), elapsed.Hours()),
		)
	}

	projects, err := a.store.GetAllProjects()
	if err != nil {
		return append(lines, "Error: "+err.Error())
	}
	now := time.Now()
	dates := make([]string, 7)
	daily := make(map[string]float64, 7)
	todayByProject := make(map[string]float64)
	for i := 0; i < 7; i++ {
		dates[i] = now.AddDate(0, 0, i-6).Format("2006-01-02")
	}
	for _, project := range projects {
		for _, task := range project.GetAllTasks() {
			for _, entry := range task.TimeEntries {
				if entry.Date >= dates[0] && entry.Date <= dates[6] {
					daily[entry.Date] += entry.Hours
				}
				if entry.Date == dates[6] {
					todayByProject[project.Name] += entry.Hours
				}
			}
		}
	}
	weekTotal, maxDaily := 0.0, 0.0
	for _, date := range dates {
		weekTotal += daily[date]
		maxDaily = maxFloat(maxDaily, daily[date])
	}
	lines = append(lines, "", "LAST 7 DAYS", fmt.Sprintf("  Week total %.2fh  •  daily average %.2fh  •  today %.2fh", weekTotal, weekTotal/7, daily[dates[6]]))
	for _, date := range dates {
		day, _ := time.Parse("2006-01-02", date)
		lines = append(lines, chartRow(day.Format("Mon 02"), daily[date], maxDaily, 24, fmt.Sprintf("%.2fh", daily[date]), "44"))
	}

	lines = append(lines, "", "TODAY BY PROJECT")
	if len(todayByProject) == 0 {
		lines = append(lines, "  No completed time entries today; the active timer is shown separately.")
	} else {
		lines = append(lines, tableHeader("  PROJECT                              HOURS   SHARE", max(1, a.width-2)))
		names := sortedFloatKeys(todayByProject)
		for i, name := range names {
			share := 0.0
			if daily[dates[6]] > 0 {
				share = todayByProject[name] / daily[dates[6]] * 100
			}
			row := fmt.Sprintf("  %-36s %6.2fh  %5.1f%%", name, todayByProject[name], share)
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, ""))
		}
	}
	if len(a.tasks) > 0 {
		task := a.tasks[a.taskIndex].task
		lines = append(lines, "", "READY TO TRACK", tableRow(fmt.Sprintf("  [%s] %-7s %s", task.ID, task.Status, task.Title), max(1, a.width-2), 0, false, statusColor(task.Status)))
	}
	return lines
}

func (a *app) sprintLines() []string {
	if a.project == nil {
		return []string{"Select a project in Workspace first."}
	}
	if len(a.project.Sprints) == 0 {
		return []string{"No sprints yet.", "", "Press n to create the first sprint."}
	}
	allTasks := make(map[string]models.Task)
	for _, task := range a.project.GetAllTasks() {
		allTasks[task.ID] = task
	}
	today := time.Now().Format("2006-01-02")
	activeCount, plannedCount, endedCount := 0, 0, 0
	for _, sprint := range a.project.Sprints {
		switch sprintPhase(sprint, today) {
		case "ACTIVE":
			activeCount++
		case "PLANNED":
			plannedCount++
		default:
			endedCount++
		}
	}
	lines := []string{
		fmt.Sprintf("  %d sprint(s)  •  %d active  •  %d planned  •  %d ended", len(a.project.Sprints), activeCount, plannedCount, endedCount),
		"",
		tableHeader("  STATE    SPRINT                 PERIOD                    ITEMS  DONE  PROGRESS", max(1, a.width-2)),
	}
	for i, sprint := range a.project.Sprints {
		done := 0
		for _, id := range sprint.TaskIDs {
			if task, ok := allTasks[id]; ok && task.Status == models.StatusDone {
				done++
			}
		}
		percent := 0.0
		if len(sprint.TaskIDs) > 0 {
			percent = float64(done) / float64(len(sprint.TaskIDs)) * 100
		}
		phase := sprintPhase(sprint, today)
		row := fmt.Sprintf("  %-8s %-22s %s → %s  %4d  %4d  %s %3.0f%%", phase, sprint.Name, sprint.StartDate, sprint.EndDate, len(sprint.TaskIDs), done, progressBar(percent, 10), percent)
		lines = append(lines, tableRow(row, max(1, a.width-2), i, i == a.sprintIndex, ""))
	}

	selected := a.project.Sprints[clamp(a.sprintIndex, 0, len(a.project.Sprints)-1)]
	counts := map[models.TaskStatus]int{}
	estimated, actual := 0.0, 0.0
	selectedTasks := make([]models.Task, 0, len(selected.TaskIDs))
	for _, id := range selected.TaskIDs {
		if task, ok := allTasks[id]; ok {
			selectedTasks = append(selectedTasks, task)
			counts[task.Status]++
			estimated += task.EstimatedHours
			actual += task.CalculateActualHours()
		}
	}
	lines = append(lines, "", "SELECTED SPRINT — "+selected.Name,
		fmt.Sprintf("  %s  •  %d tasks  •  %.2fh estimated  •  %.2fh actual  •  %.2fh variance", sprintPhase(selected, today), len(selectedTasks), estimated, actual, actual-estimated),
		chartRow("Todo", float64(counts[models.StatusTodo]), float64(max(1, len(selectedTasks))), 18, strconv.Itoa(counts[models.StatusTodo]), "250"),
		chartRow("Doing", float64(counts[models.StatusDoing]), float64(max(1, len(selectedTasks))), 18, strconv.Itoa(counts[models.StatusDoing]), "220"),
		chartRow("Done", float64(counts[models.StatusDone]), float64(max(1, len(selectedTasks))), 18, strconv.Itoa(counts[models.StatusDone]), "42"),
		chartRow("Blocked", float64(counts[models.StatusBlocked]), float64(max(1, len(selectedTasks))), 18, strconv.Itoa(counts[models.StatusBlocked]), "203"),
	)
	if len(selectedTasks) == 0 {
		lines = append(lines, "", "  No tasks assigned. Select a task in Workspace and press a here.")
	} else {
		lines = append(lines, "", tableHeader("  STATUS   ID        EST     ACTUAL  TITLE", max(1, a.width-2)))
		for i, task := range selectedTasks {
			row := fmt.Sprintf("  %-8s %-8s %6.2fh %6.2fh  %s", task.Status, task.ID, task.EstimatedHours, task.CalculateActualHours(), task.Title)
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, statusColor(task.Status)))
		}
	}
	if len(a.tasks) > 0 {
		lines = append(lines, "", fmt.Sprintf("Workspace task for assign/unassign: [%s] %s", a.tasks[a.taskIndex].task.ID, a.tasks[a.taskIndex].task.Title))
	}
	return lines
}

func (a *app) reportLines() []string {
	names := []string{"OVERVIEW", "DAILY", "WBS", "TIMELINE", "COMPARE"}
	lines := []string{"  " + strings.Join(markSelected(names, a.reportIndex), "  "), ""}
	if a.project == nil && a.reportIndex != 1 && a.reportIndex != 4 {
		return append(lines, "Select a project in Workspace first.")
	}
	switch a.reportIndex {
	case 0:
		tasks := a.project.GetAllTasks()
		counts := a.project.CountByStatus()
		estimated, actual := a.project.CalculateTotalEstimated(), a.project.CalculateTotalActual()
		lines = append(lines,
			"PROJECT SCORECARD",
			tableHeader("  METRIC                     VALUE        SIGNAL", max(1, a.width-2)),
			tableRow(fmt.Sprintf("  %-26s %8d      %s", "Tasks", len(tasks), progressBar(a.project.GetCompletionPercentage(), 16)), max(1, a.width-2), 0, false, ""),
			tableRow(fmt.Sprintf("  %-26s %7.2fh      %s", "Estimated", estimated, budgetSignal(actual, estimated)), max(1, a.width-2), 1, false, ""),
			tableRow(fmt.Sprintf("  %-26s %7.2fh      variance %+.2fh", "Actual", actual, actual-estimated), max(1, a.width-2), 2, false, ""),
			tableRow(fmt.Sprintf("  %-26s %7.1f%%      %d module(s)", "Complete", a.project.GetCompletionPercentage(), len(a.project.Modules)), max(1, a.width-2), 3, false, ""),
			"", "STATUS DISTRIBUTION",
			chartRow("Todo", float64(counts[models.StatusTodo]), float64(max(1, len(tasks))), 24, strconv.Itoa(counts[models.StatusTodo]), "250"),
			chartRow("Doing", float64(counts[models.StatusDoing]), float64(max(1, len(tasks))), 24, strconv.Itoa(counts[models.StatusDoing]), "220"),
			chartRow("Done", float64(counts[models.StatusDone]), float64(max(1, len(tasks))), 24, strconv.Itoa(counts[models.StatusDone]), "42"),
			chartRow("Blocked", float64(counts[models.StatusBlocked]), float64(max(1, len(tasks))), 24, strconv.Itoa(counts[models.StatusBlocked]), "203"),
			"", "MODULE PERFORMANCE",
			tableHeader("  MODULE                       TASKS  DONE  ESTIMATE  ACTUAL  COMPLETE", max(1, a.width-2)),
		)
		for i, module := range a.project.Modules {
			done, moduleEstimated, moduleActual := 0, 0.0, 0.0
			for _, task := range module.Tasks {
				if task.Status == models.StatusDone {
					done++
				}
				moduleEstimated += task.EstimatedHours
				moduleActual += task.CalculateActualHours()
			}
			completion := percent(done, len(module.Tasks))
			row := fmt.Sprintf("  %-28s %5d %5d %8.2fh %7.2fh %8.1f%%", module.Name, len(module.Tasks), done, moduleEstimated, moduleActual, completion)
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, ""))
		}
		if len(a.project.Modules) == 0 {
			lines = append(lines, "  No modules in this project.")
		}
		return lines
	case 1:
		projects, err := a.store.GetAllProjects()
		if err != nil {
			return append(lines, "Error: "+err.Error())
		}
		now := time.Now()
		dates := make([]string, 7)
		daily := make(map[string]float64, 7)
		todayByProject := make(map[string]float64)
		for i := range dates {
			dates[i] = now.AddDate(0, 0, i-6).Format("2006-01-02")
		}
		for _, project := range projects {
			for _, task := range project.GetAllTasks() {
				for _, entry := range task.TimeEntries {
					if entry.Date >= dates[0] && entry.Date <= dates[6] {
						daily[entry.Date] += entry.Hours
					}
					if entry.Date == dates[6] {
						todayByProject[project.Name] += entry.Hours
					}
				}
			}
		}
		maxHours, total := 0.0, 0.0
		for _, date := range dates {
			maxHours = maxFloat(maxHours, daily[date])
			total += daily[date]
		}
		lines = append(lines, "TIME TREND — LAST 7 DAYS", fmt.Sprintf("  Total %.2fh  •  average %.2fh/day", total, total/7))
		for _, date := range dates {
			day, _ := time.Parse("2006-01-02", date)
			lines = append(lines, chartRow(day.Format("Mon 02"), daily[date], maxHours, 28, fmt.Sprintf("%.2fh", daily[date]), "44"))
		}
		lines = append(lines, "", "TODAY BY PROJECT", tableHeader("  PROJECT                              HOURS", max(1, a.width-2)))
		for i, name := range sortedFloatKeys(todayByProject) {
			lines = append(lines, tableRow(fmt.Sprintf("  %-36s %7.2fh", name, todayByProject[name]), max(1, a.width-2), i, false, ""))
		}
		if len(todayByProject) == 0 {
			lines = append(lines, "  No time logged today.")
		}
		return lines
	case 2:
		lines = append(lines, "WORK BREAKDOWN STRUCTURE", tableHeader("  LOCATION             ID        STATUS    PRIORITY   EST    ACTUAL  TITLE", max(1, a.width-2)))
		row := 0
		appendTasks := func(location string, tasks []models.Task) {
			for _, task := range tasks {
				content := fmt.Sprintf("  %-20s %-8s %-9s %-9s %5.1fh %6.1fh  %s", location, task.ID, task.Status, task.Priority, task.EstimatedHours, task.CalculateActualHours(), task.Title)
				lines = append(lines, tableRow(content, max(1, a.width-2), row, false, statusColor(task.Status)))
				row++
			}
		}
		appendTasks("(project)", a.project.Tasks)
		for _, module := range a.project.Modules {
			appendTasks(module.Name, module.Tasks)
		}
		if row == 0 {
			lines = append(lines, "  No tasks in this project.")
		}
		return lines
	case 3:
		tasks := a.project.GetAllTasks()
		sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt) })
		lines = append(lines, "RECENT TASK ACTIVITY", tableHeader("  UPDATED           STATUS    ID        EST     ACTUAL  TITLE", max(1, a.width-2)))
		for i, task := range tasks {
			row := fmt.Sprintf("  %-16s %-9s %-8s %6.2fh %6.2fh  %s", formatTimeOrDash(task.UpdatedAt), task.Status, task.ID, task.EstimatedHours, task.CalculateActualHours(), task.Title)
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, statusColor(task.Status)))
		}
		return lines
	default:
		projects, err := a.store.GetAllProjects()
		if err != nil {
			return append(lines, "Error: "+err.Error())
		}
		lines = append(lines, "PORTFOLIO COMPARISON", tableHeader("  PROJECT                    TASKS  DONE  BLOCKED  ESTIMATE  ACTUAL  COMPLETE", max(1, a.width-2)))
		maxCompletion := 0.0
		for i, project := range projects {
			counts := project.CountByStatus()
			maxCompletion = maxFloat(maxCompletion, project.GetCompletionPercentage())
			row := fmt.Sprintf("  %-28s %5d %5d %7d %8.2fh %7.2fh %8.1f%%", project.Name, len(project.GetAllTasks()), counts[models.StatusDone], counts[models.StatusBlocked], project.CalculateTotalEstimated(), project.CalculateTotalActual(), project.GetCompletionPercentage())
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, ""))
		}
		lines = append(lines, "", "COMPLETION")
		for _, project := range projects {
			lines = append(lines, chartRow(project.Name, project.GetCompletionPercentage(), maxFloat(100, maxCompletion), 24, fmt.Sprintf("%.1f%%", project.GetCompletionPercentage()), "44"))
		}
		return lines
	}
}

func markSelected(names []string, selected int) []string {
	result := append([]string(nil), names...)
	if selected >= 0 && selected < len(result) {
		result[selected] = "[" + result[selected] + "]"
	}
	return result
}

func (a *app) startSettingsForm() {
	cfg := config.Get()
	a.form = &inputForm{kind: "settings", title: "Edit QIX settings", fields: []inputField{
		{label: "Jira base URL (blank disables)", value: []rune(cfg.JiraBaseURL)},
		{label: "Date format", value: []rune(cfg.DateFormat), required: true},
		{label: "Date-time format", value: []rune(cfg.DateTimeFormat), required: true},
		{label: "Backup retention days", value: []rune(strconv.Itoa(cfg.BackupRetentionDays)), required: true},
		{label: "Color output (true/false)", value: []rune(strconv.FormatBool(cfg.ColorOutput)), required: true},
		{label: "Log level (debug/info/warn/error)", value: []rune(cfg.LogLevel), required: true},
		{label: "Log file", value: []rune(cfg.LogFile), required: true},
	}}
}

func (a *app) submitSettings(form *inputForm) error {
	retention, err := strconv.Atoi(strings.TrimSpace(string(form.fields[3].value)))
	if err != nil {
		return fmt.Errorf("backup retention must be a whole number of days")
	}
	colorOutput, err := strconv.ParseBool(strings.TrimSpace(string(form.fields[4].value)))
	if err != nil {
		return fmt.Errorf("color output must be true or false")
	}
	settings := config.Settings{
		JiraBaseURL:         strings.TrimSpace(string(form.fields[0].value)),
		DateFormat:          strings.TrimSpace(string(form.fields[1].value)),
		DateTimeFormat:      strings.TrimSpace(string(form.fields[2].value)),
		BackupRetentionDays: retention,
		ColorOutput:         colorOutput,
		LogLevel:            strings.TrimSpace(string(form.fields[5].value)),
		LogFile:             strings.TrimSpace(string(form.fields[6].value)),
	}
	if err := config.SaveSettings(settings); err != nil {
		return err
	}
	logging.SetLevel(config.Get().LogLevel)
	a.form = nil
	a.message = "Settings saved to " + config.Get().ConfigFile
	return nil
}

func (a *app) settingsLines() []string {
	cfg := config.Get()
	width := max(1, a.width-2)
	lines := []string{
		"EDITABLE CONFIGURATION",
		tableHeader("  SETTING                    EFFECTIVE VALUE                                SOURCE", width),
	}
	settings := []struct{ name, value, env string }{
		{"Jira base URL", emptyFallback(cfg.JiraBaseURL, "(disabled)"), "JIRA_BASE_URL"},
		{"Date format", cfg.DateFormat, ""},
		{"Date-time format", cfg.DateTimeFormat, ""},
		{"Backup retention", fmt.Sprintf("%d days", cfg.BackupRetentionDays), ""},
		{"Color output", strconv.FormatBool(cfg.ColorOutput), ""},
		{"Log level", cfg.LogLevel, "QIX_LOG_LEVEL"},
		{"Log file", cfg.LogFile, "QIX_LOG_FILE"},
	}
	for i, setting := range settings {
		source := "config"
		if setting.env != "" && os.Getenv(setting.env) != "" {
			source = setting.env
		}
		row := fmt.Sprintf("  %-26s %-46s %s", setting.name, setting.value, source)
		lines = append(lines, tableRow(row, width, i, false, ""))
	}
	lines = append(lines,
		"",
		"STORAGE PATHS",
		tableHeader("  RESOURCE                   PATH", width),
		tableRow(fmt.Sprintf("  %-26s %s", "QIX data", cfg.QixDir), width, 0, false, ""),
		tableRow(fmt.Sprintf("  %-26s %s", "Projects", cfg.ProjectsDir), width, 1, false, ""),
		tableRow(fmt.Sprintf("  %-26s %s", "Backups", cfg.BackupDir), width, 2, false, ""),
		tableRow(fmt.Sprintf("  %-26s %s", "Config file", cfg.ConfigFile), width, 3, false, ""),
		"",
		"Environment values shown in SOURCE override the saved file when QIX restarts.",
		"Press e or Enter to edit every configurable value in one scrollable modal.",
	)
	return lines
}

func chartRow(label string, value, maximum float64, width int, display, color string) string {
	if maximum <= 0 {
		maximum = 1
	}
	ratio := value / maximum
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	bar := "\x1b[38;5;" + color + "m" + strings.Repeat("█", filled) + reset + dim + strings.Repeat("░", width-filled) + reset
	return fmt.Sprintf("  %-14s %s  %s", label, bar, display)
}

func sortedFloatKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if values[keys[i]] == values[keys[j]] {
			return keys[i] < keys[j]
		}
		return values[keys[i]] > values[keys[j]]
	})
	return keys
}

func sprintPhase(sprint models.Sprint, today string) string {
	if today < sprint.StartDate {
		return "PLANNED"
	}
	if today > sprint.EndDate {
		return "ENDED"
	}
	return "ACTIVE"
}

func formatTimeOrDash(value time.Time) string {
	if value.IsZero() {
		return "—"
	}
	return value.Format("2006-01-02 15:04")
}

func budgetSignal(actual, estimated float64) string {
	if estimated <= 0 {
		return "no estimate"
	}
	if actual > estimated {
		return red + "over budget" + reset
	}
	return green + "within budget" + reset
}

func percent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func (a *app) healthLines() []string {
	cfg := config.Get()
	lines := []string{"QIX DOCTOR", ""}
	issues, warnings := 0, 0
	for _, item := range []struct{ name, path string }{{"Data directory", cfg.QixDir}, {"Projects directory", cfg.ProjectsDir}, {"Backup directory", cfg.BackupDir}} {
		if _, err := os.Stat(item.path); err != nil {
			lines = append(lines, red+"✗ "+item.name+": "+err.Error()+reset)
			issues++
		} else {
			lines = append(lines, green+"✓ "+item.name+reset+"  "+item.path)
		}
	}
	projects, err := a.store.ListProjects()
	if err != nil {
		lines = append(lines, red+"✗ Projects: "+err.Error()+reset)
		issues++
	} else {
		lines = append(lines, fmt.Sprintf("%s✓ Project files%s  %d valid", green, reset, len(projects)))
		for _, name := range projects {
			if _, err := a.store.LoadProject(name); err != nil {
				lines = append(lines, red+"  ✗ "+name+": "+err.Error()+reset)
				issues++
			}
		}
	}
	if err := a.store.EnsureIndexFresh(); err != nil {
		lines = append(lines, red+"✗ Index refresh: "+err.Error()+reset)
		issues++
	} else if errs, err := a.store.ValidateIndex(); err != nil {
		lines = append(lines, red+"✗ Index: "+err.Error()+reset)
		issues++
	} else if len(errs) > 0 {
		lines = append(lines, yellow+fmt.Sprintf("! Index has %d inconsistencies", len(errs))+reset)
		warnings += len(errs)
	} else {
		lines = append(lines, green+"✓ Task index is consistent"+reset)
	}
	orphans := 0
	for _, name := range projects {
		refs, err := a.store.FindOrphanedReferences(name)
		if err != nil {
			continue
		}
		for _, values := range refs {
			orphans += len(values)
		}
	}
	if orphans > 0 {
		lines = append(lines, yellow+fmt.Sprintf("! %d orphaned task references", orphans)+reset)
		warnings += orphans
	} else {
		lines = append(lines, green+"✓ No orphaned task references"+reset)
	}
	lines = append(lines, "", fmt.Sprintf("Result: %d issue(s), %d warning(s)", issues, warnings), "QIX 2.0.0  •  Bubble Tea terminal interface")
	lines = append(lines, "", "BACKUPS")
	backups, err := a.store.ListBackups()
	if err != nil {
		lines = append(lines, red+"✗ "+err.Error()+reset)
	} else if len(backups) == 0 {
		lines = append(lines, "No backups yet. Press b to create one.")
	} else {
		lines = append(lines, tableHeader("  BACKUP                                 SIZE       CREATED", max(1, a.width-2)))
		for i, backup := range backups {
			row := fmt.Sprintf("  %-38s %8.2f MB  %s", backup.Name, float64(backup.Size)/1024/1024, backup.ModTime.Format("2006-01-02 15:04"))
			lines = append(lines, tableRow(row, max(1, a.width-2), i, i == a.backupIndex, ""))
		}
	}
	return lines
}
