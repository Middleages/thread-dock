package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// App is the single Wails-facing monitor binding. It can run with an injected
// source for tests or own configurable GitHub+Herdr sources in production.
type App struct {
	mu sync.RWMutex

	source    SnapshotSource
	allSource SnapshotSource

	baseEnv        map[string]string
	process        CommandRunner
	timeout        time.Duration
	store          settingsStore
	projectToolbox projectToolboxStore
	globalToolbox  globalToolboxStore
	// toolbox is retained as a compatibility alias until Task 4 removes the old Wails consumer.
	toolbox   projectToolboxStore
	toolboxMu sync.Mutex
	settings  MonitorSettings
}

func NewApp(source SnapshotSource) *App {
	return &App{source: source, allSource: source, projectToolbox: defaultProjectToolboxStore(), globalToolbox: defaultGlobalToolboxStore()}
}

func NewConfigurableApp(baseEnv map[string]string, process CommandRunner, timeout time.Duration) *App {
	store := defaultSettingsStore()
	settings := settingsFromEnv(baseEnv)
	if stored, ok, err := store.load(); err == nil && ok {
		settings = stored
	}
	app := &App{
		baseEnv:        cloneStringMap(baseEnv),
		process:        process,
		timeout:        timeout,
		store:          store,
		projectToolbox: defaultProjectToolboxStore(),
		globalToolbox:  defaultGlobalToolboxStore(),
		settings:       settings,
	}
	app.source = app.buildSource(settings, true)
	app.allSource = app.buildSource(settings, false)
	return app
}

func (a *App) buildSource(settings MonitorSettings, openOnly bool) SnapshotSource {
	env := settings.environment(a.baseEnv)
	githubProcess := a.process
	if openOnly {
		githubProcess = openOnlyCommandRunner{base: a.process}
	}
	github := NewHostedGitHubMonitor(env, githubProcess, a.timeout)
	herdr := NewHostedHerdrMonitor(env, a.process, a.timeout)
	return NewCombinedMonitor(github, herdr)
}

// GetMonitorSnapshot returns the default open-only aggregate wire for the UI.
func (a *App) GetMonitorSnapshot() (Snapshot, error) {
	return a.fetchSnapshot(false)
}

// GetMonitorSnapshotAll is deliberately separate so closed Issue/PR history is
// queried only after the user explicitly asks for the full list.
func (a *App) GetMonitorSnapshotAll() (Snapshot, error) {
	return a.fetchSnapshot(true)
}

func (a *App) fetchSnapshot(all bool) (Snapshot, error) {
	if a == nil {
		return Snapshot{}, errors.New("monitor snapshot source is nil")
	}
	a.mu.RLock()
	source := a.source
	if all {
		source = a.allSource
	}
	a.mu.RUnlock()
	if source == nil {
		return Snapshot{}, errors.New("monitor snapshot source is nil")
	}
	return source.FetchAll(context.Background())
}

