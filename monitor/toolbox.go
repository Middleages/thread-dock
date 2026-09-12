package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const toolboxFileName = "projects.json"
const toolboxVersion = 1
const projectReferencesVersion = 2
const globalToolboxFileName = "toolbox.json"
const globalToolboxVersion = 1
const maxToolboxItems = 200
const maxToolboxText = 16 * 1024

type ToolboxReference struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Type   string `json:"type"`
	Target string `json:"target"`
}

type ToolboxCommand struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Command string `json:"command"`
	Note    string `json:"note,omitempty"`
}

// ToolboxChecklistItem is a private v1 migration decoder. New writes use
// ToolboxTodo in the global store instead.
type ToolboxChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// ProjectToolbox is the private v1 on-disk migration shape.
type ProjectToolbox struct {
	References []ToolboxReference     `json:"references"`
	Commands   []ToolboxCommand       `json:"commands"`
	Checklist  []ToolboxChecklistItem `json:"checklist"`
}

type projectToolboxFile struct {
	Version  int                       `json:"version"`
	Projects map[string]ProjectToolbox `json:"projects"`
}

type ProjectReferences struct {
	References []ToolboxReference `json:"references"`
}

type ToolboxTodo struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Done       bool   `json:"done"`
	ProjectKey string `json:"projectKey,omitempty"`
}

type GlobalToolbox struct {
	Commands []ToolboxCommand `json:"commands"`
	Todos    []ToolboxTodo    `json:"todos"`
}

type projectReferencesFile struct {
	Version  int                          `json:"version"`
	Projects map[string]ProjectReferences `json:"projects"`
}

type globalToolboxFile struct {
	Version int           `json:"version"`
	Toolbox GlobalToolbox `json:"toolbox"`
}

// toolboxFileWriter is deliberately private: it is a deterministic failure
// seam for storage tests and is never exposed through Wails.
type toolboxFileWriter func(path, tempPattern string, value any) error

type projectToolboxStore struct {
	path   string
	writer toolboxFileWriter
}

type globalToolboxStore struct {
	path   string
	writer toolboxFileWriter
}

func defaultProjectToolboxStore() projectToolboxStore {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return projectToolboxStore{}
	}
	return projectToolboxStore{path: filepath.Join(dir, settingsDirectoryName, toolboxFileName)}
}

func defaultGlobalToolboxStore() globalToolboxStore {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return globalToolboxStore{}
	}
	return globalToolboxStore{path: filepath.Join(dir, settingsDirectoryName, globalToolboxFileName)}
}

func normalizeProjectToolbox(value ProjectToolbox) ProjectToolbox {
	if value.References == nil {
		value.References = []ToolboxReference{}
	}
	if value.Commands == nil {
		value.Commands = []ToolboxCommand{}
	}
	if value.Checklist == nil {
		value.Checklist = []ToolboxChecklistItem{}
	}
	return value
}

func emptyProjectReferences() ProjectReferences {
	return ProjectReferences{References: []ToolboxReference{}}
}

func normalizeProjectReferences(value ProjectReferences) ProjectReferences {
	if value.References == nil {
		value.References = []ToolboxReference{}
	}
	return value
}

func emptyGlobalToolbox() GlobalToolbox {
	return GlobalToolbox{Commands: []ToolboxCommand{}, Todos: []ToolboxTodo{}}
}

func normalizeGlobalToolbox(value GlobalToolbox) GlobalToolbox {
	if value.Commands == nil {
		value.Commands = []ToolboxCommand{}
	}
	if value.Todos == nil {
		value.Todos = []ToolboxTodo{}
	}
	for index := range value.Todos {
		value.Todos[index].ProjectKey = strings.TrimSpace(value.Todos[index].ProjectKey)
	}
	return value
}

func validateToolboxProjectKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 512 || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("프로젝트 Toolbox 키가 올바르지 않습니다")
	}
	return nil
}

