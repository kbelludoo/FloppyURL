# Experimento de port parcial do agente FloppyURL

**Objetivo:** comparar a sobrecarga da máquina de estados de `pkg/agent/agent.go` em Go e Rust.

**Escopo real do port:** construir a lista de mensagens (system + history + user), solicitar uma resposta à interface simulada do modelo, executar uma ferramenta simulada, retornar resposta final, limitar histórico aos últimos 40 registros e respeitar limite de 20 rodadas. A implementação original em Go continua intacta. Três testes verificam equivalência básica.

**Fora do escopo:** modelos reais, chamadas HTTP, APIs, execução real de ferramentas, outros módulos, tratamento concorrente, Telegram, navegador, RAM de processos externos e latência de rede. As duas versões do benchmark usam implementações correspondentes e **não executam a implementação original completa do agente**.

## Resultados reproduzidos — GitHub Actions, 21/10/2026

Runs verificados:

- [Compilação e benchmark, incluindo 1 CPU](https://github.com/kbelludoo/FloppyURL/actions/runs/39686228578) — sucesso.
- [Primeiro benchmark](https://github.com/kbelludoo/FloppyURL/actions/runs/39685974362) — sucesso.

Linux x86-64, Go 1.26.9, Rust 1.99.x. As duas versões compilaram e passaram seus testes (três testes Rust e testes Go). Ambas retornaram o mesmo checksum. O código Go e o Rust são versões experimentais correspondentes, não o agente real rodando a IA.

**Um núcleo de CPU, 1.000.000 de turnos, três repetições:** ambos os processos fixados no mesmo núcleo via `taskset`; `GOMAXPROCS=1` na versão Go.

| Rodada | Go tempo | Rust tempo | Go pico RSS | Rust pico RSS |
|---|---:|---:|---:|---:|
| 1 | 0,85 s | 1,71 s | 8.600 KB | 2.164 KB |
| 2 | 0,74 s | 1,73 s | 8.592 KB | 2.160 KB |
| 3 | 0,74 s | 1,72 s | 10.640 KB | 2.240 KB |
| **Mediana** | **0,74 s** | **1,72 s** | **8.600 KB** | **2.164 KB** |

**Conclusão deste teste sintético:** Rust apresentou cerca de **4 vezes menos RAM**, mas demorou aproximadamente **2,3 vezes mais** que Go no mesmo núcleo. Tamanho dos executáveis de teste: Go ~2,4 MB; Rust ~2,1 MB. Os números são do **processo experimental**, não de uma versão completa do FloppyURL.

**Benchmark anterior não limitado a 1 núcleo:** em 250.000 turnos, Go executou em 0,15–0,16 s com RSS de ~8,6–10,8 MB; Rust levou 0,43–0,44 s com RSS de ~2,0–2,2 MB. A versão Go chegou a >100% de CPU, por isso o teste isolado em um núcleo é mais apropriado para uma VPS de 1 vCPU.

**Limitações:** chamadas à IA e ferramentas reais foram substituídas por funções determinísticas; não há custos de serialização JSON, rede, integrações, processos externos ou efeitos da concorrência. A CPU do GitHub Actions e as configurações diferem das da VPS Oracle. Um agente real pode ter diferenças muito menores de latência ou padrões completamente diferentes de RAM. Nenhuma migração de produção foi concluída.

Referência Go: [pkg/agent/agent.go](../../pkg/agent/agent.go).
