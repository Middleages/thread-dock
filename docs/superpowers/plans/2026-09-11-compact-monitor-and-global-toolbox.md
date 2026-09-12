# Compact Monitor and Global Toolbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the permanent left/right Monitor sidebars, move project actions and Herdr status into the main content, split project-local references from a global Toolbox drawer, and support global Todo items with an optional single-project link.

**Architecture:** Keep the Monitor read-only boundary for GitHub/Herdr. Replace the current monolithic `ProjectToolbox` persistence with two local JSON stores behind Wails methods: project references in `projects.json` and global commands/Todos in `toolbox.json`. The React shell becomes top-bar + full-width content; the global Toolbox is an overlay drawer; Herdr details are collapsed by default. Existing project-level command/checklist data is migrated idempotently before it is discarded.

**Tech Stack:** Go, Wails v2, React 19, TypeScript 7, Vitest, Testing Library, CSS.

**Spec:** `docs/superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md`

## Global Constraints

- ThreadDock remains a Monitor + Skills product; this change must not add a scheduler, task execution engine, shell runner, or Agent prompt injection.
- GitHub remains the durable work source and Herdr remains the top-level execution lifecycle source.
- Toolbox content is human-only and is never automatically read by Coordinator, Feature Leader, or project Skills.
- Saved commands are copy-only. Do not add a Wails command-execution binding, shell button, terminal embedding, or fallback execution path.
- A Todo may have `projectKey = ""` (common/global) or exactly one project key. Do not support multiple project links.
- Keep JSON storage for this version. Hide storage behind Go store methods so a later SQLite implementation can replace the persistence layer without changing React contracts.
- Do not add Todo priority, due date, recurrence, assignee, history, grouping, or cloud sync.
- Preserve existing open-only polling, explicit all-history behavior, Markdown rendering, GHES support, WSL reference opening, and Herdr read-only behavior.
- Do not run the repository-wide suite after every task. Run the focused test commands listed under each task; run `make check` once in the final integration task.
- For agentic execution, prefer Sol medium for task planning/review/routing and Luna high for implementation workers where the runtime supports that distinction. Human owns the final merge.

---

## File Structure

### 2026-09-12 실행 전 정합성 보정

지정 설계를 기준으로 아래 충돌과 데이터 보존 경계를 고정했다. 원래 Task 1→6 순서는 유지한다.

- common Todo는 Go string/omitempty와 TS optional string을 사용한다. missing/null/empty/공백 입력은 common으로 정규화하고 출력은 projectKey를 생략한다.
- migration 재개 기준은 projects.json의 version1이다. global 파일이 이미 있어도 legacy project가 남으면 유효한 기존 global과 merge하고 global 저장→project v2 rewrite 순서로 재시도한다. 잘못된 파일·미지원 version·합산200개 초과는 쓰기 전 오류로 보존하며 truncation하지 않는다.
- refs 쓰기는 legacy를 migration 없이 덮어쓰지 않는다. store putReferences는 legacy에 migration-required 오류로 fail closed하고 Task2의 모든 Toolbox App 호출은 전용 mutex 아래 migration을 선행한다.
- migration은 project key 정렬로 결정적이며 semantic dedupe와 ID 충돌을 구분한다. 서로 다른 항목의 중복 ID는 결정적으로 재키하고 기존 global 항목을 우선 보존한다.
- 설계대로 Todo는 미완료 기본/완료 포함 toggle을 제공한다. unknown project는 `알 수 없는 프로젝트 (<raw key>)`로 표시하고 common/현재 프로젝트로 재지정할 수 있다. 프로젝트 Todo shortcut은 Todo 탭과 해당 필터를 함께 연다.
- App이 global Toolbox load/cache/save를 한 곳에서 소유한다. 최초 load 성공 전과 save 중에는 mutation을 막고, 실패 시 입력과 마지막 확정 값을 유지한다. Drawer는 아래 controlled props를 추가로 받는다: `toolbox: GlobalToolbox | null`, `loadError?: string`, `loading: boolean`, `saving: boolean`, `onReload: () => void`, `onSave: (next: GlobalToolbox) => Promise<GlobalToolbox>`. App은 초기 한 번 load해 count를 계산하며 오류 후 명시적 reload/reopen에서 재시도한다. ProjectDetail은 계획의 `projectTodoCount: number`를 사용하고 양수일 때 shortcut을 표시한다.
- compile-valid 중간 전이를 위해 Task1 legacy Go types/get/put, Task2 old Wails/TS contracts, Task3 old ProjectToolbox 파일은 마지막 consumer 교체까지 임시 유지한다. Task4에서 App 교체 후 기존 component/test/CSS·old bindings/methods·legacy get/put을 제거한다. legacy 디스크 decoder 타입은 private로 남길 수 있다. v2에 대한 legacy write는 거절해 schema downgrade를 막는다.
- Task2에는 project-key helper 추출만 위한 App.tsx 소유권을 추가한다. Task4에는 old API 제거를 위한 toolbox.go/toolbox_test.go/app.go/app_test.go/bindings.ts/bindings.test.ts 및 old ProjectToolbox 세 파일 소유권을 추가한다. 이 공유 경로는 직렬 작업한다.
- Task6의 사전 전체 Go/UI/build 묶음은 이미 실행한 focused 증거와 중복되므로 추가 full suite로 실행하지 않는다. 변경 영향 검증과 마지막 make check 한 번을 구분한다. Windows/native smoke는 별도 실제 근거가 없으면 not run이다.

