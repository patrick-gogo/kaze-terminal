// Package branding embeds the kaze logo files the app draws itself.
package branding

import _ "embed"

// Mark is the violet wind K on a transparent background, 512x512.
//
//go:embed kaze-mark-512.png
var Mark []byte

// ClaudeLogo and OpenAILogo are the agents' marks, white on transparent,
// 96x96 (see agents/SOURCES.txt); kaze tints them when it draws them.
//
//go:embed agents/claude-96.png
var ClaudeLogo []byte

//go:embed agents/openai-96.png
var OpenAILogo []byte
