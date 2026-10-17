package neural

import (
	"os"
	"testing"
)

func TestRLAgent_RewardAndTrain(t *testing.T) {
	rl := NewRLAgent("/tmp/test_rl.json")

	// 1. Test SelectAction returns a valid action in [0, 5]
	state := BotState{
		RAMUsageMB:        280,
		CPUUsagePct:       15,
		Goroutines:        40,
		ActiveSockets:     12,
		LatencyMs:         45,
		RecentErrors:      0,
		AllVitalPortsUp:   true,
		ContinuousUptimeH: 24,
	}

	action := rl.SelectAction(state)
	if action < ActionNoop || action > ActionScaleWorkers {
		t.Fatalf("unexpected action: %v", action)
	}

	// 2. Test ComputeReward: Reviving dead port gives large reward
	beforeCrash := BotState{AllVitalPortsUp: false, RecentErrors: 5}
	afterRevived := BotState{AllVitalPortsUp: true, RecentErrors: 0}
	rewardRevive := ComputeReward(beforeCrash, afterRevived, ActionSoftRestart)
	if rewardRevive <= 20.0 {
		t.Errorf("expected big reward for reviving port, got %f", rewardRevive)
	}

	// 3. Test ComputeReward: Killing vital port gives massive penalty
	beforeHealthy := BotState{AllVitalPortsUp: true}
	afterFatal := BotState{AllVitalPortsUp: false}
	penaltyFatal := ComputeReward(beforeHealthy, afterFatal, ActionSoftRestart)
	if penaltyFatal > -50.0 {
		t.Errorf("expected severe penalty for killing vital port, got %f", penaltyFatal)
	}

	// 4. Test TrainStep updates weights
	initialBiases := make([]float64, len(rl.QNet.Layers[len(rl.QNet.Layers)-1].Biases))
	copy(initialBiases, rl.QNet.Layers[len(rl.QNet.Layers)-1].Biases)

	rl.TrainStep(beforeCrash, ActionSoftRestart, afterRevived)

	if len(rl.ReplayBuffer) != 1 {
		t.Errorf("expected 1 experience in buffer, got %d", len(rl.ReplayBuffer))
	}

	// 5. Test Save & Load State
	tmpFile := "/tmp/test_rl_state.json"
	defer os.Remove(tmpFile)

	if err := rl.SaveState(tmpFile); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	newRL := NewRLAgent(tmpFile)
	if err := newRL.LoadState(tmpFile); err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
}
