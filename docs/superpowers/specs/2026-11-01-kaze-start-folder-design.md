# Start folder (issue #77)

The owner requested implementation of the written behavior in GitHub issue #77 and
explicitly authorized proceeding through design and implementation without further
approval while away. This document records that design; it adds no approval gate.

- Add `start_dir` to config.toml, empty by default. Keep the entered value in config;
  expand `~`/`~/`/`~\` using the user's home, resolve relative paths against the Kaze
  process working directory, and require an existing directory. Invalid types, missing
  paths and regular files produce a config warning and use home.
- The first ordinary session and new ordinary tabs use the resolved start folder.
  A tab's explicit directory wins. Workspace and restored tabs keep their directories;
  existing startup precedence stays intact. A missing restored folder still falls back
  to home. Running sessions never move when the setting changes.
- Settings > Workspaces > Startup shows “Start folder” with the current path in the
  existing path typography, a “Choose…” button using the shared native folder picker,
  and “Use home” to clear a nonempty override. Choosing applies to subsequent plain
  tabs and saves the key immediately, like other ordinary settings. Manual paths can
  be entered in config.toml. No extra dialog or text-field save flow is needed.
- The row is available even without workspaces or with an invalid workspaces file.
  Picking is asynchronous, only one dialog opens at a time, cancel writes nothing,
  errors preserve the previous choice, and an invalid returned path is not saved.
- Recheck the path when starting a new plain tab, so a folder removed since startup
  falls back to home with a visible warning. Store the actual starting directory in
  tab metadata so restoration stays correct even before the first OSC 7 report or
  after changing the setting.

Preserve the existing settings visual system and folder picker behavior. No shell
command is typed to change folders; directory strings are passed as PTY launch cwd.
