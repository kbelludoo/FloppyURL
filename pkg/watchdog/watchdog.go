package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MonitoredBot defines a bot monitored by the Watchdog.
type MonitoredBot struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Port       int    `json:"port"`
	URL        string `json:"url"`
	RestartCmd string `json:"restart_cmd"`
	Service    string `json:"service"`
}

// BotStatus represents the current health state of a bot.
type BotStatus struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Online    bool   `json:"online"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// FleetReport represents the full status report of all bots and system resources.
type FleetReport struct {
	Timestamp  string      `json:"timestamp"`
	TotalBots  int         `json:"total_bots"`
	ActiveBots int         `json:"active_bots"`
	Bots       []BotStatus `json:"bots"`
	RAMUsedMB  int64       `json:"ram_used_mb"`
	RAMTotalMB int64       `json:"ram_total_mb"`
	Uptime     string      `json:"uptime"`
}

// Registry of the 9 monitored bots on the VPS
var BotsRegistry = []MonitoredBot{
	{
		ID:         "huntera",
		Name:       "⚔️ Huntera Idle",
		Port:       3000,
		URL:        "http://127.0.0.1:3000/api/status",
		RestartCmd: "bash /home/ubuntu/huntera-go/start.sh",
		Service:    "",
	},
	{
		ID:         "tibidle",
		Name:       "🛡️ Tibidle Multi-Bot",
		Port:       3001,
		URL:        "http://127.0.0.1:3001/api/status",
		RestartCmd: "sudo systemctl restart tibidle.service",
		Service:    "tibidle.service",
	},
	{
		ID:         "sigma",
		Name:       "⚡ Pokemon Sigma",
		Port:       3002,
		URL:        "http://127.0.0.1:3002/api/status",
		RestartCmd: "sudo systemctl restart sigma-bot.service",
		Service:    "sigma-bot.service",
	},
	{
		ID:         "leveling",
		Name:       "🏹 Leveling Idle 2D",
		Port:       3003,
		URL:        "http://127.0.0.1:3003/api/status",
		RestartCmd: "sudo systemctl restart leveling-bot.service",
		Service:    "leveling-bot.service",
	},
	{
		ID:         "pokeidle",
		Name:       "⚡ PokeIdle.io",
		Port:       3006,
		URL:        "http://127.0.0.1:3006/api/status",
		RestartCmd: "sudo systemctl restart pokeidle.service",
		Service:    "pokeidle.service",
	},
	{
		ID:         "pokeidleworld",
		Name:       "🌍 Poke Idle World",
		Port:       3007,
		URL:        "http://127.0.0.1:3007/api/status",
		RestartCmd: "sudo systemctl restart pokeidleworld.service",
		Service:    "pokeidleworld.service",
	},
	{
		ID:         "kickminer",
		Name:       "🟢 Kick Miner Go",
		Port:       3009,
		URL:        "http://127.0.0.1:3009/api/status",
		RestartCmd: "sudo systemctl restart kick-miner.service",
		Service:    "kick-miner.service",
	},
	{
		ID:         "twitchminer",
		Name:       "🟣 Twitch Miner Go",
		Port:       3010,
		URL:        "http://127.0.0.1:3010/api/status",
		RestartCmd: "sudo systemctl restart twitch-miner.service",
		Service:    "twitch-miner.service",
	},
	{
		ID:         "baiak",
		Name:       "🏰 Baiak Idle Go",
		Port:       8090,
		URL:        "http://127.0.0.1:8090/api/fleet",
		RestartCmd: "sudo systemctl restart baiak-fleet.service",
		Service:    "baiak-fleet.service",
	},
}

var (
	incidentHistory = make(map[string]time.Time)
	incidentMu      sync.Mutex
	probeClient     = &http.Client{Timeout: 3500 * time.Millisecond}
)

type secretsConfig struct {
	TelegramToken  string `json:"telegramToken"`
	TelegramChatID string `json:"telegramChatId"`
}

// GetTelegramConfig retrieves token and chat ID from env or .secrets.json
func GetTelegramConfig() (string, string) {
	token := os.Getenv("TELEGRAM_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token != "" && chatID != "" {
		return token, chatID
	}

	homeDir, _ := os.UserHomeDir()
	candidatePaths := []string{
		".secrets.json",
		filepath.Join(homeDir, ".secrets.json"),
		"/home/ubuntu/.secrets.json",
	}

	for _, p := range candidatePaths {
		if data, err := os.ReadFile(p); err == nil {
			var cfg secretsConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
					return cfg.TelegramToken, cfg.TelegramChatID
				}
			}
		}
	}
	return "", ""
}

// SendTelegram sends an alert message directly to the Telegram chat.
func SendTelegram(text string) bool {
	token, chatID := GetTelegramConfig()
	if token == "" || chatID == "" {
		return false
	}
	payload, err := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodPost, "https://api.telegram.org/bot"+token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// ProbeBot tests if a bot is responding.
func ProbeBot(bot MonitoredBot) (bool, int64, string) {
	start := time.Now()
	resp, err := probeClient.Get(bot.URL)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return false, latency, err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, latency, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	// Baiak custom check
	if bot.ID == "baiak" {
		body, err := io.ReadAll(resp.Body)
		if err == nil {
			var m map[string]any
			if err := json.Unmarshal(body, &m); err == nil {
				// verify if bots map or fleet is present
				if bots, ok := m["bots"].(map[string]any); ok && len(bots) > 0 {
					return true, latency, ""
				}
				if fleet, ok := m["fleet"].(map[string]any); ok && len(fleet) > 0 {
					return true, latency, ""
				}
			}
		}
	}

	return true, latency, ""
}

// GetFleetStatus gathers real-time health data for all bots and VPS RAM.
func GetFleetStatus() FleetReport {
	report := FleetReport{
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		TotalBots: len(BotsRegistry),
		Bots:      make([]BotStatus, 0, len(BotsRegistry)),
	}

	active := 0
	for _, bot := range BotsRegistry {
		online, latency, errStr := ProbeBot(bot)
		if online {
			active++
		}
		report.Bots = append(report.Bots, BotStatus{
			ID:        bot.ID,
			Name:      bot.Name,
			Port:      bot.Port,
			Online:    online,
			LatencyMs: latency,
			Error:     errStr,
		})
	}
	report.ActiveBots = active

	// RAM info
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail int64
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if parts[0] == "MemTotal:" {
					total, _ = strconv.ParseInt(parts[1], 10, 64)
				} else if parts[0] == "MemAvailable:" {
					avail, _ = strconv.ParseInt(parts[1], 10, 64)
				}
			}
		}
		if total > 0 {
			report.RAMTotalMB = total / 1024
			report.RAMUsedMB = (total - avail) / 1024
		}
	}

	if out, err := exec.Command("uptime", "-p").Output(); err == nil {
		report.Uptime = strings.TrimSpace(string(out))
	}

	return report
}

// RestartBot executes the restart command for a specific bot by ID.
func RestartBot(botID string) (bool, string) {
	for _, bot := range BotsRegistry {
		if strings.EqualFold(bot.ID, botID) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", bot.RestartCmd)
			} else {
				cmd = exec.CommandContext(ctx, "bash", "-c", bot.RestartCmd)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				return false, fmt.Sprintf("Erro ao executar restart de %s: %v (%s)", bot.Name, err, string(out))
			}

			time.Sleep(5 * time.Second)
			online, _, probeErr := ProbeBot(bot)
			if online {
				return true, fmt.Sprintf("✅ Bot %s reiniciado com sucesso! Operando na porta %d.", bot.Name, bot.Port)
			}
			return false, fmt.Sprintf("⚠️ Comando de reinício executado para %s, mas porta %d ainda não responde: %s", bot.Name, bot.Port, probeErr)
		}
	}
	return false, fmt.Sprintf("Bot com ID %q não encontrado no registro", botID)
}

// GetBotLogs gets recent journalctl or log output for a bot.
func GetBotLogs(botID string, lines int) string {
	if lines <= 0 {
		lines = 30
	}
	for _, bot := range BotsRegistry {
		if strings.EqualFold(bot.ID, botID) {
			if bot.Service != "" {
				out, err := exec.Command("journalctl", "-u", bot.Service, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
				if err == nil && len(out) > 0 {
					return string(out)
				}
			}
			if bot.ID == "huntera" {
				if data, err := os.ReadFile("/home/ubuntu/huntera-go/nohup.out"); err == nil {
					s := string(data)
					split := strings.Split(s, "\n")
					if len(split) > lines {
						split = split[len(split)-lines:]
					}
					return strings.Join(split, "\n")
				}
			}
			// Fallback: check processes
			out, _ := exec.Command("ps", "aux").CombinedOutput()
			return fmt.Sprintf("(Nenhum log systemd direto encontrado. Processos ativos relacionados):\n%s", string(out))
		}
	}
	return "Bot não encontrado."
}

// DiagnosticianFunc is a callback that uses LLM to diagnose a crash.
type DiagnosticianFunc func(ctx context.Context, bot MonitoredBot, logs string) string

// RunWatchdogCycle runs one single healthcheck pass over all bots, performing auto-heal if needed.
func RunWatchdogCycle(ctx context.Context, diagnose DiagnosticianFunc) {
	for _, bot := range BotsRegistry {
		online, _, errStr := ProbeBot(bot)

		incidentMu.Lock()
		_, hadIncident := incidentHistory[bot.ID]
		incidentMu.Unlock()

		if online {
			if hadIncident {
				incidentMu.Lock()
				delete(incidentHistory, bot.ID)
				incidentMu.Unlock()
				log.Printf("[watchdog] Bot %s normalizou e voltou a responder.", bot.Name)
			}
			continue
		}

		if errStr == "" {
			errStr = "sem resposta"
		}
		log.Printf("[watchdog] ⚠️ ALERTA: %s (Porta %d) FORA DO AR! (%s) Tentando auto-cura...", bot.Name, bot.Port, errStr)

		// 1. Executa auto-cura
		execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(execCtx, "powershell", "-NoProfile", "-Command", bot.RestartCmd)
		} else {
			cmd = exec.CommandContext(execCtx, "bash", "-c", bot.RestartCmd)
		}
		_ = cmd.Run()
		cancel()

		// 2. Aguarda reinicialização
		time.Sleep(6 * time.Second)

		// 3. Reteste
		reOnline, _, _ := ProbeBot(bot)
		if reOnline {
			log.Printf("[watchdog] ✅ Auto-cura bem-sucedida para %s (Porta %d)!", bot.Name, bot.Port)
			msg := fmt.Sprintf(
				"🛠️ *AUTO-CURA CONCLUÍDA NA VPS (Agent Go)*\n\n"+
					"• *Bot:* %s\n"+
					"• *Porta:* `%d`\n"+
					"• *Ação:* Serviço reiniciado automaticamente pelo Watchdog em Go.\n"+
					"• *Status Atual:* ✅ ONLINE e operando normalmente!",
				bot.Name, bot.Port,
			)
			SendTelegram(msg)
			incidentMu.Lock()
			delete(incidentHistory, bot.ID)
			incidentMu.Unlock()
		} else {
			log.Printf("[watchdog] ❌ Auto-cura falhou para %s. Acionando diagnóstico com IA...", bot.Name)

			incidentMu.Lock()
			lastAlert, exists := incidentHistory[bot.ID]
			now := time.Now()
			shouldAlert := !exists || now.Sub(lastAlert) > 10*time.Minute
			if shouldAlert {
				incidentHistory[bot.ID] = now
			}
			incidentMu.Unlock()

			if shouldAlert {
				// Coleta logs
				recentLogs := GetBotLogs(bot.ID, 30)

				// Diagnóstico com IA se disponível
				var aiDiagnostic string
				if diagnose != nil {
					aiDiagnostic = diagnose(ctx, bot, recentLogs)
				}
				if strings.TrimSpace(aiDiagnostic) == "" {
					aiDiagnostic = fmt.Sprintf("A reinicialização rápida falhou. Verifique manualmente: `%s`", bot.RestartCmd)
				}

				if len(aiDiagnostic) > 800 {
					aiDiagnostic = aiDiagnostic[:800] + "..."
				}

				msg := fmt.Sprintf(
					"🚨 *ALERTA CRÍTICO: BOT FORA DO AR!*\n\n"+
						"• *Bot:* %s\n"+
						"• *Porta:* `%d`\n"+
						"• *Tentativa de Auto-Cura:* ❌ Falhou ao reiniciar\n\n"+
						"🤖 *Diagnóstico da IA (Agent Go):*\n%s\n\n"+
						"💡 *Comando manual:* `%s`",
					bot.Name, bot.Port, aiDiagnostic, bot.RestartCmd,
				)
				SendTelegram(msg)
			}
		}
	}
}

// StartBackgroundWatchdog starts the continuous autonomous monitoring loop in a background goroutine.
func StartBackgroundWatchdog(ctx context.Context, interval time.Duration, diagnose DiagnosticianFunc) {
	if interval < 15*time.Second {
		interval = 45 * time.Second
	}
	log.Printf("[watchdog] 🛡️ Watchdog Autônomo ativado! Monitorando %d bots a cada %v (Go Native, <1MB RAM)", len(BotsRegistry), interval)

	go func() {
		// Run initial check after 10s to let server stabilize
		select {
		case <-time.After(10 * time.Second):
			RunWatchdogCycle(ctx, diagnose)
		case <-ctx.Done():
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				RunWatchdogCycle(ctx, diagnose)
			case <-ctx.Done():
				log.Println("[watchdog] Loop do Watchdog finalizado.")
				return
			}
		}
	}()
}
