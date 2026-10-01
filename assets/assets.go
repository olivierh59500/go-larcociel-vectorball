// Package assets embeds the intro's original presentation data.
package assets

import "embed"

//go:embed original/*
var Files embed.FS
