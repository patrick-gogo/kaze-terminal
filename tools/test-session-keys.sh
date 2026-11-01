#!/bin/sh
set -eu
task_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
task_tmp=$(mktemp -d "${TMPDIR:-/tmp}/kaze-session-keys.XXXXXX")
trap 'rm -rf "$task_tmp"' EXIT HUP INT TERM
clang -fobjc-arc -framework AppKit "$task_root/tools/native/sessionkeys.m" -o "$task_tmp/sessionkeys"
"$task_tmp/sessionkeys"
