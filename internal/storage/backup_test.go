package storage

import (
	"os"
	"testing"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/models"
)

func TestBackupCreateListAndRestore(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := config.Init(); err != nil {
		t.Fatalf("config.Init() error: %v", err)
	}
	if err := Init(); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	store := Get()
	if _, err := store.CreateProject("launch", "original", nil); err != nil {
		t.Fatalf("CreateProject() error: %v", err)
	}
	path, err := store.CreateArchive()
	if err != nil {
		t.Fatalf("CreateBackup() error: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("backup file = %q, info = %#v, error = %v", path, info, err)
	}
	backups, err := store.ListBackups()
	if err != nil || len(backups) != 1 || backups[0].Path != path {
		t.Fatalf("ListBackups() = %#v, %v", backups, err)
	}
	if err := store.UpdateProject("launch", func(project *models.Project) error {
		project.Description = "changed"
		return nil
	}); err != nil {
		t.Fatalf("UpdateProject() error: %v", err)
	}
	if _, err := store.RestoreBackup(path); err != nil {
		t.Fatalf("RestoreBackup() error: %v", err)
	}
	project, err := store.LoadProject("launch")
	if err != nil {
		t.Fatalf("LoadProject() after restore error: %v", err)
	}
	if project.Description != "original" {
		t.Fatalf("restored description = %q, want original", project.Description)
	}
}
