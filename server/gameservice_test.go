package server

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/sirgwain/craig-stars/config"
	"github.com/sirgwain/craig-stars/cs"
	"github.com/sirgwain/craig-stars/db"
	"github.com/sirgwain/craig-stars/proto/converter"
	craig_starsv1 "github.com/sirgwain/craig-stars/proto/gen/craig_stars/v1"
	"github.com/sirgwain/craig-stars/proto/gen/craig_stars/v1/craig_starsv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestGameService(ctx context.Context) (craig_starsv1connect.GameServiceHandler, context.Context) {
	dbConn := db.NewConn()
	cfg := &config.Config{}
	cfg.Database.Filename = ":memory:"
	if err := dbConn.Connect(ctx, cfg); err != nil {
		panic(fmt.Errorf("error connecting to test database: \n%w", err))
	}

	ctx = context.WithValue(ctx, keyDbRead, dbConn.NewReadClient())
	ctx = context.WithValue(ctx, keyDbWrite, dbConn.NewReadWriteClient())
	ctx = context.WithValue(ctx, keyUserSession, userSession{ID: 1, Username: "test", Role: string(cs.RoleUser)})

	return NewGameServiceHandler(dbConn, *cfg, nil), ctx
}

func testGameSettingsProto(victoryConditions *craig_starsv1.VictoryConditions) *craig_starsv1.GameSettings {
	return &craig_starsv1.GameSettings{
		Name:              "Test Game",
		Size:              craig_starsv1.Size_SIZE_SMALL,
		Density:           craig_starsv1.Density_DENSITY_NORMAL,
		PlayerPositions:   craig_starsv1.PlayerPositions_PLAYER_POSITIONS_MODERATE,
		RandomEvents:      true,
		VictoryConditions: victoryConditions,
		Players: []*craig_starsv1.NewGamePlayer{
			{
				Type:  craig_starsv1.NewGamePlayerType_NEW_GAME_PLAYER_TYPE_HOST,
				Race:  converter.C.ConvertCSRace(cs.Humanoids()),
				Color: "#0000FF",
			},
			{
				Type:         craig_starsv1.NewGamePlayerType_NEW_GAME_PLAYER_TYPE_AI,
				AiDifficulty: craig_starsv1.AiDifficulty_AI_DIFFICULTY_NORMAL,
			},
		},
	}
}

// games created through the CreateGame API without victory conditions
// should get the same default victory conditions as any other new game
func Test_gameService_CreateGame_defaultVictoryConditions(t *testing.T) {
	svc, ctx := createTestGameService(t.Context())

	settings := testGameSettingsProto(nil)
	resp, err := svc.CreateGame(ctx, connect.NewRequest(&craig_starsv1.CreateGameRequest{Settings: settings}))
	require.NoError(t, err)

	got := resp.Msg.Game.Game.VictoryConditions
	require.NotNil(t, got)

	// should match the defaults used by every other game creation entry point
	want := cs.NewGameSettings().VictoryConditions
	assert.Equal(t, int32(want.Conditions), int32(got.Conditions))
	assert.Equal(t, int32(want.NumCriteriaRequired), got.NumCriteriaRequired)
	assert.Equal(t, int32(want.YearsPassed), got.YearsPassed)
	assert.Equal(t, int32(want.OwnPlanets), got.OwnPlanets)
	assert.Equal(t, int32(want.AttainTechLevel), got.AttainTechLevel)
	assert.Equal(t, int32(want.AttainTechLevelNumFields), got.AttainTechLevelNumFields)
	assert.Equal(t, int32(want.ExceedsScore), got.ExceedsScore)
	assert.Equal(t, int32(want.ExceedsSecondPlaceScore), got.ExceedsSecondPlaceScore)
	assert.Equal(t, int32(want.ProductionCapacity), got.ProductionCapacity)
	assert.Equal(t, int32(want.OwnCapitalShips), got.OwnCapitalShips)
	assert.Equal(t, int32(want.HighestScoreAfterYears), got.HighestScoreAfterYears)

	// the persisted game should have the same victory conditions as a game
	// created through the Gamer entry point with default settings
	dbGame, err := contextDb(ctx).GetGame(ctx, resp.Msg.Game.Game.Id)
	require.NoError(t, err)
	gamerGame := cs.NewGamer().CreateGame(1, *cs.NewGameSettings())
	assert.Equal(t, gamerGame.VictoryConditions, dbGame.VictoryConditions)
	assert.NotEqual(t, cs.Bitmask(0), dbGame.VictoryConditions.Conditions)
}

// games created through the CreateGame API with explicit victory conditions
// should keep them
func Test_gameService_CreateGame_customVictoryConditions(t *testing.T) {
	svc, ctx := createTestGameService(t.Context())

	victoryConditions := &craig_starsv1.VictoryConditions{
		Conditions: uint32(cs.Bitmask(cs.VictoryConditionOwnPlanets) |
			cs.Bitmask(cs.VictoryConditionExceedsScore) |
			cs.Bitmask(cs.VictoryConditionProductionCapacity)),
		NumCriteriaRequired: 2,
		YearsPassed:         60,
		OwnPlanets:          50,
		ExceedsScore:        15000,
		ProductionCapacity:  200,
	}
	settings := testGameSettingsProto(victoryConditions)
	resp, err := svc.CreateGame(ctx, connect.NewRequest(&craig_starsv1.CreateGameRequest{Settings: settings}))
	require.NoError(t, err)

	got := resp.Msg.Game.Game.VictoryConditions
	require.NotNil(t, got)
	assert.Equal(t, victoryConditions.Conditions, got.Conditions)
	assert.Equal(t, victoryConditions.NumCriteriaRequired, got.NumCriteriaRequired)
	assert.Equal(t, victoryConditions.YearsPassed, got.YearsPassed)
	assert.Equal(t, victoryConditions.OwnPlanets, got.OwnPlanets)
	assert.Equal(t, victoryConditions.ExceedsScore, got.ExceedsScore)
	assert.Equal(t, victoryConditions.ProductionCapacity, got.ProductionCapacity)

	// the persisted game should keep the custom victory conditions
	dbGame, err := contextDb(ctx).GetGame(ctx, resp.Msg.Game.Game.Id)
	require.NoError(t, err)
	assert.Equal(t, cs.Bitmask(victoryConditions.Conditions), dbGame.VictoryConditions.Conditions)
	assert.Equal(t, int(victoryConditions.NumCriteriaRequired), dbGame.VictoryConditions.NumCriteriaRequired)
}