// GetMonitorSettings returns the currently effective local monitor settings.
func (a *App) GetMonitorSettings() (MonitorSettings, error) {
	if a == nil {
		return MonitorSettings{}, errors.New("monitor app is nil")
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings, nil
}

// SaveMonitorSettings validates and persists settings, then immediately rebuilds
// the read-only GitHub and Herdr monitor sources without requiring an app restart.
func (a *App) SaveMonitorSettings(settings MonitorSettings) (MonitorSettings, error) {
	if a == nil {
		return MonitorSettings{}, errors.New("monitor app is nil")
	}
	if err := validateMonitorSettings(settings); err != nil {
		return MonitorSettings{}, err
	}
	if err := a.store.save(settings); err != nil {
		return MonitorSettings{}, err
	}
	newSource := a.buildSource(settings, true)
	newAllSource := a.buildSource(settings, false)
	a.mu.Lock()
	a.settings = settings
	a.source = newSource
	a.allSource = newAllSource
	a.mu.Unlock()
	return settings, nil
}

func (a *App) projectToolboxStore() projectToolboxStore {
	if strings.TrimSpace(a.projectToolbox.path) != "" {
		return a.projectToolbox
	}
	return a.toolbox
}

// GetProjectReferences returns user-local references for one selected GitHub project.
// Toolbox data is deliberately not exposed to Agent orchestration.
func (a *App) GetProjectReferences(projectKey string) (ProjectReferences, error) {
	if a == nil {
		return ProjectReferences{}, errors.New("monitor app is nil")
	}
	projectKey = strings.TrimSpace(projectKey)
	if err := validateToolboxProjectKey(projectKey); err != nil {
		return ProjectReferences{}, err
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	projects := a.projectToolboxStore()
	if err := a.globalToolbox.ensureMigrated(projects); err != nil {
		return ProjectReferences{}, err
	}
	return projects.getReferences(projectKey)
}

// SaveProjectReferences persists only user-local references for one project.
func (a *App) SaveProjectReferences(projectKey string, refs ProjectReferences) (ProjectReferences, error) {
	if a == nil {
		return ProjectReferences{}, errors.New("monitor app is nil")
	}
	projectKey = strings.TrimSpace(projectKey)
	refs = normalizeProjectReferences(refs)
	if err := validateToolboxProjectKey(projectKey); err != nil {
		return ProjectReferences{}, err
	}
	if err := validateProjectReferences(refs); err != nil {
		return ProjectReferences{}, err
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	projects := a.projectToolboxStore()
	if err := a.globalToolbox.ensureMigrated(projects); err != nil {
		return ProjectReferences{}, err
	}
	return projects.putReferences(projectKey, refs)
}

// GetGlobalToolbox returns the migration-aware global human Toolbox.
func (a *App) GetGlobalToolbox() (GlobalToolbox, error) {
	if a == nil {
		return GlobalToolbox{}, errors.New("monitor app is nil")
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	return a.globalToolbox.get(a.projectToolboxStore())
}

// SaveGlobalToolbox persists the global human Toolbox. Migration and the caller
// value are handled together so legacy, existing global, and caller data survive.
func (a *App) SaveGlobalToolbox(toolbox GlobalToolbox) (GlobalToolbox, error) {
	if a == nil {
		return GlobalToolbox{}, errors.New("monitor app is nil")
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	return a.globalToolbox.put(a.projectToolboxStore(), toolbox)
}

// GetProjectToolbox returns user-local notes for one selected GitHub project.
// Toolbox data is deliberately not exposed to Agent orchestration.
func (a *App) GetProjectToolbox(projectKey string) (ProjectToolbox, error) {
	if a == nil {
		return ProjectToolbox{}, errors.New("monitor app is nil")
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	return a.projectToolboxStore().get(projectKey)
}

// SaveProjectToolbox persists user-local references, copy-only commands and checklist items.
func (a *App) SaveProjectToolbox(projectKey string, toolbox ProjectToolbox) (ProjectToolbox, error) {
	if a == nil {
		return ProjectToolbox{}, errors.New("monitor app is nil")
	}
	a.toolboxMu.Lock()
	defer a.toolboxMu.Unlock()
	return a.projectToolboxStore().put(projectKey, toolbox)
}

// OpenToolboxReference opens only a validated user-registered absolute local path.
// Commands are never executed through this API.
func (a *App) OpenToolboxReference(referenceType, target string) error {
	if a == nil {
		return errors.New("monitor app is nil")
	}
	item := ToolboxReference{ID: "open", Label: "open", Type: referenceType, Target: target}
	if err := validateToolboxReference(item); err != nil {
		return err
	}
	switch referenceType {
	case "file":
		return launchLocalPath(target)
	case "wsl-file":
		a.mu.RLock()
		distribution := strings.TrimSpace(a.settings.WSLDistribution)
		process := a.process
		timeout := a.timeout
		a.mu.RUnlock()
		if distribution == "" {
			return errors.New("WSL 파일을 열려면 WSL 배포판을 설정하세요")
		}
		if process == nil {
			return errors.New("WSL 경로 변환기를 사용할 수 없습니다")
		}
		ctx := context.Background()
		cancel := func() {}
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		result, err := process.Run(ctx, "", "wsl.exe", "--distribution", distribution, "--exec", "wslpath", "-w", target)
		if ctx.Err() != nil {
			return fmt.Errorf("WSL 파일 경로 변환 시간이 초과되었습니다: %w", ctx.Err())
		}
		if err != nil || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
			return errors.New("WSL 파일 경로를 Windows 경로로 변환할 수 없습니다")
		}
		return launchLocalPath(strings.TrimSpace(result.Stdout))
	default:
		return errors.New("웹 자료는 GitHub Monitor의 브라우저 열기 기능을 사용하세요")
	}
}