func validateProjectToolbox(value ProjectToolbox) error {
	value = normalizeProjectToolbox(value)
	if len(value.References) > maxToolboxItems || len(value.Commands) > maxToolboxItems || len(value.Checklist) > maxToolboxItems {
		return fmt.Errorf("Toolbox 항목은 종류별 최대 %d개까지 저장할 수 있습니다", maxToolboxItems)
	}
	if err := validateReferences(value.References); err != nil {
		return err
	}
	if err := validateCommands(value.Commands); err != nil {
		return err
	}
	for _, item := range value.Checklist {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Text) == "" || len(item.Text) > 2000 {
			return errors.New("체크리스트 항목이 올바르지 않습니다")
		}
	}
	return nil
}

func validateProjectReferences(value ProjectReferences) error {
	value = normalizeProjectReferences(value)
	if len(value.References) > maxToolboxItems {
		return fmt.Errorf("자료 Toolbox 항목은 최대 %d개까지 저장할 수 있습니다", maxToolboxItems)
	}
	return validateReferences(value.References)
}

func validateReferences(items []ToolboxReference) error {
	for _, item := range items {
		if err := validateToolboxReference(item); err != nil {
			return err
		}
	}
	return nil
}

func validateCommands(items []ToolboxCommand) error {
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Command) == "" {
			return errors.New("명령어의 id, 이름, command는 비어 있을 수 없습니다")
		}
		if len(item.Label) > 300 || len(item.Command) > maxToolboxText || len(item.Note) > 2000 {
			return errors.New("명령어 Toolbox 항목이 너무 깁니다")
		}
	}
	return nil
}

