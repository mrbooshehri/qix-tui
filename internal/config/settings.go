package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Settings contains the user-editable values persisted in the QIX config file.
type Settings struct {
	DateFormat          string
	DateTimeFormat      string
	BackupRetentionDays int
	ColorOutput         bool
	JiraBaseURL         string
	LogFile             string
	LogLevel            string
}

// SaveSettings validates and atomically persists user-editable configuration.
func SaveSettings(settings Settings) error {
	settings.DateFormat = strings.TrimSpace(settings.DateFormat)
	settings.DateTimeFormat = strings.TrimSpace(settings.DateTimeFormat)
	settings.JiraBaseURL = strings.TrimSpace(settings.JiraBaseURL)
	settings.LogFile = strings.TrimSpace(settings.LogFile)
	settings.LogLevel = strings.ToLower(strings.TrimSpace(settings.LogLevel))
	if settings.DateFormat == "" || settings.DateTimeFormat == "" {
		return fmt.Errorf("date formats cannot be empty")
	}
	if settings.BackupRetentionDays < 1 {
		return fmt.Errorf("backup retention must be at least 1 day")
	}
	if settings.LogFile == "" {
		return fmt.Errorf("log file cannot be empty")
	}
	if settings.LogLevel != "debug" && settings.LogLevel != "info" && settings.LogLevel != "warn" && settings.LogLevel != "error" {
		return fmt.Errorf("log level must be debug, info, warn, or error")
	}
	if settings.JiraBaseURL != "" {
		parsed, err := url.ParseRequestURI(settings.JiraBaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("Jira URL must be an http or https URL")
		}
	}
	for name, value := range map[string]string{
		"date format": settings.DateFormat, "date-time format": settings.DateTimeFormat,
		"Jira URL": settings.JiraBaseURL, "log file": settings.LogFile, "log level": settings.LogLevel,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s cannot contain a newline", name)
		}
	}

	cfg := Get()
	content := strings.Join([]string{
		"date_format=" + settings.DateFormat,
		"datetime_format=" + settings.DateTimeFormat,
		"backup_retention_days=" + strconv.Itoa(settings.BackupRetentionDays),
		"color_output=" + strconv.FormatBool(settings.ColorOutput),
		"jira_base_url=" + settings.JiraBaseURL,
		"log_level=" + settings.LogLevel,
		"log_file=" + settings.LogFile,
		"",
	}, "\n")
	tempPath := cfg.ConfigFile + ".tmp"
	if err := os.WriteFile(tempPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tempPath, cfg.ConfigFile); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace config: %w", err)
	}
	if err := Init(); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	return nil
}
