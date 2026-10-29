// Package branding embeds the kaze logo files the app draws itself.
package branding

import _ "embed"

// Mark is the violet wind K on a transparent background, 512x512.
//
//go:embed kaze-mark-512.png
var Mark []byte
