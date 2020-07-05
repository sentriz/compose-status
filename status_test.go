package status

import (
	"testing"
)

func TestHostFromLabel(t *testing.T) {
	cases := []struct {
		name  string
		label string
		exp   string
	}{
		{"v1 extra regex", "HostRegexp:{catchall:.*}", ""},
		{"v1 extra regex", "HostRegexp:.*", ""},
		{"v1 normal", "Host:what.it.do", "what.it.do"},
		{"v1 normal", "Host:good.morning", "good.morning"},
		{"v1 normal", "Host:good.morning;Path=/notifications/hub", "good.morning"},
		{"v1 comma", "Host:what.it.do,howdy.partner", "what.it.do"},
		{"v1 comma", "Host:what.it.do,howdy.partner,what", "what.it.do"},
		{"v2 normal", "Host(`what.it.do`)", "what.it.do"},
		{"v2 number", "Host(`mp3.mixtape.fam`)", "mp3.mixtape.fam"},
		{"v2 operator", "Path(`/path`) || Host(`what.it.do`)", "what.it.do"},
		{"v2 with hyphen", "Path(`/path`) || Host(`what-dev.it.do`)", "what-dev.it.do"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actu := parseLabelHost(tc.label)
			if actu != tc.exp {
				t.Errorf("expected %q, got %q", tc.exp, actu)
			}
		})
	}
}
