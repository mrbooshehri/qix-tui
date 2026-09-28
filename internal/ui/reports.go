package ui

import (
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/mrbooshehri/qix-go/internal/models"
)

// PrintDailyReport prints a formatted daily time report
func PrintDailyReport(date string, entriesByProject map[string][]models.TimeEntry, totalHours float64) {
	PrintHeader(fmt.Sprintf("Daily Report - %s", FormatDate(date)))
	
	if len(entriesByProject) == 0 {
		PrintEmptyState(
			fmt.Sprintf("No time entries found for %s", date),
			"Start tracking with: qix track start <project> <task_id>",
		)
		return
	}
	
	// Print entries by project
	for project, entries := range entriesByProject {
		PrintSubHeader("📁 " + project)
		
		projectTotal := 0.0
		for _, entry := range entries {
			projectTotal += entry.Hours
			Cyan.Printf("   • %s\n", FormatHours(entry.Hours))
		}
		
		BoldCyan.Printf("   Subtotal: %s\n", FormatHours(projectTotal))
		fmt.Println()
	}
	
	// Print total
	PrintSeparator()
	BoldGreen.Printf("Total time logged: %s\n", FormatHours(totalHours))
	fmt.Println()
}

// PrintProjectReport prints a project performance report
func PrintProjectReport(project *models.Project, startDate, endDate string) {
	PrintHeader(fmt.Sprintf("Project Report: %s", project.Name))
	fmt.Printf("Period: %s to %s\n\n", FormatDate(startDate), FormatDate(endDate))
	
	// Summary statistics
	counts := project.CountByStatus()
	
	table := NewTableBuilder("Metric", "Value").
		Row("Total Tasks", fmt.Sprintf("%d", len(project.GetAllTasks()))).
		Row("Completed", fmt.Sprintf("%d", counts[models.StatusDone])).
		Row("In Progress", fmt.Sprintf("%d", counts[models.StatusDoing])).
		Row("Todo", fmt.Sprintf("%d", counts[models.StatusTodo])).
		Row("Blocked", fmt.Sprintf("%d", counts[models.StatusBlocked])).
		Align(1, AlignRight)
	
	table.PrintSimple()
	fmt.Println()
	
	// Time statistics
	PrintSubHeader("⏱️  Time Analysis")
	
	estimated := project.CalculateTotalEstimated()
	actual := project.CalculateTotalActual()
	
	fmt.Printf("Estimated: %s\n", FormatHours(estimated))
	fmt.Printf("Actual:    %s\n", FormatHours(actual))
	
	if estimated > 0 {
		variance := actual - estimated
		varPct := (variance / estimated) * 100
		
		fmt.Print("Variance:  ")
		if variance > 0 {
			Red.Printf("+%s (%.1f%% over estimate)\n", FormatHours(variance), varPct)
		} else {
			Green.Printf("%s (%.1f%% under estimate)\n", FormatHours(variance), -varPct)
		}
	}
	
	fmt.Println()
	
	// Completion rate
	completion := project.GetCompletionPercentage()
	fmt.Print("Completion: ")
	PrintProgressBar(completion, 50)
	fmt.Printf(" %s\n", FormatPercentage(completion))
	fmt.Println()
	
	// Module breakdown
	if len(project.Modules) > 0 {
		PrintSubHeader("📦 Module Breakdown")
		
		for _, module := range project.Modules {
			moduleDone := 0
			for _, task := range module.Tasks {
				if task.Status == models.StatusDone {
					moduleDone++
				}
			}
			
			if len(module.Tasks) > 0 {
				modCompletion := float64(moduleDone) / float64(len(module.Tasks)) * 100
				fmt.Printf("%-20s ", module.Name)
				PrintProgressBar(modCompletion, 30)
				fmt.Printf(" %d/%d\n", moduleDone, len(module.Tasks))
			}
		}
		fmt.Println()
	}
}

