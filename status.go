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

var funcMap = template.FuncMap{
	"humanDate":  humanize.Time,
	"humanBytes": humanize.IBytes,
	"humanDuration": func(d time.Duration) string {
		switch {
		case d.Seconds() < 60:
			return fmt.Sprintf("%.0f seconds", d.Seconds())
		case d.Minutes() < 60:
			return fmt.Sprintf("%.0f minutes", d.Minutes())
		case d.Hours() < 24:
			return fmt.Sprintf("%.0f hours", d.Hours())
		default:
			return fmt.Sprintf("%.0f days", d.Hours()/24)
		}
	},
	"js": func(v interface{}) template.JS {
		out, _ := json.Marshal(v)
		return template.JS(out)
	},
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
	Uptime   time.Duration
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
	histCPU      hist
	histTemp     hist
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
		c.histCPU = hist(make([]float64, dur/c.scanInterval))
		c.histTemp = hist(make([]float64, dur/c.scanInterval))
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
		Funcs(funcMap).
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
	var numCores int
	var temp float64
	for _, t := range cores {
		if match := exprTemp.MatchString(t.SensorKey); match {
			numCores++
			temp += t.Temperature
		}
	}
	if numCores == 0 {
		return 0
	}
	return temp / float64(numCores)
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
	// not checking errors here becuase some of these return lists of
	// warnings which i don't care about at the moment
	if uptime, _ := host.Uptime(); uptime != 0 {
		c.lastStats.Uptime = time.Duration(uptime * 1e+9)
	}
	if load, _ := load.Avg(); load != nil {
		c.lastStats.Load1 = load.Load1
		c.lastStats.Load5 = load.Load5
		c.lastStats.Load15 = load.Load15
	}
	if memory, _ := mem.VirtualMemory(); memory != nil {
		c.lastStats.MemUsed = memory.Used
		c.lastStats.MemTotal = memory.Total
	}
	if cpus, _ := cpu.Percent(0, false); len(cpus) > 0 {
		round := math.Round(cpus[0]*100) / 100
		c.lastStats.CPU = round
		c.histCPU.add(round)
	}
	if temps, _ := host.SensorsTemperatures(); len(temps) > 0 {
		avg := averageTemp(temps)
		c.lastStats.CPUTemp = avg
		c.histTemp.add(avg)
	}
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
		PageTitle    string
		ShowCredit   bool
		Groups       map[string][]string
		Projects     map[string][]Container
		Stats        Stats
		HistDataCPU  []float64
		HistDataTemp []float64
		HistPeriod   time.Duration
	}{
		c.pageTitle,
		c.showCredit,
		c.lastGroups,
		c.lastProjects,
		c.lastStats,
		c.histCPU,
		c.histTemp,
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
