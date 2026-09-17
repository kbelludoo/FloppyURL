# PocketWeb / FloppyURL Engine - Web Resiliente para Áreas Remotas e Baixa Conectividade

**PocketWeb** é uma plataforma web ultra-leve (Progressive Web App) e um motor de empacotamento desenhado para áreas remotas, comunidades rurais, expedições, zonas de desastre e conexões via satélite ou 2G/3G.

Permite transportar e acessar páginas web completas, guias de sobrevivência, notícias e manuais técnicos contidos diretamente no fragmento de URL (`#`), com descompressão nativa instantânea de **0 bytes adicionais** e armazenamento permanente no aparelho (IndexedDB) para uso **100% offline**.

---

## 🎯 Por que o PocketWeb foi criado?

Em regiões remotas (embarcações, florestas, áreas de mineração, missões humanitárias e zonas com internet via satélite limitada), a conexão é intermitente, cara ou inexistente. 

- **Páginas normais da Web** pesam entre 3 MB e 15 MB e quebram assim que o sinal oscila.
- **O PocketWeb** reduz páginas para meros 5 KB a 50 KB usando algoritmos DEFLATE de alta densidade e minificação AST.
- **Resiliência Total:** Uma vez aberta, a página é salva na memória local do celular ou computador, garantindo que o usuário **nunca mais perca o acesso**, mesmo em modo avião ou sem sinal de operadora.

---

## ✨ Recursos Principais

- ⚡ **Zero-Byte Runtime (Sem WASM Pesado):** Descompressão realizada pela API nativa `DecompressionStream('deflate-raw')` presente diretamente nos motores C++ do Chrome, Safari, Firefox e Edge. Nenhum binário externo precisa ser baixado.
- 📶 **Offline-First com PWA:** Equipado com Service Worker e Web App Manifest. Pode ser instalado na tela inicial do celular como um aplicativo independente.
- 💾 **Biblioteca de Bolso Offline (IndexedDB):** Salve artigos, boletins meteorológicos e manuais com 1 clique para leitura permanente sem internet.
- 🌓 **Modos Solar & Noturno OLED:** Interface de alto contraste para visibilidade sob sol forte em campo e modo noturno com fundo preto puro para economizar bateria em smartphones.
- 📦 **Compactador Integrado no Aparelho:** Crie documentos compactados diretamente no navegador, sem precisar de servidor.
- 📲 **Compartilhamento P2P sem Dados:** Transmita páginas compactadas de um aparelho para outro via Nearby Share, Bluetooth ou QR Code sem gastar 1 byte de plano de dados.
- 🔒 **Integridade Criptográfica (NIST FIPS 180-4 SHA-256):** Verificação via `crypto.subtle` garantindo que o documento não foi corrompido durante a transmissão.

---

## 🚀 Como Usar

### 1. Acesso Online / Demonstração
Acesse diretamente via GitHub Pages:
**[https://kbelludoo.github.io/FloppyURL/](https://kbelludoo.github.io/FloppyURL/)**

### 2. Uso Offline
1. Abra o link uma vez com qualquer conexão (mesmo 2G fraca).
2. O Service Worker armazenará o aplicativo localmente.
3. Agora você pode abrir e descompactar links mesmo em **modo avião** ou no meio da mata.

---

## 🛠️ Ferramenta de Empacotamento em Linha de Comando (Go CLI)

Para transformar sites inteiros ou manuais técnicos em links compactados do PocketWeb:

```bash
# Compilar o CLI
go build -o floppy-pack main.go

# Empacotar um manual ou página HTML
./floppy-pack -file manual_emergencia.html -algo deflate
```

O comando emitirá o link direto no formato:
`https://kbelludoo.github.io/FloppyURL/#v2;deflate;[1/1];<sha256>;<sha256>;<payload>`

---

## 📜 Especificação do Formato de Transporte (v2)

```text
#v2;<algoritmo>;[<parte_atual>/<total_partes>];<sha256_bloco>;<sha256_raiz>;<payload_base64url>
```

- `v2`: Versão do protocolo de dados.
- `<algoritmo>`: `deflate` (nativa de 0-bytes) ou `gzip`.
- `[i/N]`: Índice e total de volumes (para documentos particionados se exceder o limite de URL).
- `<sha256_bloco>`: Hash SHA-256 do bloco Base64 para verificação de integridade antes da descompressão.
- `<payload_base64url>`: Dados comprimidos em Base64 URL-safe (sem padding `=`).
