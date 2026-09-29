//go:build !wasi && !wasm

package cs

import (
	"encoding/json"
	"reflect"
	"testing"
)

// snapshotJSON returns a deep, deterministic snapshot of a value so tests can
// assert that report generation leaves game state untouched.
func snapshotJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to snapshot %T: %v", v, err)
	}
	return string(b)
}

// buildInvasionPlayers returns a defender and attacker pair for report tests.
func buildInvasionPlayers() (defender, attacker *Player) {
	defender = NewPlayer(1, NewRace().WithSpec(&rules)).WithNum(1).withSpec(&rules)
	attacker = NewPlayer(2, NewRace().WithSpec(&rules)).WithNum(2).withSpec(&rules)
	return defender, attacker
}

func Test_invasion_report_perspective(t *testing.T) {
	tests := []struct {
		name              string
		planet            *Planet
		attackers         int
		fleets            []*Fleet
		wantSuccessful    bool
		wantFleetName     string
		wantAttackerType  PlayerMessageType
		wantDefenderType  PlayerMessageType
		wantBothSidesLoss bool
	}{
		{
			name:             "attacker wins, single fleet named in both reports",
			planet:           &Planet{Cargo: Cargo{}.WithPopulation(10_000)},
			attackers:        10_000,
			fleets:           []*Fleet{{MapObject: MapObject{Name: "Alpha"}}},
			wantSuccessful:   true,
			wantFleetName:    "Alpha",
			wantAttackerType: PlayerMessageFleetInvadedPlanet,
			wantDefenderType: PlayerMessagePlanetInvaded,
		},
		{
			name:             "defender wins, attacker report still attacker perspective",
			planet:           &Planet{Cargo: Cargo{}.WithPopulation(10_000)},
			attackers:        5_000,
			fleets:           []*Fleet{{MapObject: MapObject{Name: "Alpha"}}},
			wantSuccessful:   false,
			wantFleetName:    "Alpha",
			wantAttackerType: PlayerMessageFleetInvadedPlanet,
			wantDefenderType: PlayerMessagePlanetInvaded,
		},
		{
			name:              "both sides take losses, shared spec marks kills for each side",
			planet:            &Planet{Cargo: Cargo{}.WithPopulation(10_000)},
			attackers:         10_000,
			fleets:            []*Fleet{{MapObject: MapObject{Name: "Alpha"}}},
			wantSuccessful:    true,
			wantFleetName:     "Alpha",
			wantAttackerType:  PlayerMessageFleetInvadedPlanet,
			wantDefenderType:  PlayerMessagePlanetInvaded,
			wantBothSidesLoss: true,
		},
		{
			name:             "multiple fleets, fleet name omitted from shared spec",
			planet:           &Planet{Cargo: Cargo{}.WithPopulation(10_000)},
			attackers:        10_000,
			fleets:           []*Fleet{{MapObject: MapObject{Name: "Alpha"}}, {MapObject: MapObject{Name: "Beta"}}},
			wantSuccessful:   true,
			wantFleetName:    "",
			wantAttackerType: PlayerMessageFleetInvadedPlanet,
			wantDefenderType: PlayerMessagePlanetInvaded,
		},
		{
			name:             "defended planet, defender wins and keeps planet",
			planet:           &Planet{Cargo: Cargo{}.WithPopulation(100_000), Spec: PlanetSpec{DefenseCoverage: .9}},
			attackers:        100_000,
			fleets:           []*Fleet{{MapObject: MapObject{Name: "Alpha"}}},
			wantSuccessful:   false,
			wantFleetName:    "Alpha",
			wantAttackerType: PlayerMessageFleetInvadedPlanet,
			wantDefenderType: PlayerMessagePlanetInvaded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defender, attacker := buildInvasionPlayers()
			tt.planet.Num = 1
			tt.planet.Name = "Earth"
			tt.planet.PlayerNum = defender.Num

			inv := invasion{
				planet:    tt.planet,
				defender:  defender,
				attacker:  attacker,
				attackers: tt.attackers,
				fleets:    tt.fleets,
			}
			result := inv.resolve(&rules)

			if result.successful != tt.wantSuccessful {
				t.Fatalf("resolve() successful = %v, want %v", result.successful, tt.wantSuccessful)
			}
			if tt.wantBothSidesLoss && (result.attackersKilled <= 0 || result.defendersKilled <= 0) {
				t.Fatalf("expected losses on both sides, got attackersKilled=%d defendersKilled=%d", result.attackersKilled, result.defendersKilled)
			}

			// snapshot state before report generation; reports must not mutate it
			planetBefore := snapshotJSON(t, tt.planet)
			defenderBefore := snapshotJSON(t, defender)
			attackerBefore := snapshotJSON(t, attacker)

			messager.planetInvaded(defender, tt.planet, inv.fleetDescription(), attacker, defender, result.attackersKilled, result.defendersKilled, result.successful)
			messager.planetInvaded(attacker, tt.planet, inv.fleetDescription(), attacker, defender, result.attackersKilled, result.defendersKilled, result.successful)

			if got := snapshotJSON(t, tt.planet); got != planetBefore {
				t.Errorf("planet mutated by report generation\nbefore: %s\nafter:  %s", planetBefore, got)
			}

			if len(defender.Messages) != 1 {
				t.Fatalf("defender got %d messages, want 1", len(defender.Messages))
			}
			if len(attacker.Messages) != 1 {
				t.Fatalf("attacker got %d messages, want 1", len(attacker.Messages))
			}

			defenderMsg := defender.Messages[0]
			attackerMsg := attacker.Messages[0]

			// each side only gets its own perspective
			if attackerMsg.Type != tt.wantAttackerType {
				t.Errorf("attacker message type = %v, want %v", attackerMsg.Type, tt.wantAttackerType)
			}
			if defenderMsg.Type != tt.wantDefenderType {
				t.Errorf("defender message type = %v, want %v", defenderMsg.Type, tt.wantDefenderType)
			}
			if attackerMsg.Type == defenderMsg.Type {
				t.Errorf("attacker and defender received the same message type %v", attackerMsg.Type)
			}

			// the shared event spec must be identical for both sides
			if attackerMsg.Spec.Invasion == nil || defenderMsg.Spec.Invasion == nil {
				t.Fatalf("missing invasion spec: attacker=%v defender=%v", attackerMsg.Spec.Invasion, defenderMsg.Spec.Invasion)
			}
			if !reflect.DeepEqual(attackerMsg.Spec.Invasion, defenderMsg.Spec.Invasion) {
				t.Errorf("shared invasion spec differs between perspectives\nattacker: %+v\ndefender: %+v", attackerMsg.Spec.Invasion, defenderMsg.Spec.Invasion)
			}

			spec := attackerMsg.Spec.Invasion
			if spec.AttackerPlayerNum != attacker.Num || spec.DefenderPlayerNum != defender.Num {
				t.Errorf("spec players = attacker %d defender %d, want %d/%d", spec.AttackerPlayerNum, spec.DefenderPlayerNum, attacker.Num, defender.Num)
			}
			if spec.Successful != tt.wantSuccessful {
				t.Errorf("spec successful = %v, want %v", spec.Successful, tt.wantSuccessful)
			}
			if spec.FleetName != tt.wantFleetName {
				t.Errorf("spec fleetName = %q, want %q", spec.FleetName, tt.wantFleetName)
			}
			if spec.AttackersKilled != result.attackersKilled || spec.DefendersKilled != result.defendersKilled {
				t.Errorf("spec kills = %d/%d, want %d/%d", spec.AttackersKilled, spec.DefendersKilled, result.attackersKilled, result.defendersKilled)
			}

			// report generation must not leak state into either player
			// (messages are the only allowed change)
			defender.Messages = nil
			attacker.Messages = nil
			if got := snapshotJSON(t, defender); got != defenderBefore {
				t.Errorf("defender state beyond messages mutated by report generation")
			}
			if got := snapshotJSON(t, attacker); got != attackerBefore {
				t.Errorf("attacker state beyond messages mutated by report generation")
			}
		})
	}
}

