package config

import (
	"os"
	"strings"
	"testing"
)

func TestSaveSettingsPersistsAndReloadsEditableConfig(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	t.Setenv("JIRA_BASE_URL", "")
	t.Setenv("QIX_LOG_LEVEL", "")
	t.Setenv("QIX_LOG_FILE", "")
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	settings := Settings{
		DateFormat:          "02 Jan 2006",
		DateTimeFormat:      "02 Jan 2006 15:04",
		BackupRetentionDays: 14,
		ColorOutput:         false,
		JiraBaseURL:         "https://jira.example.com/browse",
		LogFile:             Get().QixDir + "/custom.log",
		LogLevel:            "debug",
	}
	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings() error: %v", err)
	}
	cfg := Get()
	if cfg.JiraBaseURL != settings.JiraBaseURL || cfg.BackupRetentionDays != 14 || cfg.ColorOutput || cfg.LogLevel != "debug" {
		t.Fatalf("reloaded config = %#v", cfg)
	}
	data, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"jira_base_url=https://jira.example.com/browse", "backup_retention_days=14", "color_output=false", "log_level=debug"} {
		if !strings.Contains(string(data), expected) {
			t.Errorf("saved config missing %q:\n%s", expected, data)
		}
	}
}

func TestSaveSettingsRejectsInvalidValues(t *testing.T) {
	t.Setenv("QIX_DIR", t.TempDir())
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	settings := Settings{DateFormat: "2006-01-02", DateTimeFormat: "2006-01-02 15:04", BackupRetentionDays: 1, ColorOutput: true, JiraBaseURL: "not-a-url", LogFile: "qix.log", LogLevel: "info"}
	if err := SaveSettings(settings); err == nil {
		t.Fatal("invalid Jira URL accepted")
	}
	settings.JiraBaseURL = ""
	settings.LogLevel = "verbose"
	if err := SaveSettings(settings); err == nil {
		t.Fatal("invalid log level accepted")
	}
}
