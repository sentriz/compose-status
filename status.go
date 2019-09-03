package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/oxtoacart/bpool"
	"github.com/pkg/errors"
)

type Container struct {
	Name     string
	Status   string
	Link     string
	LastSeen time.Time
	IsDown   bool
	Project  string
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
	lastProjects map[string]*Container
	buffPool     *bpool.BufferPool
	cleanCutoff  time.Duration
	groupLabel   string
	pageTitle    string
	showCredit   bool
}

func WithCleanCutoff(dur time.Duration) func(*Controller) error {
	return func(c *Controller) error {
		c.cleanCutoff = dur
		return nil
	}
}

func WithTitle(title string) func(*Controller) error {
	return func(c *Controller) error {
		c.pageTitle = title
		return nil
	}
}

func WithGroupLabel(label string) func(*Controller) error {
	return func(c *Controller) error {
		c.groupLabel = label
		return nil
	}
}

func WithResume(file []byte) func(*Controller) error {
	return func(c *Controller) error {
		if len(file) <= 0 {
			return nil
		}
		return json.Unmarshal(file, &c.lastProjects)
	}
}

func WithCredit(c *Controller) error {
	c.showCredit = true
	return nil
}

func NewController(options ...func(*Controller) error) (*Controller, error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, errors.Wrap(err, "creating docker client")
	}
	tmpl, err := template.
		New("").
		Funcs(template.FuncMap{
			"humanDate": humanize.Time,
		}).
		Parse(homeTmpl)
	if err != nil {
		return nil, errors.Wrap(err, "parsing template")
	}
	cont := &Controller{
		tmpl:         tmpl,
		client:       client,
		lastProjects: map[string]*Container{},
		buffPool:     bpool.NewBufferPool(64),
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
	const prefix = "Host:"
	if strings.HasPrefix(label, prefix) {
		trimmed := strings.TrimPrefix(label, prefix)
		return strings.SplitN(trimmed, ",", 2)[0]
	}
	return ""
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
		if label, ok := rawTain.Labels["traefik.frontend.rule"]; ok {
			tain.Link = hostFromLabel(label)
		}
		seenIDs[tain.ID()] = struct{}{}
		c.lastProjects[tain.ID()] = tain
	}
	// set containers we haven't seen to down, and delete one that haven't
	// seen since since the cutoff
	cutoff := time.Now().Add(-1 * c.cleanCutoff)
	for id, tain := range c.lastProjects {
		if tain.LastSeen.Before(cutoff) {
			delete(c.lastProjects, id)
			continue
		}
		if _, ok := seenIDs[id]; !ok {
			tain.IsDown = true
		}
	}
	return nil
}

func (c *Controller) GetLastProjects() map[string]*Container {
	return c.lastProjects
}

func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// group the last seen by project, inserting so that the container
	// names are sorted
	projectMap := map[string][]*Container{}
	for _, tain := range c.lastProjects {
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
		Projects   map[string][]*Container
		PageTitle  string
		ShowCredit bool
	}{
		projectMap,
		c.pageTitle,
		c.showCredit,
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
	buff.WriteTo(w)
}