Task packet·검증·결정 근거는 [진행 ledger](../../operator/2026-09-12-compact-monitor-ledger.md)를 따른다. 제품 SQLite/새 engine/command 실행/Agent 주입 범위는 추가하지 않는다.

### Backend

- Modify `monitor/toolbox.go` — own Toolbox domain types, validation, project-reference store, global Toolbox store, and idempotent v1→v2 migration.
- Modify `monitor/toolbox_test.go` — focused storage, validation, migration, and failure-preservation tests.
- Modify `monitor/app.go` — expose Wails-facing project-reference/global-Toolbox methods; keep local/WSL file opening only.
- Modify `monitor/app_test.go` — Wails boundary tests for the new methods where useful.

### Frontend contracts and state

- Modify `monitor/frontend/src/bindings.ts` — replace project Toolbox contract with project references + global Toolbox contract.
- Modify `monitor/frontend/src/bindings.test.ts` — assert exact Wails calls and no fetch fallback.
- Create `monitor/frontend/src/project-key.ts` — one shared function that derives the stable Toolbox project key from a Monitor `Project`.
- Create `monitor/frontend/src/project-key.test.ts` — GHES/GitHub fallback coverage for project-key derivation.

### Frontend UI

- Create `monitor/frontend/src/TopBar.tsx` — brand, connection state, Toolbox button, Settings button.
- Create `monitor/frontend/src/ProjectDetail.tsx` — project header, work actions, `업무 / 자료` tabs, project Todo count shortcut.
- Create `monitor/frontend/src/ProjectReferences.tsx` — project-local web/Windows/WSL reference CRUD only.
- Create `monitor/frontend/src/GlobalToolboxDrawer.tsx` — overlay drawer with `명령어 / 할 일` tabs, command copy-only behavior, Todo project filtering.
- Create `monitor/frontend/src/HerdrSummary.tsx` — collapsed summary and explicit detail expansion.
- Modify `monitor/frontend/src/App.tsx` — snapshot selection/composition only; remove permanent action rail and inline detailed Herdr panel.
- Modify `monitor/frontend/src/SettingsShell.tsx` — make settings dialog controlled by the top bar instead of a floating launcher.
- Modify `monitor/frontend/src/main.tsx` — keep a single root composition after Settings/App ownership changes.

### Frontend styles/tests

- Create `monitor/frontend/src/top-bar.css`.
- Create `monitor/frontend/src/project-detail.css`.
- Create `monitor/frontend/src/project-references.css`.
- Create `monitor/frontend/src/global-toolbox-drawer.css`.
- Create `monitor/frontend/src/herdr-summary.css`.
- Modify `monitor/frontend/src/styles.css` — remove the fixed 190px/main/300px shell and action-rail rules; keep shared Monitor primitives.
- Modify or remove `monitor/frontend/src/project-toolbox.css` after the split.
- Create focused tests for each new component.
- Modify `monitor/frontend/src/AppScope.test.tsx` and `monitor/frontend/src/monitor.test.tsx` only where the DOM contract intentionally changes.
- Modify `Makefile` to include the new focused frontend test files in `FRONTEND_TESTS` and remove obsolete `ProjectToolbox.test.tsx` after replacement.

---

### Task 1: Split Toolbox persistence and add idempotent migration

**Files:**
- Modify: `monitor/toolbox.go`
- Modify: `monitor/toolbox_test.go`

**Interfaces:**
- Produces backend domain types:

```go
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
```

- Project file v2:

```go
type projectReferencesFile struct {
    Version  int                          `json:"version"`
    Projects map[string]ProjectReferences `json:"projects"`
}
```

- Global file v1:

```go
type globalToolboxFile struct {
    Version int              `json:"version"`
    Toolbox GlobalToolbox    `json:"toolbox"`
}
```

- Store methods used by Task 2:

```go
func defaultProjectToolboxStore() projectToolboxStore
func defaultGlobalToolboxStore() globalToolboxStore
func (s projectToolboxStore) getReferences(key string) (ProjectReferences, error)
func (s projectToolboxStore) putReferences(key string, value ProjectReferences) (ProjectReferences, error)
func (s globalToolboxStore) get(projects projectToolboxStore) (GlobalToolbox, error)
func (s globalToolboxStore) put(projects projectToolboxStore, value GlobalToolbox) (GlobalToolbox, error)
```

- `globalToolboxStore.get/put` must call an internal migration helper before returning or writing user-visible data.

- [ ] **Step 1: Write failing tests for the new schemas and project-reference-only writes**

