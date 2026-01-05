package http

import (
	"bytes"
	"html/template"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/githubixx/vdradmin-go/internal/infrastructure/config"
)

func TestTemplate_EPGSearchEdit_HasFormAndGlossyButtons(t *testing.T) {
	// This test protects against accidental template corruption (missing <form>,
	// mismatched tags) and missing button classes that make the page lose the
	// standard "glossy" styling.
	//
	// We cannot reliably test CSS rendering in Go unit tests, but we *can* ensure
	// the expected semantic structure and class names are present in the rendered HTML.

	tmpl := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_edit.html"),
	))

	data := map[string]any{
		// Common header inputs (normally injected by Handler.renderTemplate)
		"User":         "admin",
		"Role":         "admin",
		"Year":         2026,
		"Path":         "/epgsearch/new",
		"ThemeDefault": "light",
		"ThemeMode":    "light",

		// Page-specific inputs
		"Heading":    "Add New Search",
		"PageTitle":  "Add New Search - VDRAdmin-go",
		"FormAction": "/epgsearch/new",
		"Search": config.EPGSearch{
			Active:     true,
			Mode:       "phrase",
			InTitle:    true,
			InSubtitle: true,
			InDesc:     true,
			UseChannel: "no",
		},
		"Channels": []struct{}{},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "epgsearch_edit.html", data); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	html := buf.String()

	mustContain(t, html, "<form")
	mustContain(t, html, "class=\"config-grid\"")
	mustContain(t, html, "action=\"/epgsearch/new\"")
	mustContain(t, html, "<button type=\"submit\" name=\"action\" value=\"save\" class=\"btn btn-primary\">Save</button>")
	mustContain(t, html, "<button type=\"submit\" name=\"action\" value=\"run\" class=\"btn btn-secondary\">Run</button>")
	mustContain(t, html, "<a href=\"/epgsearch\" class=\"btn btn-secondary\">Cancel</a>")

	saveIdx := bytes.Index([]byte(html), []byte("value=\"save\""))
	runIdx := bytes.Index([]byte(html), []byte("value=\"run\""))
	cancelIdx := bytes.Index([]byte(html), []byte(">Cancel</a>"))
	if saveIdx == -1 || runIdx == -1 || cancelIdx == -1 {
		t.Fatalf("expected Save/Run/Cancel controls to be present")
	}
	if !(saveIdx < runIdx && runIdx < cancelIdx) {
		t.Fatalf("expected button order Save -> Run -> Cancel")
	}
}

func TestTemplate_EPGSearchEdit_ShowsRunButtonOnEdit(t *testing.T) {
	tmpl := template.Must(template.ParseFiles(
		filepath.Join(repoRoot(t), "web", "templates", "_nav.html"),
		filepath.Join(repoRoot(t), "web", "templates", "epgsearch_edit.html"),
	))

	data := map[string]any{
		"User":         "admin",
		"Role":         "admin",
		"Year":         2026,
		"Path":         "/epgsearch/edit?id=1",
		"ThemeDefault": "light",
		"ThemeMode":    "light",
		"Heading":      "Edit Search",
		"PageTitle":    "Edit Search - VDRAdmin-go",
		"FormAction":   "/epgsearch/edit",
		"Search": config.EPGSearch{
			ID:         1,
			Active:     true,
			Mode:       "phrase",
			InTitle:    true,
			InSubtitle: true,
			InDesc:     true,
			UseChannel: "no",
		},
		"Channels": []struct{}{},
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "epgsearch_edit.html", data); err != nil {
		t.Fatalf("execute template: %v", err)
	}

	html := buf.String()
	mustContain(t, html, "action=\"/epgsearch/edit\"")
	mustContain(t, html, "<button type=\"submit\" name=\"action\" value=\"run\" class=\"btn btn-secondary\">Run</button>")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// this file lives at: <root>/internal/adapters/primary/http/...
	root := filepath.Dir(thisFile)
	for i := 0; i < 4; i++ {
		root = filepath.Dir(root)
	}
	return root
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !bytes.Contains([]byte(haystack), []byte(needle)) {
		t.Fatalf("expected HTML to contain %q", needle)
	}
}
