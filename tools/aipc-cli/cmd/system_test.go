package cmd

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"aipc/tools/aipc-cli/pkg/output"
)

// The CLI must manage the unit set that aipc-autostart.sh starts at boot, plus
// the explicitly optional ONVIF unit. If the lists drift, `system disable` can
// silently leave a runtime unit behind.
func TestAipcServicesMatchAutostartScript(t *testing.T) {
	raw, err := os.ReadFile("../../../scripts/aipc-autostart.sh")
	if err != nil {
		t.Fatalf("read aipc-autostart.sh: %v", err)
	}
	match := regexp.MustCompile(`(?s)SERVICES=\((.*?)\)`).FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatal("SERVICES=(...) block not found in aipc-autostart.sh")
	}
	var autostart []string
	for _, line := range strings.Split(match[1], "\n") {
		line = strings.SplitN(line, "#", 2)[0]
		autostart = append(autostart, strings.Fields(line)...)
	}
	if len(autostart) == 0 {
		t.Fatal("empty SERVICES block in aipc-autostart.sh")
	}
	var cliBootServices []string
	for _, name := range aipcServices {
		if name != "onvif-device" {
			cliBootServices = append(cliBootServices, name)
		}
	}
	if len(cliBootServices) != len(autostart) {
		t.Fatalf("unit list drift: CLI has %d units %v, autostart has %d %v",
			len(cliBootServices), cliBootServices, len(autostart), autostart)
	}
	for i, name := range autostart {
		if cliBootServices[i] != name {
			t.Fatalf("unit list drift at index %d: CLI=%q autostart=%q (full CLI list %v)",
				i, cliBootServices[i], name, aipcServices)
		}
	}
}

func TestSystemDisableQuiescesAutostartBeforeRuntime(t *testing.T) {
	oldPrinter := printer
	printer = output.NewPrinter("table", false)
	printer.SetWriter(io.Discard)
	t.Cleanup(func() { printer = oldPrinter })

	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "systemctl.log")
	fakeSystemctl := filepath.Join(tmp, "systemctl")
	if err := os.WriteFile(fakeSystemctl, []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$SYSTEMCTL_LOG"
exit 0
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SYSTEMCTL_LOG", logPath)

	if err := serviceDisableCmd.RunE(serviceDisableCmd, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if !strings.HasPrefix(line, "cat ") {
			actions = append(actions, line)
		}
	}
	if len(actions) == 0 {
		t.Fatal("system disable did not invoke systemctl")
	}
	if actions[0] != "disable --now aipc-autostart.service" {
		t.Fatalf("first systemctl action = %q, want autostart disable --now; all actions: %v", actions[0], actions)
	}
	for _, action := range actions[1:] {
		if strings.Contains(action, "aipc-autostart.service") {
			t.Fatalf("autostart was managed again after runtime operations: %v", actions)
		}
	}
}
