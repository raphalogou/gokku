package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("output = %q, want %q", out.String(), version)
	}
}

func TestRunRejectsUnknownAndMissingCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"deploy"}} {
		var stderr bytes.Buffer
		if err := run(args, &bytes.Buffer{}, &stderr); err == nil {
			t.Errorf("run(%q) succeeded", args)
		}
		if !strings.Contains(stderr.String(), "Usage: gokku") {
			t.Errorf("run(%q) did not print usage", args)
		}
	}
}