Add tests that prove:

```go
func TestProjectReferencesStorePersistsOnlyReferences(t *testing.T) { /* ... */ }
func TestGlobalToolboxStorePersistsCommandsAndTodos(t *testing.T) { /* ... */ }
func TestGlobalTodoAcceptsCommonOrOneProjectKey(t *testing.T) { /* ... */ }
```

Test `ProjectReferences` with one HTTP reference and verify the resulting `projects.json` does not contain `commands`, `checklist`, or `todos` keys.

Test `GlobalToolbox` with one command and two Todos:

```go
Todos: []ToolboxTodo{
    {ID: "todo-common", Text: "GitHub token 확인", Done: false},
    {ID: "todo-jmj", Text: "staging migration 확인", Done: false, ProjectKey: "github.example/repo:acme/jmj"},
}
```

- [ ] **Step 2: Run the focused tests and confirm they fail**

Run:

```bash
go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo)' -count=1
```

Expected: FAIL because `ProjectReferences`, `GlobalToolbox`, `ToolboxTodo`, and the new store methods do not exist yet.

- [ ] **Step 3: Implement the v2 project-reference store and global Toolbox store**

In `monitor/toolbox.go`:

1. Keep `ToolboxReference` and `ToolboxCommand` validation behavior.
2. Replace new writes of `ProjectToolbox` with `ProjectReferences`.
3. Add `globalToolboxStore{path string}` using `%APPDATA%/ThreadDock/toolbox.json` through `os.UserConfigDir()`.
4. Use the same atomic temp-file + rename pattern for both stores.
5. Keep limits:
   - references: max `maxToolboxItems` per project
   - global commands: max `maxToolboxItems`
   - global Todos: max `maxToolboxItems`
   - command/reference text limits as today
   - Todo text max 2000
   - Todo project key: empty or valid via `validateToolboxProjectKey`
6. Keep project-key validation independent from Agent/Herdr state.

- [ ] **Step 4: Write failing migration tests using the existing v1 `projects.json` shape**

Cover all of these cases:

```go
func TestToolboxMigrationPreservesProjectReferences(t *testing.T) { /* ... */ }
func TestToolboxMigrationMovesCommandsToGlobalStore(t *testing.T) { /* ... */ }
func TestToolboxMigrationLinksLegacyChecklistToOriginProject(t *testing.T) { /* ... */ }
func TestToolboxMigrationDeduplicatesCommandsByLabelAndCommand(t *testing.T) { /* ... */ }
func TestToolboxMigrationKeepsSameTodoTextFromDifferentProjectsSeparate(t *testing.T) { /* ... */ }
func TestToolboxMigrationIsIdempotentAfterPartialCompletion(t *testing.T) { /* ... */ }
func TestToolboxMigrationLeavesLegacyProjectFileUntouchedWhenGlobalWriteFails(t *testing.T) { /* ... */ }
```

Use legacy v1 data equivalent to:

```json
{
  "version": 1,
  "projects": {
    "github.example/repo:acme/jmj": {
      "references": [{"id":"ref-jmj","label":"JMJ Wiki","type":"web","target":"https://example.com/jmj"}],
      "commands": [{"id":"cmd-psql","label":"psql","command":"psql -h localhost"}],
      "checklist": [{"id":"check-jmj","text":"migration 확인","done":false}]
    },
    "github.example/repo:acme/piece": {
      "references": [],
      "commands": [{"id":"cmd-psql-2","label":"psql","command":"psql -h localhost"}],
      "checklist": [{"id":"check-piece","text":"migration 확인","done":true}]
    }
  }
}
```

Expected migration:
- one global `psql` command after dedupe by exact trimmed `label + command`
- two Todos named `migration 확인`, one linked to JMJ and one linked to PIECE
- preserved references in project v2
- global file saved before project v2 rewrite
- retry after partial migration produces no duplicate commands/Todos

For deterministic partial-write failure injection, give `globalToolboxStore` and/or project store a small internal write hook used only by tests, for example:

```go
type toolboxFileWriter func(path, tempPattern string, value any) error
```

Default it to the real atomic JSON writer. Do not expose this through Wails.

- [ ] **Step 5: Implement idempotent migration**

Migration rules:

1. Detect project v1 by the JSON `version` and legacy project item shape.
2. Load existing `toolbox.json` if present; otherwise start empty.
3. Merge legacy commands into global commands, deduping exact trimmed `label + command`.
4. Convert each legacy checklist item to a `ToolboxTodo` with `ProjectKey` equal to that legacy project key.
5. Deduplicate retry-generated Todos by `projectKey + trimmed text`; when duplicates disagree on `done`, keep `done=false` if any copy is incomplete.
6. Atomically save merged global toolbox first.
7. Only after that succeeds, atomically rewrite `projects.json` as v2 references-only.
8. If project rewrite fails after global success, a retry must merge idempotently and retry the v2 rewrite.

- [ ] **Step 6: Run focused backend tests**

Run:

