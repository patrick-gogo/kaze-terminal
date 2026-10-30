# kaze: agent browser design

Date: 2026-10-30. Owner: Patrick. Issue: #30. Status: draft, reviewed by a builder check; waiting for the owner's approval.

Builds on the [main design](2026-10-28-kaze-terminal-design.md) (section 7, item 1, "Browser button") and the [agent sidebar design](2026-10-30-kaze-agent-sidebar-design.md). Everything there stands unless this document changes it.

## 1. Intent and why

kaze is built around running Claude Code all day. Agents often check the web app they are building in a browser (Playwright and similar tools), and today they launch their own hidden browser that the owner never sees. When the owner spots a problem in the app, they screenshot it and describe it to the agent by hand.

The owner wants **a dedicated browser per kaze session that is a shared view of the web app**: the agent drives it, the owner can look at it any time, and either side can hand the other what it sees. In the owner's words, agents should get "better or direct access to the web app", instead of the owner "screenshoting some issues and describing it".

Nothing has gone wrong that forces this; the owner sees it as a feature that belongs in a terminal built for agents.

## 2. Goals

1. **Agents use kaze's browser** instead of their own headless one. Claude Code and Codex both work, through the Chrome DevTools Protocol (CDP).
2. **Background by default.** When an agent opens the browser, it stays out of sight and never takes focus. The owner brings it up from the session's row to watch.
3. **Direct access for the agent:** besides clicking and screenshots, the agent can read the page's console errors, failed network requests and page structure.
4. **"Send to agent":** the owner captures what they see in the browser (screenshot, URL, recent console errors and failed requests, optionally one picked element) and it lands in that session's Claude input box as a short reference, ready for the owner to add a sentence.
5. **Costs nothing until needed.** No browser process runs until the owner opens it or an agent uses it.
6. **Hands-free setup:** kaze's Claude Code setup check registers the browser tool once (and the same for Codex), so no per-project setup is needed.

## 3. Non-goals

- A browser drawn inside the kaze window (embedded WebView2 / WKWebView). The browser is a separate window kaze launches and manages.
- Redirecting a project's own `playwright test` suite to kaze's browser. That needs a config line per project; later, if ever.
- Sharing the owner's everyday browser profile or sign-ins.
- A general-purpose browser for the owner's own browsing (tabs, bookmarks, history UI).
- Docking or snapping the browser window next to kaze. Possible after real panes; not in this design.

## 4. Anti-goals (would make this a failure even if it works)

- **The browser steals focus** or pops in front while the owner is typing in another session. (Same reason as the CLAUDE.md rule on stray input.)
- **Idle cost.** A browser, a poll loop or a ticker running while no one uses it. kaze's idle CPU must stay at zero.
- **An agent acts on another session's pages** (clicks or reads a page that belongs to a different session).
- **An agent can act as the owner** on sites unrelated to the work (email, GitHub, bank).

## 5. Decisions made by the owner

| Topic | Decision |
|---|---|
| Visibility | Background by default. The owner clicks to bring it up and watch. |
| Starting | Lazy. No browser until the owner opens it or an agent in that session uses a browser tool (for example after the owner says "check this in the browser"). |
| Closing | When the session closes, when the owner closes it, and after it sits idle. |
| Profile | Clean: a kaze-only profile signed in to nothing. The owner signs in only to the apps the agent needs. |
| Sharing between sessions | One kaze profile shared by all sessions: sign in once, every session benefits. |
| Agent tools | Wrap an existing Node tool (Playwright MCP or Chrome DevTools MCP). Node is an accepted dependency. |
| If isolation and sharing conflict | Isolation wins: one browser context per session, sign in once per session. |

## 6. How it behaves

### 6.1 One browser, one window per session

- kaze runs at most **one Chromium process**, using the kaze profile (6.4). Each session that uses the browser gets **its own window** in that process.
- Browser: Edge on Windows (always installed), Chrome on macOS. Windows are **normal browser windows with the address bar**, not app mode, so the owner can type a URL and reach sign-in pages. *(My call. Edge and Chrome come from the main design 7.1.)* If no supported browser is found, the browser mark and the agent tool report that plainly; nothing else breaks.
- The process starts when the first session needs a window and exits when the last window closes.

### 6.2 Starting

