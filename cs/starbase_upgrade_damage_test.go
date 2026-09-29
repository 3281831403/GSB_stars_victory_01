//go:build !wasi && !wasm

package cs

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupDamagedStarbaseUpgrade builds a player and planet with a (possibly damaged)
// Space Station starbase and a queued upgrade to an improved design
func setupDamagedStarbaseUpgrade(t *testing.T, damage float64, cargo Cargo) (player *Player, planet *Planet, oldDesign, newDesign *ShipDesign) {
	t.Helper()
	player, planet = newTestPlayerPlanet()

	oldDesign = NewShipDesign(player.Num, 2).WithHull(SpaceStation.Name).WithSpec(&rules, player)
	newDesign = NewShipDesign(player.Num, 3).WithHull(SpaceStation.Name).
		WithSlots([]ShipDesignSlot{
			{HullComponent: MassDriver5.Name, HullSlotIndex: 1, Quantity: 1},
		}).WithSpec(&rules, player)
	player.Designs = append(player.Designs, oldDesign, newDesign)

	starbase := newStarbase(player, planet, oldDesign, "Starbase")
	starbase.Spec = ComputeFleetSpec(&rules, player, &starbase)
	if damage > 0 {
		starbase.Tokens[0].QuantityDamaged = 1
		starbase.Tokens[0].Damage = damage
	}
	planet.Starbase = &starbase
	planet.ProductionQueue = []ProductionQueueItem{
		{Type: QueueItemTypeStarbase, Quantity: 1, DesignNum: newDesign.Num, design: newDesign},
	}
	planet.Cargo = cargo
	planet.Spec = ComputePlanetSpec(&rules, player, planet)
	return player, planet, oldDesign, newDesign
}

// snapshotStarbaseUpgradePlanet deep copies the planet and its starbase token
// state into a comparable string for field-by-field equality checks
func snapshotStarbaseUpgradePlanet(t *testing.T, planet *Planet) string {
	t.Helper()
	planetJSON, err := json.Marshal(planet)
	require.NoError(t, err)
	var tokenJSON []byte
	if planet.Starbase != nil {
		tokenJSON, err = json.Marshal(planet.Starbase.Tokens)
		require.NoError(t, err)
	}
	return string(planetJSON) + "|" + string(tokenJSON)
}

func Test_costCalculate_StarbaseRepairCost(t *testing.T) {
	player, _ := newTestPlayerPlanet()
	calc := NewCostCalculator()

	design := NewShipDesign(player.Num, 2).WithHull(SpaceStation.Name).WithSpec(&rules, player)
	fullCost, err := calc.GetDesignCost(&rules, player.TechLevels, player.Race.Spec, design)
	require.NoError(t, err)
	require.NotEqual(t, Cost{}, fullCost)
	armor := float64(design.Spec.Armor)
	require.Greater(t, armor, 0.0)

	// scale the full design cost the same way the docs describe:
	// linear with the fraction of armor damaged, rounded up
	scale := func(fraction float64) Cost {
		return MultiplyCost(fullCost.ToCostFloat64(), fraction).Round(math.Ceil).ToCost().MinZero()
	}

	tests := []struct {
		name    string
		design  *ShipDesign
		damage  float64
		want    Cost
		wantErr bool
	}{
		{name: "no damage", design: design, damage: 0, want: Cost{}},
		{name: "negative damage", design: design, damage: -50, want: Cost{}},
		{name: "quarter damage", design: design, damage: armor * 0.25, want: scale(0.25)},
		{name: "half damage", design: design, damage: armor * 0.5, want: scale(0.5)},
		{name: "full damage", design: design, damage: armor, want: fullCost},
		{name: "overkill damage capped at full cost", design: design, damage: armor * 2, want: fullCost},
		{name: "nil design", design: nil, damage: 100, wantErr: true},
		{name: "unknown hull", design: NewShipDesign(player.Num, 3).WithHull("NoSuchHull"), damage: 100, wantErr: true},
		{name: "zero armor design", design: NewShipDesign(player.Num, 4).WithHull(SpaceStation.Name), damage: 100, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, tt.design, tt.damage)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	// repair cost grows monotonically with damage
	quarter, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, design, armor*0.25)
	require.NoError(t, err)
	half, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, design, armor*0.5)
	require.NoError(t, err)
	assert.Less(t, quarter.Resources, half.Resources)
	assert.Less(t, half.Resources, fullCost.Resources)

	// repeated calls are deterministic
	for i := 0; i < 20; i++ {
		again, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, design, armor*0.5)
		require.NoError(t, err)
		assert.Equal(t, half, again)
	}
}

