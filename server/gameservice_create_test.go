package server

import (
	"testing"

	"github.com/sirgwain/craig-stars/cs"
	"github.com/sirgwain/craig-stars/proto/converter"
	craig_starsv1 "github.com/sirgwain/craig-stars/proto/gen/craig_stars/v1"
	"github.com/stretchr/testify/assert"
)

// games hosted through the CreateGame service interface must end up with
// victory conditions, whether or not the request specifies them
func Test_gameService_CreateGame_VictoryConditions(t *testing.T) {

	t.Run("request without victory conditions gets the defaults", func(t *testing.T) {
		gr := createTestGameRunner(t.Context())

		// convert the proto request the same way the CreateGame handler does
		settings := converter.C.ConvertGameSettings(&craig_starsv1.GameSettings{
			Name:            "api created game",
			Size:            craig_starsv1.Size_SIZE_SMALL,
			Density:         craig_starsv1.Density_DENSITY_NORMAL,
			PlayerPositions: craig_starsv1.PlayerPositions_PLAYER_POSITIONS_MODERATE,
		})
		settings.WithHost(cs.Humanoids()).WithAIPlayer(cs.AIDifficultyNormal, 0)

		fullGame, err := gr.HostGame(1, &settings)
		if err != nil {
			t.Fatalf("host game returned error \n%v", err)
		}

		assert.NotZero(t, fullGame.VictoryConditions.Conditions)
		assert.Equal(t, cs.NewGameSettings().VictoryConditions, fullGame.VictoryConditions)

		// the game should advance turns without error
		if _, err := gr.GenerateTurn(fullGame.ID); err != nil {
			t.Errorf("GenerateTurn failed: \n%v", err)
		}
	})

	t.Run("request with victory conditions applies them", func(t *testing.T) {
		gr := createTestGameRunner(t.Context())

		settings := converter.C.ConvertGameSettings(&craig_starsv1.GameSettings{
			Name:            "api created game with conditions",
			Size:            craig_starsv1.Size_SIZE_SMALL,
			Density:         craig_starsv1.Density_DENSITY_NORMAL,
			PlayerPositions: craig_starsv1.PlayerPositions_PLAYER_POSITIONS_MODERATE,
			VictoryConditions: &craig_starsv1.VictoryConditions{
				Conditions:          uint32(cs.VictoryConditionOwnPlanets | cs.VictoryConditionExceedsScore),
				NumCriteriaRequired: 2,
				OwnPlanets:          45,
				ExceedsScore:        12000,
			},
		})
		settings.WithHost(cs.Humanoids()).WithAIPlayer(cs.AIDifficultyNormal, 0)

		fullGame, err := gr.HostGame(1, &settings)
		if err != nil {
			t.Fatalf("host game returned error \n%v", err)
		}

		assert.Equal(t, cs.Bitmask(cs.VictoryConditionOwnPlanets)|cs.Bitmask(cs.VictoryConditionExceedsScore), fullGame.VictoryConditions.Conditions)
		assert.Equal(t, 2, fullGame.VictoryConditions.NumCriteriaRequired)
		assert.Equal(t, 45, fullGame.VictoryConditions.OwnPlanets)
		assert.Equal(t, 12000, fullGame.VictoryConditions.ExceedsScore)
	})
}
