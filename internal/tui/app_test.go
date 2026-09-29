package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/models"
	"github.com/mrbooshehri/qix-go/internal/storage"
)

func TestSplitCommaSeparated(t *testing.T) {
	got := splitCommaSeparated("backend, urgent, ,api")
	want := []string{"backend", "urgent", "api"}
	if len(got) != len(want) {
		t.Fatalf("splitCommaSeparated() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitCommaSeparated()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProjectDetailLinesIncludeProjectKPIs(t *testing.T) {
	project := &models.Project{
		Name:        "launch",
		Description: "Ship the release",
		Tags:        []string{"release", "backend"},
		CreatedAt:   time.Date(2026, time.September, 28, 10, 30, 0, 0, time.UTC),
		Modules:     []models.Module{{Name: "api"}},
		Sprints:     []models.Sprint{{Name: "v1"}},
		Tasks: []models.Task{
			{ID: "one", Title: "Build", Status: models.StatusDone, EstimatedHours: 2, TimeEntries: []models.TimeEntry{{Hours: 1.5}}},
			{ID: "two", Title: "Test", Status: models.StatusDoing, EstimatedHours: 1},
		},
	}
	a := &app{project: project, focus: 0}
	joined := strings.Join(a.projectDetailLines(), "\n")
	for _, expected := range []string{
		"launch",
		"Tasks: 2    Modules: 1    Sprints: 1",
		"Todo: 0    Doing: 1    Done: 1    Blocked: 0",
		"Estimated: 3.00h    Actual: 1.50h",
		"Completion",
		"Description: Ship the release",
		"Tags: release, backend",
		"Modules: api",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("project details missing %q:\n%s", expected, joined)
		}
	}
}

func TestProgressBarClampsPercentage(t *testing.T) {
	if got := progressBar(-20, 5); !strings.Contains(got, "▱▱▱▱▱") || strings.Contains(got, "#") {
		t.Fatalf("negative progress = %q", got)
	}
	if got := progressBar(120, 5); !strings.Contains(got, "▰▰▰▰▰") || strings.Contains(got, "-") {
		t.Fatalf("overflow progress = %q", got)
	}
}

func TestTaskTableIncludesCreatedAndUpdatedDates(t *testing.T) {
	task := models.Task{
		ID:        "abcdef12",
		Title:     "Build dashboard",
		Status:    models.StatusDoing,
		CreatedAt: time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.September, 29, 11, 0, 0, 0, time.UTC),
	}
	wide := taskTableHeader(80) + "\n" + taskTableRow("[ ]", task, 80)
	for _, expected := range []string{"CREATED", "UPDATED", "2026-09-28", "2026-09-29"} {
		if !strings.Contains(wide, expected) {
			t.Errorf("wide task table missing %q: %s", expected, wide)
		}
	}
	compact := taskTableHeader(50) + "\n" + taskTableRow("[ ]", task, 50)
	for _, expected := range []string{"CRTD", "UPDTD", "09-28", "09-29", "DNG"} {
		if !strings.Contains(compact, expected) {
			t.Errorf("compact task table missing %q: %s", expected, compact)
		}
	}
}

func TestProjectCreateAndDeleteWorkflow(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := storage.Init(); err != nil {
		t.Fatalf("storage.Init() error: %v", err)
	}

	a := &app{store: storage.Get()}
	a.startProjectForm()
	a.form.fields[0].value = []rune("launch")
	a.form.fields[1].value = []rune("Ship the release")
	a.form.fields[2].value = []rune("release, backend")
	if err := a.submitForm(); err != nil {
		t.Fatalf("submitForm() error: %v", err)
	}

	project, err := a.store.LoadProject("launch")
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if project.Description != "Ship the release" {
		t.Errorf("description = %q", project.Description)
	}
	if got := strings.Join(project.Tags, ","); got != "release,backend" {
		t.Errorf("tags = %q", got)
	}

	a.confirmation = &confirmation{kind: "delete-project", expected: "launch", input: []rune("launch")}
	if err := a.updateConfirmation(keyEvent{name: "enter"}); err != nil {
		t.Fatalf("updateConfirmation() error: %v", err)
	}
	if a.store.ProjectExists("launch") {
		t.Fatal("project still exists after confirmed deletion")
	}
}

