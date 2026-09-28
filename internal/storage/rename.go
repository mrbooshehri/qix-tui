package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenameProject changes a project's display name and backing filename while
// preserving tasks, modules, sprints, tracking state, and index integrity.
func (s *Storage) RenameProject(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if filepath.Base(newName) != newName || newName == "." || newName == ".." {
		return fmt.Errorf("project name cannot contain path separators")
	}
	if oldName == newName {
		return nil
	}
	if s.ProjectExists(newName) {
		return fmt.Errorf("project '%s' already exists", newName)
	}

	project, err := s.LoadProject(oldName)
	if err != nil {
		return err
	}
	tracking, err := s.LoadTrackingData()
	if err != nil {
		return fmt.Errorf("load tracking data: %w", err)
	}
	trackingChanged := false
	if tracking.ActiveSession != nil {
		path := tracking.ActiveSession.Path
		if path == oldName || strings.HasPrefix(path, oldName+"/") {
			tracking.ActiveSession.Path = newName + strings.TrimPrefix(path, oldName)
			trackingChanged = true
		}
	}
	oldPath := s.config.GetProjectPath(oldName)
	newPath := s.config.GetProjectPath(newName)
	project.Name = newName
	if err := writeJSONFile(newPath, project); err != nil {
		project.Name = oldName
		return fmt.Errorf("write renamed project: %w", err)
	}
	if err := os.Remove(oldPath); err != nil {
		project.Name = oldName
		_ = os.Remove(newPath)
		return fmt.Errorf("remove old project file: %w", err)
	}

	if trackingChanged {
		if err := s.SaveTrackingData(tracking); err != nil {
			project.Name = oldName
			_ = writeJSONFile(oldPath, project)
			_ = os.Remove(newPath)
			return fmt.Errorf("update tracking path: %w", err)
		}
	}

	s.InvalidateCache(oldName)
	s.InvalidateCache(newName)
	if err := s.RebuildIndex(); err != nil {
		return fmt.Errorf("rebuild task index: %w", err)
	}
	return nil
}
