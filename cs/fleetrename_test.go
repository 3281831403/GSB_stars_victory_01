//go:build !wasi && !wasm

package cs

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// snapshot the observable state of a fleet for deep before/after comparisons
func fleetSnapshot(t *testing.T, fleet *Fleet) string {
	t.Helper()
	data, err := json.Marshal(fleet)
	assert.NoError(t, err)
	return string(data)
}

// find a fleet by its unique identity
func fleetByNum(fleets []*Fleet, playerNum, num int) *Fleet {
	for _, fleet := range fleets {
		if fleet.PlayerNum == playerNum && fleet.Num == num {
			return fleet
		}
	}
	return nil
}

func newRenameTestPlayer() *Player {
	return NewPlayer(1, NewRace().WithSpec(&rules))
}

func newRenameTestFleet(player *Player, num int, name string) *Fleet {
	return NewFleet(player, num, name, []Waypoint{NewPositionWaypoint(Vector{}, 5)})
}

func TestValidateFleetName(t *testing.T) {
	tests := []struct {
		name    string
		fleet   string
		wantErr bool
	}{
		{"simple name", "Alpha", false},
		{"name with spaces and digits", "Fleet 7 East", false},
		{"name with punctuation", "A-1 (scout).", false},
		{"max length name", strings.Repeat("a", MaxFleetNameLength), false},
		{"empty name", "", true},
		{"whitespace only name", "   ", true},
		{"too long name", strings.Repeat("a", MaxFleetNameLength+1), true},
		{"name with newline", "bad\nname", true},
		{"name with tab", "bad\tname", true},
		{"name with control character", "bad\x00name", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFleetName(tt.fleet)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFleet_Rename_success(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 1, "Long Range Scout")

	assert.NoError(t, fleet.Rename("Alpha"))

	// the new name is exactly what was supplied; the fleet number is not written into it
	assert.Equal(t, "Alpha", fleet.BaseName)
	assert.Equal(t, "Alpha", fleet.Name)
	assert.NotContains(t, fleet.Name, "#")
	// the number is preserved as the fleet's unique identity
	assert.Equal(t, 1, fleet.Num)
}

func TestFleet_Rename_invalidLeavesFleetUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		renameTo string
	}{
		{"empty name", ""},
		{"whitespace only name", "  "},
		{"too long name", strings.Repeat("x", MaxFleetNameLength+1)},
		{"name with illegal character", "bad\nname"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := newRenameTestPlayer()
			fleet := newRenameTestFleet(player, 3, "Scout")
			before := fleetSnapshot(t, fleet)

			assert.Error(t, fleet.Rename(tt.renameTo))

			// a rejected rename leaves the fleet field-for-field identical
			assert.Equal(t, before, fleetSnapshot(t, fleet))
			assert.Equal(t, "Scout", fleet.BaseName)
			assert.Equal(t, "Scout #3", fleet.Name)
			assert.Equal(t, 3, fleet.Num)
		})
	}
}

func TestFleet_Rename_sameNameIsNoOp(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 2, "Scout")
	assert.NoError(t, fleet.Rename("Scout"))
	before := fleetSnapshot(t, fleet)

	// renaming to the same name succeeds and changes nothing
	assert.NoError(t, fleet.Rename("Scout"))
	assert.Equal(t, before, fleetSnapshot(t, fleet))
}

func TestFleet_Rename_deletedFleetFails(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 1, "Scout")
	fleet.Delete = true
	before := fleetSnapshot(t, fleet)

	assert.Error(t, fleet.Rename("Alpha"))
	assert.Equal(t, before, fleetSnapshot(t, fleet))
}

func TestFleet_Rename_consecutiveRenames(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 4, "Scout")

	for _, name := range []string{"Alpha", "Beta", "Gamma", "Alpha"} {
		assert.NoError(t, fleet.Rename(name))
		assert.Equal(t, name, fleet.BaseName)
		assert.Equal(t, name, fleet.Name)
		// the identity never changes across renames
		assert.Equal(t, 4, fleet.Num)
	}
}

func TestFleet_Rename_multipleFleetsIndependent(t *testing.T) {
	player := newRenameTestPlayer()
	fleet1 := newRenameTestFleet(player, 1, "Scout")
	fleet2 := newRenameTestFleet(player, 2, "Freighter")
	fleets := []*Fleet{fleet1, fleet2}
	before2 := fleetSnapshot(t, fleet2)

	assert.NoError(t, fleet1.Rename("Alpha"))

	// the other fleet is untouched
	assert.Equal(t, before2, fleetSnapshot(t, fleet2))
	// both fleets keep their unique identities
	assert.Equal(t, 1, fleet1.Num)
	assert.Equal(t, 2, fleet2.Num)
	assert.Same(t, fleet1, fleetByNum(fleets, player.Num, 1))
	assert.Same(t, fleet2, fleetByNum(fleets, player.Num, 2))

	// two fleets may share a display name; identity is still unique by Num
	assert.NoError(t, fleet2.Rename("Alpha"))
	assert.Equal(t, "Alpha", fleet1.Name)
	assert.Equal(t, "Alpha", fleet2.Name)
	assert.NotEqual(t, fleet1.Num, fleet2.Num)
	assert.Same(t, fleet1, fleetByNum(fleets, player.Num, 1))
	assert.Same(t, fleet2, fleetByNum(fleets, player.Num, 2))
}

