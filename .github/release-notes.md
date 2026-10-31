## What’s new in Kaze 0.13

- **Agents settings:** Claude Code and Codex have separate setup checks, with shared alert controls and Claude-only usage.
- **Codex session awareness:** hooks report needs you, running, done and interrupts. Rows and the palette identify the agent and support Codex slash commands and `$skill` names.
- **Codex setup with a preview:** Fix preserves other hooks, backs up existing hooks.json, and checks for edits made while the preview is open. Inline hooks are read to avoid duplicate handlers. Codex’s config.toml and notify command are left unchanged.
- **After Codex Fix:** open Codex and run `/hooks` to review and trust Kaze’s hooks. Kaze does not grant hook trust or make approval decisions. Windows builds are included; Windows hook execution still needs a manual check.
- **Site refresh:** native Kaze 0.12 captures, an Appearance gallery option, and a mobile layout fix.

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
