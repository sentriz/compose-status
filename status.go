package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

type container struct {
	Name     string
	Status   string
	Link     string
	LastSeen time.Time
	IsDown   bool
	Project  string
}

type settings struct {
	pageTitle    string
	cleanCutoff  int
	scanInterval int
	listenAddr   string
	dbPath       string
}

type controller struct {
	tmpl     *template.Template
	client   *docker.Client
	settings *settings
	last     map[string]*container
	buffPool *bpool.BufferPool
}

func (c *controller) getProjects() error {
	seenIDs := map[string]struct{}{}
	// insert the current time for any container we see
	containers, err := c.client.ListContainers(
		docker.ListContainersOptions{},
	)
	if err != nil {
		return errors.Wrap(err, "listing containers")
	}
	for _, tain := range containers {
		project, ok := tain.Labels["com.docker.compose.project"]
		if !ok {
			continue
		}
		if len(tain.Names) == 0 {
			return fmt.Errorf("%q does not have a name", tain.ID)
		}
		seenIDs[tain.ID] = struct{}{}
		c.last[tain.ID] = &container{
			Name:     tain.Names[0],
			Project:  project,
			Status:   strings.ToLower(tain.Status),
			LastSeen: time.Now(),
		}
	}
	// set containers we haven't seen to down, and delete one that haven't
	// seen since since the cutoff
	cutoff := time.Now().Add(
		-1 * time.Duration(c.settings.cleanCutoff) * time.Second,
	)
	for id, tain := range c.last {
		if tain.LastSeen.Before(cutoff) {
			delete(c.last, id)
			continue
		}
		if _, ok := seenIDs[id]; !ok {
			tain.IsDown = true
		}
	}
	return nil
}

func (c *controller) handleWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w /* hello */, "not found", http.StatusNotFound)
		return
	}
	//
	// group the last seen by project, inserting so that the container
	// names are sorted
	projectMap := map[string][]*container{}
	for _, tain := range c.last {
		current := projectMap[tain.Project]
		i := sort.Search(len(current), func(i int) bool {
			return current[i].Name >= tain.Name
		})
		current = append(current, nil)
		copy(current[i+1:], current[i:])
		current[i] = tain
		projectMap[tain.Project] = current
	}
	//
	tmplData := struct {
		Projects  map[string][]*container
		PageTitle string
	}{
		projectMap,
		c.settings.pageTitle,
	}
	//
	// using a pool of buffers, we can write to one first to catch template
	// errors, which avoids a superfluous write to the response writer
	buff := c.buffPool.Get()
	defer c.buffPool.Put(buff)
	if err := c.tmpl.Execute(buff, tmplData); err != nil {
		http.Error(w, fmt.Sprintf("error executing template: %v", err), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buff.WriteTo(w)
}

func parseArgs() (*settings, error) {
	set := flag.NewFlagSet("compose-status", flag.ExitOnError)
	pageTitle := set.String(
		"page-title", "server status",
		"title to show at the top of the page (optional)",
	)
	cleanCutoff := set.Int(
		"clean-cutoff", 259200,
		"(in seconds) to wait before forgetting about a down container (optional)",
	)
	scanInterval := set.Int(
		"scan-interval", 10,
		"(in seconds) time to wait between background scans (optional)",
	)
	listenAddr := set.String(
		"listen-addr", ":9293",
		"listen address (optional)",
	)
	dbPath := set.String(
		"db-path", "db.db",
		"path to database (optional)",
	)
	if err := ff.Parse(set,
		os.Args[1:],
		ff.WithEnvVarPrefix("CS"),
	); err != nil {
		return nil, errors.Wrap(err, "parsing args")
	}
	return &settings{
		pageTitle:    *pageTitle,
		cleanCutoff:  *cleanCutoff,
		scanInterval: *scanInterval,
		listenAddr:   *listenAddr,
		dbPath:       *dbPath,
	}, nil
}

func main() {
	sett, err := parseArgs()
	if err != nil {
		log.Fatalf("error parsing args: %v\n", err)
	}
	client, err := docker.NewClientFromEnv()
	if err != nil {
		log.Fatalf("error creating docker client: %v\n", err)
	}
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"humanDate": humanize.Time,
		}).
		Parse(homeTmpl)
	if err != nil {
		log.Fatalf("error creating template: %v\n", err)
	}
	cont := &controller{
		tmpl:     tmpl,
		client:   client,
		settings: sett,
		last:     map[string]*container{},
		buffPool: bpool.NewBufferPool(64),
	}
	go func() {
		for {
			if err := cont.getProjects(); err != nil {
				log.Printf("error getting projects: %v\n", err)
			}
			time.Sleep(time.Duration(sett.scanInterval) * time.Second)
		}
	}()
	http.HandleFunc("/", cont.handleWeb)
	fmt.Printf("listening on %q\n", sett.listenAddr)
	if err := http.ListenAndServe(sett.listenAddr, nil); err != nil {
		log.Fatalf("error running server: %v\n", err)
	}
}
