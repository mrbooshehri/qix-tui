package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mrbooshehri/qix-go/internal/config"
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
	items := []string{"W Workspace", "T Tracking", "S Sprints", "R Reports", "H Health"}
	for i := range items {
		if strings.Contains(items[i], active[:1]) {
			items[i] = cyan + bold + "[" + items[i] + "]" + reset
		} else {
			items[i] = dim + " " + items[i] + " " + reset
		}
	}
	return fit(" QIX / "+active+"  "+strings.Join(items, "  "), width)
}

func (a *app) updateSection(key keyEvent) error {
	visible := max(1, a.height-5)
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
	default:
		title, body, footer = "HEALTH", a.healthLines(), " ↑/↓ backup  pgup/pgdn scroll  b create  e export  o restore  c cleanup"
	}
	visibleHeight := max(1, height-5)
	a.sectionScroll = clamp(a.sectionScroll, 0, max(0, len(body)-visibleHeight))
	visibleBody := scrollWindow(body, a.sectionScroll, visibleHeight)
	lines := box(scrollOffsetTitle(title, a.sectionScroll, len(body), visibleHeight), visibleBody, width, height-3, true)
	var b strings.Builder
	b.WriteString(a.navigation(width, title) + "\n")
	b.WriteString(dim + fit(a.projectSummary(), width) + reset + "\n")
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	color := green
	if a.isError {
		color = red
	}
	status := footer
	if a.message != "" {
		status = a.message + "  • " + footer
	}
	b.WriteString(color + fit(status, width) + reset)
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
	lines := []string{"TIME TRACKING", ""}
	active, err := a.store.GetActiveSession()
	if err != nil {
		return append(lines, "Error: "+err.Error())
	}
	if active == nil {
		lines = append(lines, "No active session.")
	} else {
		lines = append(lines,
			green+bold+"● ACTIVE"+reset,
			fmt.Sprintf("Task: %s / %s", active.Path, active.TaskID),
			"Started: "+active.StartTime.Format("2006-01-02 15:04:05"),
			"Elapsed: "+time.Since(active.StartTime).Round(time.Second).String(),
		)
	}
	lines = append(lines, "", "TODAY")
	entries, err := a.store.GetTimeEntriesForDate(time.Now().Format("2006-01-02"))
	if err != nil {
		return append(lines, "Error: "+err.Error())
	}
	total := 0.0
	for project, projectEntries := range entries {
		hours := 0.0
		for _, entry := range projectEntries {
			hours += entry.Hours
		}
		total += hours
		lines = append(lines, fmt.Sprintf("%-24s %7.2fh", project, hours))
	}
	lines = append(lines, "", fmt.Sprintf("Total logged today: %.2fh", total))
	if len(a.tasks) > 0 {
		lines = append(lines, "", fmt.Sprintf("Workspace selection: [%s] %s", a.tasks[a.taskIndex].task.ID, a.tasks[a.taskIndex].task.Title))
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
	lines := []string{tableHeader("  SPRINT                    PERIOD                    TASKS   DONE   PROGRESS", max(1, a.width-2))}
	for i, sprint := range a.project.Sprints {
		done := 0
		for _, id := range sprint.TaskIDs {
			if task, _, err := a.store.FindTask(a.project.Name, id); err == nil && task.Status == models.StatusDone {
				done++
			}
		}
		percent := 0.0
		if len(sprint.TaskIDs) > 0 {
			percent = float64(done) / float64(len(sprint.TaskIDs)) * 100
		}
		row := fmt.Sprintf("  %-24s %s → %s   %3d    %3d   %s %.0f%%", sprint.Name, sprint.StartDate, sprint.EndDate, len(sprint.TaskIDs), done, progressBar(percent, 12), percent)
		lines = append(lines, tableRow(row, max(1, a.width-2), i, i == a.sprintIndex, ""))
	}
	if len(a.tasks) > 0 {
		lines = append(lines, "", fmt.Sprintf("Task used by assign/unassign: [%s] %s", a.tasks[a.taskIndex].task.ID, a.tasks[a.taskIndex].task.Title))
	}
	return lines
}

func (a *app) reportLines() []string {
	names := []string{"OVERVIEW", "DAILY", "WBS", "TIMELINE", "COMPARE"}
	lines := []string{"Reports  " + strings.Join(markSelected(names, a.reportIndex), "  "), ""}
	if a.project == nil && a.reportIndex != 1 && a.reportIndex != 4 {
		return append(lines, "Select a project in Workspace first.")
	}
	switch a.reportIndex {
	case 0:
		return append(lines, a.projectDetailLines()...)
	case 1:
		date := time.Now().Format("2006-01-02")
		entries, err := a.store.GetTimeEntriesForDate(date)
		if err != nil {
			return append(lines, "Error: "+err.Error())
		}
		lines = append(lines, "Time logged on "+date, "")
		for project, list := range entries {
			hours := 0.0
			for _, entry := range list {
				hours += entry.Hours
			}
			lines = append(lines, fmt.Sprintf("%-28s %.2fh", project, hours))
		}
		return lines
	case 2:
		lines = append(lines, a.project.Name, "├─ project tasks")
		for _, task := range a.project.Tasks {
			lines = append(lines, fmt.Sprintf("│  └─ [%s] %s (%s)", task.ID, task.Title, task.Status))
		}
		for _, module := range a.project.Modules {
			lines = append(lines, "├─ "+module.Name)
			for _, task := range module.Tasks {
				lines = append(lines, fmt.Sprintf("│  └─ [%s] %s (%s)", task.ID, task.Title, task.Status))
			}
		}
		return lines
	case 3:
		lines = append(lines, "Recent activity", "")
		for _, task := range a.project.GetAllTasks() {
			lines = append(lines, fmt.Sprintf("%s  %-8s  [%s] %s", task.UpdatedAt.Format("2006-01-02"), task.Status, task.ID, task.Title))
		}
		return lines
	default:
		projects, err := a.store.GetAllProjects()
		if err != nil {
			return append(lines, "Error: "+err.Error())
		}
		lines = append(lines, tableHeader("  PROJECT                    TASKS  DONE  ESTIMATE  ACTUAL  COMPLETE", max(1, a.width-2)))
		for i, project := range projects {
			counts := project.CountByStatus()
			row := fmt.Sprintf("  %-28s %5d %5d %8.2fh %7.2fh %8.1f%%", project.Name, len(project.GetAllTasks()), counts[models.StatusDone], project.CalculateTotalEstimated(), project.CalculateTotalActual(), project.GetCompletionPercentage())
			lines = append(lines, tableRow(row, max(1, a.width-2), i, false, ""))
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
