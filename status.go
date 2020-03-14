package status

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
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

const (
	labelGroup   = "xyz.senan.compose-status.group"
	labelProject = "com.docker.compose.project"
	exprTempStr  = "coretemp_core[0-9]+_input"
	exprHostStr  = "Host:(.+?)(?:,|;|$|\b)"
)

var (
	exprTemp *regexp.Regexp
	exprHost *regexp.Regexp
)

func init() {
	var err error
	exprTemp, err = regexp.Compile(exprTempStr)
	if err != nil {
		log.Fatalf("error compiling temp expr: %v\n", err)
	}
	exprHost, err = regexp.Compile(exprHostStr)
	if err != nil {
		log.Fatalf("error compiling host expr: %v\n", err)
	}
}

type Container struct {
	Name   string
	Status string
	Link   string
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

type Controller struct {
	tmpl         *template.Template
	client       *docker.Client
	buffPool     *bpool.BufferPool
	scanInterval time.Duration
	pageTitle    string
	showCredit   bool
	LastGroups   map[string][]string
	LastProjects map[string][]Container
	LastStats    Stats
}

type ControllerOpt func(*Controller) error

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
		tmpl:      tmpl,
		client:    client,
		buffPool:  bpool.NewBufferPool(64),
		LastStats: Stats{},
	}
	for _, option := range options {
		if err := option(cont); err != nil {
			return nil, errors.Wrap(err, "running option")
		}
	}
	return cont, nil
}

func parseLabelHost(label string) string {
	match := exprHost.FindStringSubmatch(label)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func parseLabelsLink(labels map[string]string) string {
	if label, ok := labels["traefik.web.frontend.rule"]; ok {
		return label
	}
	if label, ok := labels["traefik.frontend.rule"]; ok {
		return label
	}
	return ""
}

func parseStatus(status string) string {
	// TODO: remove (healthy)
	return strings.ToLower(status)
}

func (c *Controller) GetProjects() error {
	responses, err := c.client.ListContainers(
		docker.ListContainersOptions{},
	)
	if err != nil {
		return errors.Wrap(err, "listing containers")
	}
	c.LastGroups = map[string][]string{}
	c.LastProjects = map[string][]Container{}
	groupedProjects := map[string]struct{}{}
	// insert the current time for any container we see
	for _, resp := range responses {
		if len(resp.Names) == 0 {
			return fmt.Errorf("%q does not have a name", resp.ID)
		}
		project, ok := resp.Labels[labelProject]
		if !ok {
			continue
		}
		if group, ok := resp.Labels[labelGroup]; ok {
			c.LastGroups[group] = append(c.LastGroups[group], project)
			groupedProjects[project] = struct{}{}
		}
		c.LastProjects[project] = append(c.LastProjects[project], Container{
			Name:   resp.Names[0],
			Status: parseStatus(resp.Status),
			Link:   parseLabelsLink(resp.Labels),
		})
	}
	for project := range c.LastProjects {
		if _, ok := groupedProjects[project]; !ok {
			// put the ungrouped projects into the "~" pseudo group
			c.LastGroups["~"] = append(c.LastGroups["~"], project)
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
		if match := exprTemp.MatchString(t.SensorKey); !match {
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
	tmplData := struct {
		PageTitle  string
		ShowCredit bool
		Groups     map[string][]string
		Projects   map[string][]Container
		Stats      Stats
	}{
		c.pageTitle,
		c.showCredit,
		c.LastGroups,
		c.LastProjects,
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
