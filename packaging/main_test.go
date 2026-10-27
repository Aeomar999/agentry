package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMacZipReadmeLeadsTheFirstOpenWithOpenAnyway(t *testing.T) {
	builds := t.TempDir()
	for _, p := range platforms {
		if err := os.WriteFile(filepath.Join(builds, "agentry-"+p.os+"-"+p.arch), []byte("program"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range platforms {
		if p.os != "darwin" {
			continue
		}
		name := filepath.Join(t.TempDir(), p.label+".zip")
		if err := pack(name, p, builds, "v1.2.3"); err != nil {
			t.Fatal(err)
		}
		text := readZipFile(t, name, "Agentry/README.txt")
		if !strings.Contains(text, "Open Anyway") || strings.Contains(text, "right-click") {
			t.Errorf("%s: not Open Anyway alone in\n%s", p.label, text)
		}
	}
}

func readZipFile(t *testing.T, zipPath, name string) string {
	t.Helper()
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	f, err := z.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