// Test_invasion_report_deterministic_replay ensures the same input sequence
// produces identical reports for both sides on replay.
func Test_invasion_report_deterministic_replay(t *testing.T) {
	generate := func() (attackerMsgs, defenderMsgs []PlayerMessage) {
		defender, attacker := buildInvasionPlayers()
		planet := &Planet{Cargo: Cargo{}.WithPopulation(10_000)}
		planet.Num = 1
		planet.Name = "Earth"
		planet.PlayerNum = defender.Num
		inv := invasion{
			planet:    planet,
			defender:  defender,
			attacker:  attacker,
			attackers: 10_000,
			fleets:    []*Fleet{{MapObject: MapObject{Name: "Alpha"}}, {MapObject: MapObject{Name: "Beta"}}},
		}
		result := inv.resolve(&rules)
		messager.planetInvaded(defender, planet, inv.fleetDescription(), attacker, defender, result.attackersKilled, result.defendersKilled, result.successful)
		messager.planetInvaded(attacker, planet, inv.fleetDescription(), attacker, defender, result.attackersKilled, result.defendersKilled, result.successful)
		return attacker.Messages, defender.Messages
	}

	attacker1, defender1 := generate()
	attacker2, defender2 := generate()

	if !reflect.DeepEqual(attacker1, attacker2) {
		t.Errorf("attacker reports differ across identical replays\nfirst:  %+v\nsecond: %+v", attacker1, attacker2)
	}
	if !reflect.DeepEqual(defender1, defender2) {
		t.Errorf("defender reports differ across identical replays\nfirst:  %+v\nsecond: %+v", defender1, defender2)
	}
}

// Test_invader_multi_fleet_aggregation ensures multiple fleets from the same
// attacker merge into one invasion event with no single-fleet name.
func Test_invader_multi_fleet_aggregation(t *testing.T) {
	defender, attacker := buildInvasionPlayers()
	planet := &Planet{Cargo: Cargo{}.WithPopulation(10_000)}
	planet.Num = 1
	planet.PlayerNum = defender.Num

	inv := newInvader()
	inv.addInvasion(invasion{planet: planet, defender: defender, attacker: attacker, attackers: 5_000, fleets: []*Fleet{{MapObject: MapObject{Name: "Alpha"}}}})
	inv.addInvasion(invasion{planet: planet, defender: defender, attacker: attacker, attackers: 5_000, fleets: []*Fleet{{MapObject: MapObject{Name: "Beta"}}}})

	invasions := inv.invasionsByPlanet[planet.Num]
	if len(invasions) != 1 {
		t.Fatalf("got %d invasions for planet, want 1 merged invasion", len(invasions))
	}
	if invasions[0].attackers != 10_000 {
		t.Errorf("merged attackers = %d, want 10000", invasions[0].attackers)
	}
	if got := invasions[0].fleetDescription(); got != "" {
		t.Errorf("fleetDescription() = %q, want empty for multiple fleets", got)
	}

	results := inv.resolveInvasions(&rules)
	if len(results) != 1 {
		t.Fatalf("got %d invasion results, want 1", len(results))
	}
	if !results[0].successful {
		t.Errorf("merged 10000 attackers vs 10000 defenders should succeed")
	}
}
