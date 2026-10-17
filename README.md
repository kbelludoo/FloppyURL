# Agente IA em Go (Ultra-Leve & Autônomo)

Agente autônomo e servidor Web desenvolvido em Go, com consumo mínimo de memória (~1.5 MB de RAM) e integração com IA via **NVIDIA NIM** (Kimi-k3 / Llama-3.3 / Nemotron).

---

## 🚀 Como Iniciar

### No Windows:
Basta dar duplo clique em:
```cmd
start.bat
```
Ou via terminal:
```cmd
agent-go.exe -serve 8888 -web-dir Website -agent -router
```

### No Linux (VPS):
```bash
./agent-go-linux -serve 8888 -web-dir Website -agent -router
```

---

## 📡 Endpoints Disponíveis

- **Interface Web:** `http://localhost:8888`
- **Agente IA (POST):** `http://localhost:8888/agent`
  - Payload JSON: `{"message": "sua mensagem ou prompt"}`
  - Resposta JSON: `{"response": "resposta gerada pela IA", "history": [...]}`
- **Router OpenAI-Compatible:** `http://localhost:8888/v1/chat/completions`

---

## 🛠️ Ferramentas Nativas do Agente (Tools)

O agente possui capacidade autônoma de executar ações no sistema quando solicitado:
- `browser_navigate`: Navega em URLs e extrai conteúdo limpo.
- `bash_exec`: Executa comandos de terminal.
- `read_file` & `write_file`: Lê e cria arquivos no disco.
- `system_status`: Retorna uso de CPU, RAM e disco do servidor.

---

## ⚙️ Configuração (.env)

As chaves e modelos de IA ficam configurados no arquivo `.env`:
- `LLM_BASE_URL`: `https://integrate.api.nvidia.com/v1`
- `LLM_API_KEY`: Chave da NVIDIA NIM
- `LLM_MODEL`: `moonshotai/kimi-k3` (ou `meta/llama-3.3-70b-instruct`)

---

## 🆓 DeepSeek Web grátis (sem API key)

Portado da Cassandra (Python) para Go: o agente conversa com o **chat.deepseek.com**
usando a sessão do seu navegador, sem chave de API. Inclui o solver de
Proof-of-Work (DeepSeekHashV1) rodando o WASM oficial via `wazero` (Go puro).

### 1. Abra o Chrome com debug remoto e faça login

```powershell
& "C:\Program Files\Google\Chrome\Application\chrome.exe" `
  --remote-debugging-port=9222 `
  --user-data-dir="$env:LOCALAPPDATA\deepseek-chrome" `
  --no-first-run "https://chat.deepseek.com/"
```

### 2. Capture a sessão

```cmd
agent-go.exe -deepseek-login
```

A sessão é salva em `session/session.json` (reaproveitada por 24h).

### 3. Use

```cmd
REM Chat no terminal (grátis, sem API key)
agent-go.exe -deepseek-chat

REM Ou via API OpenAI-compatible no router
agent-go.exe -serve 8888 -router
```

No `/v1/chat/completions`, use os modelos `deepseek-web`, `deepseek-web-thinking`,
`deepseek-web-expert` ou `deepseek-web-expert-thinking`.

### Variáveis de ambiente

- `DEEPSEEK_CDP_URL`: endpoint CDP do Chrome (padrão `http://127.0.0.1:9222`)
- `DEEPSEEK_SESSION_FILE`: caminho do `session.json`
- `DEEPSEEK_MIN_REQUEST_INTERVAL`: intervalo mínimo entre requests (padrão `3.0`s)

O cliente falha de forma segura: respeita o intervalo mínimo, não repete após
401/403/429 e abre um disjuntor (circuit breaker) em sinais de CAPTCHA/WAF.
Nunca resolve CAPTCHA automaticamente.

