package neural

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BotAction represents an automated optimization or recovery action taken on a bot.
type BotAction int

const (
	ActionNoop BotAction = iota
	ActionSoftRestart
	ActionTuneTCP
	ActionForceGC
	ActionFlushSwap
	ActionScaleWorkers
)

func (a BotAction) String() string {
	switch a {
	case ActionNoop:
		return "NOOP"
	case ActionSoftRestart:
		return "SOFT_RESTART"
	case ActionTuneTCP:
		return "TUNE_TCP_SOCKET"
	case ActionForceGC:
		return "FORCE_GC"
	case ActionFlushSwap:
		return "FLUSH_SWAP"
	case ActionScaleWorkers:
		return "SCALE_WORKERS"
	default:
		return "UNKNOWN"
	}
}

// BotState encodes live telemetry from the VPS and gaming bots into a normalized state vector.
type BotState struct {
	RAMUsageMB        float64 `json:"ram_usage_mb"`
	CPUUsagePct       float64 `json:"cpu_usage_pct"`
	Goroutines        float64 `json:"goroutines"`
	ActiveSockets     float64 `json:"active_sockets"`
	LatencyMs         float64 `json:"latency_ms"`
	RecentErrors      float64 `json:"recent_errors"`
	AllVitalPortsUp   bool    `json:"all_vital_ports_up"`
	ContinuousUptimeH float64 `json:"continuous_uptime_h"`
}

// ToVector normalizes the state into 8 float64 values in [0.0, 1.0].
func (s *BotState) ToVector() []float64 {
	vitalUp := 0.0
	if s.AllVitalPortsUp {
		vitalUp = 1.0
	}
	return []float64{
		math.Min(s.RAMUsageMB/1000.0, 1.0),
		math.Min(s.CPUUsagePct/100.0, 1.0),
		math.Min(s.Goroutines/1000.0, 1.0),
		math.Min(s.ActiveSockets/200.0, 1.0),
		math.Min(s.LatencyMs/500.0, 1.0),
		math.Min(s.RecentErrors/20.0, 1.0),
		vitalUp,
		math.Min(s.ContinuousUptimeH/100.0, 1.0),
	}
}

// Experience stores a transition tuple (s, a, r, s') for online reinforcement learning.
type Experience struct {
	State     BotState  `json:"state"`
	Action    BotAction `json:"action"`
	Reward    float64   `json:"reward"`
	NextState BotState  `json:"next_state"`
	Timestamp time.Time `json:"timestamp"`
}

// RLAgent implements an online reinforcement learning policy in pure Go with safe bounds.
type RLAgent struct {
	mu            sync.RWMutex
	QNet          *NeuralNetwork // 8 inputs -> 16 hidden -> 6 output Q-values
	Gamma         float64        // Discount factor (e.g. 0.95)
	Alpha         float64        // Learning rate (e.g. 0.01)
	Epsilon       float64        // Exploration rate (e.g. 0.10)
	ReplayBuffer  []Experience
	MaxBufferSize int
	TotalReward   float64
	StoragePath   string
}

// NewRLAgent initializes the reinforcement learning agent with 8 inputs and 6 actions.
func NewRLAgent(storagePath string) *RLAgent {
	qnet, _ := NewNetwork([]int{8, 16, 6}, []string{"relu", "linear"})
	return &RLAgent{
		QNet:          qnet,
		Gamma:         0.95,
		Alpha:         0.01,
		Epsilon:       0.10,
		ReplayBuffer:  make([]Experience, 0, 500),
		MaxBufferSize: 500,
		StoragePath:   storagePath,
	}
}

// SelectAction chooses an action using an epsilon-greedy policy.
func (rl *RLAgent) SelectAction(state BotState) BotAction {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	if rng.Float64() < rl.Epsilon {
		// Explore
		return BotAction(rng.Intn(6))
	}

	// Exploit: argmax Q(s, a)
	vec := state.ToVector()
	qValues := rl.QNet.Predict(vec)
	if len(qValues) < 6 {
		return ActionNoop
	}

	bestAction := 0
	bestQ := qValues[0]
	for a := 1; a < len(qValues); a++ {
		if qValues[a] > bestQ {
			bestQ = qValues[a]
			bestAction = a
		}
	}
	return BotAction(bestAction)
}

