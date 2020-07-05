package status

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
)

const (
	exprTempStr = "coretemp_core[0-9]+_input"
	exprHostStr = "Host(?::|\\(\\`)([0-9a-z\\.-]+)"
	// host prefix & suffix. slightly stupid but needs to support:
	// traefik v1 `traefik.frontend.rule`
	// traefik v1 `traefik.<name>.frontend.rule`
	// traefik v2 `traefik.http.routers.<name>.rule`
	labelHostPrefix = "traefik."
	labelHostSuffix = ".rule"
	labelGroup      = "xyz.senan.compose-status.group"
	labelProject    = "com.docker.compose.project"
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

type hist []float64

func (h *hist) add(n float64) {
	*h = append(*h, n)
	*h = (*h)[1:len(*h)]
}

type Controller struct {
	tmpl         *template.Template
	client       *docker.Client
	buffPool     *bpool.BufferPool
	scanInterval time.Duration
	pageTitle    string
	showCredit   bool
	lastGroups   map[string][]string
	lastProjects map[string][]Container
	lastStats    Stats
	cpuHist      hist
}

type ControllerOpt func(*Controller) error

func WithTitle(title string) ControllerOpt {
	return func(c *Controller) error {
		c.pageTitle = title
		return nil
	}
}

func WithScanInternal(dur time.Duration) ControllerOpt {
	return func(c *Controller) error {
		c.scanInterval = dur
		return nil
	}
}

func WithHistWindow(dur time.Duration) ControllerOpt {
	return func(c *Controller) error {
		c.cpuHist = hist(make([]float64, dur/c.scanInterval))
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
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"humanDate":  humanize.Time,
			"humanBytes": humanize.Bytes,
			"js": func(v interface{}) template.JS {
				out, _ := json.Marshal(v)
				return template.JS(out)
			},
		}).
		Parse(homeTmpl)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}
	cont := &Controller{
		tmpl:      tmpl,
		client:    client,
		buffPool:  bpool.NewBufferPool(64),
		lastStats: Stats{},
	}
	for _, option := range options {
		if err := option(cont); err != nil {
			return nil, fmt.Errorf("running option: %w", err)
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
	for k, v := range labels {
		prefix := strings.HasPrefix(k, labelHostPrefix)
		suffix := strings.HasSuffix(k, labelHostSuffix)
		if prefix && suffix {
			return parseLabelHost(v)
		}
	}
	return ""
}

func parseStatus(status string) string {
	// TODO: remove "(healthy)" er put it elsewhere
	return strings.ToLower(status)
}

func averageTemp(cores []host.TemperatureStat) float64 {
	var coreNo int
	var temp float64
	for _, t := range cores {
		if match := exprTemp.MatchString(t.SensorKey); !match {
			continue
		}
		coreNo++
		temp += t.Temperature
	}
	if coreNo == 0 {
		return 0
	}
	return temp / float64(coreNo)
}

func (c *Controller) GetProjects() error {
	responses, err := c.client.ListContainers(
		docker.ListContainersOptions{},
	)
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}
	c.lastGroups = map[string][]string{}
	c.lastProjects = map[string][]Container{}
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
			c.lastGroups[group] = append(c.lastGroups[group], project)
			groupedProjects[project] = struct{}{}
		}
		c.lastProjects[project] = append(c.lastProjects[project], Container{
			Name:   resp.Names[0],
			Status: parseStatus(resp.Status),
			Link:   parseLabelsLink(resp.Labels),
		})
	}
	for project := range c.lastProjects {
		if _, ok := groupedProjects[project]; !ok {
			// put the ungrouped projects into the "~" pseudo group
			c.lastGroups["~"] = append(c.lastGroups["~"], project)
		}
	}
	return nil
}

func (c *Controller) GetStats() error {
	// ** begin load
	loadStat, err := load.Avg()
	if err != nil {
		return fmt.Errorf("get load stat: %w", err)
	}
	c.lastStats.Load1 = loadStat.Load1
	c.lastStats.Load5 = loadStat.Load5
	c.lastStats.Load15 = loadStat.Load15
	// ** begin mem
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return fmt.Errorf("get mem stat: %w", err)
	}
	c.lastStats.MemUsed = memStat.Used
	c.lastStats.MemTotal = memStat.Total
	// ** begin cpu
	percent, err := cpu.Percent(0, false)
	if err != nil {
		return fmt.Errorf("get cpu stat: %w", err)
	}
	percentRound := math.Round(percent[0]*100) / 100
	c.lastStats.CPU = percentRound
	c.cpuHist.add(percentRound)
	// ** begin cpu temp
	temps, err := host.SensorsTemperatures()
	if err != nil {
		return fmt.Errorf("get temp stat: %w", err)
	}
	c.lastStats.CPUTemp = averageTemp(temps)
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
		HistData   []float64
		HistPeriod time.Duration
	}{
		c.pageTitle,
		c.showCredit,
		c.lastGroups,
		c.lastProjects,
		c.lastStats,
		c.cpuHist,
		c.scanInterval,
	}
	for _, projects := range tmplData.Groups {
		sort.Strings(projects)
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
	if _, err := buff.WriteTo(w); err != nil {
		log.Printf("error writing response buffer: %v\n", err)
	}
}
