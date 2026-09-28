# QIX

> A keyboard-first project and task manager for the terminal, with a full-screen TUI and a scriptable CLI.

[![Go 1.21+](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

QIX keeps projects, modules, tasks, sprints, time entries, and reports close to the command line. Run `qix` for the interactive workspace, or use its subcommands in scripts and automation.

## Highlights

- Full-screen [Bubble Tea](https://github.com/charmbracelet/bubbletea) interface with keyboard navigation
- Centered, scrollable modal forms that keep the workspace visible behind them
- Multi-select task actions for bulk status changes and deletion
- Fast task-status workflow: `todo`, `doing`, `done`, and `blocked`
- Dashboard-style tracking, sprint, and report screens with tables and terminal charts
- In-app settings editor for Jira, date formats, backups, colors, and logging
- Hierarchical projects with modules and project-level tasks
- Priorities, tags, dependencies, parent/child links, recurrence, and Jira links
- Start/stop time tracking and manual time entry
- KPI, WBS, timeline, daily, comparison, project, and sprint reports
- JSON persistence with an indexed task lookup and atomic writes
- Backup, restore, export, cleanup, and installation health checks
- Bash and Zsh completions
- Existing CLI retained for scripting

## TUI preview

```text
 QIX / PROJECT WORKSPACE
 launch  •  8 tasks  •  2 doing  •  38% complete
┌─ Projects ──────────┐ ┌─ Tasks • backend ───────────────────────────┐
│> launch             │ │> doing  a17bc332  Build terminal workspace │
│  website            │ │  todo   811ca0f1  Write onboarding guide  │
├─ Modules ───────────┤ └────────────────────────────────────────────┘
│  (project tasks)    │ ┌─ Details ───────────────────────────────────┐
│> backend (2)        │ │Build terminal workspace                    │
│  frontend (3)       │ │Status: doing  Priority: high               │
└─────────────────────┘ └────────────────────────────────────────────┘
```

## Requirements

- Go 1.21 or newer to build from source
- A terminal with ANSI escape-sequence support
- Minimum recommended terminal size: 72 columns by 22 rows

## Installation

### Build from source

```bash
git clone https://github.com/mrbooshehri/qix-go.git
cd qix-go
make build
./qix
```

Without `make`:

```bash
go build -trimpath -o qix .
./qix
```

### Install with Go

```bash
go install github.com/mrbooshehri/qix-go@latest
qix
```

Make sure `$(go env GOPATH)/bin` is on your `PATH` when using `go install`.

## Using the TUI

Run QIX without a subcommand:

```bash
qix
```

The explicit command can optionally select a project:

```bash
qix tui
qix tui --project launch
```

### Keyboard controls

The top navigation exposes six screens. Use uppercase `W`, `T`, `S`, `R`, `H`, and `G` from anywhere outside a modal.

| Key | Screen |
| --- | --- |
| `W` | Project, module, and task workspace |
| `T` | Active timer and today's time summary |
| `S` | Sprint planning and progress |
| `R` | Overview, daily, WBS, timeline, and comparison reports |
| `H` | Doctor checks and backup management |
| `G` | View and edit QIX settings and storage paths |

Workspace controls:

| Key | Action |
| --- | --- |
| `↑` / `↓`, `j` / `k` | Move or vertically scroll the focused pane |
| `Page Up` / `Page Down`, `Home` / `End` | Scroll a page or jump to a boundary |
| `Tab`, `←` / `→` | Change focus between projects, modules, tasks, and details |
| `n` | Create a task in the selected project or module |
| `p` | Create a project with a name, description, and tags |
| `m` | Create a module with a name, description, and tags |
| `e` | Edit the focused project, module, or task |
| `Space` | Mark or unmark the focused task for bulk actions |
| `a` | Mark or unmark every task in the visible project/module scope |
| `d` | Delete marked tasks (or the focused item); task deletion uses Enter/Esc confirmation |
| `x` | Cycle the focused task's status, or every marked task from its status |
| `1` / `2` / `3` / `4` | Set marked tasks (or the focused task) to `todo` / `doing` / `done` / `blocked` |
| `Esc` | Clear marked tasks; inside a modal, cancel it |
| `l` | Link the selected task to a parent task |
| `y` | Add a dependency to the selected task |
| `c` / `u` | Set or remove recurrence |
| `C` | Complete a task and advance its recurring due date |
| `t` | Log time manually |
| `o` | Open the selected task's Jira issue |
| `r` | Reload projects and tasks from disk |
| `?` | Toggle help |
| `q` or `Ctrl-C` | Quit |
| `Enter` | Advance to the next modal field or save on the last field |
| `Ctrl-S` | Save every field in the current modal immediately |
| `Ctrl-G` | Cancel a modal form |

Every modal shows all of its fields together; `↑`/`↓`, `Tab`, and `Shift-Tab` move between fields, and compact terminals scroll the form to keep the active field visible. Project names, descriptions, and tags are editable, as are module names/descriptions/tags, sprint names/dates, and all task metadata. Renaming a project also updates its filename, task index, and active tracking path.

List panes use alternating table rows, status colors, and a highlighted cursor row. Task rows include `[ ]`/`[x]` selection markers; selections remain active while moving between modules in the same project so one bulk action can span scopes. Every pane is vertically scrollable, including task details and long report or health views.

Screen-specific controls:

| Screen | Controls |
| --- | --- |
| Tracking | Active timer, seven-day hours chart, today-by-project table; `s` start/switch, `x` stop, `t` manually log time |
| Sprints | Lifecycle and progress table, status charts, selected-sprint task table; `↑`/`↓` select, `n` create, `e` edit, `a` assign, `u` unassign, `d` remove |
| Reports | Scorecards, status/time charts, WBS/activity/portfolio tables; `←`/`→` cycle reports and `↑`/`↓` or `Page Up`/`Page Down` scroll |
| Health | `↑`/`↓` select backup, `Page Up`/`Page Down` scroll, `b` create, `e` export, `o` restore, `c` clean expired backups |
| Settings | `e` or `Enter` edits Jira URL, date formats, backup retention, color output, log level, and log file; `r` reloads from disk |

The TUI is built with Bubble Tea `v1.2.4` and Lip Gloss `v1.0.0`, versions selected to retain the repository's Go 1.21 compatibility.

## CLI

All command-oriented workflows remain available:

```bash
qix project create launch "Launch planning"
qix module create launch/backend "Backend services"
qix task create launch/backend "Implement API" --priority high --estimated 4
qix task list launch --all
qix task update launch TASK_ID doing
qix track start launch TASK_ID
qix track stop
qix report project launch
qix backup create
qix doctor
```

Use `qix --help` or `qix COMMAND --help` for the complete command and flag reference.

| Command | Purpose |
| --- | --- |
| `qix tui` | Open the interactive workspace |
| `qix project` | Create, inspect, delete, and summarize projects |
| `qix module` | Manage modules inside projects |
| `qix task` | Create, edit, link, schedule, and update tasks |
| `qix sprint` | Create sprints, assign tasks, and report progress |
| `qix track` | Track or manually log time |
| `qix report` | Generate daily, project, KPI, WBS, timeline, and comparison reports |
| `qix jira` | Open the Jira issue attached to a task |
| `qix backup` | Create, list, export, restore, and clean up backups |
| `qix doctor` | Validate directories, project data, and the task index |
| `qix completion` | Generate Bash or Zsh completion scripts |

## Data and configuration

QIX stores its data under `~/.qix` by default:

```text
~/.qix/
├── projects/      # one JSON file per project
├── backups/       # timestamped backups
├── config         # properties-format configuration
├── index.json     # generated task index
├── tracking.json  # active and historical tracking state
└── qix.log        # application log
```

Set `QIX_DIR` to use another data directory, which is especially useful for isolated environments and testing:

```bash
QIX_DIR="$PWD/.qix-data" qix
```

Example `~/.qix/config`:

```properties
date_format=2006-01-02
datetime_format=2006-01-02T15:04:05Z07:00
backup_retention_days=30
color_output=true
jira_base_url=https://your-domain.atlassian.net/browse
log_level=info
log_file=/home/you/.qix/qix.log
```

Supported environment overrides:

| Variable | Purpose |
| --- | --- |
| `QIX_DIR` | Data and configuration directory |
| `QIX_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` |
| `QIX_LOG_FILE` | Log file path |
| `JIRA_BASE_URL` | Base URL used by `qix jira open` |

The global `--no-color` flag disables color in command output. The full-screen TUI requires ANSI terminal support.

## Shell completion

Bash:

```bash
qix completion bash > /etc/bash_completion.d/qix
source /etc/bash_completion.d/qix
```

Zsh:

```bash
mkdir -p ~/.zsh/completions
qix completion zsh > ~/.zsh/completions/_qix
autoload -U compinit && compinit
```

## Development

```bash
make fmt       # format Go sources
make test      # run all package tests and vet checks
make build     # create ./qix
make run       # build and open the TUI
```

The main packages are organized as follows:

```text
cmd/               Cobra commands and TUI entry point
internal/models/   Project, task, sprint, and tracking models
internal/storage/  JSON persistence, cache, task index, and backups
internal/tui/      Full-screen terminal application
internal/ui/       Formatted CLI output and reports
```

## License

QIX is available under the [MIT License](LICENSE).