// ComputeReward calculates the objective reward from live telemetry before and after an action.
func ComputeReward(before, after BotState, action BotAction) float64 {
	reward := 0.0

	// 1. Critical Invariant: If vital ports were lost, severe penalty!
	if before.AllVitalPortsUp && !after.AllVitalPortsUp {
		return -100.0
	}
	if !before.AllVitalPortsUp && after.AllVitalPortsUp {
		reward += 40.0 // Successfully revived dead port!
	}

	// 2. Latency improvement
	latDiff := before.LatencyMs - after.LatencyMs
	if latDiff > 5.0 {
		reward += math.Min(latDiff*0.2, 20.0) // Lower latency is rewarded
	} else if latDiff < -20.0 {
		reward -= 10.0 // Increased latency penalized
	}

	// 3. Memory improvement (freed RAM without crashing)
	ramDiff := before.RAMUsageMB - after.RAMUsageMB
	if ramDiff > 10.0 {
		reward += math.Min(ramDiff*0.5, 25.0)
	}

	// 4. Error reduction
	errDiff := before.RecentErrors - after.RecentErrors
	if errDiff > 0 {
		reward += errDiff * 5.0
	} else if errDiff < 0 {
		reward += errDiff * 5.0
	}

	// 5. Penalize excessive actions when everything was already healthy
	if before.RecentErrors == 0 && before.LatencyMs < 60 && before.RAMUsageMB < 350 {
		if action != ActionNoop {
			reward -= 2.0 // Unnecessary intervention
		} else {
			reward += 5.0 // Rewarded for letting stable bots run undisturbed
		}
	}

	return reward
}

// TrainStep records an experience and updates Q-network weights with temporal difference learning.
func (rl *RLAgent) TrainStep(before BotState, action BotAction, after BotState) float64 {
	reward := ComputeReward(before, after, action)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.TotalReward += reward

	// Store in replay buffer
	exp := Experience{
		State:     before,
		Action:    action,
		Reward:    reward,
		NextState: after,
		Timestamp: time.Now(),
	}
	rl.ReplayBuffer = append(rl.ReplayBuffer, exp)
	if len(rl.ReplayBuffer) > rl.MaxBufferSize {
		rl.ReplayBuffer = rl.ReplayBuffer[1:]
	}

	// TD Update: Q(s, a) <- Q(s, a) + alpha * [r + gamma * max_a' Q(s', a') - Q(s, a)]
	sVec := before.ToVector()
	sNextVec := after.ToVector()

	currentQ := rl.QNet.Predict(sVec)
	nextQ := rl.QNet.Predict(sNextVec)

	maxNextQ := nextQ[0]
	for _, q := range nextQ[1:] {
		if q > maxNextQ {
			maxNextQ = q
		}
	}

	target := reward + rl.Gamma*maxNextQ
	tdError := target - currentQ[int(action)]

	// Bounded gradient update on the active layer
	if len(rl.QNet.Layers) > 0 {
		lastLayer := &rl.QNet.Layers[len(rl.QNet.Layers)-1]
		aIdx := int(action)
		if aIdx < len(lastLayer.Biases) {
			lastLayer.Biases[aIdx] += rl.Alpha * tdError
			for inIdx := range lastLayer.Weights[aIdx] {
				lastLayer.Weights[aIdx][inIdx] += rl.Alpha * tdError * 0.1
			}
		}
	}

	return reward
}

// SaveState persists the RL network weights and statistics to disk.
func (rl *RLAgent) SaveState(path string) error {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(rl.QNet, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal qnet: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// LoadState loads RL network weights from disk.
func (rl *RLAgent) LoadState(path string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var loaded NeuralNetwork
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	rl.QNet = &loaded
	return nil
}
