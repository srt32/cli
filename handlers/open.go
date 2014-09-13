package handlers

import (
	"fmt"
	"log"

	"github.com/codegangsta/cli"
	"github.com/exercism/cli/config"
	"github.com/toqueteos/webbrowser"
)

func Open(ctx *cli.Context) {
	c, err := config.Read(ctx.GlobalString("config"))
	if err != nil {
		log.Fatal(err)
	}
	args := ctx.Args()

	var url string
	switch len(args) {
	case 0:
		url = fmt.Sprintf("%s/%s?key=%s", c.ProblemsHost, "v2/exercises/current", c.APIKey)
	default:
		msg := "Usage: exercism open"
		log.Fatal(msg)
	}

	webbrowser.Open(url)
}
