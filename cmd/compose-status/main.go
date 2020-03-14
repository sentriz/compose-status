package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/peterbourgon/ff"

	status "senan.xyz/g/compose-status"
)

var (
	progName     = "compose-status"
	progPrefix   = "CS"
	argSet       = flag.NewFlagSet(progName, flag.ExitOnError)
	argPageTitle = argSet.String(
		"page-title", "server status",
		"title to show at the top of the page (optional)",
	)
	argScanInterval = argSet.Int(
		"scan-interval", 5,
		"(in seconds) time to wait between background scans (optional)",
	)
	argListenAddr = argSet.String(
		"listen-addr", ":9293",
		"listen address (optional)",
	)
)

func main() {
	err := ff.Parse(argSet,
		os.Args[1:],
		ff.WithEnvVarPrefix(progPrefix),
	)
	if err != nil {
		log.Fatalf("error parsing args: %v\n", err)
	}
	cont, err := status.NewController(
		status.WithScanInternal(time.Duration(*argScanInterval)*time.Second),
		status.WithTitle(*argPageTitle),
		status.WithCredit,
	)
	if err != nil {
		log.Fatalf("error creating controller: %v\n", err)
	}
	go cont.Start()
	http.Handle("/", cont)
	log.Printf("listening on %q\n", *argListenAddr)
	if err := http.ListenAndServe(*argListenAddr, nil); err != nil {
		log.Fatalf("error running server: %v\n", err)
	}
}
