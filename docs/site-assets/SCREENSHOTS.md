# Product captures

Native macOS window captures (2200 × 1496 px, Retina) taken October 31, 2026, from Kaze 0.12 (main build 044336a).

- `kaze-sessions.png`: the session sidebar with three demo repos (one a git worktree), the "Todo API"
  workspace running, and its API member floating over the `todo-api` session.
- `kaze-palette.png`: the palette opened over the same session. The skills are five demo
  `SKILL.md` files in the throwaway home folder.
- `kaze-scratch.png`: the scratch shell over the `todo-api` session, showing `git status -sb` and
  `git diff --stat`.
- `kaze-workspaces.png`: the redesigned Workspaces section of Settings with the Todo API workspace card
  and its API, Web and Health members running.
- `kaze-about.png`: the redesigned About section displaying the shipped logo and byline, version 0.12,
  platform, chip and license facts.
- `kaze-settings.png`: Appearance settings with five theme cards, Midnight selected, and the live
  terminal preview.

How they were made:

- Everything ran in a throwaway home folder with demo projects (demo git author, example.com
  module). No personal or work data appears in the demo projects; the shipped About branding is
  unchanged. The owner's settings and workspace files were not changed. Demo folders: `todo-api`,
  its `todo-api-auth` worktree on `feature/auth`, `web-app` on `feature/dark-mode`, and `notes-cli` on `fix/search`, each with an uncommitted diff.
- The terminal output is real: `make check` (git log, Go tests, ls), git commands, and two
  `python3 -m http.server` processes with a `curl` loop that makes the API log. The Appearance
  live preview is Kaze's built-in sample, not output from an additional command.
- Claude Code was not run. The sidebar states (needs you, running, done), the slash commands and
  the alert toast come from demo shell scripts calling Kaze's real hook command (`kaze notify
  prompt`, `notify needs`, `notify done`) with the same input Claude Code's hooks send.
- A temporary build overlay (not part of shipped code) opened the demo sessions, floated the pane,
  showed the scratch shell, and opened the palette or a Settings page. It did not change the UI
  renderer. Each capture was inspected for stray input and cut-off text. No generated or retouched UI.
- Captured directly with macOS `screencapture`, without desktop shadows or surrounding
  applications. No global keyboard or mouse input was injected. The demo app, server and sleep
  processes were stopped after each take.
