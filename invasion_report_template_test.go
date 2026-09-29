package main

import (
	"os"
	"strings"
	"testing"
)

const planetMessageDetailPath = "frontend/src/routes/(user)/games/(game)/[id]/messages/PlanetMessageDetail.svelte"

// readSection extracts the template block between the marker containing
// startNeedle and the next occurrence of endNeedle.
func readSection(t *testing.T, content, startNeedle, endNeedle string) string {
	t.Helper()
	start := strings.Index(content, startNeedle)
	if start == -1 {
		t.Fatalf("template is missing section %q", startNeedle)
	}
	end := strings.Index(content[start:], endNeedle)
	if end == -1 {
		t.Fatalf("template section %q is missing terminator %q", startNeedle, endNeedle)
	}
	return content[start : start+end]
}

// Test_invasion_report_template_perspective guards the invasion report
// template: each faction's report may only contain sentences written from
// that faction's own perspective.
func Test_invasion_report_template_perspective(t *testing.T) {
	data, err := os.ReadFile(planetMessageDetailPath)
	if err != nil {
		t.Fatalf("failed to read message template: %v", err)
	}
	content := string(data)

	attackerSection := readSection(t, content, "PlayerMessageType.FLEET_INVADED_PLANET", "PlayerMessageType.PLANET_INVADED")
	defenderSection := readSection(t, content, "PlayerMessageType.PLANET_INVADED", "PlayerMessageType.PLANET_POPULATION_DECREASED_OVERCROWDING")

	// the attacker's report keeps its own first-person sentences
	if !strings.Contains(attackerSection, "Your troops beaming down") {
		t.Errorf("attacker report lost its own perspective sentence")
	}

	// the defender's report must not contain attacker-perspective sentences,
	// whether the invasion succeeded or failed
	for _, phrase := range []string{"Your troops", "Your valiant fighters"} {
		if strings.Contains(defenderSection, phrase) {
			t.Errorf("defender report contains attacker-perspective phrase %q", phrase)
		}
	}

	// the defender's report still describes the shared event from their side
	if !strings.Contains(defenderSection, "have successfully invaded") {
		t.Errorf("defender report is missing the shared successful-invasion event")
	}
	if !strings.Contains(defenderSection, "tried to invade") {
		t.Errorf("defender report is missing the shared failed-invasion event")
	}

	// both perspectives still render the shared casualty numbers
	for _, field := range []string{"invasion.attackersKilled", "invasion.defendersKilled"} {
		if !strings.Contains(attackerSection, field) {
			t.Errorf("attacker report is missing shared field %q", field)
		}
		if !strings.Contains(defenderSection, field) {
			t.Errorf("defender report is missing shared field %q", field)
		}
	}
}
