package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mrbooshehri/qix-go/internal/models"
)

// GenerateTaskID generates a unique 8-character hex ID
func GenerateTaskID() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// FindTask locates a task and returns it with its location
func (s *Storage) FindTask(projectName, taskID string) (*models.Task, string, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, "", err
	}

	// Check project-level tasks
	for i := range project.Tasks {
		if project.Tasks[i].ID == taskID {
			return &project.Tasks[i], "project", nil
		}
	}

	// Check module tasks
	for i := range project.Modules {
		for j := range project.Modules[i].Tasks {
			if project.Modules[i].Tasks[j].ID == taskID {
				location := fmt.Sprintf("module:%s", project.Modules[i].Name)
				return &project.Modules[i].Tasks[j], location, nil
			}
		}
	}

	return nil, "", fmt.Errorf("task '%s' not found", taskID)
}

// AddTask adds a task to a project or module
func (s *Storage) AddTask(projectName, moduleName string, task models.Task) error {
	// Generate ID if not provided
	if task.ID == "" {
		task.ID = GenerateTaskID()
	}

	// Set timestamps
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	// Initialize slices
	if task.TimeEntries == nil {
		task.TimeEntries = make([]models.TimeEntry, 0)
	}
	if task.Dependencies == nil {
		task.Dependencies = make([]string, 0)
	}
	if task.Tags == nil {
		task.Tags = make([]string, 0)
	}

	// Set defaults
	if task.Status == "" {
		task.Status = models.StatusTodo
	}
	if task.Priority == "" {
		task.Priority = models.PriorityMedium
	}

	return s.UpdateProject(projectName, func(p *models.Project) error {
		if moduleName == "" {
			// Add to project-level tasks
			p.Tasks = append(p.Tasks, task)
		} else {
			// Add to module tasks
			found := false
			for i := range p.Modules {
				if p.Modules[i].Name == moduleName {
					p.Modules[i].Tasks = append(p.Modules[i].Tasks, task)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("module '%s' not found", moduleName)
			}
		}
		return nil
	})
}

// UpdateTask updates a task by ID
func (s *Storage) UpdateTask(projectName, taskID string, updater func(*models.Task) error) error {
	return s.UpdateProject(projectName, func(p *models.Project) error {
		// Try project-level tasks
		for i := range p.Tasks {
			if p.Tasks[i].ID == taskID {
				if err := updater(&p.Tasks[i]); err != nil {
					return err
				}
				p.Tasks[i].UpdatedAt = time.Now()
				return nil
			}
		}

		// Try module tasks
		for i := range p.Modules {
			for j := range p.Modules[i].Tasks {
				if p.Modules[i].Tasks[j].ID == taskID {
					if err := updater(&p.Modules[i].Tasks[j]); err != nil {
						return err
					}
					p.Modules[i].Tasks[j].UpdatedAt = time.Now()
					return nil
				}
			}
		}

		return fmt.Errorf("task '%s' not found", taskID)
	})
}

// RemoveTask removes a task by ID
func (s *Storage) RemoveTask(projectName, taskID string) error {
	_, err := s.RemoveTasks(projectName, []string{taskID})
	return err
}

// RemoveTasks removes multiple tasks atomically and cleans references to them.
func (s *Storage) RemoveTasks(projectName string, taskIDs []string) (int, error) {
	ids := uniqueTaskIDs(taskIDs)
	if len(ids) == 0 {
		return 0, fmt.Errorf("select at least one task")
	}

	removed := 0
	err := s.UpdateProject(projectName, func(p *models.Project) error {
		found := make(map[string]bool, len(ids))
		for _, task := range p.GetAllTasks() {
			if ids[task.ID] {
				found[task.ID] = true
			}
		}
		for id := range ids {
			if !found[id] {
				return fmt.Errorf("task '%s' not found", id)
			}
		}

		p.Tasks, removed = removeTasksFromSlice(p.Tasks, ids)
		for i := range p.Modules {
			var moduleRemoved int
			p.Modules[i].Tasks, moduleRemoved = removeTasksFromSlice(p.Modules[i].Tasks, ids)
			removed += moduleRemoved
		}

		forEachTask(p, func(task *models.Task) {
			if ids[task.ParentID] {
				task.ParentID = ""
			}
			dependencies := task.Dependencies[:0]
			for _, dependency := range task.Dependencies {
				if !ids[dependency] {
					dependencies = append(dependencies, dependency)
				}
			}
			task.Dependencies = dependencies
		})
		for i := range p.Sprints {
			taskIDs := p.Sprints[i].TaskIDs[:0]
			for _, id := range p.Sprints[i].TaskIDs {
				if !ids[id] {
					taskIDs = append(taskIDs, id)
				}
			}
			p.Sprints[i].TaskIDs = taskIDs
		}
		return nil
	})
	return removed, err
}

func uniqueTaskIDs(taskIDs []string) map[string]bool {
	ids := make(map[string]bool, len(taskIDs))
	for _, id := range taskIDs {
		if id != "" {
			ids[id] = true
		}
	}
	return ids
}

func removeTasksFromSlice(tasks []models.Task, ids map[string]bool) ([]models.Task, int) {
	kept := tasks[:0]
	removed := 0
	for _, task := range tasks {
		if ids[task.ID] {
			removed++
			continue
		}
		kept = append(kept, task)
	}
	return kept, removed
}

func forEachTask(project *models.Project, apply func(*models.Task)) {
	for i := range project.Tasks {
		apply(&project.Tasks[i])
	}
	for i := range project.Modules {
		for j := range project.Modules[i].Tasks {
			apply(&project.Modules[i].Tasks[j])
		}
	}
}

// UpdateTasksStatus updates multiple task statuses in one project write.
func (s *Storage) UpdateTasksStatus(projectName string, taskIDs []string, status models.TaskStatus) (int, error) {
	ids := uniqueTaskIDs(taskIDs)
	if len(ids) == 0 {
		return 0, fmt.Errorf("select at least one task")
	}

	updated := 0
	err := s.UpdateProject(projectName, func(p *models.Project) error {
		found := make(map[string]bool, len(ids))
		for _, task := range p.GetAllTasks() {
			if ids[task.ID] {
				found[task.ID] = true
			}
		}
		for id := range ids {
			if !found[id] {
				return fmt.Errorf("task '%s' not found", id)
			}
		}

		now := time.Now()
		forEachTask(p, func(task *models.Task) {
			if ids[task.ID] {
				task.Status = status
				task.UpdatedAt = now
				updated++
			}
		})
		return nil
	})
	return updated, err
}

// UpdateTaskStatus updates a task's status
func (s *Storage) UpdateTaskStatus(projectName, taskID string, status models.TaskStatus) error {
	_, err := s.UpdateTasksStatus(projectName, []string{taskID}, status)
	return err
}

// AddTimeEntry adds a time entry to a task
func (s *Storage) AddTimeEntry(projectName, taskID string, entry models.TimeEntry) error {
	entry.LoggedAt = time.Now()

	return s.UpdateTask(projectName, taskID, func(t *models.Task) error {
		t.TimeEntries = append(t.TimeEntries, entry)
		return nil
	})
}

// SetTaskRecurrence sets or updates recurrence for a task
func (s *Storage) SetTaskRecurrence(projectName, taskID string, recurrence models.Recurrence) error {
	return s.UpdateTask(projectName, taskID, func(t *models.Task) error {
		t.Recurrence = &recurrence
		return nil
	})
}

// RemoveTaskRecurrence removes recurrence from a task
func (s *Storage) RemoveTaskRecurrence(projectName, taskID string) error {
	return s.UpdateTask(projectName, taskID, func(t *models.Task) error {
		t.Recurrence = nil
		return nil
	})
}

// LinkTaskAsChild sets a parent-child relationship
func (s *Storage) LinkTaskAsChild(projectName, childID, parentID string) error {
	// Verify parent exists
	if _, _, err := s.FindTask(projectName, parentID); err != nil {
		return fmt.Errorf("parent task not found: %w", err)
	}

	return s.UpdateTask(projectName, childID, func(t *models.Task) error {
		// Check for circular dependency
		if t.ID == parentID {
			return fmt.Errorf("task cannot be its own parent")
		}
		t.ParentID = parentID
		return nil
	})
}

// AddTaskDependency adds a dependency to a task
func (s *Storage) AddTaskDependency(projectName, taskID, dependsOnID string) error {
	// Verify dependency exists
	if _, _, err := s.FindTask(projectName, dependsOnID); err != nil {
		return fmt.Errorf("dependency task not found: %w", err)
	}

	return s.UpdateTask(projectName, taskID, func(t *models.Task) error {
		// Check if already dependent
		for _, depID := range t.Dependencies {
			if depID == dependsOnID {
				return nil // Already exists
			}
		}

		// Check for circular dependency
		if t.ID == dependsOnID {
			return fmt.Errorf("task cannot depend on itself")
		}

		t.Dependencies = append(t.Dependencies, dependsOnID)
		return nil
	})
}

// GetTasksByStatus returns all tasks with a specific status
func (s *Storage) GetTasksByStatus(projectName string, status models.TaskStatus) ([]models.Task, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	tasks := make([]models.Task, 0)
	for _, task := range project.GetAllTasks() {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

// GetRecurringTasksDue returns recurring tasks that are due
func (s *Storage) GetRecurringTasksDue(projectName, date string) ([]models.Task, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	tasks := make([]models.Task, 0)
	for _, task := range project.GetAllTasks() {
		if task.IsRecurring() && task.Recurrence.NextDue <= date {
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
}

// GetChildTasks returns all tasks that have the given task as parent
func (s *Storage) GetChildTasks(projectName, parentID string) ([]models.Task, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	children := make([]models.Task, 0)
	for _, task := range project.GetAllTasks() {
		if task.ParentID == parentID {
			children = append(children, task)
		}
	}

	return children, nil
}

// GetDependentTasks returns tasks that depend on the given task
func (s *Storage) GetDependentTasks(projectName, taskID string) ([]models.Task, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	dependents := make([]models.Task, 0)
	for _, task := range project.GetAllTasks() {
		for _, depID := range task.Dependencies {
			if depID == taskID {
				dependents = append(dependents, task)
				break
			}
		}
	}

	return dependents, nil
}

// ListTasksInModule returns all tasks in a specific module
func (s *Storage) ListTasksInModule(projectName, moduleName string) ([]models.Task, error) {
	project, err := s.LoadProject(projectName)
	if err != nil {
		return nil, err
	}

	for _, module := range project.Modules {
		if module.Name == moduleName {
			return module.Tasks, nil
		}
	}

	return nil, fmt.Errorf("module '%s' not found", moduleName)
}
