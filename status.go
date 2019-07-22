package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	bolt "github.com/etcd-io/bbolt"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"
	"github.com/peterbourgon/ff"
	"github.com/pkg/errors"
)

var (
	bucketKey = []byte("SeenProjects")
	bufpool   = bpool.NewBufferPool(64)
)

func i64ToBytes(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func bytesToi64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

type container struct {
	ID       string
	Status   string
	LastSeen time.Time
	Link     string
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
	db       *bolt.DB
	client   *docker.Client
	settings *settings
}

func (c *controller) projectsDo(cb func(project string, tain *container) error) error {
	containers, err := c.client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		return errors.Wrap(err, "listing containers")
	}
	for _, cRaw := range containers {
		project, ok := cRaw.Labels["com.docker.compose.project"]
		if !ok {
			continue
		}
		tain := &container{
			ID:     cRaw.Names[0],
			Status: strings.ToLower(cRaw.Status),
		}
		if link, ok := cRaw.Labels["traefik.frontend.rule"]; ok {
			tain.Link = link
		}
		if err := cb(project, tain); err != nil {
			return errors.Wrap(err, "projects callback")
		}
	}
	return nil
}

func (c *controller) getProjects() error {
	//
	// insert the current time for any container we see
	now := i64ToBytes(time.Now().Unix())
	if err := c.projectsDo(func(project string, tain *container) error {
		tainKey := fmt.Sprintf("%s___%s", project, tain.ID)
		return c.db.Update(func(tx *bolt.Tx) error {
			return tx.
				Bucket(bucketKey).
				Put([]byte(tainKey), now)
		})
	}); err != nil {
		return errors.Wrap(err, "background scan")
	}
	//
	// delete old containers that are last seen before a cut off
	cutoff := time.Now().Add(
		-1 * time.Duration(c.settings.cleanCutoff) * time.Second,
	)
	if err := c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketKey)
		return b.ForEach(func(k, v []byte) error {
			lastTime := time.Unix(bytesToi64(v), 0)
			if lastTime.Before(cutoff) {
				return b.Delete(k)
			}
			return nil
		})
	}); err != nil {
		return errors.Wrap(err, "background clean up")
	}
	return nil
}

func (c *controller) handleWeb(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	projects := map[string][]*container{}
	//
	// iterate the up containers, appending to projects
	//
	// keep track of the container ids that are up so we know
	// which are down in the view step
	seenKeys := map[string]struct{}{}
	if err := c.projectsDo(func(project string, tain *container) error {
		projects[project] = append(projects[project], tain)
		tainKey := fmt.Sprintf("%s___%s", project, tain.ID)
		seenKeys[tainKey] = struct{}{}
		return nil
	}); err != nil {
		http.Error(w, fmt.Sprintf("error getting containers: %v", err), 500)
		return
	}
	//
	// find old containers, show give them a LastSeen to display in red
	if err := c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketKey)
		return b.ForEach(func(k, lastSeenBytes []byte) error {
			keyStr := string(k)
			if _, ok := seenKeys[keyStr]; ok {
				// this container id is up
				return nil
			}
			keyParts := strings.SplitN(keyStr, "___", 2)
			project := keyParts[0]
			id := keyParts[1]
			list, ok := projects[project]
			if !ok {
				list = []*container{}
			}
			lastSeenI := bytesToi64(lastSeenBytes)
			lastSeen := time.Unix(lastSeenI, 0)
			list = append(list, &container{
				ID:       id,
				LastSeen: lastSeen,
			})
			projects[project] = list
			return nil
		})
	}); err != nil {
		http.Error(w, fmt.Sprintf("error finding old containers: %v", err), 500)
		return
	}
	//
	// using a pool of buffers, we can write to one first to catch template
	// errors, which avoids a superfluous write to the response writer
	buff := bufpool.Get()
	defer bufpool.Put(buff)
	tmplData := struct {
		Projects  map[string][]*container
		PageTitle string
	}{
		projects,
		c.settings.pageTitle,
	}
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
		ff.WithEnvVarPrefix("SC"),
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
	db, err := bolt.Open(sett.dbPath, 0644, nil)
	if err != nil {
		log.Fatalf("error opening database: %v\n", err)
	}
	defer db.Close()
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketKey)
		return err
	}); err != nil {
		log.Fatalf("error setting up database: %v\n", err)
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
		db:       db,
		client:   client,
		settings: sett,
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
	fmt.Println("listening")
	if err := http.ListenAndServe(sett.listenAddr, nil); err != nil {
		log.Fatalf("error running server: %v\n", err)
	}
}