// PrintKPIReport prints KPI metrics report
func PrintKPIReport(project *models.Project) {
	PrintHeader(fmt.Sprintf("KPI Report: %s", project.Name))
	
	allTasks := project.GetAllTasks()
	counts := project.CountByStatus()
	
	// 1. Estimation Accuracy
	PrintSubHeader("📊 Estimation Accuracy")
	
	tasksWithEstimates := 0
	totalEstimated := 0.0
	totalActual := 0.0
	
	for _, task := range allTasks {
		if task.EstimatedHours > 0 {
			tasksWithEstimates++
			totalEstimated += task.EstimatedHours
			totalActual += task.CalculateActualHours()
		}
	}
	
	if tasksWithEstimates > 0 {
		accuracy := 100.0
		if totalEstimated > 0 {
			variance := ((totalActual - totalEstimated) / totalEstimated) * 100
			if variance < 0 {
				accuracy = 100 + variance
			} else {
				accuracy = 100 - variance
			}
			if accuracy < 0 {
				accuracy = 0
			}
		}
		
		fmt.Printf("Tasks with estimates: %d\n", tasksWithEstimates)
		fmt.Printf("Estimated:           %s\n", FormatHours(totalEstimated))
		fmt.Printf("Actual:              %s\n", FormatHours(totalActual))
		fmt.Printf("Accuracy:            ")
		
		if accuracy >= 80 {
			Green.Printf("%.1f%%\n", accuracy)
		} else if accuracy >= 60 {
			Yellow.Printf("%.1f%%\n", accuracy)
		} else {
			Red.Printf("%.1f%%\n", accuracy)
		}
	} else {
		Dim.Println("No tasks with estimates yet")
	}
	
	fmt.Println()
	
	// 2. Velocity (last 7 days)
	PrintSubHeader("🚀 Velocity (Last 7 days)")
	
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	completedLastWeek := 0
	
	for _, task := range allTasks {
		if task.Status == models.StatusDone && task.UpdatedAt.Format("2006-01-02") >= weekAgo {
			completedLastWeek++
		}
	}
	
	dailyAvg := float64(completedLastWeek) / 7.0
	
	fmt.Printf("Completed:     %d tasks\n", completedLastWeek)
	fmt.Printf("Daily average: %.1f tasks/day\n", dailyAvg)
	fmt.Println()
	
	// 3. Task Distribution
	PrintSubHeader("📈 Task Distribution")
	
	total := len(allTasks)
	if total > 0 {
		distribution := map[string]float64{
			"Todo":    float64(counts[models.StatusTodo]) / float64(total) * 100,
			"Doing":   float64(counts[models.StatusDoing]) / float64(total) * 100,
			"Done":    float64(counts[models.StatusDone]) / float64(total) * 100,
			"Blocked": float64(counts[models.StatusBlocked]) / float64(total) * 100,
		}
		
		PrintChart(distribution, 40, true)
	}
	fmt.Println()
	
	// 4. Priority Breakdown
	PrintSubHeader("🎯 Priority Breakdown")
	
	priorityCounts := make(map[models.Priority]int)
	for _, task := range allTasks {
		if task.Status != models.StatusDone {
			priorityCounts[task.Priority]++
		}
	}
	
	table := NewTableBuilder("Priority", "Count", "Percentage").
		Align(1, AlignRight).
		Align(2, AlignRight)
	
	active := len(allTasks) - counts[models.StatusDone]
	if active > 0 {
		if count := priorityCounts[models.PriorityHigh]; count > 0 {
			pct := float64(count) / float64(active) * 100
			table.ColoredRow(
				[]string{"High", fmt.Sprintf("%d", count), FormatPercentage(pct)},
				[]*color.Color{Red, Red, Red},
			)
		}
		if count := priorityCounts[models.PriorityMedium]; count > 0 {
			pct := float64(count) / float64(active) * 100
			table.ColoredRow(
				[]string{"Medium", fmt.Sprintf("%d", count), FormatPercentage(pct)},
				[]*color.Color{Yellow, Yellow, Yellow},
			)
		}
		if count := priorityCounts[models.PriorityLow]; count > 0 {
			pct := float64(count) / float64(active) * 100
			table.ColoredRow(
				[]string{"Low", fmt.Sprintf("%d", count), FormatPercentage(pct)},
				[]*color.Color{Green, Green, Green},
			)
		}
	}
	
	table.PrintSimple()
	fmt.Println()
	
	// 5. Efficiency Score
	PrintSubHeader("⚡ Efficiency")
	
	doneEstimated := 0.0
	doneActual := 0.0
	
	for _, task := range allTasks {
		if task.Status == models.StatusDone && task.EstimatedHours > 0 {
			doneEstimated += task.EstimatedHours
			doneActual += task.CalculateActualHours()
		}
	}
	
	if doneEstimated > 0 && doneActual > 0 {
		efficiency := (doneEstimated / doneActual) * 100
		
		fmt.Printf("Completed (estimated): %s\n", FormatHours(doneEstimated))
		fmt.Printf("Completed (actual):    %s\n", FormatHours(doneActual))
		fmt.Print("Efficiency:            ")
		
		if efficiency > 100 {
			Green.Printf("%.1f%% (working faster than estimated!)\n", efficiency)
		} else if efficiency >= 80 {
			Green.Printf("%.1f%% (good estimation)\n", efficiency)
		} else {
			Red.Printf("%.1f%% (consider adjusting estimates)\n", efficiency)
		}
	} else {
		Dim.Println("No completed tasks with estimates")
	}
	
	fmt.Println()
}

