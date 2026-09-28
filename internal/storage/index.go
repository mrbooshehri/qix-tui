package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mrbooshehri/qix-go/internal/models"
)

// LoadIndex loads the task index from disk
func (s *Storage) LoadIndex() error {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()

	if err := readJSONFile(s.config.IndexFile, &s.cache.index); err != nil {
		// Index doesn't exist or is corrupted
		s.cache.index = make(models.TaskIndex)
		return err
	}

	return nil
}

// SaveIndex saves the task index to disk
func (s *Storage) SaveIndex() error {
	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()

	return writeJSONFile(s.config.IndexFile, s.cache.index)
}

// RebuildIndex rebuilds the entire task index from all projects
func (s *Storage) RebuildIndex() error {
	newIndex := make(models.TaskIndex)

	projects, err := s.ListProjects()
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	for _, projectName := range projects {
		project, err := s.LoadProject(projectName)
		if err != nil {
			// Skip corrupted projects
			continue
		}

		// Index project-level tasks
		for _, task := range project.Tasks {
			newIndex[task.ID] = models.TaskLocation{
				Project:  projectName,
				Location: "project",
			}
		}

		// Index module tasks
		for _, module := range project.Modules {
			for _, task := range module.Tasks {
				newIndex[task.ID] = models.TaskLocation{
					Project:  projectName,
					Location: fmt.Sprintf("module:%s", module.Name),
				}
			}
		}
	}

	// Update cache
	s.cache.mu.Lock()
	s.cache.index = newIndex
	s.cache.mu.Unlock()

	// Save to disk
	return s.SaveIndex()
}

// indexProject indexes all tasks in a single project
func (s *Storage) indexProject(projectName string, project *models.Project) error {
	s.cache.mu.Lock()

	// Remove old entries for this project
	for taskID, loc := range s.cache.index {
		if loc.Project == projectName {
			delete(s.cache.index, taskID)
		}
	}

	// Add project-level tasks
	for _, task := range project.Tasks {
		s.cache.index[task.ID] = models.TaskLocation{
			Project:  projectName,
			Location: "project",
		}
	}

	// Add module tasks
	for _, module := range project.Modules {
		for _, task := range module.Tasks {
			s.cache.index[task.ID] = models.TaskLocation{
				Project:  projectName,
				Location: fmt.Sprintf("module:%s", module.Name),
			}
		}
	}

	// Finish the index write before reporting the project save as complete. This
	// keeps rapid consecutive mutations ordered and avoids leaving a writer
	// running after callers (including tests) have finished with the data path.
	s.cache.mu.Unlock()
	return s.SaveIndex()
}

// LookupTask uses the index for fast task lookup
func (s *Storage) LookupTask(taskID string) (string, string, error) {
	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()

	loc, exists := s.cache.index[taskID]
	if !exists {
		return "", "", fmt.Errorf("task '%s' not found in index", taskID)
	}

	return loc.Project, loc.Location, nil
}

// IsIndexStale checks if the index needs rebuilding
func (s *Storage) IsIndexStale() (bool, error) {
	indexInfo, err := os.Stat(s.config.IndexFile)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // Index doesn't exist
		}
		return false, err
	}

	indexModTime := indexInfo.ModTime()

	// Check if any project file is newer than index
	projects, err := s.ListProjects()
	if err != nil {
		return false, err
	}

	for _, projectName := range projects {
		path := s.config.GetProjectPath(projectName)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.ModTime().After(indexModTime) {
			return true, nil
		}
	}

	return false, nil
}

// EnsureIndexFresh rebuilds index if it's stale
func (s *Storage) EnsureIndexFresh() error {
	stale, err := s.IsIndexStale()
	if err != nil {
		return err
	}

	if stale {
		return s.RebuildIndex()
	}

	return nil
}

// GetIndexStats returns statistics about the index
func (s *Storage) GetIndexStats() map[string]interface{} {
	s.cache.mu.RLock()
	defer s.cache.mu.RUnlock()

	projectCounts := make(map[string]int)
	locationCounts := make(map[string]int)

	for _, loc := range s.cache.index {
		projectCounts[loc.Project]++

		if loc.Location == "project" {
			locationCounts["project-level"]++
		} else {
			locationCounts["module-level"]++
		}
	}

	return map[string]interface{}{
		"total_tasks":        len(s.cache.index),
		"projects":           projectCounts,
		"location_breakdown": locationCounts,
	}
}

// FindOrphanedReferences finds task IDs that are referenced but don't exist
func (s *Storage) FindOrphanedReferences(projectName string) (map[string][]string, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	orphaned := make(map[string][]string)
	allTasks := project.GetAllTasks()

	// Create a set of existing task IDs
	existingIDs := make(map[string]bool)
	for _, task := range allTasks {
		existingIDs[task.ID] = true
	}

	// Check parent references
	for _, task := range allTasks {
		if task.ParentID != "" && !existingIDs[task.ParentID] {
			orphaned["parent_references"] = append(orphaned["parent_references"],
				fmt.Sprintf("Task %s references non-existent parent %s", task.ID, task.ParentID))
		}

		// Check dependencies
		for _, depID := range task.Dependencies {
			if !existingIDs[depID] {
				orphaned["dependency_references"] = append(orphaned["dependency_references"],
					fmt.Sprintf("Task %s depends on non-existent task %s", task.ID, depID))
			}
		}
	}

	// Check sprint task references
	for _, sprint := range project.Sprints {
		for _, taskID := range sprint.TaskIDs {
			if !existingIDs[taskID] {
				orphaned["sprint_references"] = append(orphaned["sprint_references"],
					fmt.Sprintf("Sprint %s references non-existent task %s", sprint.Name, taskID))
			}
		}
	}

	return orphaned, nil
}

// ValidateIndex checks if the index matches actual data
func (s *Storage) ValidateIndex() ([]string, error) {
	errors := make([]string, 0)

	// Check each indexed task actually exists
	s.cache.mu.RLock()
	indexCopy := make(map[string]models.TaskLocation)
	for k, v := range s.cache.index {
		indexCopy[k] = v
	}
	s.cache.mu.RUnlock()

	for taskID, loc := range indexCopy {
		_, _, err := s.FindTask(loc.Project, taskID)
		if err != nil {
			errors = append(errors,
				fmt.Sprintf("Index references task %s in %s but task not found", taskID, loc.Project))
		}
	}

	// Check for tasks not in index
	projects, err := s.ListProjects()
	if err != nil {
		return errors, err
	}

	for _, projectName := range projects {
		project, err := s.LoadProject(projectName)
		if err != nil {
			continue
		}

		for _, task := range project.GetAllTasks() {
			if _, exists := indexCopy[task.ID]; !exists {
				errors = append(errors,
					fmt.Sprintf("Task %s in project %s not indexed", task.ID, projectName))
			}
		}
	}

	return errors, nil
}

// CompactIndex removes entries for deleted projects
func (s *Storage) CompactIndex() error {
	projects, err := s.ListProjects()
	if err != nil {
		return err
	}

	projectSet := make(map[string]bool)
	for _, p := range projects {
		projectSet[p] = true
	}

	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()

	// Remove entries for non-existent projects
	for taskID, loc := range s.cache.index {
		if !projectSet[loc.Project] {
			delete(s.cache.index, taskID)
		}
	}

	return s.SaveIndex()
}

// GetProjectPath is a helper to get the full path for a project file
func (s *Storage) GetProjectPath(projectName string) string {
	return filepath.Join(s.config.ProjectsDir, projectName+".json")
}
