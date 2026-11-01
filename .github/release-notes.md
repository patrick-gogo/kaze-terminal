## What’s new in Kaze 0.19

- **Subagents read like a conversation:** the subagent pane shows the prompt it was given, one line on what it has done so far, and its report when it finishes. Individual tool calls are no longer listed. Works for Claude Code subagents and Codex helpers.
- **Start folder:** choose where new tabs open under Settings > Workspaces > Startup (Choose… or Use home), or set `start_dir` in `config.toml`. Workspaces and restored sessions keep their own folders; a missing folder falls back to home with a warning.

## Download

| | File |
|---|---|
| macOS 13 or newer (Apple Silicon and Intel) | `Kaze-macOS.zip` |
| Windows 10 or 11 (most PCs) | `Kaze-Windows-x64.zip` |
| Windows on ARM | `Kaze-Windows-arm64.zip` |

## Opening it the first time

Kaze isn't signed by Apple or Microsoft yet, so both warn you the first time.

**macOS:** unzip, then drag **Kaze** into Applications and open it. macOS says it can't verify the app: click **Done**, open **System Settings › Privacy & Security**, scroll down and click **Open Anyway** next to Kaze. You only do this once. (Or, in Terminal: `xattr -dr com.apple.quarantine /Applications/Kaze.app`.)

**Windows:** unzip and run `kaze.exe`. If SmartScreen says "Windows protected your PC", click **More info**, then **Run anyway**. If Smart App Control is on, it may block unsigned apps with no way past it; building from source works then.

Build from source instead: see the [README](https://github.com/patrick-gogo/kaze-terminal#build-and-run-from-a-clone).
