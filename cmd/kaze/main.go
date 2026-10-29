// Command kaze is a lightweight terminal with tabs.
package main

import (
	"fmt"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/bench"
	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/ui"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// r is set at link time (-X main.r=...) only to give a rebuild a new hash when
// Smart App Control blocks a fresh binary.
var r string

func main() {
	_ = r
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version.Text()) // before any window code
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "notify" {
		os.Exit(runNotify(os.Args[2:], stdinForNotify(), os.Getenv)) // before any window code
	}
	if len(os.Args) > 1 && os.Args[1] == "statusline" {
		os.Exit(runStatusline(stdinForNotify(), os.Stdout, os.Getenv, time.Now()))
	}
	rec := bench.FromEnv(os.Getenv) // nil, and free, unless KAZE_BENCH is set
	cfg, warns := loadConfig()
	shell := cfg.Shell
	if len(shell) == 0 {
		shell = ptyx.DefaultShell()
	}
	shell = ptyx.Resolve(shell) // once, so opening a tab never searches PATH
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	go func() {
		w := new(app.Window)
		w.Option(app.Title("kaze"), app.Size(unit.Dp(1100), unit.Dp(720)), app.Decorated(!ui.CustomTitleBar))
		if err := ui.Run(w, ui.Options{Config: cfg, Warnings: warns, Shell: shell, Dir: home, Bench: rec}); err != nil {
			reportFatal(err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

// loadConfig never fails: a broken file gives defaults and says so in the first tab.
func loadConfig() (config.Config, []string) {
	path := config.Path()
	if path == "" {
		return config.Default(), nil
	}
	cfg, warns, err := config.Load(path)
	if err != nil {
		return config.Default(), append(warns, err.Error()+"; using defaults")
	}
	return cfg, warns
}
