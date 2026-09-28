package storage

import (
	"testing"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/models"
)

func TestRemoveTasksCleansRelationshipsAndSprintAssignments(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	store := Get()
	if _, err := store.CreateProject("launch", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProject("launch", func(project *models.Project) error {
		project.Tasks = []models.Task{
			{ID: "parent", Title: "Parent", Status: models.StatusTodo},
			{ID: "keeper", Title: "Keeper", Status: models.StatusTodo, ParentID: "parent", Dependencies: []string{"module-task", "parent", "other"}},
		}
		project.Modules = []models.Module{{
			Name:  "api",
			Tasks: []models.Task{{ID: "module-task", Title: "Module task", Status: models.StatusDoing}},
		}}
		project.Sprints = []models.Sprint{{Name: "sprint-1", TaskIDs: []string{"parent", "module-task", "keeper"}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := store.RemoveTasks("launch", []string{"parent", "module-task", "parent"})
	if err != nil {
		t.Fatalf("RemoveTasks() error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	project, err := store.LoadProject("launch")
	if err != nil {
		t.Fatal(err)
	}
	if len(project.Tasks) != 1 || project.Tasks[0].ID != "keeper" || len(project.Modules[0].Tasks) != 0 {
		t.Fatalf("tasks after removal = project:%#v modules:%#v", project.Tasks, project.Modules)
	}
	keeper := project.Tasks[0]
	if keeper.ParentID != "" {
		t.Fatalf("keeper parent = %q, want empty", keeper.ParentID)
	}
	if len(keeper.Dependencies) != 1 || keeper.Dependencies[0] != "other" {
		t.Fatalf("keeper dependencies = %#v, want [other]", keeper.Dependencies)
	}
	if got := project.Sprints[0].TaskIDs; len(got) != 1 || got[0] != "keeper" {
		t.Fatalf("sprint task IDs = %#v, want [keeper]", got)
	}
}

func TestUpdateTasksStatusAcrossProjectAndModules(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatal(err)
	}
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	store := Get()
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

	updated, err := store.UpdateTasksStatus("launch", []string{"project-task", "module-task"}, models.StatusDone)
	if err != nil {
		t.Fatalf("UpdateTasksStatus() error: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2", updated)
	}
	for _, id := range []string{"project-task", "module-task"} {
		task, _, err := store.FindTask("launch", id)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status != models.StatusDone || task.UpdatedAt.IsZero() {
			t.Fatalf("task %s = %#v", id, task)
		}
	}
}
