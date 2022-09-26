package status

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	exprTempStr = "coretemp_core_[0-9]+"
	exprHostStr = "Host\\w*(?::|\\(\\`)([0-9a-z\\.-]+)"
	// host prefix & suffix. slightly stupid but needs to support:
	// traefik v1 `traefik.frontend.rule`
	// traefik v1 `traefik.<name>.frontend.rule`
	// traefik v2 `traefik.http.routers.<name>.rule`
	labelHostPrefix   = "traefik."
	labelHostSuffix   = ".rule"
	labelGroup        = "xyz.senan.compose-status.group"
	labelCheckMethod  = "xyz.senan.compose-status.check.method"
	labelCheckPort    = "xyz.senan.compose-status.check.port"
	labelCheckPath    = "xyz.senan.compose-status.check.path"
	labelCheckExpCode = "xyz.senan.compose-status.check.code"
	labelProject      = "com.docker.compose.project"
)

//go:embed tmpl.html
var homeTmpl string

//go:embed chart.js
var chartJS []byte

var (
	exprTemp = regexp.MustCompile(exprTempStr)
	exprHost = regexp.MustCompile(exprHostStr)
)

var funcMap = template.FuncMap{
	"humanDate":  humanize.Time,
	"humanBytes": humanize.IBytes,
	"humanDuration": func(d time.Duration) string {
		switch {
		case d.Milliseconds() < 1000:
			return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
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
	HTTP   HTTPCheck
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
	tmpl              *template.Template
	dockerNetworkName string
	dockerClient      *docker.Client
	httpClient        *http.Client
	buffPool          *bpool.BufferPool
	scanInterval      time.Duration
	pageTitle         string
	showCredit        bool
	lastGroups        map[string][]string
	lastProjects      map[string][]Container
	lastStats         Stats
	histCPU           hist
	histTemp          hist

	*http.ServeMux
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

func NewController(dockerNetworkName string, options ...ControllerOpt) (*Controller, error) {
	dockerClient, err := docker.NewClientFromEnv()
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
		tmpl:              tmpl,
		dockerClient:      dockerClient,
		dockerNetworkName: dockerNetworkName,
		httpClient: &http.Client{
			Timeout: 25 * time.Millisecond,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		buffPool:  bpool.NewBufferPool(64),
		lastStats: Stats{},
		ServeMux:  http.NewServeMux(),
	}
	for _, option := range options {
		if err := option(cont); err != nil {
			return nil, fmt.Errorf("running option: %w", err)
		}
	}
	cont.ServeMux.HandleFunc("/chart.js", cont.serveChartJS)
	cont.ServeMux.HandleFunc("/", cont.serveHome)
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
	status = strings.ReplaceAll(status, "(healthy)", "")
	status = strings.TrimSpace(status)
	status = strings.ToLower(status)
	return status
}

type HTTPCheck struct {
	OK       bool
	Code     int
	Duration time.Duration
	Timeout  bool
}

func checkHTTP(httpClient *http.Client, dockerNetworkID string, dockerContainer docker.APIContainers) (*HTTPCheck, error) {
	portRaw, ok := dockerContainer.Labels[labelCheckPort]
	if !ok {
		return nil, nil
	}
	port, _ := strconv.Atoi(portRaw)

	var method string = http.MethodHead
	if m, ok := dockerContainer.Labels[labelCheckMethod]; ok {
		method = m
	}
	var path string = "/"
	if p, ok := dockerContainer.Labels[labelCheckPath]; ok {
		path = p
	}
	var expCode int = 200
	if c, ok := dockerContainer.Labels[labelCheckExpCode]; ok {
		expCode, _ = strconv.Atoi(c)
	}
	var ip string
	for _, v := range dockerContainer.Networks.Networks {
		if v.NetworkID != dockerNetworkID {
			continue
		}
		ip = v.IPAddress
		break
	}
	if ip == "" {
		return nil, nil
	}

	url := fmt.Sprintf("http://%s:%d/%s", ip, port, strings.TrimPrefix(path, "/"))
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	start := time.Now()

	res, err := httpClient.Do(req)
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &HTTPCheck{Timeout: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("make request: %w", err)
	}
	check := &HTTPCheck{
		Code:     res.StatusCode,
		Duration: time.Since(start),
	}
	if (res.StatusCode >= 200 && res.StatusCode < 300) || res.StatusCode == expCode {
		check.OK = true
	}
	return check, err
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

func (c *Controller) Refresh() error {
	dockerNetworks, err := c.dockerClient.ListNetworks()
	if err != nil {
		return fmt.Errorf("list docker networks: %w", err)
	}
	var dockerNetworkID string
	for _, dn := range dockerNetworks {
		if dn.Name == c.dockerNetworkName {
			dockerNetworkID = dn.ID
			break
		}
	}
	if dockerNetworkID == "" {
		return fmt.Errorf("can't find docker network %q", c.dockerNetworkName)
	}
	dockerContainers, err := c.dockerClient.ListContainers(
		docker.ListContainersOptions{},
	)
	if err != nil {
		return fmt.Errorf("listing containers: %w", err)
	}
	c.lastGroups = map[string][]string{}
	c.lastProjects = map[string][]Container{}
	groupedProjects := map[string]struct{}{}
	// insert the current time for any container we see
	for _, dockerContainer := range dockerContainers {
		if len(dockerContainer.Names) == 0 {
			return fmt.Errorf("%q does not have a name", dockerContainer.ID)
		}
		project, ok := dockerContainer.Labels[labelProject]
		if !ok {
			continue
		}
		if group, ok := dockerContainer.Labels[labelGroup]; ok {
			c.lastGroups[group] = append(c.lastGroups[group], project)
			groupedProjects[project] = struct{}{}
		}
		container := Container{
			Name:   dockerContainer.Names[0],
			Status: parseStatus(dockerContainer.Status),
			Link:   parseLabelsLink(dockerContainer.Labels),
		}
		check, err := checkHTTP(c.httpClient, dockerNetworkID, dockerContainer)
		if err != nil {
			log.Printf("error getting http check for %q: %v\n", container.Name, err)
		}
		if check != nil {
			container.HTTP = *check
		}
		c.lastProjects[project] = append(c.lastProjects[project], container)
	}
	for project := range c.lastProjects {
		if _, ok := groupedProjects[project]; !ok {
			// put the ungrouped projects into the "~" pseudo group
			c.lastGroups["~"] = append(c.lastGroups["~"], project)
		}
	}

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
	if err := c.Refresh(); err != nil {
		log.Printf("error refreshing: %v\n", err)
	}

	ticker := time.NewTicker(c.scanInterval)
	for range ticker.C {
		if err := c.Refresh(); err != nil {
			log.Printf("error refreshing: %v\n", err)
		}
	}
}

func (c *Controller) serveHome(w http.ResponseWriter, r *http.Request) {
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

func (c *Controller) serveChartJS(w http.ResponseWriter, r *http.Request) {
	http.ServeContent(w, r, "chart.js", time.Unix(0, 0), bytes.NewReader(chartJS))
}
