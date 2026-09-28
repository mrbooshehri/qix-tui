package cmd

import (
	"fmt"
	"os"

	"github.com/mrbooshehri/qix-go/internal/config"
	"github.com/mrbooshehri/qix-go/internal/logging"
	"github.com/mrbooshehri/qix-go/internal/storage"
	"github.com/mrbooshehri/qix-go/internal/tui"
	"github.com/mrbooshehri/qix-go/internal/ui"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	noColor      bool
	verbose      bool
	logLevelFlag string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "qix",
	Short: "QIX - Quick Insight X: Work Breakdown Structure & KPI Tracker",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(storage.Get(), "")
	},
	Long: `QIX is a powerful project management tool that helps you:
  • Organize projects into hierarchical modules
  • Track tasks with time estimates and actual hours
  • Manage dependencies and parent-child relationships
  • Track time with start/stop sessions
  • Generate insightful reports and KPIs
  • Plan sprints and recurring tasks

Version 2.0 - Rewritten in Go for blazing fast performance.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize configuration
		if err := config.Init(); err != nil {
			ui.PrintError("Failed to initialize configuration: %v", err)
			os.Exit(1)
		}

		// Initialize logging before other subsystems
		cfg := config.Get()
		if err := logging.Init(cfg.LogFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		}

		if cmd.Flags().Changed("log-level") {
			cfg.LogLevel = logLevelFlag
		}
		logging.SetLevel(cfg.LogLevel)
		logging.Infof("Starting command: %s %v", cmd.CommandPath(), args)

		// Override color setting if --no-color flag is used
		if noColor {
			cfg.ColorOutput = false
		}

		// Initialize UI
		ui.Init()

		// Initialize storage
		if err := storage.Init(); err != nil {
			ui.PrintError("Failed to initialize storage: %v", err)
			os.Exit(1)
		}
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Flush any cached changes
		if err := storage.Get().FlushAll(); err != nil {
			ui.PrintWarning("Failed to save all changes: %v", err)
		}
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "info", "Log level (debug, info, warn, error)")

	// Add subcommands
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(moduleCmd)
	rootCmd.AddCommand(taskCmd)
	rootCmd.AddCommand(trackCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(sprintCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(jiraCmd)
	rootCmd.AddCommand(tuiCmd)
}

// versionCmd displays version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Display version information",
	Run: func(cmd *cobra.Command, args []string) {
		ui.PrintHeader("QIX - Quick Insight X")
		fmt.Println("Version:    2.0.0")
		fmt.Println("Build:      Go " + getGoVersion())
		fmt.Println("Author:     mrbooshehri")
		fmt.Println("License:    MIT")
		fmt.Println()
		ui.PrintInfo("Rewritten in Go for 100x performance improvement!")
	},
}

// doctorCmd checks system health
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check data integrity and system health",
	Run: func(cmd *cobra.Command, args []string) {
		runDoctor()
	},
}

func runDoctor() {
	ui.PrintHeader("QIX Doctor - System Health Check")

	store := storage.Get()
	cfg := config.Get()

	issues := 0
	warnings := 0

	// 1. Check directories
	ui.PrintSubHeader("📁 Checking directories...")

	dirs := []string{cfg.QixDir, cfg.ProjectsDir, cfg.BackupDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			ui.PrintError("Directory missing: %s", dir)
			issues++
		} else {
			ui.PrintSuccess("Directory exists: %s", dir)
		}
	}
	fmt.Println()

	// 2. Check permissions
	ui.PrintSubHeader("🔒 Checking permissions...")

	info, err := os.Stat(cfg.QixDir)
	if err == nil {
		perms := info.Mode().Perm()
		if perms != 0700 {
			ui.PrintWarning("QIX directory permissions: %o (recommended: 700)", perms)
			warnings++
		} else {
			ui.PrintSuccess("QIX directory permissions secure (700)")
		}
	}
	fmt.Println()

	// 3. Validate project files
	ui.PrintSubHeader("📄 Validating project files...")

	projects, err := store.ListProjects()
	if err != nil {
		ui.PrintError("Failed to list projects: %v", err)
		issues++
	} else {
		ui.PrintInfo("Found %d project(s)", len(projects))

		for _, name := range projects {
			if _, err := store.LoadProject(name); err != nil {
				ui.PrintError("Corrupted project: %s (%v)", name, err)
				issues++
			} else {
				ui.PrintSuccess("Valid: %s", name)
			}
		}
	}
	fmt.Println()

	// 4. Check index
	ui.PrintSubHeader("📇 Checking task index...")

	if err := store.EnsureIndexFresh(); err != nil {
		ui.PrintError("Index error: %v", err)
		issues++
	} else {
		ui.PrintSuccess("Index is up to date")
	}

	indexStats := store.GetIndexStats()
	ui.PrintInfo("Index contains %v task(s)", indexStats["total_tasks"])

	// Validate index
	if errors, err := store.ValidateIndex(); err != nil {
		ui.PrintError("Index validation failed: %v", err)
		issues++
	} else if len(errors) > 0 {
		ui.PrintWarning("Index inconsistencies found:")
		for _, e := range errors {
			ui.Dim.Println("  • " + e)
		}
		warnings += len(errors)
	} else {
		ui.PrintSuccess("Index is consistent")
	}
	fmt.Println()

	// 5. Check for orphaned references
	ui.PrintSubHeader("🔗 Checking task relationships...")

	orphanCount := 0
	for _, projectName := range projects {
		orphaned, err := store.FindOrphanedReferences(projectName)
		if err != nil {
			continue
		}

		for refType, refs := range orphaned {
			if len(refs) > 0 {
				ui.PrintWarning("Orphaned %s in %s:", refType, projectName)
				for _, ref := range refs {
					ui.Dim.Println("  • " + ref)
				}
				orphanCount += len(refs)
			}
		}
	}

	if orphanCount == 0 {
		ui.PrintSuccess("No orphaned references found")
	} else {
		warnings += orphanCount
	}
	fmt.Println()

	// 6. Cache statistics
	ui.PrintSubHeader("💾 Cache statistics...")

	cacheStats := store.GetCacheStats()
	ui.PrintInfo("Cached projects: %v", cacheStats["cached_projects"])
	ui.PrintInfo("Dirty projects:  %v", cacheStats["dirty_projects"])
	fmt.Println()

	// Summary
	ui.PrintSeparator()

	if issues == 0 && warnings == 0 {
		ui.PrintSuccess("All checks passed! Your QIX installation is healthy. ✨")
	} else if issues == 0 {
		ui.PrintWarning("%d warning(s) found (non-critical)", warnings)
		fmt.Println()
		ui.Yellow.Println("Recommendations:")
		ui.Dim.Println("  • Run 'qix backup create' to create a backup")
		if orphanCount > 0 {
			ui.Dim.Println("  • Remove orphaned references manually or recreate relationships")
		}
	} else {
		ui.PrintError("%d issue(s) and %d warning(s) found", issues, warnings)
		fmt.Println()
		ui.Yellow.Println("Recommendations:")
		ui.Dim.Println("  • Restore from backup if data is corrupted")
		ui.Dim.Println("  • Run 'qix backup create' to create a safety backup")
		ui.Dim.Println("  • Re-run doctor after fixing issues")
	}
}

func getGoVersion() string {
	// This would be set at build time
	return "1.21+"
}
