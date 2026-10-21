package http

import (
	"bytes"
	"strings"
	"testing"
)

func TestApplicationOwnedNavigation(t *testing.T) {
	for _, role := range []string{"admin", "guest", ""} {
		header, err := navigationHeader(map[string]any{
			"Role": role, "Path": "/channels", "BrandHref": "/timers", "PageName": "<Channels>",
		})
		if err != nil {
			t.Fatal(err)
		}
		if header.Brand != "VDRAdmin-go" || header.BrandHref != "/timers" {
			t.Fatal("application branding lost")
		}
		jobs := false
		for _, item := range header.Items {
			if item.Href == "/recordings/archive/jobs" {
				jobs = true
			}
			if item.Href == "/channels" && !item.Active {
				t.Fatal("active page lost")
			}
		}
		if jobs != (role == "admin") || (header.Badge == "Admin") != (role == "admin") {
			t.Fatal("administrator menu visibility changed")
		}
	}
	if _, err := navigationHeader(map[string]any{"Role": 42}); err == nil {
		t.Fatal("invalid navigation input accepted")
	}
	tmpl, err := ParsePageTemplates("../../../../web/templates/_nav.html")
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	if err := tmpl.ExecuteTemplate(&result, "nav_header", map[string]any{"PageName": "<test>"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.String(), "&lt;test&gt;") || strings.Contains(result.String(), "<test>") {
		t.Fatal("navigation title not escaped")
	}
}
