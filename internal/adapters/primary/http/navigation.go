package http

import (
	"fmt"
	"html/template"

	chromaui "github.com/githubixx/chroma-ui"
)

// ParsePageTemplates combines embedded navigation with application-owned pages.
func ParsePageTemplates(files ...string) (*template.Template, error) {
	t, err := chromaui.ParseTemplates(template.New("").Funcs(template.FuncMap{"vdrHeader": navigationHeader}))
	if err != nil {
		return nil, err
	}
	return t.ParseFiles(files...)
}

func navigationHeader(data map[string]any) (chromaui.Header, error) {
	values := make(map[string]string, 4)
	for _, key := range []string{"BrandHref", "PageName", "Path", "Role"} {
		if value := data[key]; value != nil {
			text, ok := value.(string)
			if !ok {
				return chromaui.Header{}, fmt.Errorf("navigation %s must be a string", key)
			}
			values[key] = text
		}
	}
	header := chromaui.Header{
		Brand: "VDRAdmin-go", BrandHref: values["BrandHref"],
		PageTitle: values["PageName"], MenuLabel: "Menu",
	}
	admin := values["Role"] == "admin"
	if admin {
		header.Badge = "Admin"
	}
	path := values["Path"]
	add := func(label, href string, active bool) {
		header.Items = append(header.Items, chromaui.MenuItem{Label: label, Href: href, Active: active})
	}
	add("What's on now", "/now", path == "/" || path == "/now")
	add("Channels", "/channels", path == "/channels")
	add("Watch TV", "/watch", path == "/watch")
	add("Playing Today", "/playing", path == "/playing")
	add("Timers", "/timers", path == "/timers")
	add("Recordings", "/recordings", values["PageName"] == "Recordings")
	if admin {
		add("Jobs", "/recordings/archive/jobs", path == "/recordings/archive/jobs" || path == "/recordings/archive/job")
	}
	add("Search", "/search", path == "/search")
	add("EPG Search", "/epgsearch", path == "/epgsearch")
	add("Configurations", "/configurations", path == "/configurations")
	return header, nil
}