```bash
go test ./monitor -run 'Test(ProjectReferencesStore|GlobalToolboxStore|GlobalTodo|ToolboxMigration)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```bash
git add monitor/toolbox.go monitor/toolbox_test.go
git commit -m "feat: split project references and global toolbox storage"
```

---

### Task 2: Replace Wails Toolbox contracts and frontend bindings

**Files:**
- Modify: `monitor/app.go`
- Modify: `monitor/app_test.go`
- Modify: `monitor/frontend/src/bindings.ts`
- Modify: `monitor/frontend/src/bindings.test.ts`
- Create: `monitor/frontend/src/project-key.ts`
- Create: `monitor/frontend/src/project-key.test.ts`

**Interfaces:**

Backend Wails methods:

```go
func (a *App) GetProjectReferences(projectKey string) (ProjectReferences, error)
func (a *App) SaveProjectReferences(projectKey string, refs ProjectReferences) (ProjectReferences, error)
func (a *App) GetGlobalToolbox() (GlobalToolbox, error)
func (a *App) SaveGlobalToolbox(toolbox GlobalToolbox) (GlobalToolbox, error)
func (a *App) OpenToolboxReference(referenceType, target string) error
```

Frontend types:

```ts
export type ToolboxReferenceType = 'web' | 'file' | 'wsl-file'

export interface ProjectReferences {
  references: ToolboxReference[]
}

export interface ToolboxTodo {
  id: string
  text: string
  done: boolean
  projectKey?: string
}

export interface GlobalToolbox {
  commands: ToolboxCommand[]
  todos: ToolboxTodo[]
}
```

Project-key helper:

```ts
export function toolboxKeyFor(project: Project): string
```

It must preserve the current behavior: prefer a valid project/work URL hostname plus `project.projectId`; otherwise use `project.projectId` alone.

- [ ] **Step 1: Update binding tests first**

In `bindings.test.ts`, assert missing Wails bindings fail explicitly for all four new methods and never call `fetch`.

Then install fake bindings and assert:

```ts
await getProjectReferences('github.example/repo:acme/app')
await saveProjectReferences('github.example/repo:acme/app', refs)
await getGlobalToolbox()
await saveGlobalToolbox(toolbox)
```

call the exact Wails method names and arguments.

- [ ] **Step 2: Write project-key unit tests before moving the helper**

Cover:

```ts
expect(toolboxKeyFor(githubDotComProject)).toBe('github.com/repo:acme/app')
expect(toolboxKeyFor(ghesProject)).toBe('github.samsungds.net/repo:FDYPhotoDX/jmj')
expect(toolboxKeyFor(projectWithoutURL)).toBe(projectWithoutURL.projectId)
```

- [ ] **Step 3: Run frontend focused tests and confirm failure**

Run:

```bash
npm --prefix monitor/frontend test -- src/bindings.test.ts src/project-key.test.ts
```

Expected: FAIL until the new methods/helper exist.

- [ ] **Step 4: Update `App` store ownership and Wails methods**

Change `App` fields from one project Toolbox store to two stores:

```go
projectToolbox projectToolboxStore
globalToolbox  globalToolboxStore
```

Initialize both in `NewApp` and `NewConfigurableApp`.

Implement the four Wails methods using Task 1 stores. `GetGlobalToolbox` and `SaveGlobalToolbox` must use the migration-aware global store methods.

Keep `OpenToolboxReference` unchanged except for type names needed by the refactor. Do not add command execution.

- [ ] **Step 5: Replace TypeScript bindings and move `toolboxKeyFor`**

Remove old `GetProjectToolbox` / `SaveProjectToolbox` frontend contracts and expose the four new methods.

Move `toolboxKeyFor` out of `App.tsx` into `project-key.ts` so project references, Todo filters, and project Todo shortcuts share one identity implementation.

- [ ] **Step 6: Run focused tests**

Run:

```bash
go test ./monitor -run 'Test.*Toolbox|Test.*ProjectReferences' -count=1
npm --prefix monitor/frontend test -- src/bindings.test.ts src/project-key.test.ts
```

Expected: PASS.

- [ ] **Step 7: Commit Task 2**

```bash
git add monitor/app.go monitor/app_test.go monitor/frontend/src/bindings.ts monitor/frontend/src/bindings.test.ts monitor/frontend/src/project-key.ts monitor/frontend/src/project-key.test.ts
git commit -m "refactor: expose project references and global toolbox bindings"
```

---

### Task 3: Split project references from global Toolbox drawer

**Files:**
- Create: `monitor/frontend/src/ProjectReferences.tsx`
- Create: `monitor/frontend/src/ProjectReferences.test.tsx`
- Create: `monitor/frontend/src/project-references.css`
- Create: `monitor/frontend/src/GlobalToolboxDrawer.tsx`
- Create: `monitor/frontend/src/GlobalToolboxDrawer.test.tsx`
- Create: `monitor/frontend/src/global-toolbox-drawer.css`
- Delete after replacement: `monitor/frontend/src/ProjectToolbox.tsx`
- Delete after replacement: `monitor/frontend/src/ProjectToolbox.test.tsx`
- Delete after replacement: `monitor/frontend/src/project-toolbox.css`

**Interfaces:**

```ts
export function ProjectReferences(props: {
  projectKey: string
  projectName: string
  onStatus?: (message: string) => void
}): JSX.Element
```

```ts
export function GlobalToolboxDrawer(props: {
  open: boolean
  projects: Project[]
  initialProjectKey?: string
  onClose: () => void
  onStatus?: (message: string) => void
}): JSX.Element | null
```

Drawer local UI state:

```ts
type ToolboxTab = 'commands' | 'todos'
type TodoFilter = 'all' | 'common' | string // projectKey for project filter
```

- [ ] **Step 1: Write `ProjectReferences` tests**

Assert:
- loads project references by key
- renders only `자료`, never `명령어` or `체크리스트`
- adds/deletes web and local references through `saveProjectReferences`
- web uses `openExternalURL`
- Windows/WSL references use `openToolboxReference`
- save failure preserves form input and shows an inline error

- [ ] **Step 2: Write Drawer tests before implementation**

Assert:
- returns `null` when closed
- opens with `명령어` and `할 일` tabs
- closes via close button, backdrop, and Escape
- command rows have `복사` and `삭제` but no `실행`
- Todo form has project selector with `공통` plus all supplied projects
- adding a Todo with JMJ selected saves exactly one `projectKey`
- `전체`, `공통`, and project filters show the expected Todo rows
- completed items remain visible in MVP and can be unchecked
- `initialProjectKey` opens/selects the matching project filter when the Todo tab is active

Use a sample project map with JMJ and PIECE so filtering behavior is explicit.

- [ ] **Step 3: Run focused tests and confirm failure**

```bash
npm --prefix monitor/frontend test -- src/ProjectReferences.test.tsx src/GlobalToolboxDrawer.test.tsx
```

Expected: FAIL because the components do not exist.

- [ ] **Step 4: Implement `ProjectReferences` by extracting only the reference portion of the old component**

Reuse the current form fields:
- label
- type (`web`, `file`, `wsl-file`)
- target

Keep immediate save semantics after add/delete. Keep copy/open actions. Do not import or render command/Todo data.

- [ ] **Step 5: Implement `GlobalToolboxDrawer`**

Behavior:
1. Load `getGlobalToolbox()` only when the drawer first opens; reload after a previous load error when reopened.
2. Persist the full current global Toolbox with `saveGlobalToolbox(next)` after add/delete/check changes.
3. Commands keep `label`, optional `note`, `command`, and copy-only behavior.
4. Todos keep `text`, `done`, and optional `projectKey`.
5. Project selector options are derived from `projects.map(project => ({ key: toolboxKeyFor(project), name: project.name }))` and deduped by key.
6. A Todo whose saved project key is no longer present in current Monitor projects is still shown in `전체`; show its raw project key as the association label rather than dropping it.
7. Drawer uses `role="dialog"`, `aria-modal="true"`, visible close button, backdrop close, and Escape close.
8. Do not add command execution or Agent actions.

- [ ] **Step 6: Run focused tests**

```bash
npm --prefix monitor/frontend test -- src/ProjectReferences.test.tsx src/GlobalToolboxDrawer.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Remove obsolete ProjectToolbox files and commit**

