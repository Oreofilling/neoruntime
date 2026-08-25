package cmd

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The CLI must manage exactly the unit set that aipc-autostart.sh enables and
// starts at boot. If the lists drift, `aipc-cli system stop/disable` silently
// leaves units behind (list too short) or hard-fails on installs that lack a
// unit (list too long).
func TestAipcServicesMatchAutostartScript(t *testing.T) {
	raw, err := os.ReadFile("../../../scripts/aipc-autostart.sh")
	if err != nil {
		t.Fatalf("read aipc-autostart.sh: %v", err)
	}
	match := regexp.MustCompile(`(?s)SERVICES=\((.*?)\)`).FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatal("SERVICES=(...) block not found in aipc-autostart.sh")
	}
	autostart := strings.Fields(match[1])
	if len(autostart) == 0 {
		t.Fatal("empty SERVICES block in aipc-autostart.sh")
	}
	if len(aipcServices) != len(autostart) {
		t.Fatalf("unit list drift: CLI has %d units %v, autostart has %d %v",
			len(aipcServices), aipcServices, len(autostart), autostart)
	}
	for i, name := range autostart {
		if aipcServices[i] != name {
			t.Fatalf("unit list drift at index %d: CLI=%q autostart=%q (full CLI list %v)",
				i, aipcServices[i], name, aipcServices)
		}
	}
}
