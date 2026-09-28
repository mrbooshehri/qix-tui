package storage

import (
	"testing"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/models"
)

func TestRenameProjectPreservesDataAndTracking(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	store := Get()
	if _, err := store.CreateProject("old-name", "description", []string{"tag"}); err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}
	if err := store.AddTask("old-name", "", models.Task{ID: "task-1", Title: "Tracked task"}); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}
	if err := store.StartTracking("old-name", "", "task-1"); err != nil {
		t.Fatalf("StartTracking() error: %v", err)
	}

	if err := store.RenameProject("old-name", "new-name"); err != nil {
		t.Fatalf("RenameProject() error: %v", err)
	}
	if store.ProjectExists("old-name") || !store.ProjectExists("new-name") {
		t.Fatalf("project files after rename: old=%v new=%v", store.ProjectExists("old-name"), store.ProjectExists("new-name"))
	}
	project, err := store.LoadProject("new-name")
	if err != nil {
		t.Fatalf("LoadProject() error: %v", err)
	}
	if project.Name != "new-name" || project.Description != "description" || len(project.Tasks) != 1 {
		t.Fatalf("renamed project = %#v", project)
	}
	session, err := store.GetActiveSession()
	if err != nil {
		t.Fatalf("GetActiveSession() error: %v", err)
	}
	if session == nil || session.Path != "new-name" {
		t.Fatalf("tracking session after rename = %#v", session)
	}
}

func TestRenameProjectRejectsDuplicateAndUnsafeNames(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	store := Get()
	if _, err := store.CreateProject("one", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateProject("two", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.RenameProject("one", "two"); err == nil {
		t.Fatal("duplicate project name accepted")
	}
	if err := store.RenameProject("one", "../unsafe"); err == nil {
		t.Fatal("unsafe project name accepted")
	}
}
