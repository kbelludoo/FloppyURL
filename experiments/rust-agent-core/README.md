# Experimento de port parcial do agente FloppyURL

**Objetivo:** comparar a sobrecarga da máquina de estados de `pkg/agent/agent.go` em Go e Rust.

**Escopo real do port:** construir a lista de mensagens (system + history + user), solicitar uma resposta à interface simulada do modelo, executar uma ferramenta simulada, retornar resposta final, limitar histórico aos últimos 40 registros e respeitar limite de 20 rodadas. A implementação original em Go continua intacta. Três testes verificam equivalência básica.

**Fora do escopo:** modelos reais, chamadas HTTP, APIs, execução real de ferramentas, outros módulos, tratamento concorrente, Telegram, navegador, RAM de processos externos e latência de rede. As duas versões do benchmark usam implementações correspondentes e **não executam a implementação original completa do agente**.

O workflow compila as duas versões de forma otimizada no mesmo runner GitHub Actions, executa 250.000 solicitações simuladas em cada uma, três vezes, e informa tempo de execução, pico de RSS e tamanho do binário. O checksum de saída precisa coincidir. São números de microbenchmark, **não prova de que o FloppyURL inteiro seria mais rápido em Rust**.

Referência Go: [pkg/agent/agent.go](../../pkg/agent/agent.go).
