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
		"Completion: [##########----------] 50.0%",
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
	if got := progressBar(-20, 5); got != "[-----]" {
		t.Fatalf("negative progress = %q", got)
	}
	if got := progressBar(120, 5); got != "[#####]" {
		t.Fatalf("overflow progress = %q", got)
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
	if details := strings.Join(a.moduleDetailLines(), "\n"); !strings.Contains(details, "Completion: [####################] 100.0%") {
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
	if err := a.submitForm(); err != nil {
		t.Fatalf("edit task submitForm() error: %v", err)
	}
	updated, _, err := store.FindTask("launch", created.ID)
	if err != nil {
		t.Fatalf("FindTask() error: %v", err)
	}
	if updated.Title != "Build stable API" || updated.Status != models.StatusDone {
		t.Fatalf("updated task = %#v", updated)
	}
}

func TestFormsRenderAsCenteredModal(t *testing.T) {
	a := &app{width: 100, height: 30}
	a.form = &inputForm{kind: "project", title: "Create project", fields: []inputField{{label: "Name", value: []rune("launch")}}}

	view := a.View()
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