- **The owner opens it:** a browser mark on the session's sidebar row (and a "Open browser" action, with a key under `[keys]`, name and default left to the builder). The window comes to the front.
- **An agent uses it:** the agent's first browser action starts the browser, or opens this session's window if the process is already running. The window opens **in the background without activating** and stays there.
- Claude Code and Codex start their tool servers (MCP) when a session starts, which is too early. So the agent-side helper (6.6) stays idle until the agent's first browser action, and only then asks kaze for the browser.
- *My call:* when the agent starts it, kaze opens the session's start page as `about:blank` and lets the agent navigate. When the owner opens it and the session is a workspace member with a port wait, it opens `http://localhost:<port>`; otherwise `about:blank`.

### 6.3 Closing

- **Session closes:** its window closes.
- **The owner closes it:** the window's close button, or "Close browser" on the row.
- **Idle:** no CDP traffic from the session's agent (no commands sent, not merely an open connection, because MCP tools keep their connection open for the whole Claude session; *my call*), and the window not in front, for `browser_idle_minutes` (default **10**, *my call*; `0` turns idle closing off). kaze sets one timer when the window goes quiet and cancels it on any use: the same one-shot-timer pattern as the usage readout's reset. No ticker.
- After closing, the next open or agent request starts it fresh. The agent's helper (6.6) must survive this: its next tool call reconnects and kaze starts the browser again.
- If an agent closes the window or it crashes, kaze treats it as closed; the next request starts it again.

### 6.4 The kaze profile

- Lives in kaze's config folder (next to `config.toml`), for example `<config dir>/browser-profile/`. Survives kaze restarts. Deleting the folder resets every sign-in.
- Starts signed in to nothing. The owner signs in where the agent needs it (usually their local app) once, and every session sees it.
- *My call:* a "Reset browser sign-ins" button in Settings, Claude (with a confirm dialog) that closes the browser and deletes the profile folder.

### 6.5 Session isolation

"Session" here means **one kaze tab** (one `KAZE_TAB_ID`), whatever the sidebar shows it as: a session row, a workspace member or a floating pane. The per-session map is keyed by tab id, and the mark shows wherever that tab is listed. *(My call.)*

Hard rule (4): an agent must only see and act on **its own session's window**. With one Chromium process, a plain CDP connection sees every window, so kaze does not hand out the browser's raw debugging port. Instead each session gets **its own address on 127.0.0.1** that kaze controls, and kaze only lets that connection reach that session's pages (and new pages it opens).

How kaze enforces this (a filtering CDP proxy per session, separate browser contexts, or another way) is **for the spike (8) to settle**, because it decides whether shared sign-ins and isolation can both hold. If they cannot both hold, **the owner's fallback is separate browser contexts per session**: agents stay fully apart and the owner signs in once per session instead of once overall (owner, 2026-10-30). No need to come back to the owner for this case.

The session's address goes in the shell env as `KAZE_BROWSER_CDP` (alongside `KAZE_TAB_ID`). Because the address belongs to kaze, a connection to it is also how laziness works: kaze can start the browser on the first connection.

### 6.6 How agents connect: `kaze browser-mcp`

- A new subcommand that Claude Code and Codex run as an MCP server. It reads the session's address from `KAZE_BROWSER_CDP` and serves browser tools against it.
- **Wrap an existing Node tool** (owner, 2026-10-30: Node is acceptable): Playwright MCP (`--cdp-endpoint`) or Chrome DevTools MCP (`--browserUrl`); the spike picks which. No kaze-built tool set. Node only runs when the agent uses the browser. Whichever is chosen must:
  - stay idle (no browser, no connection) until the first tool call,
  - give the agent clicking, typing, navigation and screenshots,
  - give the agent the console errors, failed network requests and page structure (goal 3),
  - work the same for Claude Code and Codex,
  - fail with a clear message, and exit, outside a kaze session (no `KAZE_BROWSER_CDP`).
- If it depends on Node (`npx`), the setup check reports a missing Node plainly.
- Agents that write their own Playwright scripts can use `chromium.connectOverCDP(process.env.KAZE_BROWSER_CDP)`. kaze does not enforce this; it is just available.

### 6.7 Setup

