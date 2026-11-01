## What’s new in Kaze 0.18

- **Your sessions return:** automatically reopen folders, names, order, colors and the selected session after restarting Kaze, including Update and restart.
- **The same Claude/Codex conversations:** configured agent hooks provide exact conversation IDs. Idle resumption does not submit another prompt or pick another chat in the same repo.
- **A startup choice:** disable restoration under Settings > Workspaces > Startup to start fresh. Explicit workspace launches retain their precedence.
- **No command replay:** plain terminals start fresh shells and workspace recipes are not rerun. Running commands, servers, scratch shells and floating layout still stop at exit.
- **Verification:** full tests/vet, focused race checks and macOS/Windows builds pass. Real isolated CLI round trips used local stub responses. Windows runtime and a real installed-app updater restart remain manual checks.

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