```bash
git rm monitor/frontend/src/ProjectToolbox.tsx monitor/frontend/src/ProjectToolbox.test.tsx monitor/frontend/src/project-toolbox.css
git add monitor/frontend/src/ProjectReferences.tsx monitor/frontend/src/ProjectReferences.test.tsx monitor/frontend/src/project-references.css monitor/frontend/src/GlobalToolboxDrawer.tsx monitor/frontend/src/GlobalToolboxDrawer.test.tsx monitor/frontend/src/global-toolbox-drawer.css
git commit -m "feat: split project references from global toolbox drawer"
```

---

### Task 4: Replace permanent sidebars with top bar and inline work actions

**Files:**
- Create: `monitor/frontend/src/TopBar.tsx`
- Create: `monitor/frontend/src/TopBar.test.tsx`
- Create: `monitor/frontend/src/top-bar.css`
- Create: `monitor/frontend/src/ProjectDetail.tsx`
- Create: `monitor/frontend/src/ProjectDetail.test.tsx`
- Create: `monitor/frontend/src/project-detail.css`
- Modify: `monitor/frontend/src/App.tsx`
- Modify: `monitor/frontend/src/SettingsShell.tsx`
- Modify: `monitor/frontend/src/main.tsx`
- Modify: `monitor/frontend/src/styles.css`
- Modify: `monitor/frontend/src/settings.css`

**Interfaces:**

```ts
export function TopBar(props: {
  connectionDegraded: boolean
  onOpenToolbox: () => void
  onOpenSettings: () => void
}): JSX.Element
```

