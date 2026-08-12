// Package web holds the embedded gosaics browser UI: markup, styles, script,
// and the translation string tables.
package web

import "embed"

// FS contains every static asset served to the browser.
//
//go:embed index.html styles.css app.js strings.en.json
var FS embed.FS
