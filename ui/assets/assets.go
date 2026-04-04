// Package assets provides embedded static assets for the Tinkerbell web UI.
package assets

import "embed"

//go:generate go run ../../script/tailwindcss -i css/input.css -o css/output.css --minify

// Artwork contains embedded artwork files (logos, icons).
//
//go:embed artwork/*
var Artwork embed.FS

// CSS contains embedded CSS files.
//
//go:embed css/output.css
var CSS embed.FS

// JS contains embedded JavaScript files.
//
//go:embed js/*
var JS embed.FS

// Fonts contains embedded font files.
//
//go:embed fonts/*
var Fonts embed.FS
