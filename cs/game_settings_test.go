//go:build !wasi && !wasm

package cs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGame_WithSettings_VictoryConditions(t *testing.T) {
	defaults := NewGame().VictoryConditions

	t.Run("settings without victory conditions keep the defaults", func(t *testing.T) {
		game := NewGame().WithSettings(GameSettings{Name: "no conditions"})
		assert.NotZero(t, game.VictoryConditions.Conditions)
		assert.Equal(t, defaults, game.VictoryConditions)
	})

	t.Run("settings with victory conditions override the defaults", func(t *testing.T) {
		conditions := VictoryConditions{
			Conditions:          Bitmask(VictoryConditionExceedsScore) | Bitmask(VictoryConditionOwnCapitalShips),
			NumCriteriaRequired: 2,
			ExceedsScore:        15000,
			OwnCapitalShips:     50,
		}
		settings := NewGameSettings()
		settings.VictoryConditions = conditions
		game := NewGame().WithSettings(*settings)
		assert.Equal(t, conditions, game.VictoryConditions)
	})
}

func TestGamer_CreateGame_VictoryConditions(t *testing.T) {
	gamer := NewGamer()

	t.Run("settings missing victory conditions get the defaults", func(t *testing.T) {
		game := gamer.CreateGame(1, GameSettings{Name: "api game"})
		assert.NotZero(t, game.VictoryConditions.Conditions)
		assert.Equal(t, NewGameSettings().VictoryConditions, game.VictoryConditions)
	})

	t.Run("default settings keep their victory conditions", func(t *testing.T) {
		game := gamer.CreateGame(1, *NewGameSettings())
		assert.Equal(t, NewGameSettings().VictoryConditions, game.VictoryConditions)
	})

	t.Run("custom victory conditions are applied", func(t *testing.T) {
		settings := NewGameSettings()
		settings.VictoryConditions = VictoryConditions{
			Conditions:               Bitmask(VictoryConditionOwnPlanets) | Bitmask(VictoryConditionAttainTechLevels) | Bitmask(VictoryConditionProductionCapacity),
			NumCriteriaRequired:      3,
			OwnPlanets:               40,
			AttainTechLevel:          20,
			AttainTechLevelNumFields: 3,
			ProductionCapacity:       200,
		}
		game := gamer.CreateGame(1, *settings)
		assert.Equal(t, settings.VictoryConditions, game.VictoryConditions)
	})
}
