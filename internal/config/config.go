package config

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds application configuration
type Config struct {
	QixDir              string
	ProjectsDir         string
	TrackFile           string
	IndexFile           string
	ConfigFile          string
	BackupDir           string
	DateFormat          string
	DateTimeFormat      string
	BackupRetentionDays int
	ColorOutput         bool
	JiraBaseURL         string
	LogFile             string
	LogLevel            string
}

var globalConfig *Config

// Init initializes the configuration
func Init() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Get QIX_DIR from environment or use default
	qixDir := os.Getenv("QIX_DIR")
	if qixDir == "" {
		qixDir = filepath.Join(homeDir, ".qix")
	}

	// Create directories
	projectsDir := filepath.Join(qixDir, "projects")
	backupDir := filepath.Join(qixDir, "backups")

	if err := os.MkdirAll(projectsDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return err
	}

	// Set up viper for config file
	configFile := filepath.Join(qixDir, "config")
	viper.SetConfigFile(configFile)
	viper.SetConfigType("properties")

	// Set defaults
	viper.SetDefault("date_format", "2006-01-02")
	viper.SetDefault("datetime_format", "2006-01-02T15:04:05Z07:00")
	viper.SetDefault("backup_retention_days", 30)
	viper.SetDefault("color_output", true)
	viper.SetDefault("jira_base_url", "")
	viper.BindEnv("jira_base_url", "JIRA_BASE_URL")
	viper.SetDefault("log_level", "info")
	viper.BindEnv("log_level", "QIX_LOG_LEVEL")
	viper.SetDefault("log_file", filepath.Join(qixDir, "qix.log"))
	viper.BindEnv("log_file", "QIX_LOG_FILE")

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		// Config file doesn't exist, create it with defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	globalConfig = &Config{
		QixDir:              qixDir,
		ProjectsDir:         projectsDir,
		TrackFile:           filepath.Join(qixDir, "tracking.json"),
		IndexFile:           filepath.Join(qixDir, "index.json"),
		ConfigFile:          configFile,
		BackupDir:           backupDir,
		DateFormat:          viper.GetString("date_format"),
		DateTimeFormat:      viper.GetString("datetime_format"),
		BackupRetentionDays: viper.GetInt("backup_retention_days"),
		ColorOutput:         viper.GetBool("color_output"),
		JiraBaseURL:         viper.GetString("jira_base_url"),
		LogFile: firstNonEmpty(
			viper.GetString("log_file"),
			filepath.Join(qixDir, "qix.log"),
		),
		LogLevel: firstNonEmpty(
			viper.GetString("log_level"),
			"info",
		),
	}

	return nil
}

// Get returns the global configuration
func Get() *Config {
	if globalConfig == nil {
		if err := Init(); err != nil {
			panic(err)
		}
	}
	return globalConfig
}

// GetProjectPath returns the full path to a project file
func (c *Config) GetProjectPath(projectName string) string {
	return filepath.Join(c.ProjectsDir, projectName+".json")
}

// ProjectExists checks if a project file exists
func (c *Config) ProjectExists(projectName string) bool {
	_, err := os.Stat(c.GetProjectPath(projectName))
	return err == nil
}

// ListProjectFiles returns all project JSON files
func (c *Config) ListProjectFiles() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(c.ProjectsDir, "*.json"))
	if err != nil {
		return nil, err
	}

	// Extract just the project names (without path and .json extension)
	projects := make([]string, 0, len(files))
	for _, file := range files {
		base := filepath.Base(file)
		name := base[:len(base)-5] // Remove .json
		projects = append(projects, name)
	}

	return projects, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