func Test_production_starbaseUpgradeCostIncludesDamageRepair(t *testing.T) {
	damages := []float64{0, 100, 250, 500, 1000}
	costs := make([]Cost, len(damages))
	var player *Player
	var oldDesign, newDesign *ShipDesign
	for i, damage := range damages {
		p, planet, od, nd := setupDamagedStarbaseUpgrade(t, damage, Cargo{1000, 1000, 1000, 10000})
		player, oldDesign, newDesign = p, od, nd
		producer := newProducer(testLogger, &rules, planet, player)
		cost, err := producer.getItemCost(&rules, player, planet, planet.ProductionQueue[0])
		require.NoError(t, err)
		costs[i] = cost
	}

	// an undamaged starbase pays exactly the classic upgrade cost
	plainUpgradeCost, err := NewCostCalculator().StarbaseUpgradeCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, newDesign)
	require.NoError(t, err)
	assert.Equal(t, plainUpgradeCost, costs[0])

	// damage increases the cost monotonically, and strictly below the cap
	for i := 1; i < len(damages); i++ {
		assert.GreaterOrEqual(t, costs[i].Ironium, costs[i-1].Ironium)
		assert.GreaterOrEqual(t, costs[i].Resources, costs[i-1].Resources)
	}
	assert.Less(t, costs[0].Resources, costs[1].Resources)
	assert.Less(t, costs[1].Resources, costs[2].Resources)
	assert.Less(t, costs[2].Resources, costs[3].Resources)
	// damage beyond 100% of armor is capped
	assert.Equal(t, costs[3], costs[4])

	// the extra charge equals the repair cost of the old design
	repairCost, err := NewCostCalculator().StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, 250)
	require.NoError(t, err)
	assert.Equal(t, plainUpgradeCost.Add(repairCost), costs[2])
}

func Test_production_produceStarbaseUpgradeDamagedSuccess(t *testing.T) {
	damage := 250.0
	player, planet, oldDesign, newDesign := setupDamagedStarbaseUpgrade(t, damage, Cargo{1000, 1000, 1000, 10000})

	calc := NewCostCalculator()
	upgradeCost, err := calc.StarbaseUpgradeCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, newDesign)
	require.NoError(t, err)
	repairCost, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, damage)
	require.NoError(t, err)
	expectedCost := upgradeCost.Add(repairCost)

	producer := newProducer(testLogger, &rules, planet, player)
	result, err := producer.produce()
	require.NoError(t, err)

	// the upgrade was built and the queue is empty
	assert.Equal(t, newDesign, result.starbase)
	assert.Empty(t, planet.ProductionQueue)

	// minerals were deducted by exactly the upgrade + repair cost
	assert.Equal(t, 1000-expectedCost.Ironium, planet.Cargo.Ironium)
	assert.Equal(t, 1000-expectedCost.Boranium, planet.Cargo.Boranium)
	assert.Equal(t, 1000-expectedCost.Germanium, planet.Cargo.Germanium)
	assert.Equal(t, 10000, planet.Cargo.Colonists)
}