```ts
export function ProjectDetail(props: {
  project: Project
  selectedWorkId: string | null
  herdr?: HerdrSnapshot
  onSelectWork: (id: string) => void
  onOpenProjectTodos: (projectKey: string) => void
  onStatus: (message: string) => void
}): JSX.Element
```

Settings ownership:

```ts
export function SettingsShell(): JSX.Element
```

`SettingsShell` owns `settingsOpen` state and renders:

```tsx
<App onOpenSettings={() => setSettingsOpen(true)} />
{settingsOpen && <SettingsDialog ... />}
```

`App` owns global Toolbox drawer state because it already owns the current project list and can pass those projects directly to the drawer.

- [ ] **Step 1: Write TopBar tests**

Assert:
- `ThreadDock` brand visible
- connection displays `정상` or `확인 필요`
- Toolbox button calls `onOpenToolbox`
- Settings button calls `onOpenSettings`
- there is no left-nav semantic/sidebar requirement

- [ ] **Step 2: Write ProjectDetail tests**

Assert:
- project tabs are exactly `업무` and `자료`
- selected Issue/PR identity appears in header
- `작업 정보 복사` remains functional using the existing handoff text semantics
- `GitHub에서 보기` appears only when a safe URL exists
- `자료` renders `ProjectReferences`
- a project with two incomplete linked Todos can render `할 일 2개` and clicking `보기` calls `onOpenProjectTodos(toolboxKeyFor(project))`

To avoid loading all Todo data independently inside every project row, let `ProjectDetail` receive a project Todo count from `App` if implementation finds that cleaner. If so, use this exact alternate prop instead of a second store fetch:

```ts
projectTodoCount: number
```

and keep `onOpenProjectTodos(projectKey)` unchanged.

- [ ] **Step 3: Run focused tests and confirm failure**

```bash
npm --prefix monitor/frontend test -- src/TopBar.test.tsx src/ProjectDetail.test.tsx
```

Expected: FAIL until the new components exist.

- [ ] **Step 4: Extract `ProjectDetail` from `App.tsx` and move action-rail actions into its header**

Move/reuse:
- work identity calculation
- `buildHandoffText`
- clipboard behavior
- safe GitHub URL open behavior
- `업무 / 자료` tabs

Delete the permanent `ActionRail` component after the equivalent actions are covered by `ProjectDetail` tests.

Do not copy degraded/stale status into the project header; keep the existing global degraded banner as the single source for connection failures.

- [ ] **Step 5: Add TopBar and make Settings dialog controlled**

Refactor `SettingsShell` so it no longer adds a floating `.settings-launcher` button. Instead pass a settings-open callback to `App`, and have `TopBar` call it.

Keep existing settings load/save behavior and form contents unchanged.

- [ ] **Step 6: Replace the 3-column shell CSS**

Change `.monitor-shell` from:

```css
grid-template-columns: 190px minmax(0, 1fr) 300px;
```

into a single-column page shell. Remove permanent `.top-nav` sidebar and `.action-rail` width/border rules.

Use a centered content container such as:

```css
.main-content {
  width: min(1440px, 100%);
  margin: 0 auto;
  padding: 28px clamp(18px, 3vw, 42px) 48px;
}
```

Do not recreate fixed sidebars at any responsive breakpoint.

- [ ] **Step 7: Integrate Global Toolbox drawer in `App`**

Add App state:

```ts
const [toolboxOpen, setToolboxOpen] = useState(false)
const [toolboxProjectFilter, setToolboxProjectFilter] = useState<string | undefined>()
```

Top-bar open:

```ts
setToolboxProjectFilter(undefined)
setToolboxOpen(true)
```

Project Todo shortcut:

```ts
setToolboxProjectFilter(projectKey)
setToolboxOpen(true)
```

Pass current sorted `projects` to `GlobalToolboxDrawer`.

For Todo counts in project detail, prefer one global Toolbox load owned by App/drawer state rather than N per-project file reads. Keep this simple: one in-memory `GlobalToolbox | null` cache refreshed after Drawer save, then derive counts by `projectKey`.

- [ ] **Step 8: Run focused UI tests**

```bash
npm --prefix monitor/frontend test -- src/TopBar.test.tsx src/ProjectDetail.test.tsx src/GlobalToolboxDrawer.test.tsx src/AppScope.test.tsx
```

Expected: PASS.

- [ ] **Step 9: Commit Task 4**

```bash
git add monitor/frontend/src/TopBar.tsx monitor/frontend/src/TopBar.test.tsx monitor/frontend/src/top-bar.css monitor/frontend/src/ProjectDetail.tsx monitor/frontend/src/ProjectDetail.test.tsx monitor/frontend/src/project-detail.css monitor/frontend/src/App.tsx monitor/frontend/src/SettingsShell.tsx monitor/frontend/src/main.tsx monitor/frontend/src/styles.css monitor/frontend/src/settings.css
git commit -m "refactor: center monitor around full-width project content"
```

---

### Task 5: Collapse Herdr details into a summary row

