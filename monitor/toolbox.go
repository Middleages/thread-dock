package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const toolboxFileName = "projects.json"
const toolboxVersion = 1
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

type ToolboxChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type ProjectToolbox struct {
	References []ToolboxReference     `json:"references"`
	Commands   []ToolboxCommand       `json:"commands"`
	Checklist  []ToolboxChecklistItem `json:"checklist"`
}

type projectToolboxFile struct {
	Version  int                       `json:"version"`
	Projects map[string]ProjectToolbox `json:"projects"`
}

type projectToolboxStore struct {
	path string
}

func defaultProjectToolboxStore() projectToolboxStore {
	dir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return projectToolboxStore{}
	}
	return projectToolboxStore{path: filepath.Join(dir, settingsDirectoryName, toolboxFileName)}
}

func emptyProjectToolbox() ProjectToolbox {
	return ProjectToolbox{References: []ToolboxReference{}, Commands: []ToolboxCommand{}, Checklist: []ToolboxChecklistItem{}}
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
	for _, item := range value.References {
		if err := validateToolboxReference(item); err != nil {
			return err
		}
	}
	for _, item := range value.Commands {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Label) == "" || strings.TrimSpace(item.Command) == "" {
			return errors.New("명령어의 id, 이름, command는 비어 있을 수 없습니다")
		}
		if len(item.Label) > 300 || len(item.Command) > maxToolboxText || len(item.Note) > 2000 {
			return errors.New("명령어 Toolbox 항목이 너무 깁니다")
		}
	}
	for _, item := range value.Checklist {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Text) == "" || len(item.Text) > 2000 {
			return errors.New("체크리스트 항목이 올바르지 않습니다")
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

func (s projectToolboxStore) get(key string) (ProjectToolbox, error) {
	if err := validateToolboxProjectKey(key); err != nil {
		return ProjectToolbox{}, err
	}
	file, err := s.load()
	if err != nil {
		return ProjectToolbox{}, err
	}
	value, ok := file.Projects[strings.TrimSpace(key)]
	if !ok {
		return emptyProjectToolbox(), nil
	}
	return normalizeProjectToolbox(value), nil
}

func (s projectToolboxStore) put(key string, value ProjectToolbox) (ProjectToolbox, error) {
	key = strings.TrimSpace(key)
	value = normalizeProjectToolbox(value)
	if err := validateToolboxProjectKey(key); err != nil {
		return ProjectToolbox{}, err
	}
	if err := validateProjectToolbox(value); err != nil {
		return ProjectToolbox{}, err
	}
	file, err := s.load()
	if err != nil {
		return ProjectToolbox{}, err
	}
	file.Projects[key] = value
	if err := s.saveFile(file); err != nil {
		return ProjectToolbox{}, err
	}
	return value, nil
}

func (s projectToolboxStore) saveFile(value projectToolboxFile) error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("사용자 Toolbox 경로를 확인할 수 없습니다")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("Toolbox 폴더를 만들 수 없습니다: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("Toolbox를 직렬화할 수 없습니다: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "projects-*.tmp")
	if err != nil {
		return fmt.Errorf("Toolbox 임시 파일을 만들 수 없습니다: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("Toolbox 임시 파일을 쓸 수 없습니다: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("Toolbox 파일을 저장할 수 없습니다: %w", err)
	}
	return nil
}
