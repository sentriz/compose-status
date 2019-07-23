package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/peterbourgon/ff"

	status "senan.xyz/g/compose-status"
)

var (
	progName     = "compose-status"
	progPrefix   = "CS"
	argSet       = flag.NewFlagSet(progName, flag.ExitOnError)
	argPageTitle = argSet.String(
		"page-title", "",
		"title to show at the top of the page (optional)",
	)
	argCleanCutoff = argSet.Int(
		"clean-cutoff", 0,
		"(in seconds) to wait before forgetting about a down container (optional) (default 3days)",
	)
	argScanInterval = argSet.Int(
		"scan-interval", 5,
		"(in seconds) time to wait between background scans (optional)",
	)
	argListenAddr = argSet.String(
		"listen-addr", ":9293",
		"listen address (optional)",
	)
	argSavePath = argSet.String(
		"save-path", "save.json",
		"path to save file (optional)",
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
	save, _ := ioutil.ReadFile(*argSavePath)
	cont, err := status.NewController(
		status.WithCleanCutoff(time.Duration(*argCleanCutoff)*time.Second),
		status.WithResume(save),
		status.WithTitle(*argPageTitle),
		status.WithCredit,
	)
	if err != nil {
		log.Fatalf("error creating controller: %v\n", err)
	}
	// if a save file exists it needs to be loaded into the status.Controller
	go func() {
		for {
			if err := cont.GetProjects(); err != nil {
				log.Printf("error getting projects: %v\n", err)
			}
			time.Sleep(time.Duration(*argScanInterval) * time.Second)
		}
	}()
	// ensure we save to disk on SIGTERM. for example Ctrl-C
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		lastData, err := json.Marshal(cont.GetLastProjects())
		if err != nil {
			log.Fatalf("error marshalling last to json: %v\n", err)
		}
		if err := ioutil.WriteFile(*argSavePath, lastData, 0644); err != nil {
			log.Fatalf("error saving last to disk: %v\n", err)
		}
		os.Exit(0)
	}()
	http.Handle("/", cont)
	log.Printf("listening on %q\n", *argListenAddr)
	if err := http.ListenAndServe(*argListenAddr, nil); err != nil {
		log.Fatalf("error running server: %v\n", err)
	}
}
