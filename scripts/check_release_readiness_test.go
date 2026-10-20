package scripts

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseReadiness(t *testing.T) {
	script, err := os.ReadFile("check-release-readiness.sh")
	if err != nil {
		t.Fatal(err)
	}
	const module = "github.com/githubixx/chroma-ui"
	const version = "v1.0.0"
	const metadata = "module " + module + "\n\ngo 1.26.0\n"
	const manifest = "module example.com/release-fixture\n\ngo 1.26.0\n\nrequire " + module + " " + version + "\n"

	for _, name := range []string{"pinned", "missing pin", "replacement", "manifest drift", "workspace ignored", "workspace cannot supply pin"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			proxy := t.TempDir()
			cache := t.TempDir()
			write := func(path, content string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			// Fixture revisions exist only in this test's local file proxy.
			versions := filepath.Join(proxy, filepath.FromSlash(module), "@v")
			write(filepath.Join(versions, version+".mod"), metadata)
			write(filepath.Join(versions, version+".info"), `{"Version":"`+version+`","Time":"2026-01-01T00:00:00Z"}`)
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			for path, contents := range map[string]string{"go.mod": metadata, "theme.go": "package chromaui\n"} {
				file, err := writer.Create(module + "@" + version + "/" + path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.Write([]byte(contents)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(versions, version+".zip"), archive.String())
			write(filepath.Join(root, "scripts", "check-release-readiness.sh"), string(script))
			write(filepath.Join(root, "go.mod"), manifest)
			write(filepath.Join(root, "main.go"), "package main\nimport _ \""+module+"\"\nfunc main() {}\n")
			environment := append(os.Environ(),
				"GOWORK=off", "GOFLAGS=-modcacherw", "GOTOOLCHAIN=local",
				"GOENV=off", "GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=",
				"GOPROXY=file://"+filepath.ToSlash(proxy), "GOSUMDB=off",
				"GOMODCACHE="+cache,
			)
			run := func(command string, args ...string) ([]byte, error) {
				cmd := exec.Command(command, args...)
				cmd.Dir = root
				cmd.Env = environment
				return cmd.CombinedOutput()
			}
			if output, err := run("go", "mod", "tidy"); err != nil {
				t.Fatalf("prepare local proxy fixture: %v\n%s", err, output)
			}
			switch name {
			case "missing pin":
				write(filepath.Join(root, "go.mod"), "module example.com/release-fixture\n\ngo 1.26.0\n")
			case "replacement":
				local := t.TempDir()
				write(filepath.Join(local, "go.mod"), metadata)
				write(filepath.Join(local, "theme.go"), "package chromaui\n")
				write(filepath.Join(root, "go.mod"), manifest+"\nreplace "+module+" => "+filepath.ToSlash(local)+"\n")
			case "manifest drift":
				write(filepath.Join(root, "main.go"), "package main\nfunc main() {}\n")
			case "workspace ignored", "workspace cannot supply pin":
				local := t.TempDir()
				write(filepath.Join(local, "go.mod"), metadata)
				write(filepath.Join(local, "theme.go"), "package chromaui\n")
				work := filepath.Join(t.TempDir(), "go.work")
				write(work, "go 1.26.0\n\nuse (\n"+filepath.ToSlash(root)+"\n"+filepath.ToSlash(local)+"\n)\n")
				environment = append(environment, "GOWORK="+work)
				if name == "workspace cannot supply pin" {
					write(filepath.Join(root, "go.mod"), "module example.com/release-fixture\n\ngo 1.26.0\n")
				}
			}
			before := make(map[string][]byte)
			for _, path := range []string{"go.mod", "go.sum"} {
				before[path], err = os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatal(err)
				}
			}
			output, err := run("sh", "scripts/check-release-readiness.sh")
			wantPass := name == "pinned" || name == "workspace ignored"
			if wantPass && err != nil {
				t.Fatalf("expected success: %v\n%s", err, output)
			}
			if !wantPass && (err == nil || !strings.Contains(string(output), "Release blocked:")) {
				t.Fatalf("expected explicit release rejection: %v\n%s", err, output)
			}
			expected := "Standalone dependency preflight passed: " + module + " " + version
			switch name {
			case "missing pin", "workspace cannot supply pin":
				expected = "pin a real published Chroma UI revision"
			case "replacement":
				expected = "without a replace directive"
			case "manifest drift":
				expected = "go.mod/go.sum are not tidy"
			}
			if !strings.Contains(string(output), expected) {
				t.Errorf("missing expected diagnostic %q:\n%s", expected, output)
			}
			for path, previous := range before {
				current, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || !bytes.Equal(current, previous) {
					t.Errorf("preflight changed %s: %v", path, err)
				}
			}
		})
	}
}
