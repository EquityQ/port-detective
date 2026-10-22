package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EquityQ/port-detective/internal/core"
)

func TestProjectPresetValidation(t *testing.T) {
	for _, test := range []struct{ name, config string }{
		{"missing", ""},
		{"malformed", "{"},
		{"wrong type", `{"ports":[3000]}`},
		{"empty", `{"ports":""}`},
		{"absent field", `{}`},
		{"null", `null`},
		{"null field", `{"ports":null}`},
		{"trailing object", `{"ports":"3000"}{}`},
		{"unknown field", `{"ports":"3000","prot":"tcp"}`},
		{"oversized", `{"ports":"3000"}` + strings.Repeat(" ", 65536)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.name != "missing" {
				if err := os.WriteFile(filepath.Join(dir, ".port-detective.json"), []byte(test.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readProjectPorts(dir); err == nil {
				t.Fatalf("expected invalid config %q", test.config)
			}
		})
	}
}

func TestProjectPresetReadsNearestRootOnlyWhenSelected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"preset-test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".port-detective.json"), []byte(`{"ports":"8081,3000-3002,3001"}`), 0600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "src")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	root := NewRootCommand()
	scan, _, err := root.Find([]string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := scan.Flags().Set("preset", "project"); err != nil {
		t.Fatal(err)
	}
	if err := scan.Flags().Set("project", nested); err != nil {
		t.Fatal(err)
	}
	ports, scope, err := selectScanPorts(scan, "", "project", nested)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 4 || ports[0] != 3000 || ports[3] != 8081 || scope != filepath.Join(resolved, ".port-detective.json") {
		t.Fatalf("unexpected project preset %v %q", ports, scope)
	}
	defaultRoot := NewRootCommand()
	defaultScan, _, _ := defaultRoot.Find([]string{"scan"})
	defaults, _, err := selectScanPorts(defaultScan, "", "", "")
	if err != nil || len(defaults) != len(core.DefaultDevPorts) {
		t.Fatalf("default scan changed: %v, %v", defaults, err)
	}
}

func TestScanPresetFlagsRejectConflictsAndEmptyValues(t *testing.T) {
	for _, args := range [][]string{
		{"scan", "--ports", "3000", "--preset", "web"},
		{"scan", "--ports="},
		{"scan", "--preset="},
		{"scan", "--preset", "unknown"},
		{"scan", "--project", "."},
		{"scan", "--preset", "web", "--project", "."},
		{"scan", "--preset", "project", "--project="},
		{"clean", "--project=", "--dry-run"},
		{"clean", "--ports=", "--dry-run"},
	} {
		if _, _, err := captureRun(t, args...); err == nil {
			t.Errorf("expected invalid flags: %v", args)
		}
	}
}

func TestBuiltInScanPresets(t *testing.T) {
	for _, name := range []string{"web", "python", "database"} {
		root := NewRootCommand()
		scan, _, _ := root.Find([]string{"scan"})
		if err := scan.Flags().Set("preset", name); err != nil {
			t.Fatal(err)
		}
		ports, _, err := selectScanPorts(scan, "", name, "")
		if err != nil || len(ports) == 0 {
			t.Fatalf("%s: %v %v", name, ports, err)
		}
		for i := 1; i < len(ports); i++ {
			if ports[i] <= ports[i-1] {
				t.Fatalf("preset %s is not a sorted set: %v", name, ports)
			}
		}
	}
}