func TestFleet_Rename_failureThenSuccessEqualsCleanSuccess(t *testing.T) {
	player := newRenameTestPlayer()

	// fleet that suffers a rejected rename before a successful one
	fleetA := newRenameTestFleet(player, 1, "Scout")
	assert.Error(t, fleetA.Rename(""))
	assert.Error(t, fleetA.Rename(strings.Repeat("x", MaxFleetNameLength+1)))
	assert.NoError(t, fleetA.Rename("Alpha"))

	// fleet that only ever gets the successful rename
	fleetB := newRenameTestFleet(player, 1, "Scout")
	assert.NoError(t, fleetB.Rename("Alpha"))

	// a failed rename must not pollute a later success
	assert.Equal(t, fleetSnapshot(t, fleetB), fleetSnapshot(t, fleetA))
}

func TestFleet_Rename_deterministicReplay(t *testing.T) {
	player := newRenameTestPlayer()
	renameSequence := []string{"Alpha", "", "Beta", "Beta", "Gamma", strings.Repeat("y", MaxFleetNameLength+1), "Delta"}

	runSequence := func() []string {
		fleets := []*Fleet{
			newRenameTestFleet(player, 1, "Scout"),
			newRenameTestFleet(player, 2, "Freighter"),
		}
		for _, name := range renameSequence {
			for _, fleet := range fleets {
				_ = fleet.Rename(name)
			}
		}
		snapshots := make([]string, len(fleets))
		for i, fleet := range fleets {
			snapshots[i] = fleetSnapshot(t, fleet)
		}
		return snapshots
	}

	// the same input sequence always produces the same observable result
	assert.Equal(t, runSequence(), runSequence())
}

func TestFleet_Rename_repeatedCallsIdempotent(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 1, "Scout")

	assert.NoError(t, fleet.Rename("Alpha"))
	afterFirst := fleetSnapshot(t, fleet)

	// repeating the same rename is a no-op
	assert.NoError(t, fleet.Rename("Alpha"))
	assert.Equal(t, afterFirst, fleetSnapshot(t, fleet))
}

func TestFleet_Rename_doesNotAffectFleetNumbering(t *testing.T) {
	player := newRenameTestPlayer()
	fleet1 := newRenameTestFleet(player, 1, "Scout")
	fleet2 := newRenameTestFleet(player, 2, "Freighter")
	fleet3 := newRenameTestFleet(player, 3, "Tanker")
	fleets := []*Fleet{fleet1, fleet2, fleet3}

	// deleting a fleet frees its number for the next created fleet
	fleets = []*Fleet{fleet1, fleet3}
	assert.Equal(t, 2, player.GetNextFleetNum(fleets))

	// renaming never changes numbering
	assert.NoError(t, fleet1.Rename("Alpha"))
	assert.NoError(t, fleet3.Rename("Beta"))
	assert.Equal(t, 2, player.GetNextFleetNum(fleets))

	// a newly created fleet reuses the freed number; renames don't collide with it
	fleet4 := newRenameTestFleet(player, player.GetNextFleetNum(fleets), "Gamma")
	fleets = append(fleets, fleet4)
	assert.Equal(t, 2, fleet4.Num)
	assert.Equal(t, 4, player.GetNextFleetNum(fleets))
	assert.Same(t, fleet1, fleetByNum(fleets, player.Num, 1))
	assert.Same(t, fleet4, fleetByNum(fleets, player.Num, 2))
	assert.Same(t, fleet3, fleetByNum(fleets, player.Num, 3))
}

func TestFleet_Rename_keepsOrderAndHistoryIdentity(t *testing.T) {
	player := newRenameTestPlayer()
	target := newRenameTestFleet(player, 1, "Scout")
	chaser := newRenameTestFleet(player, 2, "Freighter")

	// orders reference the target fleet by its unique identity
	chaser.Waypoints = append(chaser.Waypoints, NewFleetWaypoint(Vector{10, 10}, target.Num, target.PlayerNum, target.Name, 5))

	// history records reference the target fleet by its unique identity
	historyTarget := PlayerMessageTarget{TargetType: TargetFleet, TargetName: target.Name, TargetPlayerNum: target.PlayerNum, TargetNum: target.Num}

	assert.NoError(t, target.Rename("Alpha"))

	fleets := []*Fleet{target, chaser}
	// orders still resolve to the same fleet after the rename
	wp := chaser.Waypoints[1]
	assert.Same(t, target, fleetByNum(fleets, wp.TargetPlayerNum, wp.TargetNum))
	// history records still resolve to the same fleet after the rename
	assert.Same(t, target, fleetByNum(fleets, historyTarget.TargetPlayerNum, historyTarget.TargetNum))
	// the recorded historical name is a snapshot; the identity is what matches
	assert.Equal(t, "Scout #1", historyTarget.TargetName)
	assert.Equal(t, target.Num, historyTarget.TargetNum)
}

func TestFleet_Rename_stringIdentityStable(t *testing.T) {
	player := newRenameTestPlayer()
	fleet := newRenameTestFleet(player, 7, "Scout")

	// the unique identity of a fleet is its Num, independent of its display name
	assert.Equal(t, "Fleet Scout #7", fmt.Sprintf("%s", fleet))
	assert.NoError(t, fleet.Rename("Alpha"))
	assert.Equal(t, "Fleet Alpha #7", fmt.Sprintf("%s", fleet))
	assert.Equal(t, 7, fleet.Num)
}
