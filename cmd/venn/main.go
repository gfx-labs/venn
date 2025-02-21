package main

import (
	_ "net/http/pprof"

	"github.com/alecthomas/kong"
)

func main() {
	ctx := kong.Parse(&cli)
	// Call the Run() method of the selected parsed command.
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