**Files:**
- Create: `monitor/frontend/src/HerdrSummary.tsx`
- Create: `monitor/frontend/src/HerdrSummary.test.tsx`
- Create: `monitor/frontend/src/herdr-summary.css`
- Modify: `monitor/frontend/src/App.tsx`
- Modify: `monitor/frontend/src/ProjectDetail.tsx`
- Modify: `monitor/frontend/src/styles.css`

**Interfaces:**

```ts
export function HerdrSummary(props: {
  herdr: HerdrSnapshot
  project?: Project
  work?: WorkItem
}): JSX.Element
```

The component may reuse/move these existing helpers from `App.tsx`:
- `connectionsForWork`
- `herdrGuidance`
- `HerdrConnections`

- [ ] **Step 1: Write failing HerdrSummary tests**

Cover:
- default render contains one compact `실행 상태` summary and a `자세히` button
- connected/working displays the chosen session name and `작업 중`
- blocked/offline/stale/unverified are visible in collapsed text without automatic expansion
- workspace/tab/pane/cwd are not visible before expansion
- clicking `자세히` reveals detailed connections, observed sessions, notices, and unconnected agents
- clicking `접기` hides details again

- [ ] **Step 2: Run the focused test and confirm failure**

```bash
npm --prefix monitor/frontend test -- src/HerdrSummary.test.tsx
```

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement the summary using existing Herdr selection semantics**

Priority for collapsed summary:
1. exact issue connection
2. matching project connection
3. repository coordinator fallback
4. no matching connection

Render one line similar to:

```text
실행 상태  ● 연결됨 · feature-123 · 작업 중    [자세히]
```

Do not change matching rules while moving them. This task is a density refactor, not a Herdr semantics change.

- [ ] **Step 4: Replace the always-expanded `HerdrPanel` in App**

Remove the large standalone Herdr panel from the bottom of the page. Render `HerdrSummary` inside/below the selected project detail so it is visually associated with the work the user is viewing.

- [ ] **Step 5: Run focused tests**