- kaze's Claude Code setup check (`internal/claudesetup`) gains an entry for the browser tool: OK / missing / broken / taken, and **Fix** with the usual show-the-change, confirm and timestamped `.bak` copy. It registers `kaze browser-mcp` once at user level so every project gets it.
- Codex gets the same check and Fix for its own config. *Builder to confirm Codex's MCP config file and format.*

### 6.8 Watching: the row's browser mark

- A small browser mark on a session's sidebar row while that session's window exists. It shows a different state while an agent is connected (look left to the builder, matching the existing row marks; static, no animation).
- Clicking it brings the session's window to the front. Clicking again (or the window losing focus) leaves it where it is; kaze does not hide it again by itself.

### 6.9 "Send to agent"

- Available only while the session's window exists. Triggered from the row (and a key under `[keys]`, name and default left to the builder).
- kaze collects through CDP:
  - a screenshot of the visible page,
  - the URL,
  - recent console errors and failed network requests (kaze records these only while the window exists; the last 20 of each, *my call*),
  - optionally **one picked element**: kaze turns on the browser's built-in inspect mode, the owner clicks an element, kaze records its selector and outer HTML (capped, *my call*: 4 KB).
- kaze saves the bundle in a temp folder per session (the PNG plus a short text summary file), cleaned up when the session closes.
- kaze writes a one-line reference into **that session's** input, for example `[browser: localhost:3000/cart, 2 console errors, see /path/summary.md]`, **without pressing Enter**. The owner adds a sentence and sends. Claude reads the summary and the screenshot from the paths. kaze writes this into its own session's PTY, which is not global input.
- If the session isn't running Claude (a plain shell), "Send to agent" still writes the reference; that is the owner's call to use.

## 7. Constraints

- Idle CPU zero; no growth in memory when idle. The browser is a separate process and does not count against kaze's own memory.
- Never inject global input; never take focus from the owner when an agent starts or uses the browser.
- All debugging addresses bind to 127.0.0.1 only, on random ports, and are passed only to the session they belong to. CDP has no auth of its own, so this is the protection; the design accepts that a local program that finds a port could use it (same trust level as the rest of the machine).
- Same code on Windows and macOS where possible; OS-specific bits behind build tags as elsewhere in kaze.
- No secrets in config. The profile folder holds sign-in cookies; it is never committed or copied anywhere by kaze.
- Strict TDD and safe RED tests, as in CLAUDE.md. GUI checks never use global input.

## 8. Spike first

The first plan covers **the spike only**. The build plan is written after its findings come back. *(My call.)*

The main design says attaching agents must be proven with a spike before building. The spike answers, on Windows and macOS:

1. Can one Chromium process with one persistent profile give each session a window that only its own CDP connection can see and drive, while sign-ins stay shared? Which mechanism works (6.5)?
2. Can a window be opened and driven without ever activating or coming to the front, on both OSes?
3. Does Playwright MCP or Chrome DevTools MCP behave correctly through kaze's per-session address (connect, list pages, open new pages, screenshots, console and network)? Or is a small kaze-served tool set simpler?
4. Does the lazy start work end to end: the MCP helper idle at session start, the browser launched on the first tool call, inside a reasonable time (target under 3s)?

The spike is throwaway (in `spike/`, like the Gio spike). Its findings go back to the owner before the plan is written if any answer changes this design.

## 9. Left to the builder

- Key names and defaults, the exact look of the row mark, the summary file format.
- The tool-set choice in 6.6, within the rules listed there.
- Package layout (*suggestion:* `internal/browser` for the process, windows and per-session addresses; `cmd/kaze` for `browser-mcp`).
- How kaze finds Edge and Chrome on each OS.

## 10. Calls made in this document (for the owner to check)

- Idle close after 10 minutes, setting `browser_idle_minutes`, `0` = never.
- Owner-opened windows go to the workspace port when there is one, else `about:blank`.
- A "Reset browser sign-ins" button in Settings, Claude.
- Last 20 console errors and failed requests per window; picked element HTML capped at 4 KB.
- The reference line is typed into the session without pressing Enter.
- Isolation beats shared sign-ins if the spike shows they conflict.
- Normal browser windows with an address bar, not app mode.
- Idle means no agent commands, not merely no open connection.
- One browser window per kaze tab (sessions, workspace members and floating panes alike).
- The first plan is the spike only.