// PrintWBSReport prints work breakdown structure report
func PrintWBSReport(project *models.Project) {
	PrintHeader(fmt.Sprintf("WBS Progress: %s", project.Name))
	
	// Overall progress
	completion := project.GetCompletionPercentage()
	total := len(project.GetAllTasks())
	done := project.CountByStatus()[models.StatusDone]
	
	fmt.Printf("Overall Progress: %.1f%% (%d/%d tasks)\n", completion, done, total)
	PrintProgressBar(completion, 60)
	fmt.Print("\n\n")
	
	// Project-level tasks
	if len(project.Tasks) > 0 {
		PrintSubHeader("📦 Project-Level Tasks")
		
		for _, task := range project.Tasks {
			PrintTask(task, "  ")
		}
		fmt.Println()
	}
	
	// Modules
	if len(project.Modules) > 0 {
		for _, module := range project.Modules {
			PrintSubHeader(fmt.Sprintf("📂 %s", module.Name))
			
			if module.Description != "" {
				Dim.Println("   " + module.Description)
			}
			
			// Module progress
			moduleDone := 0
			for _, task := range module.Tasks {
				if task.Status == models.StatusDone {
					moduleDone++
				}
			}
			
			if len(module.Tasks) > 0 {
				modCompletion := float64(moduleDone) / float64(len(module.Tasks)) * 100
				fmt.Print("   Progress: ")
				PrintProgressBar(modCompletion, 40)
				fmt.Printf(" %d/%d\n\n", moduleDone, len(module.Tasks))
			}
			
			// Module tasks
			for _, task := range module.Tasks {
				PrintTask(task, "   ")
			}
			fmt.Println()
		}
	}
}

// PrintSprintReport prints sprint progress report
func PrintSprintReport(project *models.Project, sprint *models.Sprint) {
	PrintHeader(fmt.Sprintf("Sprint Report: %s", sprint.Name))
	
	fmt.Printf("Period: %s → %s\n", FormatDate(sprint.StartDate), FormatDate(sprint.EndDate))
	
	// Calculate days remaining
	endDate, _ := time.Parse("2006-01-02", sprint.EndDate)
	today := time.Now()
	daysRemaining := int(endDate.Sub(today).Hours() / 24)
	
	if daysRemaining > 0 {
		Cyan.Printf("Status: Active (%d days remaining)\n", daysRemaining)
	} else if daysRemaining == 0 {
		Yellow.Println("Status: Ends today")
	} else {
		Green.Println("Status: Completed")
	}
	
	fmt.Println()
	
	// Sprint tasks
	if len(sprint.TaskIDs) == 0 {
		PrintEmptyState("No tasks assigned to this sprint", "")
		return
	}
	
	// Collect sprint tasks
	sprintTasks := make([]models.Task, 0)
	for _, taskID := range sprint.TaskIDs {
		for _, task := range project.GetAllTasks() {
			if task.ID == taskID {
				sprintTasks = append(sprintTasks, task)
				break
			}
		}
	}
	
	// Statistics
	statusCounts := make(map[models.TaskStatus]int)
	totalEst := 0.0
	totalAct := 0.0
	
	for _, task := range sprintTasks {
		statusCounts[task.Status]++
		totalEst += task.EstimatedHours
		totalAct += task.CalculateActualHours()
	}
	
	done := statusCounts[models.StatusDone]
	completion := float64(done) / float64(len(sprintTasks)) * 100
	
	// Print summary
	table := NewTableBuilder("Metric", "Value").
		Row("Total Tasks", fmt.Sprintf("%d", len(sprintTasks))).
		Row("✅ Done", fmt.Sprintf("%d", done)).
		Row("🔄 Doing", fmt.Sprintf("%d", statusCounts[models.StatusDoing])).
		Row("⭕ Todo", fmt.Sprintf("%d", statusCounts[models.StatusTodo])).
		Row("🚫 Blocked", fmt.Sprintf("%d", statusCounts[models.StatusBlocked])).
		Row("", "").
		Row("Estimated", FormatHours(totalEst)).
		Row("Actual", FormatHours(totalAct)).
		Align(1, AlignRight)
	
	table.PrintSimple()
	fmt.Println()
	
	fmt.Print("Completion: ")
	PrintProgressBar(completion, 50)
	fmt.Printf(" %s\n", FormatPercentage(completion))
	fmt.Println()
	
	// Velocity calculation
	if daysRemaining >= 0 {
		startDate, _ := time.Parse("2006-01-02", sprint.StartDate)
		daysPassed := int(today.Sub(startDate).Hours() / 24)
		
		if daysPassed > 0 {
			velocity := float64(done) / float64(daysPassed)
			fmt.Printf("Velocity:           %.2f tasks/day\n", velocity)
			
			if daysRemaining > 0 {
				projected := done + int(velocity*float64(daysRemaining))
				fmt.Printf("Projected at end:   %d/%d tasks\n", projected, len(sprintTasks))
			}
			fmt.Println()
		}
	}
	
	// List tasks
	PrintSubHeader("Sprint Tasks")
	for _, task := range sprintTasks {
		PrintTask(task, "  ")
	}
}