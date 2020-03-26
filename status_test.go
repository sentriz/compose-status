package status

import "testing"

func TestHostFromLabel(t *testing.T) {
	cases := []struct {
		name  string
		label string
		exp   string
	}{
		{"extra regex", "HostRegexp:{catchall:.*}", ""},
		{"extra regex", "HostRegexp:.*", ""},
		{"normal", "Host:what.it.do", "what.it.do"},
		{"normal", "Host:good.morning", "good.morning"},
		{"normal", "Host:good.morning;Path=/notifications/hub", "good.morning"},
		{"comma", "Host:what.it.do,howdy.partner", "what.it.do"},
		{"comma", "Host:what.it.do,howdy.partner,what", "what.it.do"},
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
