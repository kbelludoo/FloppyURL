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

## 🧠 OpenCode via ACP (CLI oficial)

Selecione `OPENCODE_TRANSPORT=acp` para delegar os turnos ao CLI oficial com
[`opencode-agent-sdk-go`](https://github.com/ethpandaops/opencode-agent-sdk-go),
fixado em `v0.0.3`. Requer Go 1.26+ e OpenCode CLI >= 1.14.20. O SDK valida a
versão e inicia `opencode acp` por stdio JSON-RPC; não exige `opencode serve`
aberto nem a interface desktop. O CLI gerencia suas credenciais e a sessão.

Faça login pelo próprio CLI:

```powershell
opencode auth login
```

Adicione ao `.env`, carregado por `start.bat`:

```dotenv
OPENCODE_TRANSPORT=acp
OPENCODE_MODEL=opencode/deepseek-v4.1-flash-free
OPENCODE_TIMEOUT=2m
# Opcional, se o CLI não estiver no PATH:
# OPENCODE_CLI_PATH=C:\caminho\opencode.exe
# Opcional: diretório de trabalho; padrão = diretório do agent-go:
# OPENCODE_CWD=C:\meu-projeto
# Opcional: orçamento observado pelo SDK por turno, em dólares:
# OPENCODE_MAX_BUDGET_USD=0.05
```

Ao executar pelo terminal sem `start.bat`, exporte essas variáveis na sessão;
o executável e o exemplo Go não carregam `.env` automaticamente.

Exemplo de um turno no PowerShell:

```powershell
$env:OPENCODE_TRANSPORT = "acp"
$env:OPENCODE_MODEL = "opencode/deepseek-v4.1-flash-free"
go run ./examples/opencode-acp "Explique canais em Go em uma frase."
```

No gateway `/v1/chat/completions`, use `model: "default"`, um ID
`opencode/<modelo>` ou um ID terminado em `-free`. `/v1/models` inclui o modelo
ACP configurado. A disponibilidade de cada modelo continua sendo decidida
pelo OpenCode e pelo provedor.

Cada chamada inicia um processo e uma sessão próprios e os encerra ao terminar
ou cancelar. O histórico completo é enviado como contexto textual com seus
rótulos de papel; ACP não oferece mensagens de sistema por sessão equivalentes
às de Chat Completions. `/agent` registra as ferramentas Go nativas através do
MCP local autenticado do SDK, e o OpenCode conduz o ciclo de ferramentas.
As regras de permissão existentes do OpenCode são respeitadas; pedidos de
permissão sem callback são recusados pelo SDK.

O gateway aceita conversas textuais. `stream: true` entrega SSE após o turno
ser concluído, sem transmissão de tokens em tempo real. Schemas de ferramentas
OpenAI enviados pelo consumidor (`tools`/`functions`) são recusados; use
`/agent` para as ferramentas implementadas no servidor. Parâmetros de geração
como `temperature` e `max_tokens` não são encaminhados por ACP; configure o
agente e o provedor no OpenCode. O orçamento usa os dados de uso informados
pelo CLI e não garante um teto exato de cobrança.

Erros ACP são devolvidos ao consumidor, sem fallback automático para outro
modelo ou conta. A dependência SDK usa licença GPL-3.0 e inclui SQLite e
OpenTelemetry; esse modo aumenta o tamanho do binário.

### Acesso HTTP direto aos provedores

Sem `OPENCODE_TRANSPORT=acp`, permanece o fluxo HTTP direto abaixo.

O `agent-go` lê o mesmo `opencode.json`/`opencode.jsonc` e `auth.json` que o
OpenCode usa e chama os provedores **diretamente** — não é preciso `opencode
serve` nem abrir a interface do OpenCode.

Arquivos lidos (nesta ordem, o último sobrescreve o anterior):

- `$OPENCODE_CONFIG`, se definido
- `~/.config/opencode/opencode.json` e `opencode.jsonc`
- `opencode.json` / `.opencode/opencode.json` do diretório atual
- `$XDG_DATA_HOME/opencode/auth.json`, `~/.local/share/opencode/auth.json`,
  `~/.opencode/auth.json` (ou `$OPENCODE_AUTH`)

Liste os modelos disponíveis:

```cmd
agent-go.exe -opencode-models
```

Eles aparecem automaticamente em `/v1/models` e no `/v1/chat/completions` pelo
ID `provedor/modelo` (ex.: `codecraft/claude-fable-5.1`). O modelo `default`
usa o modelo padrão configurado no OpenCode. No agente (`/agent`), o modelo
padrão do OpenCode é o primeiro da cadeia de fallback; defina `OPENCODE_MODEL`
para escolher outro.

> O `agent-go` usa as **suas** credenciais/chaves dos provedores configurados
> no OpenCode. O plano gratuito do gateway do OpenCode é restrito ao próprio
> cliente OpenCode e não é contornado por este projeto.

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