func TestModuleCreateEditAndDeleteWorkflow(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := storage.Init(); err != nil {
		t.Fatalf("storage.Init() error: %v", err)
	}
	store := storage.Get()
	if _, err := store.CreateProject("launch", "", nil); err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}

	a := &app{store: store, moduleIndex: -1}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatalf("loadProjects() error: %v", err)
	}
	a.startModuleForm()
	a.form.fields[0].value = []rune("api")
	a.form.fields[1].value = []rune("Backend API")
	a.form.fields[2].value = []rune("backend, critical")
	if err := a.submitForm(); err != nil {
		t.Fatalf("create module submitForm() error: %v", err)
	}

	module, err := store.GetModule("launch", "api")
	if err != nil {
		t.Fatalf("GetModule() error: %v", err)
	}
	if module.Description != "Backend API" || strings.Join(module.Tags, ",") != "backend,critical" {
		t.Fatalf("created module = %#v", module)
	}

	if err := store.AddTask("launch", "api", models.Task{Title: "Build endpoint", Status: models.StatusDone, EstimatedHours: 3}); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}
	if err := a.loadProject(); err != nil {
		t.Fatalf("loadProject() error: %v", err)
	}
	if len(a.tasks) != 1 || a.tasks[0].location != "api" {
		t.Fatalf("module task scope = %#v", a.tasks)
	}
	if details := strings.Join(a.moduleDetailLines(), "\n"); !strings.Contains(details, "Completion") || !strings.Contains(details, "100.0%") {
		t.Fatalf("module details missing completion KPI:\n%s", details)
	}

	a.startModuleEditForm()
	a.form.fields[0].value = []rune("service")
	a.form.fields[1].value = []rune("Public service")
	if err := a.submitForm(); err != nil {
		t.Fatalf("edit module submitForm() error: %v", err)
	}
	if _, err := store.GetModule("launch", "service"); err != nil {
		t.Fatalf("renamed module not found: %v", err)
	}

	a.confirmation = &confirmation{kind: "delete-module", expected: "service", input: []rune("service")}
	if err := a.updateConfirmation(keyEvent{name: "enter"}); err != nil {
		t.Fatalf("delete module confirmation error: %v", err)
	}
	if _, err := store.GetModule("launch", "service"); err == nil {
		t.Fatal("module still exists after confirmed removal")
	}
}

func TestTaskCreateAndEditWorkflowUsesFullTaskFields(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := storage.Init(); err != nil {
		t.Fatalf("storage.Init() error: %v", err)
	}
	store := storage.Get()
	if _, err := store.CreateProject("launch", "", nil); err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}

	a := &app{store: store, moduleIndex: -1, width: 100, height: 30}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatalf("loadProjects() error: %v", err)
	}
	a.startTaskForm()
	values := []string{"Build API", "Public endpoint", "doing", "high", "3.5", "backend, urgent", "QIX-42"}
	for i, value := range values {
		a.form.fields[i].value = []rune(value)
	}
	if err := a.submitForm(); err != nil {
		t.Fatalf("create task submitForm() error: %v", err)
	}
	if len(a.tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(a.tasks))
	}
	created := a.tasks[0].task
	if created.Description != "Public endpoint" || created.Status != models.StatusDoing || created.Priority != models.PriorityHigh || created.EstimatedHours != 3.5 || created.JiraIssue != "QIX-42" {
		t.Fatalf("created task fields = %#v", created)
	}

	a.startTaskEditForm()
	a.form.fields[0].value = []rune("Build stable API")
	a.form.fields[2].value = []rune("done")
	a.form.fields[5].value = []rune("2.25")
	if err := a.submitForm(); err != nil {
		t.Fatalf("edit task submitForm() error: %v", err)
	}
	updated, _, err := store.FindTask("launch", created.ID)
	if err != nil {
		t.Fatalf("FindTask() error: %v", err)
	}
	if updated.Title != "Build stable API" || updated.Status != models.StatusDone || updated.CalculateActualHours() != 2.25 {
		t.Fatalf("updated task = %#v", updated)
	}
}