```bash
npm --prefix monitor/frontend test -- src/HerdrSummary.test.tsx src/monitor.test.tsx src/AppScope.test.tsx
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

```bash
git add monitor/frontend/src/HerdrSummary.tsx monitor/frontend/src/HerdrSummary.test.tsx monitor/frontend/src/herdr-summary.css monitor/frontend/src/App.tsx monitor/frontend/src/ProjectDetail.tsx monitor/frontend/src/styles.css
git commit -m "refactor: collapse herdr details into work summary"
```

---

### Task 6: Update regression gates, docs, and run final verification once

**Files:**
- Modify: `Makefile`
- Modify: `README.md`
- Modify: `docs/usability-checklist.md`
- Modify: `docs/superpowers/reviews/2026-09-11-project-toolbox-verification.md` or create a new verification note for this redesign if preserving historical review context is clearer.
- Modify only if required by intentional DOM changes: `monitor/frontend/src/monitor.test.tsx`, `monitor/frontend/src/AppScope.test.tsx`

**Interfaces:**
- No new product interfaces. This task closes regression coverage and user-facing documentation.

- [ ] **Step 1: Update `FRONTEND_TESTS` in Makefile**

Ensure the list includes:

```make
src/bindings.test.ts \
src/project-key.test.ts \
src/monitor.test.tsx \
src/WorkTable.test.tsx \
src/AppScope.test.tsx \
src/MarkdownBody.test.tsx \
src/ProjectReferences.test.tsx \
src/GlobalToolboxDrawer.test.tsx \
src/TopBar.test.tsx \
src/ProjectDetail.test.tsx \
src/HerdrSummary.test.tsx
```

Remove obsolete `ProjectToolbox.test.tsx`.

- [ ] **Step 2: Update README behavior, not architecture marketing**

Document only current user-visible behavior:
- top bar instead of permanent sidebars
- project `업무 / 자료`
- global Toolbox drawer `명령어 / 할 일`
- Todo optional single-project association and filters
- `%APPDATA%\ThreadDock\projects.json` references
- `%APPDATA%\ThreadDock\toolbox.json` commands/Todos
- commands are copy-only
- Toolbox remains human-only

Do not imply SQLite is used.

- [ ] **Step 3: Update usability checklist for the redesign**

Add manual checks:
- 1280px: no permanent left/right sidebar
- project Issue body has materially more width
- top bar connection/Toolbox/Settings remain visible
- Drawer does not resize main content
- Drawer Escape/backdrop close
- common/project Todo filters
- project shortcut opens Drawer prefiltered
- command has no execute control
- migrated legacy items visible
- Herdr details collapsed by default and expandable
- narrow viewport keeps actions usable

- [ ] **Step 4: Run formatting and focused package/build checks before the full gate**

Run:

```bash
gofmt -w monitor/toolbox.go monitor/toolbox_test.go monitor/app.go monitor/app_test.go
go test ./monitor -count=1
npm --prefix monitor/frontend test -- src/bindings.test.ts src/project-key.test.ts src/ProjectReferences.test.tsx src/GlobalToolboxDrawer.test.tsx src/TopBar.test.tsx src/ProjectDetail.test.tsx src/HerdrSummary.test.tsx src/AppScope.test.tsx src/MarkdownBody.test.tsx src/WorkTable.test.tsx src/monitor.test.tsx
npm --prefix monitor/frontend run build
```

Expected: all PASS.

- [ ] **Step 5: Run the repository-wide gate once**

Run:

```bash
make check
```

Expected: PASS.

If `make check` fails outside files changed by this redesign, record the exact failing command/output before deciding whether it is caused by this branch. Do not hide or relabel an unrelated failure as passing.

- [ ] **Step 6: Windows package smoke**

On Windows:

```powershell
cd monitor
wails build
.\build\bin\ThreadDockMonitor.exe
```

Manual smoke:
1. Confirm top bar replaces both fixed sidebars.
2. Open a project and verify `업무 / 자료`.
3. Add/open a web reference and a local or WSL file reference.
4. Open global Toolbox Drawer and add a `psql` command; verify only copy is offered.
5. Add one common Todo and one project-linked Todo; filter `전체 / 공통 / <project>`.
6. From the project detail, use `할 일 N개 보기` and verify the drawer opens with that project filter.
7. Expand/collapse Herdr details.
8. Restart the app and verify references/commands/Todos persist.
9. If legacy `projects.json` exists, verify migration preserves all prior references, commands, and checklist items.

- [ ] **Step 7: Write verification note with exact evidence**

Record:
- final commit SHA
- exact focused test commands/results
- `make check` result
- Windows `wails build` result
- manual GHES/WSL/Herdr smoke result or explicit `not run`
- migration smoke result or explicit `not run`

Never promote earlier PR #87 verification to this branch's current SHA.

- [ ] **Step 8: Commit Task 6**

```bash
git add Makefile README.md docs/usability-checklist.md docs/superpowers/reviews monitor/frontend/src/monitor.test.tsx monitor/frontend/src/AppScope.test.tsx
git commit -m "docs: close compact monitor verification gate"
```

---

## Final Review Checklist

Before opening or updating a PR, the coordinator/reviewer must confirm:

- [ ] No permanent `190px` left sidebar remains.
- [ ] No permanent `300px` action rail remains.
- [ ] No command execution method was added to Go or TypeScript bindings.
- [ ] Project detail tabs are exactly `업무 / 자료`.
- [ ] Global Drawer tabs are `명령어 / 할 일`.
- [ ] Todo supports zero or one `projectKey`, never an array.
- [ ] Common/project filters are derived from one global Todo dataset, not duplicated per project.
- [ ] Legacy project commands/checklists migrate without silent loss.
- [ ] Project references remain project-local.
- [ ] Toolbox is not referenced from project Agent/Skill files.
- [ ] Herdr matching semantics did not change while UI density changed.
- [ ] Closed-history mode still does not auto-poll.
- [ ] Markdown rendering tests still pass.
- [ ] GHES and WSL reference behavior remain covered.
- [ ] `make check` ran only at final integration, not as a per-worker bottleneck.

## Next-session execution prompt

Use this prompt to start implementation in the next session:

```text
@GitHub Middleages/thread-dock의 agent/compact-monitor-toolbox 브랜치에서 구현을 시작해줘.

먼저 아래 두 문서를 읽고 그대로 작업해.
- docs/superpowers/specs/2026-09-11-compact-monitor-and-global-toolbox-design.md
- docs/superpowers/plans/2026-09-11-compact-monitor-and-global-toolbox.md

목표:
- 고정 좌/우 sidebar 제거, top bar + 넓은 main content
- 프로젝트 내부는 업무/자료
- 전역 Toolbox는 오른쪽 Drawer의 명령어/할 일
- Todo는 공통 또는 프로젝트 1개만 선택적으로 연결, 전체/공통/프로젝트 필터
- 기존 프로젝트별 command/checklist는 안전하게 migration
- Herdr는 기본 한 줄 summary + 명시적 펼치기
- command는 복사 전용이며 실행 기능 추가 금지
- Toolbox는 Agent/Skill에 자동 주입 금지

작업 지침:
- 계획/검토/태스크 분배는 Sol medium 중심으로 해.
- 구현 worker는 가능하면 Luna high를 사용하고, 독립적인 Task는 병렬화해도 된다.
- 각 Task는 계획서의 focused test만 실행해. worker마다 full suite를 돌리지 마.
- 최종 통합 후에만 make check를 1회 실행해.
- 큰 작업단위마다 commit하고, 최종적으로 PR을 만들어 검토 가능한 상태로 올려.
- merge는 하지 마. 최종 merge는 내가 한다.
- 테스트/빌드/Windows smoke를 실제로 실행하지 않았다면 통과했다고 쓰지 마.
- 기존 GitHub/Herdr Monitor 경계와 open-only/all-history 동작을 깨지 마.

계획 Task 1부터 순서대로 시작해.
```
