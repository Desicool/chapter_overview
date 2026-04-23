package main

import (
	"embed"
	"io/fs"

	"github.com/desico/chapter-overview/cmd"
)

//go:embed all:web/dist
var webDist embed.FS

func init() {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		panic("web/dist embed: " + err.Error())
	}
	cmd.WebFS = sub
}