func TestReconcileActualHoursAdjustsNewestEntries(t *testing.T) {
	entries := []models.TimeEntry{{Date: "2026-09-28", Hours: 1}, {Date: "2026-09-29", Hours: 2}}
	updated := reconcileActualHours(entries, 1.5)
	if len(updated) != 2 || updated[0].Hours != 1 || updated[1].Hours != 0.5 {
		t.Fatalf("reconciled entries = %#v", updated)
	}
	if increased := reconcileActualHours(updated, 2.25); len(increased) != 3 || increased[2].Hours != 0.75 {
		t.Fatalf("increased entries = %#v", increased)
	}
}

func TestTaskDetailsShowTimeLogTable(t *testing.T) {
	a := &app{width: 100}
	task := models.Task{ID: "t1", Title: "Build", EstimatedHours: 4, TimeEntries: []models.TimeEntry{{Date: "2026-09-29", Hours: 1.5}}}
	details := strings.Join(a.taskDetailLines(taskItem{task: task, location: "project"}), "\n")
	for _, expected := range []string{"TIME LOG", "DATE", "2026-09-29", "1.50h", "Press e here to edit"} {
		if !strings.Contains(details, expected) {
			t.Errorf("task details missing %q:\n%s", expected, details)
		}
	}
}

func TestTaskMultiSelectChangesStatusAcrossScopes(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Init(); err != nil {
		t.Fatal(err)
	}
	store := storage.Get()
	if _, err := store.CreateProject("launch", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask("launch", "", models.Task{ID: "project-task", Title: "Project task"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddModule("launch", models.Module{Name: "api"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddTask("launch", "api", models.Task{ID: "module-task", Title: "Module task"}); err != nil {
		t.Fatal(err)
	}

	a := &app{store: store, moduleIndex: -1, focus: 2}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.updateKey(keyEvent{name: "space", r: ' '}); err != nil {
		t.Fatal(err)
	}
	a.moduleIndex = 0
	if err := a.loadTasks(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.updateKey(keyEvent{name: "space", r: ' '}); err != nil {
		t.Fatal(err)
	}
	if a.selectedTaskCount() != 2 {
		t.Fatalf("selected = %d, want 2", a.selectedTaskCount())
	}
	if _, err := a.updateKey(keyEvent{r: '3'}); err != nil {
		t.Fatal(err)
	}
	if a.selectedTaskCount() != 0 {
		t.Fatalf("selection not cleared after bulk status: %#v", a.selectedTasks)
	}
	for _, id := range []string{"project-task", "module-task"} {
		task, _, err := store.FindTask("launch", id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != models.StatusDone {
			t.Fatalf("task %s status = %s, want done", id, task.Status)
		}
	}
}

func TestTaskDeleteConfirmsWithEnterAndSupportsBulkSelection(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Init(); err != nil {
		t.Fatal(err)
	}
	store := storage.Get()
	if _, err := store.CreateProject("launch", "", nil); err != nil {
		t.Fatal(err)
	}
	for _, task := range []models.Task{{ID: "one", Title: "One"}, {ID: "two", Title: "Two"}} {
		if err := store.AddTask("launch", "", task); err != nil {
			t.Fatal(err)
		}
	}

	a := &app{store: store, moduleIndex: -1, focus: 2, width: 100, height: 30}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.updateKey(keyEvent{r: 'a'}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.updateKey(keyEvent{r: 'd'}); err != nil {
		t.Fatal(err)
	}
	if a.confirmation == nil || a.confirmation.expected != "" || len(a.confirmation.taskIDs) != 2 {
		t.Fatalf("bulk confirmation = %#v", a.confirmation)
	}
	if view := stripANSI(a.View()); !strings.Contains(view, "enter confirms") || strings.Contains(view, "type the exact value") {
		t.Fatalf("task confirmation still asks for typed ID:\n%s", view)
	}
	if err := a.updateConfirmation(keyEvent{name: "enter"}); err != nil {
		t.Fatal(err)
	}
	if len(a.project.GetAllTasks()) != 0 {
		t.Fatalf("tasks remain after bulk delete: %#v", a.project.GetAllTasks())
	}
}

func TestFormsRenderAsCenteredModal(t *testing.T) {
	a := &app{width: 100, height: 30}
	a.projects = []string{"background-project"}
	a.form = &inputForm{kind: "project", title: "Create project", fields: []inputField{
		{label: "Name", value: []rune("launch")},
		{label: "Description", value: []rune("Visible in the same modal")},
		{label: "Tags", value: []rune("release")},
	}}

	view := a.View()
	for _, expected := range []string{"Create project", "launch", "Description", "Visible in the same modal", "Tags", "release", "background-project"} {
		if !strings.Contains(stripANSI(view), expected) {
			t.Fatalf("modal missing %q:\n%s", expected, view)
		}
	}
	if strings.Contains(stripANSI(view), "····") {
		t.Fatalf("modal still replaces background with dots:\n%s", view)
	}
	if !strings.Contains(view, "Create project") || !strings.Contains(view, "launch") {
		t.Fatalf("modal content missing:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	firstContent := -1
	for i, line := range lines {
		if strings.Contains(stripANSI(line), "Create project") {
			firstContent = i
			break
		}
	}
	if firstContent < 5 || firstContent > 15 {
		t.Fatalf("modal title line = %d, want vertically centered", firstContent)
	}
}

func TestFormNavigationKeepsFieldsInOneForm(t *testing.T) {
	a := &app{form: &inputForm{fields: []inputField{{label: "One"}, {label: "Two"}, {label: "Three"}}}}
	if err := a.updateForm(keyEvent{name: "down"}); err != nil {
		t.Fatal(err)
	}
	if a.form == nil || a.form.index != 1 {
		t.Fatalf("form after down = %#v", a.form)
	}
	if err := a.updateForm(keyEvent{name: "up"}); err != nil {
		t.Fatal(err)
	}
	if a.form == nil || a.form.index != 0 {
		t.Fatalf("form after up = %#v", a.form)
	}
}

func TestProjectEditCanRenameProject(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Init(); err != nil {
		t.Fatal(err)
	}
	store := storage.Get()
	if _, err := store.CreateProject("launch", "old", []string{"one"}); err != nil {
		t.Fatal(err)
	}
	a := &app{store: store, moduleIndex: -1}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatal(err)
	}
	a.startProjectEditForm()
	a.form.fields[0].value = []rune("release")
	a.form.fields[1].value = []rune("new description")
	a.form.fields[2].value = []rune("two, three")
	if err := a.submitForm(); err != nil {
		t.Fatalf("submitForm() error: %v", err)
	}
	if store.ProjectExists("launch") || !store.ProjectExists("release") {
		t.Fatalf("project paths after edit: launch=%v release=%v", store.ProjectExists("launch"), store.ProjectExists("release"))
	}
	if a.project == nil || a.project.Name != "release" || a.project.Description != "new description" {
		t.Fatalf("selected project after rename = %#v", a.project)
	}
}

func TestDetailPaneScrollsVertically(t *testing.T) {
	a := &app{
		focus:   3,
		height:  22,
		project: &models.Project{Name: "launch", Description: strings.Repeat("details ", 20)},
	}
	a.showHelp = true
	if err := a.move(3); err != nil {
		t.Fatal(err)
	}
	if a.detailScroll == 0 {
		t.Fatal("detail pane did not scroll")
	}
}

func TestDashboardSectionsRenderTablesChartsAndConsistentFooters(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := storage.Init(); err != nil {
		t.Fatal(err)
	}
	store := storage.Get()
	project, err := store.CreateProject("launch", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	project.Tasks = []models.Task{
		{ID: "one", Title: "Build", Status: models.StatusDoing, Priority: models.PriorityHigh, EstimatedHours: 4, UpdatedAt: now, TimeEntries: []models.TimeEntry{{Date: now.Format("2006-01-02"), Hours: 1.5}}},
		{ID: "two", Title: "Ship", Status: models.StatusDone, Priority: models.PriorityMedium, EstimatedHours: 2, UpdatedAt: now.Add(-time.Hour)},
	}
	project.Modules = []models.Module{{Name: "api", Tasks: []models.Task{{ID: "three", Title: "Test", Status: models.StatusBlocked, Priority: models.PriorityHigh}}}}
	project.Sprints = []models.Sprint{{Name: "release", StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"), EndDate: now.AddDate(0, 0, 5).Format("2006-01-02"), TaskIDs: []string{"one", "two", "three"}}}
	if err := store.SaveProject("launch", project); err != nil {
		t.Fatal(err)
	}

	a := &app{store: store, moduleIndex: -1, width: 120, height: 32}
	if err := a.loadProjects("launch"); err != nil {
		t.Fatal(err)
	}
	for name, view := range map[string]string{
		"tracking": strings.Join(a.trackingLines(), "\n"),
		"sprints":  strings.Join(a.sprintLines(), "\n"),
		"overview": strings.Join(a.reportLines(), "\n"),
	} {
		plain := stripANSI(view)
		for _, expected := range map[string][]string{
			"tracking": {"LAST 7 DAYS", "TODAY BY PROJECT", "READY TO TRACK", "█"},
			"sprints":  {"SELECTED SPRINT", "STATUS", "EST", "ACTUAL", "█"},
			"overview": {"PROJECT SCORECARD", "STATUS DISTRIBUTION", "MODULE PERFORMANCE", "█"},
		}[name] {
			if !strings.Contains(plain, expected) {
				t.Errorf("%s dashboard missing %q:\n%s", name, expected, plain)
			}
		}
	}
	for index, expected := range []string{"PROJECT SCORECARD", "TIME TREND", "WORK BREAKDOWN STRUCTURE", "RECENT TASK ACTIVITY", "PORTFOLIO COMPARISON"} {
		a.reportIndex = index
		if report := stripANSI(strings.Join(a.reportLines(), "\n")); !strings.Contains(report, expected) {
			t.Errorf("report %d missing %q:\n%s", index, expected, report)
		}
	}
	a.reportIndex = 0

	for _, section := range []int{sectionWorkspace, sectionTracking, sectionSprints, sectionReports, sectionHealth, sectionSettings} {
		a.section = section
		view := a.View()
		lines := strings.Split(view, "\n")
		if len(lines) != a.height {
			t.Errorf("section %d rendered %d lines, want %d", section, len(lines), a.height)
		}
		if section != sectionWorkspace {
			hint := lines[len(lines)-2]
			if !strings.Contains(hint, dim) || strings.Contains(hint, green) {
				t.Errorf("section %d hint styling is not dim: %q", section, hint)
			}
		}
	}
}

func TestSettingsFormPersistsValues(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	t.Setenv("JIRA_BASE_URL", "")
	t.Setenv("QIX_LOG_LEVEL", "")
	t.Setenv("QIX_LOG_FILE", "")
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	a := &app{width: 110, height: 30}
	a.startSettingsForm()
	values := []string{"https://jira.example.com/browse", "2006-01-02", "2006-01-02 15:04", "21", "false", "warn", config.Get().QixDir + "/qix-custom.log"}
	for i, value := range values {
		a.form.fields[i].value = []rune(value)
	}
	if err := a.submitForm(); err != nil {
		t.Fatalf("submit settings: %v", err)
	}
	cfg := config.Get()
	if cfg.JiraBaseURL != values[0] || cfg.BackupRetentionDays != 21 || cfg.ColorOutput || cfg.LogLevel != "warn" {
		t.Fatalf("saved settings = %#v", cfg)
	}
	a.section = sectionSettings
	plain := stripANSI(a.View())
	for _, expected := range []string{"SETTINGS", "Jira base URL", values[0], "STORAGE PATHS"} {
		if !strings.Contains(plain, expected) {
			t.Errorf("settings view missing %q:\n%s", expected, plain)
		}
	}
	navigation := stripANSI(a.navigation(110, "SETTINGS"))
	if !strings.Contains(navigation, "[G Settings]") || strings.Contains(navigation, "[S Sprints]") {
		t.Fatalf("settings navigation selection is ambiguous: %s", navigation)
	}
}

func TestRecurrenceParsing(t *testing.T) {
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC) // Monday
	recurrence, err := parseRecurrence("weekly:friday", now)
	if err != nil {
		t.Fatalf("parseRecurrence() error: %v", err)
	}
	if recurrence.Type != models.RecurWeekly || recurrence.NextDue != "2026-10-02" {
		t.Fatalf("recurrence = %#v", recurrence)
	}
	if _, err := parseRecurrence("monthly:40", now); err == nil {
		t.Fatal("invalid monthly recurrence accepted")
	}
}