func Test_production_produceStarbaseUpgradeDamagedInsufficientResources(t *testing.T) {
	// fully damaged starbase, almost no minerals: the upgrade cannot complete
	player, planet, oldDesign, _ := setupDamagedStarbaseUpgrade(t, 500, Cargo{1, 1, 1, 1})
	planet.Spec.ResourcesPerYearAvailable = 1

	producer := newProducer(testLogger, &rules, planet, player)
	result, err := producer.produce()
	require.NoError(t, err)

	// the upgrade was not built
	assert.Nil(t, result.starbase)

	// the starbase keeps its old design, damage and health
	require.NotNil(t, planet.Starbase)
	assert.Equal(t, oldDesign.Num, planet.Starbase.Tokens[0].DesignNum)
	assert.Equal(t, 500.0, planet.Starbase.Tokens[0].Damage)
	assert.Equal(t, 1, planet.Starbase.Tokens[0].QuantityDamaged)

	// the item stays queued (partially allocated) for a later turn
	require.Len(t, planet.ProductionQueue, 1)
	assert.Equal(t, QueueItemTypeStarbase, planet.ProductionQueue[0].Type)
	assert.Equal(t, 1, planet.ProductionQueue[0].Quantity)
}

func Test_production_produceStarbaseUpgradeErrorRollsBack(t *testing.T) {
	player, planet, _, newDesign := setupDamagedStarbaseUpgrade(t, 250, Cargo{1000, 1000, 1000, 10000})

	// queue a valid mine item ahead of a starbase upgrade with an unknown component,
	// so the failure happens after the planet was already mutated
	badDesign := NewShipDesign(player.Num, 4).WithHull(SpaceStation.Name).
		WithSlots([]ShipDesignSlot{{HullComponent: "NoSuchComponent", HullSlotIndex: 1, Quantity: 1}})
	player.Designs = append(player.Designs, badDesign)
	planet.ProductionQueue = []ProductionQueueItem{
		{Type: QueueItemTypeMine, Quantity: 5},
		{Type: QueueItemTypeStarbase, Quantity: 1, DesignNum: badDesign.Num, design: badDesign},
	}

	before := snapshotStarbaseUpgradePlanet(t, planet)
	producer := newProducer(testLogger, &rules, planet, player)
	result, err := producer.produce()
	require.Error(t, err)

	// the failure rolls back atomically: result is empty and the deep-copied
	// snapshot (cargo, mines, queue, starbase damage) matches field for field
	assert.Equal(t, productionResult{}, result)
	assert.Equal(t, before, snapshotStarbaseUpgradePlanet(t, planet))

	// a failed call must not pollute a later successful call
	planet.ProductionQueue = []ProductionQueueItem{
		{Type: QueueItemTypeStarbase, Quantity: 1, DesignNum: newDesign.Num, design: newDesign},
	}
	retryProducer := newProducer(testLogger, &rules, planet, player)
	retryResult, err := retryProducer.produce()
	require.NoError(t, err)
	assert.Equal(t, newDesign, retryResult.starbase)

	// the result matches a fresh planet that never saw the failed call
	_, freshPlanet, _, freshNewDesign := setupDamagedStarbaseUpgrade(t, 250, Cargo{1000, 1000, 1000, 10000})
	freshProducer := newProducer(testLogger, &rules, freshPlanet, player)
	freshResult, err := freshProducer.produce()
	require.NoError(t, err)
	assert.Equal(t, freshNewDesign, freshResult.starbase)
	assert.Equal(t, snapshotStarbaseUpgradePlanet(t, freshPlanet), snapshotStarbaseUpgradePlanet(t, planet))
}