func validateGlobalToolbox(value GlobalToolbox) error {
	value = normalizeGlobalToolbox(value)
	if len(value.Commands) > maxToolboxItems || len(value.Todos) > maxToolboxItems {
		return fmt.Errorf("전역 Toolbox 항목은 종류별 최대 %d개까지 저장할 수 있습니다", maxToolboxItems)
	}
	if err := validateCommands(value.Commands); err != nil {
		return err
	}
	for _, item := range value.Todos {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Text) == "" || len(item.Text) > 2000 {
			return errors.New("할 일 Toolbox 항목이 올바르지 않습니다")
		}
		if strings.TrimSpace(item.ProjectKey) != "" {
			if err := validateToolboxProjectKey(item.ProjectKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateToolboxReference(item ToolboxReference) error {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Target) == "" {
		return errors.New("자료의 id, 이름, 대상은 비어 있을 수 없습니다")
	}
	if len(item.Label) > 300 || len(item.Target) > maxToolboxText {
		return errors.New("자료 Toolbox 항목이 너무 깁니다")
	}
	target := strings.TrimSpace(item.Target)
	switch item.Type {
	case "web":
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return errors.New("웹 자료는 http 또는 https URL이어야 합니다")
		}
	case "file":
		if !isAbsoluteLocalPath(target) {
			return errors.New("로컬 파일은 절대 경로여야 합니다")
		}
	case "wsl-file":
		if !strings.HasPrefix(target, "/") || strings.ContainsRune(target, '\x00') {
			return errors.New("WSL 파일은 /로 시작하는 절대 경로여야 합니다")
		}
	default:
		return errors.New("자료 유형은 web, file, wsl-file 중 하나여야 합니다")
	}
	return nil
}

func isAbsoluteLocalPath(value string) bool {
	if filepath.IsAbs(value) {
		return true
	}
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	return strings.HasPrefix(value, `\\`)
}

func (s projectToolboxStore) load() (projectToolboxFile, error) {
	result := projectToolboxFile{Version: toolboxVersion, Projects: map[string]ProjectToolbox{}}
	if strings.TrimSpace(s.path) == "" {
		return result, errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("Toolbox 파일을 읽을 수 없습니다: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("Toolbox 파일 형식이 올바르지 않습니다: %w", err)
	}
	if result.Version != toolboxVersion {
		return result, fmt.Errorf("지원하지 않는 Toolbox 버전입니다: %d", result.Version)
	}
	if result.Projects == nil {
		result.Projects = map[string]ProjectToolbox{}
	}
	return result, nil
}

func (s projectToolboxStore) readVersion() (int, error) {
	if strings.TrimSpace(s.path) == "" {
		return 0, errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return -1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("Toolbox 파일을 읽을 수 없습니다: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, fmt.Errorf("Toolbox 파일 형식이 올바르지 않습니다: %w", err)
	}
	return header.Version, nil
}

func (s projectToolboxStore) loadReferences() (projectReferencesFile, error) {
	result := projectReferencesFile{Version: projectReferencesVersion, Projects: map[string]ProjectReferences{}}
	if strings.TrimSpace(s.path) == "" {
		return result, errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("Toolbox 파일을 읽을 수 없습니다: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return result, fmt.Errorf("Toolbox 파일 형식이 올바르지 않습니다: %w", err)
	}
	if header.Version == toolboxVersion {
		return result, errors.New("프로젝트 Toolbox migration이 필요합니다")
	}
	if header.Version != projectReferencesVersion {
		return result, fmt.Errorf("지원하지 않는 프로젝트 Toolbox 버전입니다: %d", header.Version)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("프로젝트 자료 파일 형식이 올바르지 않습니다: %w", err)
	}
	if result.Projects == nil {
		result.Projects = map[string]ProjectReferences{}
	}
	for key, value := range result.Projects {
		if err := validateToolboxProjectKey(key); err != nil {
			return result, err
		}
		value = normalizeProjectReferences(value)
		if err := validateProjectReferences(value); err != nil {
			return result, err
		}
		result.Projects[key] = value
	}
	return result, nil
}

func (s projectToolboxStore) getReferences(key string) (ProjectReferences, error) {
	key = strings.TrimSpace(key)
	if err := validateToolboxProjectKey(key); err != nil {
		return ProjectReferences{}, err
	}
	file, err := s.loadReferences()
	if err != nil {
		return ProjectReferences{}, err
	}
	value, ok := file.Projects[key]
	if !ok {
		return emptyProjectReferences(), nil
	}
	return normalizeProjectReferences(value), nil
}

func (s projectToolboxStore) putReferences(key string, value ProjectReferences) (ProjectReferences, error) {
	key = strings.TrimSpace(key)
	value = normalizeProjectReferences(value)
	if err := validateToolboxProjectKey(key); err != nil {
		return ProjectReferences{}, err
	}
	if err := validateProjectReferences(value); err != nil {
		return ProjectReferences{}, err
	}
	file, err := s.loadReferences()
	if err != nil {
		return ProjectReferences{}, err
	}
	file.Projects[key] = value
	file.Version = projectReferencesVersion
	if err := s.saveReferences(file); err != nil {
		return ProjectReferences{}, err
	}
	return value, nil
}

func (s projectToolboxStore) saveReferences(value projectReferencesFile) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	writer := s.writer
	if writer == nil {
		writer = writeToolboxFile
	}
	if err := writer(s.path, "projects-*.tmp", value); err != nil {
		return fmt.Errorf("프로젝트 자료 파일을 저장할 수 없습니다: %w", err)
	}
	return nil
}

func (s globalToolboxStore) load() (globalToolboxFile, error) {
	result := globalToolboxFile{Version: globalToolboxVersion, Toolbox: emptyGlobalToolbox()}
	if strings.TrimSpace(s.path) == "" {
		return result, errors.New("사용자 전역 Toolbox 경로를 확인할 수 없습니다")
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("전역 Toolbox 파일을 읽을 수 없습니다: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("전역 Toolbox 파일 형식이 올바르지 않습니다: %w", err)
	}
	if result.Version != globalToolboxVersion {
		return result, fmt.Errorf("지원하지 않는 전역 Toolbox 버전입니다: %d", result.Version)
	}
	result.Toolbox = normalizeGlobalToolbox(result.Toolbox)
	if err := validateGlobalToolbox(result.Toolbox); err != nil {
		return result, err
	}
	return result, nil
}

func (s globalToolboxStore) save(value GlobalToolbox) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("사용자 전역 Toolbox 경로를 확인할 수 없습니다")
	}
	writer := s.writer
	if writer == nil {
		writer = writeToolboxFile
	}
	file := globalToolboxFile{Version: globalToolboxVersion, Toolbox: normalizeGlobalToolbox(value)}
	if err := writer(s.path, "toolbox-*.tmp", file); err != nil {
		return fmt.Errorf("전역 Toolbox 파일을 저장할 수 없습니다: %w", err)
	}
	return nil
}

func (s globalToolboxStore) get(projects projectToolboxStore) (GlobalToolbox, error) {
	if err := s.ensureMigrated(projects); err != nil {
		return GlobalToolbox{}, err
	}
	file, err := s.load()
	if err != nil {
		return GlobalToolbox{}, err
	}
	return normalizeGlobalToolbox(file.Toolbox), nil
}

func (s globalToolboxStore) put(projects projectToolboxStore, value GlobalToolbox) (GlobalToolbox, error) {
	value = normalizeGlobalToolbox(value)
	if err := validateGlobalToolbox(value); err != nil {
		return GlobalToolbox{}, err
	}
	version, err := projects.readVersion()
	if err != nil {
		return GlobalToolbox{}, err
	}
	if version == toolboxVersion {
		merged, err := s.migrateLegacy(projects, &value)
		if err != nil {
			return GlobalToolbox{}, err
		}
		return merged, nil
	}
	if version != -1 && version != projectReferencesVersion {
		return GlobalToolbox{}, fmt.Errorf("지원하지 않는 프로젝트 Toolbox 버전입니다: %d", version)
	}
	// A normal v2/empty put is a full replacement, but an existing global file
	// must still be readable and valid before it can be overwritten.
	if _, err := s.load(); err != nil {
		return GlobalToolbox{}, err
	}
	if err := s.save(value); err != nil {
		return GlobalToolbox{}, err
	}
	return value, nil
}

// ensureMigrated resumes from a v1 projects file. The global file is written
// first; only a successful global write permits the references-only rewrite.
func (s globalToolboxStore) ensureMigrated(projects projectToolboxStore) error {
	version, err := projects.readVersion()
	if err != nil {
		return err
	}
	if version == -1 || version == projectReferencesVersion {
		return nil
	}
	if version != toolboxVersion {
		return fmt.Errorf("지원하지 않는 프로젝트 Toolbox 버전입니다: %d", version)
	}
	_, err = s.migrateLegacy(projects, nil)
	return err
}

func (s globalToolboxStore) migrateLegacy(projects projectToolboxStore, caller *GlobalToolbox) (GlobalToolbox, error) {
	legacy, err := projects.load()
	if err != nil {
		return GlobalToolbox{}, err
	}
	keys := make([]string, 0, len(legacy.Projects))
	for key, value := range legacy.Projects {
		if err := validateToolboxProjectKey(key); err != nil {
			return GlobalToolbox{}, err
		}
		if err := validateProjectToolbox(value); err != nil {
			return GlobalToolbox{}, err
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	global, err := s.load()
	if err != nil {
		return GlobalToolbox{}, err
	}
	merged, err := mergeLegacyToolbox(global.Toolbox, legacy, keys, caller)
	if err != nil {
		return GlobalToolbox{}, err
	}
	if err := s.save(merged); err != nil {
		return GlobalToolbox{}, err
	}
	converted := projectReferencesFile{Version: projectReferencesVersion, Projects: make(map[string]ProjectReferences, len(legacy.Projects))}
	for key, value := range legacy.Projects {
		converted.Projects[key] = normalizeProjectReferences(ProjectReferences{References: value.References})
	}
	if err := projects.saveReferences(converted); err != nil {
		return GlobalToolbox{}, err
	}
	return merged, nil
}

func mergeLegacyToolbox(existing GlobalToolbox, legacy projectToolboxFile, projectKeys []string, caller *GlobalToolbox) (GlobalToolbox, error) {
	merged := emptyGlobalToolbox()
	commandSeen := map[string]int{}
	commandIDs := map[string]bool{}
	todoSeen := map[string]int{}
	todoIDs := map[string]bool{}
	addCommand := func(command ToolboxCommand, origin string) {
		command = normalizeCommand(command)
		semantic := toolboxPairKey(command.Label, command.Command)
		if previous, ok := commandSeen[semantic]; ok {
			merged.Commands[previous].Note = preferCommandNote(merged.Commands[previous].Note, command.Note)
			return
		}
		if commandIDs[command.ID] {
			command.ID = stableToolboxID("command", command.ID, origin+"\x00"+semantic, commandIDs)
		}
		commandSeen[semantic] = len(merged.Commands)
		commandIDs[command.ID] = true
		merged.Commands = append(merged.Commands, command)
	}
	addTodo := func(todo ToolboxTodo, origin string) {
		todo = normalizeTodo(todo)
		semantic := toolboxPairKey(todo.ProjectKey, todo.Text)
		if previous, ok := todoSeen[semantic]; ok {
			merged.Todos[previous].Done = merged.Todos[previous].Done && todo.Done
			return
		}
		if todoIDs[todo.ID] {
			todo.ID = stableToolboxID("todo", todo.ID, origin+"\x00"+semantic, todoIDs)
		}
		todoSeen[semantic] = len(merged.Todos)
		todoIDs[todo.ID] = true
		merged.Todos = append(merged.Todos, todo)
	}
	for _, command := range normalizeGlobalToolbox(existing).Commands {
		addCommand(command, "existing")
	}
	for _, todo := range normalizeGlobalToolbox(existing).Todos {
		addTodo(todo, "existing")
	}
	for _, projectKey := range projectKeys {
		value := normalizeProjectToolbox(legacy.Projects[projectKey])
		for _, command := range value.Commands {
			addCommand(command, projectKey)
		}
		for _, item := range value.Checklist {
			addTodo(ToolboxTodo{ID: item.ID, Text: item.Text, Done: item.Done, ProjectKey: projectKey}, projectKey)
		}
	}
	if caller != nil {
		for _, command := range caller.Commands {
			addCommand(command, "caller")
		}
		for _, todo := range caller.Todos {
			addTodo(todo, "caller")
		}
	}
	merged = normalizeGlobalToolbox(merged)
	if err := validateGlobalToolbox(merged); err != nil {
		return GlobalToolbox{}, err
	}
	return merged, nil
}

func normalizeCommand(command ToolboxCommand) ToolboxCommand {
	command.ID = strings.TrimSpace(command.ID)
	command.Label = strings.TrimSpace(command.Label)
	command.Command = strings.TrimSpace(command.Command)
	return command
}

func normalizeTodo(todo ToolboxTodo) ToolboxTodo {
	todo.ID = strings.TrimSpace(todo.ID)
	todo.Text = strings.TrimSpace(todo.Text)
	todo.ProjectKey = strings.TrimSpace(todo.ProjectKey)
	return todo
}

func preferCommandNote(first, second string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return second
}

// Length-prefixing keeps composite keys unambiguous even when values contain
// arbitrary separators.
func toolboxPairKey(first, second string) string {
	return fmt.Sprintf("%d:%s%d:%s", len(first), first, len(second), second)
}

func stableToolboxID(kind, original, semantic string, used map[string]bool) string {
	digest := sha256.Sum256([]byte(kind + "\x00" + original + "\x00" + semantic))
	base := "legacy-" + kind + "-" + hex.EncodeToString(digest[:])[:16]
	id := base
	for suffix := 2; used[id]; suffix++ {
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
	return id
}

func writeToolboxFile(path, tempPattern string, value any) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("Toolbox를 직렬화할 수 없습니다: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}
