# Subagents (Tree, Live Pane, Board) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** show each Claude Code subagent under its session in the sidebar, live in a read-only floating pane, and on one board across all sessions, from documented hooks only.

**Architecture:** `kaze notify` gains three hook kinds (`agent-start`, `agent-stop`, `tool`) and forwards the session identity (issue #23 Part 1) on every message. A new Gio-free package `internal/subagents` folds those messages into a per-tab tracker (pairing, steps, waiting, limits, resets). The UI reads the tracker to draw the sidebar tree, a new `paneAgent` floating pane kind, and a full-page board like Settings.

**Tech Stack:** Go 1.27, Gio v0.10.3.

**Spec:** `docs/superpowers/specs/2026-10-31-kaze-subagents-design.md` (owner's decision: the live pane shows steps and the report only, no transcript reading). Mockup: `docs/design/agents-combined-mockup.html`.

## Global Constraints

- Hooks only. Nothing reads `transcript_path`, `~/.claude/projects/...jsonl` or `meta.json`.
- Every hook command exits 0, prints nothing, and stays inside `notifyTotal` (1 s), with or without kaze running.
- `kaze notify tool` with no `agent_id` and a tool other than `Agent` / `Task` returns before dialing.
- Idle CPU zero: the only new timer is the once-a-minute elapsed refresh, armed only while a subagent is Running or Waiting.
- Every hook string is cleaned (no control, format or line-separator runes) and capped before it reaches the UI.
- Limits: 16 pending Agent calls per tab, 200 steps per subagent, 6 tree lines per session, 50 Done cards, step summary 120 runes.
- Keys: `agents_board` = Cmd+Shift+A (macOS), Ctrl+Shift+A (Windows).
- Theme tokens only; readable in all five themes, Washi included.
- No test reads or writes the owner's real `~/.claude`, `config.toml` or `workspaces.toml`. Never inject global input.
- Commits: `type(scope): Verb ...`, no AI attribution.

## Review Focus

1. **A huge `tool_input`** (Write with a multi-MB file): the hook must still exit inside 1 s and send nothing broken; a step may be lost, nothing may hang. Test in Task 3.
2. **A long `last_assistant_message`** (a 50 KB report): it must reach the pane, cut at 24 KiB, not be silently dropped by the 4 KiB body cap. Tests in Tasks 2 and 3.
3. **Hook messages from a scratch shell running Claude**: they count on the parent session's row (existing `hookTab`) but keep their own `session_id`, so the two conversations' subagents never merge or reset each other. Test in Task 5.
4. **A tab closing while its subagents run, with their panes open**: panes close, cards leave the board, the minute timer stops when nothing else runs. Test in Task 6.
5. **Stop arriving before SubagentStop** (background subagents): the row says done while the tree still shows running ones, and the chip counts them. Test in Task 5.

---

### Task 1: Spike: capture the real hook inputs

Throwaway, no product code. Confirms field names and whether a subagent's `permission_prompt` Notification carries `agent_id` (spec 5.4).

**Files:**
- Create (scratchpad only, never committed): `<scratch>/hookspike/.claude/settings.local.json`, `<scratch>/hookspike/log.sh`
- Modify: `docs/superpowers/specs/2026-10-31-kaze-subagents-design.md` (5.4, record the finding)

- [ ] **Step 1:** In a new empty folder under the scratchpad, write `log.sh` that appends its stdin plus a newline to `log.jsonl` in that folder, and a project `.claude/settings.local.json` that runs it for `SubagentStart`, `SubagentStop`, `PreToolUse` (matcher `*`) and `Notification` (no matcher). Do not touch `~/.claude`.
- [ ] **Step 2:** Ask the owner to run `claude` in that folder from a kaze tab and send: `Use a general-purpose subagent to run the Bash command "date" and report the output.` and approve the permission prompt when it appears. (Interactive, because `-p` never asks for permission. Never type into their session.)
- [ ] **Step 3:** Read `log.jsonl`. Record: does the `permission_prompt` Notification have `agent_id`? Do `PreToolUse` events inside the subagent have `agent_id` and `agent_type`? What is the Agent tool's `tool_name` (`Agent` or `Task`)? Is `session_id` the parent's in the subagent's events?
- [ ] **Step 4:** Write the answers into spec 5.4 as "Checked on Claude Code <version>, 2026-..: ...". If `PreToolUse` inside a subagent has no `agent_id`, stop and tell the owner: steps (5.3) can't work as specified.
- [ ] **Step 5:** Commit: `docs(agents): Record the real subagent hook inputs`

### Task 2: Notify messages carry session identity and subagent fields

**Files:**
- Modify: `internal/notify/notify.go`
- Test: `internal/notify/notify_test.go`

**Interfaces:**
- Produces:
  - Kinds: `KindAgentStart Kind = "agent-start"`, `KindAgentStop Kind = "agent-stop"`, `KindTool Kind = "tool"`.
  - `Msg` new `omitempty` fields: `Session string` (`json:"session"`), `Transcript string` (`json:"transcript"`), `Cwd string` (`json:"cwd"`), `AgentID string` (`json:"agent_id"`), `AgentType string` (`json:"agent_type"`), `Tool string` (`json:"tool"`), `Summary string` (`json:"summary"`), `Description string` (`json:"description"`), `Report string` (`json:"report"`).
  - Caps: `MaxSession = 128` bytes, `MaxPath = 1024` bytes, `MaxAgentField = 128` runes, `MaxReport = 24 << 10` bytes; `maxBody` raised from 4 KiB to `32 << 10`.
  - `func CleanLine(s string, max int) string`: like `CleanArgs` but with a cap parameter (`CleanArgs` becomes `CleanLine(s, maxArgsRunes)`).
  - `func CleanReport(s string, max int) string`: keeps `\n` and `\t`, drops every other control, format and U+2028/2029 rune, cuts to `max` bytes on a rune boundary.

- [ ] **Step 1: Write the failing tests**

```go
func TestMsgSubagentFieldsRoundTrip(t *testing.T)   // Send a KindTool Msg with every new field set; the server's onMsg gets them unchanged
func TestServerAcceptsLargeReport(t *testing.T)      // a KindAgentStop with a 24 KiB Report arrives; a 40 KiB body is refused (bounded)
func TestCleanLineCapsAndStrips(t *testing.T)        // CleanLine("a\x1b[31mb‮c\nd", 3) == "a[3"; never more than max runes
func TestCleanReportKeepsNewlines(t *testing.T)      // CleanReport("one\ntwo\x07 ", 100) == "one\ntwo"; a 30 KiB input cut to <= MaxReport bytes and utf8.ValidString
```

- [ ] **Step 2: Run** `go test ./internal/notify/` — Expected: FAIL (undefined `KindTool`, `CleanLine`, ...).
- [ ] **Step 3: Implement** the kinds, fields, caps and cleaners above in `internal/notify/notify.go`.
- [ ] **Step 4: Run** `go test ./internal/notify/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(notify): Carry the session identity and subagent fields`

### Task 3: `kaze notify` forwards the session and the subagent hooks

**Files:**
- Modify: `cmd/kaze/notify.go`
- Create: `cmd/kaze/agents.go` (tool summaries)
- Test: `cmd/kaze/notify_test.go`, `cmd/kaze/agents_test.go`

**Interfaces:**
- Consumes: Task 2's kinds, fields, `CleanLine`, `CleanReport`, caps.
- Produces:
  - `notifyArgs` accepts `agent-start`, `agent-stop`, `tool` (Claude only; with `--agent codex` they are rejected).
  - Every kind decodes `session_id`, `transcript_path`, `cwd` and forwards them capped (`MaxSession`, `MaxPath`); missing or malformed fields leave them empty and the message still goes.
  - `needs` also forwards `agent_id`.
  - `func toolSummary(tool string, input json.RawMessage) string` in `cmd/kaze/agents.go`, per spec 5.3 (Bash `command` first line; Read/Edit/Write `file_path` cut to its last two folders plus the name; Grep/Glob `pattern` plus ` in <path>` when set; WebFetch host+path; WebSearch `query`; else ""), then `CleanLine(..., 120)`.
  - `tool`: with `agent_id` → `Msg{Kind: KindTool, AgentID, AgentType, Tool, Summary}`. Without `agent_id` and `tool_name` is `Agent` or `Task` → `Msg{Kind: KindTool, Tool: "Agent", Description: tool_input.description, AgentType: tool_input.subagent_type}`. Otherwise return 0 **before** `kazeTab`/dial.
  - `agent-start`: `AgentID`, `AgentType`. `agent-stop`: `AgentID`, `AgentType`, `Report: CleanReport(last_assistant_message, MaxReport)`.
  - `stdinCap` raised from 64 KiB to `2 << 20`; `stdinWait` and `notifyTotal` unchanged.

- [ ] **Step 1: Write the failing tests**

```go
func TestNotifyForwardsSessionIdentity(t *testing.T)     // done/needs/prompt with session_id, transcript_path, cwd → Msg has all three; an over-long session_id is cut to MaxSession
func TestNotifyMissingSessionStillSends(t *testing.T)    // old-style JSON with none of them → message still sent, fields empty
func TestNotifyToolInSubagent(t *testing.T)              // {"agent_id":"a1","agent_type":"Explore","tool_name":"Grep","tool_input":{"pattern":"auth","path":"src"}} → KindTool, Summary "auth in src"
func TestNotifyToolMainAgentCall(t *testing.T)           // no agent_id, tool_name "Agent", tool_input {description:"Find the auth middleware", subagent_type:"Explore"} → Description and AgentType set
func TestNotifyToolMainOtherNeverDials(t *testing.T)     // no agent_id, tool_name "Bash": env points at a listener that fails the test if anything connects; returns 0
func TestNotifyAgentStopReport(t *testing.T)             // 50 KiB last_assistant_message → Report <= MaxReport bytes, valid UTF-8, received by a real notify.Server
func TestNotifyHugeToolInputBounded(t *testing.T)        // 8 MiB tool_input on a pipe: returns 0 within notifyTotal+200ms, prints nothing
func TestNotifyNeedsCarriesAgentID(t *testing.T)         // permission_prompt with agent_id "a1" → Msg.AgentID "a1"
func TestNotifyCodexRejectsAgentKinds(t *testing.T)      // "tool --agent codex" → nothing sent
func TestToolSummaryTable(t *testing.T)                  // one case per row of spec 5.3, plus control chars stripped and a 500-rune command cut to 120
```

- [ ] **Step 2: Run** `go test ./cmd/kaze/` — Expected: FAIL.
- [ ] **Step 3: Implement** in `cmd/kaze/notify.go` and `cmd/kaze/agents.go`. Decode `tool_input` as `json.RawMessage` so only the needed keys are parsed.
- [ ] **Step 4: Run** `go test ./cmd/kaze/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(notify): Forward the Claude session and subagent hooks`

### Task 4: The setup check installs the three hooks

**Files:**
- Modify: `internal/claudesetup/setup.go`, `internal/ui/settings_claude.go`
- Test: `internal/claudesetup/setup_test.go`, `internal/ui/settings_claude_test.go`

**Interfaces:**
- Produces: `Report` gains `AgentStart, AgentStop, Tools Entry`. Args `argsAgentStart = "notify agent-start"`, `argsAgentStop = "notify agent-stop"`, `argsTool = "notify tool"`. Events `SubagentStart`, `SubagentStop`, `PreToolUse` (matcher `"*"`). `Unreadable` sets all seven.

- [ ] **Step 1: Write the failing tests**

```go
func TestInspectSubagentHooks(t *testing.T)   // settings with the three kaze entries → OK; without → Missing; exe gone → Broken
func TestFixAddsSubagentHooks(t *testing.T)   // Fix on {} adds all six hooks and the status line; PreToolUse group has "matcher":"*"; summary lists "Add the subagent start hook (shows subagents)", "Add the subagent stop hook (subagent reports)", "Add the tool hook (each subagent's steps)"
func TestFixKeepsOtherPreToolUse(t *testing.T) // an existing non-kaze PreToolUse group stays byte-identical after Fix
func TestSetupRowsListSubagentHooks(t *testing.T) // the setup card shows "Subagents start", "Subagent reports", "Subagent steps" rows with hook names SubagentStart / SubagentStop / PreToolUse hook
```

- [ ] **Step 2: Run** `go test ./internal/claudesetup/ ./internal/ui/ -run 'Subagent|Fix|Inspect|Setup'` — Expected: FAIL.
- [ ] **Step 3: Implement** the Report fields, the three rows in `Fix`'s table, and three entries in `setupRows` (icons: `agents` if it exists in `iconNames`, else `prompt`; check `iconNames` before using a name, an unknown name panics).
- [ ] **Step 4: Run** the same command — Expected: PASS. Then `go test ./internal/ui/` for the whole settings suite.
- [ ] **Step 5: Commit** `feat(claude): Check and install the subagent hooks`

### Task 5: The subagent tracker

Pure Go, no Gio, no clock reads (time is passed in).

**Files:**
- Create: `internal/subagents/subagents.go`
- Test: `internal/subagents/subagents_test.go`

**Interfaces:**
- Consumes: nothing from `ui`; the UI translates `notify.Msg` into `Event`.
- Produces:

```go
type State uint8
const (Running State = iota; Waiting; Done)

type Key struct{ Session, ID string }

type Step struct {
	At            time.Time
	Tool, Summary string
}

type Agent struct {
	Key           Key
	Tab           int
	Type, Task    string
	Parent        Key   // zero when the main conversation started it
	Depth         int   // 0 = top level, capped at 1 for drawing (spec 6.1)
	State         State
	Started, Ended time.Time
	Steps         []Step // at most 200, oldest dropped
	Dropped       int    // steps dropped off the front
	Report        string
	InTree        bool   // false once the session's next prompt came after it finished
}

type EventKind uint8
const (EvStart EventKind = iota; EvStop; EvTool; EvAgentCall; EvNeeds; EvPrompt; EvSession)

type Event struct {
	Kind                          EventKind
	Tab                           int // the row's tab (after hookTab): Tree and Board use it
	Src                           int // the tab the hook ran in (a scratch shell's own id): session resets use it
	Session, AgentID, AgentType   string
	Tool, Summary, Desc, Report   string
	At                            time.Time
}

type Tracker struct{ /* unexported */ }

func (t *Tracker) Apply(e Event) bool                  // reports whether anything visible changed
func (t *Tracker) CloseTab(tab int)
func (t *Tracker) Tree(tab int) (shown []*Agent, more int) // InTree agents in start order, parents before children, at most 6, more = the rest
func (t *Tracker) Board() (running, waiting, done []*Agent) // done newest first, at most 50
func (t *Tracker) SessionWaits() []int                 // tabs waiting with no agent named (spec 5.4 fallback) that have a Running subagent
func (t *Tracker) Get(k Key) *Agent
func (t *Tracker) Active(tab int) int                   // Running+Waiting in tab; tab 0 = all tabs
```

Rules the tests pin (spec 5.2–5.4, 6.1, 6.3):
- `EvAgentCall` with no `AgentID` queues `{Desc, AgentType}` for `(Tab, Session)`, at most 16 (oldest dropped). With an `AgentID` it queues under that agent as parent.
- `EvStart` takes the oldest queued call in that `(Tab, Session)` with the same type; Task = its Desc, else the type. Parent from the call.
- `EvTool` with an `AgentID` appends a step; a Waiting agent becomes Running.
- `EvNeeds` with an `AgentID` sets that agent Waiting; without one, the tab is session-waiting until its next `EvPrompt` or `EvSession`.
- `EvStop` sets Done, Ended, Report.
- `EvPrompt` sets `InTree=false` on Done agents of that tab.
- `EvSession` compares `Session` with the last one seen from `Src`. When it changed, the Running and Waiting agents of the old `(Src, Session)` and its queue are removed; Done stay on the board. A scratch shell (`Src` 9, `Tab` 1) and its session (`Src` 1, `Tab` 1) never reset each other.
- `CloseTab(tab)` removes agents whose `Tab` or `Src` is `tab`.
- Unknown `AgentID` on `EvTool`/`EvStop` creates the agent with Type from the event and Task = Type (kaze started late).

- [ ] **Step 1: Write the failing tests**

```go
func TestPairsCallsInOrder(t *testing.T)            // two Explore calls "A","B", two starts → Tasks "A","B"
func TestStartWithoutCallUsesType(t *testing.T)     // Task == "Explore"
func TestNestedParentAndDepth(t *testing.T)         // a call from agent a1 → new agent Parent a1, Depth 1; a grandchild also Depth 1
func TestStepsCapAt200(t *testing.T)                // 205 tools → len(Steps)==200, Dropped==5, last step is the 205th
func TestNeedsWithAgentID(t *testing.T)             // Waiting; next EvTool → Running; SessionWaits() empty
func TestNeedsWithoutAgentID(t *testing.T)          // no agent Waiting; SessionWaits()==[tab] while a subagent runs; cleared by EvPrompt
func TestPromptHidesFinishedFromTree(t *testing.T)  // Done agent leaves Tree, stays in Board's done
func TestNewSessionResets(t *testing.T)             // running agent of old session gone from Tree and Board running; done kept
func TestScratchSessionKeptApart(t *testing.T)      // tabs 1 and 9 (scratch) report different sessions; a new session on 9 leaves tab 1's agents alone
func TestBackgroundAgentOutlivesStop(t *testing.T)  // main Stop is not an Event here; Active(tab) stays 1 until EvStop
func TestTreeLimit(t *testing.T)                    // 8 running → 6 shown, more == 2
func TestDoneCap(t *testing.T)                      // 55 stops → Board done has 50, newest first
func TestQueueCap(t *testing.T)                     // 20 calls → 16 kept, oldest dropped
func TestCloseTab(t *testing.T)                     // all of the tab's agents gone, Active(0)==0
```

- [ ] **Step 2: Run** `go test ./internal/subagents/` — Expected: FAIL (package missing).
- [ ] **Step 3: Implement** `internal/subagents/subagents.go`. Agents in a map by Key plus a start-ordered slice per tab; done ring of 50.
- [ ] **Step 4: Run** `go test ./internal/subagents/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(subagents): Track subagents from hook events`

### Task 6: Wire the tracker into the window

**Files:**
- Modify: `internal/ui/alerts_ui.go` (`onNotify`, `deliver`), `internal/ui/agent.go`, `internal/ui/window.go` (field, prune, timer)
- Create: `internal/ui/subagents_ui.go`
- Test: `internal/ui/subagents_ui_test.go`

**Interfaces:**
- Consumes: Task 2 `Msg` fields; Task 5 `Tracker`, `Event`.
- Produces:
  - `win.subs subagents.Tracker` (UI goroutine only).
  - `agentState` gains `session, transcript, cwd string` (latest from any message, #23 Part 1).
  - `func eventOf(m notify.Msg, src int, now time.Time) (subagents.Event, bool)`: maps `KindAgentStart→EvStart`, `KindAgentStop→EvStop`, `KindTool` with AgentID→`EvTool`, `KindTool` without→`EvAgentCall`, `KindNeeds→EvNeeds`, `KindPrompt→EvPrompt`. Every message with a `Session` is also preceded by an `EvSession`.
  - `func (win *window) subTimer(now time.Time) (time.Time, bool)`: next whole minute while `win.subs.Active(0) > 0`, else none.
- Routing: `KindAgentStart`, `KindAgentStop`, `KindTool` go through `inbox.put` like `KindPrompt` (state only: never a chime, flash, toast or mark). `KindNeeds` keeps today's alert path unchanged. `onNotify` keeps the source tab id before `hookTab` remaps it and passes it in the queued item; `eventOf` sets `Src` to it and `Tab` to the remapped row tab.
- `deliver` calls `win.subs.Apply` for every message with an event; `pruneAgents` also calls `win.subs.CloseTab` for closed tabs.

- [ ] **Step 1: Write the failing tests**

```go
func TestAgentKindsNeverChime(t *testing.T)        // onNotify with agent-start/tool/agent-stop on a background tab: chime and flash funcs never called, no toast, no mark
func TestDeliverFeedsTracker(t *testing.T)         // start + tool via deliver → win.subs.Tree(tab) has one agent with one step
func TestSessionIdentityStored(t *testing.T)       // a done message with session/transcript/cwd → agentOf(tab) has them
func TestScratchMessagesUseSourceSession(t *testing.T) // scratch tab's messages: tree shows on the parent row, session reset keyed by the scratch tab
func TestTimerOnlyWhileActive(t *testing.T)        // subTimer: none with zero agents; next minute with one running; none after its stop
func TestCloseTabDropsAgents(t *testing.T)         // closing the tab empties Board and Active(0)
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'AgentKinds|DeliverFeeds|SessionIdentity|ScratchMessages|TimerOnly|CloseTabDrops'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Arm the timer the same way the done-toast expiry asks for a frame (`op.InvalidateCmd{At: ...}`), only when `subTimer` says so.
- [ ] **Step 4: Run** the same, then `go test ./internal/ui/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Feed subagent hooks into a tracker`

### Task 7: The sidebar tree

**Files:**
- Modify: `internal/ui/sidebar_model.go`, `internal/ui/sidebar.go`, `internal/ui/sidebar_window.go`
- Test: `internal/ui/sidebar_model_test.go`, `internal/ui/sidebar_test.go`

**Interfaces:**
- Consumes: `win.subs.Tree`, `Active`.
- Produces:
  - `sideRow` gains `Agents []sideAgent`, `MoreAgents int`, `AgentCount int` (Running+Waiting, for the chip).
  - `type sideAgent struct { Key subagents.Key; Type, Task, Action, Elapsed string; State subagents.State; Depth int }` with `Action` = latest step as `Tool + " " + Summary`, `Elapsed` = `"<1m"` or `"4m"` (Done: `"took 6m"`).
  - `sideInput` gains `Agents func(tabs.TabID) ([]sideAgent, int)`.
  - `sideGroup` gains `AgentCount int` (sum of rows').
  - Hits: `sideAgentHit{Key subagents.Key; Tab tabs.TabID; Rect image.Rectangle}`; a "+N more" hit opens the board filtered to that tab (Task 9's `openBoard(tab)`; until Task 9 lands, wire it to nothing and leave a test that Task 9 turns on).
- Drawing per spec 6.1: indented lines under the row, state mark (ring/amber/green check), type in magenta, task, elapsed right-aligned, a dimmer action line. Chip `"N agents"` (`"1 agent"`) beside the state word; group header adds its count.
- `sideRow.matches` also matches agent types and tasks.

- [ ] **Step 1: Write the failing tests**

```go
func TestBuildSideAddsAgents(t *testing.T)       // a row gets its agents, MoreAgents, AgentCount; group AgentCount sums
func TestFilterMatchesAgentTask(t *testing.T)    // filter "middleware" keeps the row whose agent task contains it
func TestElapsedText(t *testing.T)               // 30s "<1m", 4m10s "4m", done after 6m "took 6m"
func TestSidebarDrawsAgentLines(t *testing.T)    // layoutSidebar returns one sideAgentHit per shown agent, inside the row's band, below the row
func TestAgentChipText(t *testing.T)             // 1 → "1 agent", 3 → "3 agents", 0 → no chip
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'BuildSideAddsAgents|FilterMatchesAgent|ElapsedText|SidebarDrawsAgent|AgentChip'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Clicking an agent line calls `win.openAgentPane(tab, key)` (Task 8); until then a no-op stub with that signature.
- [ ] **Step 4: Run** the same, then `go test ./internal/ui/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(sidebar): Show each session's subagents`

### Task 8: The live agent pane

**Files:**
- Modify: `internal/ui/panes.go`, `internal/ui/panes_draw.go`, `internal/ui/panes_input.go`
- Create: `internal/ui/panes_agent.go`
- Test: `internal/ui/panes_agent_test.go`

**Interfaces:**
- Consumes: `win.subs.Get`, Task 7's click.
- Produces:
  - `paneAgent paneKind`; `pane` gains `agent subagents.Key` and `scroll int`, `follow bool`.
  - Agent panes get synthetic ids from `func (f *floats) agentID(k subagents.Key) tabs.TabID`: negative, stable per Key for the session (a map), so `find`, `raise` and `remove` keep working and no real tab can collide.
  - `func (win *window) openAgentPane(tab tabs.TabID, k subagents.Key)`: selects the tab if not active, then opens or raises the pane. Counts toward `maxPanes`; the oldest member **or agent** pane is replaced first, never the scratch shell.
  - Agent panes never get the keyboard except scroll and the copy key (copies the selected report text, or the whole report when nothing is selected).
- Drawing per spec 6.2: header (state mark, type, task, elapsed, ×); body lines `HH:MM:SS  Tool  summary` (tool in mono), a "N earlier steps" line when `Dropped > 0`, then a "Report" block (wrapped plain text); an amber strip "Waiting for your approval in the session below" while Waiting. Follows the bottom unless scrolled up; scrolling back to the bottom resumes following.
- Closing the session closes its agent panes (existing prune path; agent panes have no tab, so `prune` must drop them by owner).

- [ ] **Step 1: Write the failing tests**

```go
func TestAgentPaneIDsStableAndNegative(t *testing.T) // same Key → same id; two Keys → different ids; all < 0
func TestOpenAgentPaneRaisesExisting(t *testing.T)   // opening twice keeps one pane and raises it
func TestAgentPaneReplacesOldestNotScratch(t *testing.T) // scratch + 3 agent panes + 1 more → oldest agent pane dropped, scratch kept
func TestAgentPaneLines(t *testing.T)               // a Done agent with 2 steps, Dropped 3 → lines: "3 earlier steps", two step lines, "Report", report text
func TestAgentPaneWaitingStrip(t *testing.T)        // Waiting → strip text present
func TestAgentPaneFollowsUnlessScrolled(t *testing.T) // new step moves to bottom; after scrolling up it doesn't; back at bottom it follows again
func TestAgentPaneClosesWithSession(t *testing.T)   // closing the owning tab removes its agent panes
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'AgentPane'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Reuse `layoutPane` for the frame; the body is a new `drawAgentBody(gtx, r, a *subagents.Agent, p *pane)` using `page`-style sans text and `chrome.mono` for times and tools.
- [ ] **Step 4: Run** the same, then `go test ./internal/ui/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(panes): Show a subagent's steps and report live`

### Task 9: The board, its button, status bar text, palette and key

**Files:**
- Create: `internal/ui/board.go`
- Modify: `internal/ui/commands.go`, `internal/ui/logic.go`, `internal/config/config.go`, `internal/ui/sidebar.go` (Agents button), `internal/ui/status.go` and its drawing (agents text), `internal/ui/window.go` (page routing), `internal/ui/settings_commands.go` if the Shortcuts list is not built from `kazeCommands`
- Test: `internal/ui/board_test.go`, `internal/config/config_test.go`, `internal/ui/logic_test.go`

**Interfaces:**
- Consumes: `win.subs.Board`, `SessionWaits`, `Active`; Task 8 `openAgentPane`; Task 4's `Report` (hint when hooks missing).
- Produces:
  - `type boardState struct{ open bool; tab tabs.TabID /* 0 = all */; back tabs.TabID }` on `win.board`.
  - `func (win *window) openBoard(tab tabs.TabID)`, `func (win *window) closeBoard()`; `actAgentsBoard` toggles. Board and Settings are exclusive: opening one closes the other.
  - `kazeCommands` gains `{"agents_board", "Show agents", actAgentsBoard}` (so the palette lists it); `defaultBindings` gains `"agents_board": {Name: "A", Mods: key.ModCommand | key.ModShift}` on darwin and `{Name: "A", Mods: cs}` elsewhere; `config` accepts `agents_board` under `[keys]`.
  - `statusParts` gains `Agents int` (Running+Waiting across all tabs; text "N agents running", hidden at 0); clicking it calls `openBoard(0)`.
  - The sidebar's top icon row gains an Agents button with a badge (count; amber when anything Waiting or `SessionWaits` non-empty; hidden at 0).
- Board per spec 6.3: three columns (Running, Waiting on you, Done), stacked under a narrow width (Claude's call: below 720 dp); cards with type, task, session title and repo, elapsed or "took 6m", action or the report's first line; session cards from `SessionWaits` in Waiting with "a subagent may be asking"; "Back to <session> ×"; Esc and the same key close it; a card click closes the board, selects the tab and calls `openAgentPane`. With any of `AgentStart`, `AgentStop`, `Tools` not OK: the line "kaze needs three more Claude Code hooks to see subagents." and a button that opens Settings at the Agents section.

- [ ] **Step 1: Write the failing tests**

```go
func TestAgentsBoardDefaultKeys(t *testing.T)    // darwin Cmd+Shift+A, windows Ctrl+Shift+A; config accepts agents_board
func TestPaletteListsShowAgents(t *testing.T)    // the palette's kaze section has "Show agents"
func TestBoardColumns(t *testing.T)              // running/waiting/done agents land in their columns; a SessionWaits tab shows a session card in Waiting
func TestBoardCardOpensPane(t *testing.T)        // clicking a card: board closed, tab selected, agent pane open
func TestBoardEscAndKeyClose(t *testing.T)       // Esc closes; the key toggles; back returns to the tab it came from
func TestBoardExclusiveWithSettings(t *testing.T) // opening settings closes the board and back
func TestBoardHooksMissingHint(t *testing.T)     // Report with Tools Missing → hint text and button present
func TestStatusAgentsText(t *testing.T)          // 0 → empty, 4 → "4 agents running", 1 → "1 agent running"
func TestMoreAgentsOpensFilteredBoard(t *testing.T) // Task 7's "+N more" → board open with tab filter
```

- [ ] **Step 2: Run** `go test ./internal/ui/ ./internal/config/ -run 'AgentsBoard|ShowAgents|Board|StatusAgents|MoreAgents'` — Expected: FAIL.
- [ ] **Step 3: Implement.** Draw the board with the Settings page's `page`, card and text helpers so it matches the 0.12 look.
- [ ] **Step 4: Run** the same, then `go test ./... && go vet ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Add the agents board`

### Task 10: Docs, and a look in the real app

**Files:**
- Modify: `README.md` (feature section and Roadmap), `docs/HANDOFF.md` (status), `docs/superpowers/specs/2026-10-28-kaze-terminal-design.md` (6.3: one line pointing to the subagents spec as its replacement)

- [ ] **Step 1:** Build `go build -o kaze ./cmd/kaze` into the scratchpad (not over `/Applications/Kaze.app`) and run it with a scratch config folder (as the earlier screenshot runs did) in bench mode, so no real input reaches it and the owner's files are untouched. Feed it hook messages with `kaze notify` from a script inside one of its tabs (no global input injection). Screenshot the tree, a pane and the board in Midnight and Washi into the scratchpad and check them against the mockup.
- [ ] **Step 2:** Update the docs above.
- [ ] **Step 3:** `go test ./... && go vet ./...` — Expected: PASS.
- [ ] **Step 4: Commit** `docs: Describe the subagents views`

After the PR merges, CLAUDE.md's "Shipping a feature" release (0.14) follows: version bump, changelog row, lander card, and a note that the screenshots need retaking.
