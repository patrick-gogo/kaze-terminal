// Package fonts embeds the bundled fonts so the binary needs no font files on disk.
package fonts

import _ "embed"

// MonoRegular and MonoBold are JetBrains Mono 2.304 (SIL OFL 1.1, see OFL-JetBrainsMono.txt).
var (
	//go:embed JetBrainsMono-Regular.ttf
	MonoRegular []byte
	//go:embed JetBrainsMono-Bold.ttf
	MonoBold []byte
)

// SansRegular, SansMedium and SansSemiBold are Manrope (SIL OFL 1.1, see
// OFL-Manrope.txt), one static instance per weight; kaze's settings page uses them.
var (
	//go:embed Manrope-Regular.ttf
	SansRegular []byte
	//go:embed Manrope-Medium.ttf
	SansMedium []byte
	//go:embed Manrope-SemiBold.ttf
	SansSemiBold []byte
)

// CJK is Noto Sans JP Regular (SIL OFL 1.1, see OFL-NotoSansJP.txt), cut down to the
// characters Windows code page 932 can encode plus all kana, CJK punctuation and
// fullwidth forms, which keeps it at about 2.3MB instead of 9.6MB. See SOURCES.txt.
//
//go:embed NotoSansJP-Regular-subset.ttf
var CJK []byte