func Test_production_produceStarbaseUpgradeConsecutive(t *testing.T) {
	player, planet, oldDesign, newDesign := setupDamagedStarbaseUpgrade(t, 100, Cargo{1000, 1000, 1000, 10000})
	newDesign2 := NewShipDesign(player.Num, 4).WithHull(SpaceStation.Name).
		WithSlots([]ShipDesignSlot{
			{HullComponent: MassDriver5.Name, HullSlotIndex: 1, Quantity: 1},
			{HullComponent: MassDriver5.Name, HullSlotIndex: 2, Quantity: 1},
		}).WithSpec(&rules, player)
	player.Designs = append(player.Designs, newDesign2)

	calc := NewCostCalculator()
	upgradeCost1, err := calc.StarbaseUpgradeCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, newDesign)
	require.NoError(t, err)
	repairCost1, err := calc.StarbaseRepairCost(&rules, player.TechLevels, player.Race.Spec, oldDesign, 100)
	require.NoError(t, err)
	turn1Cost := upgradeCost1.Add(repairCost1)

	// turn 1: damaged starbase upgrades to the first new design
	producer1 := newProducer(testLogger, &rules, planet, player)
	result1, err := producer1.produce()
	require.NoError(t, err)
	require.Equal(t, newDesign, result1.starbase)
	assert.Equal(t, 1000-turn1Cost.Ironium, planet.Cargo.Ironium)

	// turn generation swaps in the new starbase; queue the next upgrade
	upgraded := newStarbase(player, planet, newDesign, "Starbase")
	upgraded.Spec = ComputeFleetSpec(&rules, player, &upgraded)
	planet.Starbase = &upgraded
	planet.ProductionQueue = []ProductionQueueItem{
		{Type: QueueItemTypeStarbase, Quantity: 1, DesignNum: newDesign2.Num, design: newDesign2},
	}
	planet.Spec = ComputePlanetSpec(&rules, player, planet)

	// turn 2: the now undamaged starbase upgrades again at the plain upgrade cost
	upgradeCost2, err := calc.StarbaseUpgradeCost(&rules, player.TechLevels, player.Race.Spec, newDesign, newDesign2)
	require.NoError(t, err)
	producer2 := newProducer(testLogger, &rules, planet, player)
	result2, err := producer2.produce()
	require.NoError(t, err)
	require.Equal(t, newDesign2, result2.starbase)

	// resources, health and upgrade level are self-consistent after both turns
	assert.Equal(t, 1000-turn1Cost.Ironium-upgradeCost2.Ironium, planet.Cargo.Ironium)
	assert.Equal(t, 1000-turn1Cost.Boranium-upgradeCost2.Boranium, planet.Cargo.Boranium)
	assert.Equal(t, 1000-turn1Cost.Germanium-upgradeCost2.Germanium, planet.Cargo.Germanium)
	assert.Equal(t, 0, planet.Starbase.Tokens[0].QuantityDamaged)
	assert.Empty(t, planet.ProductionQueue)
}

func Test_production_produceStarbaseUpgradeMultiplePlanetsDeterministic(t *testing.T) {
	player1, planet1, _, newDesign1 := setupDamagedStarbaseUpgrade(t, 150, Cargo{500, 500, 500, 5000})
	player2, planet2, _, newDesign2 := setupDamagedStarbaseUpgrade(t, 150, Cargo{500, 500, 500, 5000})

	// two identical starbases upgrading at the same time produce identical outcomes
	producer1 := newProducer(testLogger, &rules, planet1, player1)
	result1, err := producer1.produce()
	require.NoError(t, err)
	producer2 := newProducer(testLogger, &rules, planet2, player2)
	result2, err := producer2.produce()
	require.NoError(t, err)

	assert.Equal(t, newDesign1.Name, result1.starbase.Name)
	assert.Equal(t, newDesign2.Name, result2.starbase.Name)
	assert.Equal(t, planet1.Cargo, planet2.Cargo)
	assert.Equal(t, snapshotStarbaseUpgradePlanet(t, planet1), snapshotStarbaseUpgradePlanet(t, planet2))
}

func Test_production_produceStarbaseUpgradeReplayDeterministic(t *testing.T) {
	// the same input sequence replayed from scratch always yields the same state
	run := func() string {
		player, planet, _, _ := setupDamagedStarbaseUpgrade(t, 300, Cargo{200, 200, 200, 2000})
		producer := newProducer(testLogger, &rules, planet, player)
		_, err := producer.produce()
		require.NoError(t, err)
		return snapshotStarbaseUpgradePlanet(t, planet)
	}

	first := run()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, run())
	}
}
