# Product captures

Native macOS window captures taken October 31, 2026, from Kaze 0.10 (main build 28517a2).

- `kaze-sessions.png`: the session sidebar with three demo repos (one a git worktree), the "Todo API"
  workspace running, and its API member floating over the `todo-api` session.
- `kaze-palette.png`: the palette opened over the same session. The skills are five demo
  `SKILL.md` files in the throwaway home folder.
- `kaze-scratch.png`: the scratch shell over the `todo-api` session, showing `git status -sb` and
  `git diff --stat`.
- `kaze-workspaces.png`: the Workspaces section of Settings with the demo workspace running.
- `kaze-about.png`: the About section displaying the logo and build information.

How they were made:

- Everything ran in a throwaway home folder with demo projects (demo git author, example.com
  module). No personal or work names appear. The owner's settings and workspace files were not
  changed.
- The terminal output is real: `make check` (git log, Go tests, ls), git commands, and two
  `python3 -m http.server` processes with a `curl` loop that makes the API log.
- Claude Code was not run. The sidebar states (needs you, running, done), the slash commands and
  the alert toast come from demo shell scripts calling Kaze's real hook command (`kaze notify
  prompt`, `notify needs`, `notify done`) with the same input Claude Code's hooks send.
- A temporary build overlay (not part of shipped code) opened the demo sessions, floated the pane,
  showed the scratch shell, and opened the palette or a Settings page. It did not change the UI
  renderer. Takes with stray keystrokes were thrown away and retaken. No generated or retouched UI.
- Captured directly with macOS `screencapture`, without desktop shadows or surrounding
  applications.
