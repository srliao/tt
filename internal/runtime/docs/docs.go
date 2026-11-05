// Package docs embeds the userscript ctx API cheatsheet. It is the single
// source: tt-cli's `help scripts` prints it and the web script editor imports
// the same file.
package docs

import _ "embed"

// CtxCheatsheet is the markdown reference for the ctx object scripts get.
//
//go:embed ctx-cheatsheet.md
var CtxCheatsheet string
