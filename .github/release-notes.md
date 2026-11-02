## What’s new in Kaze 0.20

- **CPU and RAM in the status bar:** the status bar shows how much CPU and memory Kaze and the shells, agents and servers it runs are using. Hover it for a breakdown by session. It refreshes on activity, at most every 2 seconds, and pauses while Kaze is minimized.
- **Fixed: resumed conversations:** after an update or a launch from Finder, restored tabs resume their Claude Code or Codex conversation again, including when the agent is installed as a shell alias.
- **Fixed: update prompt:** the update confirmation text no longer overlaps the Cancel and Update now buttons.
- **Fixed: workspace status:** a closed workspace tab shows as stopped and no longer counts as a running server. The count reads N/M UP.

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
