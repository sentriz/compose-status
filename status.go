package status

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
)

var (
	tempMatch = "coretemp_core[0-9]+_input"
	tempExpr  *regexp.Regexp
	hostMatch = "Host:(.+?)(?:,|;|$|\b)"
	hostExpr  *regexp.Regexp
)

func init() {
	var err error
	tempExpr, err = regexp.Compile(tempMatch)
	if err != nil {
		log.Fatalf("error compiling temp expr: %v\n", err)
	}
	hostExpr, err = regexp.Compile(hostMatch)
	if err != nil {
		log.Fatalf("error compiling host expr: %v\n", err)
	}
}

type Container struct {
	Name     string
	Status   string
	Link     string
	LastSeen time.Time
	IsDown   bool
	Project  string
}

type Stats struct {
	Load1    float64
	Load5    float64
	Load15   float64
	MemUsed  uint64
	MemTotal uint64
	CPU      float64
	CPUTemp  float64
}

// we need some sort of unique identifier for containers (when tracking ups
// and downs). the "ID" field from the engine won't do, because we want a
// recreated container with probably a different ID to be considered the same
func (c *Container) ID() string {
	return fmt.Sprintf("%s___%s", c.Project, c.Name)
}

type Controller struct {
	tmpl         *template.Template
	client       *docker.Client
	buffPool     *bpool.BufferPool
	cleanCutoff  time.Duration
	scanInterval time.Duration
	groupLabel   string
	pageTitle    string
	showCredit   bool
	LastProjects map[string]*Container
	LastStats    *Stats
}

type ControllerOpt func(*Controller) error

func WithCleanCutoff(dur time.Duration) ControllerOpt {
	return func(c *Controller) error {
		c.cleanCutoff = dur
		return nil
	}
}

func WithScanInternal(dur time.Duration) ControllerOpt {
	return func(c *Controller) error {
		c.scanInterval = dur
		return nil
	}
}

func WithTitle(title string) ControllerOpt {
	return func(c *Controller) error {
		c.pageTitle = title
		return nil
	}
}

func WithGroupLabel(label string) ControllerOpt {
	return func(c *Controller) error {
		c.groupLabel = label
		return nil
	}
}

func WithResume(file []byte) ControllerOpt {
	return func(c *Controller) error {
		if len(file) <= 0 {
			return nil
		}
		return json.Unmarshal(file, &c.LastProjects)
	}
}

func WithCredit(c *Controller) error {
	c.showCredit = true
	return nil
}

func NewController(options ...ControllerOpt) (*Controller, error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, errors.Wrap(err, "creating docker client")
	}
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"humanDate":  humanize.Time,
			"humanBytes": humanize.Bytes,
		}).
		Parse(homeTmpl)
	if err != nil {
		return nil, errors.Wrap(err, "parsing template")
	}
	cont := &Controller{
		tmpl:         tmpl,
		client:       client,
		buffPool:     bpool.NewBufferPool(64),
		LastProjects: map[string]*Container{},
		LastStats:    &Stats{},
		// defaults
		cleanCutoff: 3 * 24 * time.Hour,
		pageTitle:   "server status",
		groupLabel:  "com.docker.compose.project",
	}
	for _, option := range options {
		if err := option(cont); err != nil {
			return nil, errors.Wrap(err, "running option")
		}
	}
	return cont, nil
}

func hostFromLabel(label string) string {
	match := hostExpr.FindStringSubmatch(label)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func (c *Controller) GetProjects() error {
	seenIDs := map[string]struct{}{}
	containers, err := c.client.ListContainers(
		docker.ListContainersOptions{},
	)
	if err != nil {
		return errors.Wrap(err, "listing containers")
	}
	// insert the current time for any container we see
	for _, rawTain := range containers {
		project, ok := rawTain.Labels[c.groupLabel]
		if !ok {
			continue
		}
		if len(rawTain.Names) == 0 {
			return fmt.Errorf("%q does not have a name", rawTain.ID)
		}
		tain := &Container{
			Name:     rawTain.Names[0],
			Project:  project,
			Status:   strings.ToLower(rawTain.Status),
			LastSeen: time.Now(),
		}
		if label, ok := rawTain.Labels["traefik.web.frontend.rule"]; ok {
			tain.Link = hostFromLabel(label)
		}
		if label, ok := rawTain.Labels["traefik.frontend.rule"]; ok {
			tain.Link = hostFromLabel(label)
		}
		seenIDs[tain.ID()] = struct{}{}
		c.LastProjects[tain.ID()] = tain
	}
	// set containers we haven't seen to down, and delete one that haven't
	// seen since since the cutoff
	cutoff := time.Now().Add(-1 * c.cleanCutoff)
	for id, tain := range c.LastProjects {
		if tain.LastSeen.Before(cutoff) {
			delete(c.LastProjects, id)
			continue
		}
		if _, ok := seenIDs[id]; !ok {
			tain.IsDown = true
		}
	}
	return nil
}

func (c *Controller) GetStats() error {
	// begin load
	loadStat, err := load.Avg()
	if err != nil {
		return errors.Wrap(err, "get load stat")
	}
	c.LastStats.Load1 = loadStat.Load1
	c.LastStats.Load5 = loadStat.Load5
	c.LastStats.Load15 = loadStat.Load15
	// begin mem
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return errors.Wrap(err, "get mem stat")
	}
	c.LastStats.MemUsed = memStat.Used
	c.LastStats.MemTotal = memStat.Total
	// begin cpu
	percent, err := cpu.Percent(5*time.Second, false)
	if err != nil {
		return errors.Wrap(err, "get cpu stat")
	}
	if len(percent) != 1 {
		return fmt.Errorf("invalid cpu response")
	}
	c.LastStats.CPU = percent[0]
	// begin cpu temp
	temps, err := host.SensorsTemperatures()
	if err != nil {
		return errors.Wrap(err, "get temp stat")
	}
	var tempCores int
	var temp float64
	for _, t := range temps {
		if match := tempExpr.MatchString(t.SensorKey); !match {
			continue
		}
		tempCores++
		temp += t.Temperature
	}
	if tempCores == 0 {
		return nil
	}
	c.LastStats.CPUTemp = temp / float64(tempCores)
	return nil
}

func (c *Controller) Start() {
	ticker := time.NewTicker(c.scanInterval)
	for range ticker.C {
		if err := c.GetProjects(); err != nil {
			log.Printf("error getting projects: %v\n", err)
		}
		if err := c.GetStats(); err != nil {
			log.Printf("error getting stats: %v\n", err)
		}
	}
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// group the last seen by project, inserting so that the container
	// names are sorted
	projectMap := map[string][]*Container{}
	for _, tain := range c.LastProjects {
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
		PageTitle  string
		ShowCredit bool
		Projects   map[string][]*Container
		Stats      *Stats
	}{
		c.pageTitle,
		c.showCredit,
		projectMap,
		c.LastStats,
	}
	// using a pool of buffers, we can write to one first to catch template
	// errors, which avoids a superfluous write to the response writer
	buff := c.buffPool.Get()
	defer c.buffPool.Put(buff)
	if err := c.tmpl.Execute(buff, tmplData); err != nil {
		http.Error(w, fmt.Sprintf("error executing template: %v", err), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := buff.WriteTo(w)
	if err != nil {
		log.Printf("error writing response buffer: %v\n", err)
	}
}
