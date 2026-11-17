// Command korokmoted is the Korokmote PC daemon.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jose00735/korokmote/daemon/internal/app"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var opts app.Options
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.StringVar(&opts.ConfigDir, "config-dir", "", "use this folder for configuration and data")
	flag.BoolVar(&opts.Console, "console", false, "also log to stderr and print the admin page URL")
	flag.BoolVar(&opts.Fake, "fake", false, "use the fake platform (no real input; for development)")
	flag.BoolVar(&opts.Verbose, "verbose", false, "debug logging")
	flag.BoolVar(&opts.Pair, "pair", false, "open the pairing page on start")
	flag.Parse()
	opts.Version = version
	if *showVersion {
		fmt.Println("korokmoted", version)
		return
	}
	if !app.SingleInstance() {
		if opts.Pair {
			app.Fatal("Korokmote is already running. Use the tray icon and choose \"Pair a device…\".")
		}
		os.Exit(0)
	}
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		os.Exit(0)
	}()
	if err := app.Run(opts); err != nil {
		app.Fatal("Korokmote could not start: " + err.Error())
		os.Exit(1)
	}
}
