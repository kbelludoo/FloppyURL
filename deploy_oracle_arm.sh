#!/bin/bash
# ================================================================
# POCKETWEB - DEPLOY AUTOMATICO ORACLE CLOUD ARM64
# Execute como: bash deploy_oracle_arm.sh
# ================================================================
set -e
echo "================================================"
echo "  PocketWeb Deploy - Oracle Cloud ARM64"
echo "================================================"

# 1. Cria o diretório de trabalho
mkdir -p /opt/pocketweb
cd /opt/pocketweb

# 2. Baixa o binário ARM64 pré-compilado diretamente do GitHub
echo "📦 Baixando servidor PocketWeb ARM64..."
curl -fL "https://github.com/kbelludoo/FloppyURL/releases/latest/download/pocketweb_server_arm64" -o pocketweb_server 2>/dev/null || {
  echo "⚙️  Compilando do código-fonte (Go necessário)..."
  curl -fL https://raw.githubusercontent.com/kbelludoo/FloppyURL/main/colab_server.go -o colab_server.go
  if ! command -v go &>/dev/null; then
    echo "📦 Instalando Go..."
    curl -fL https://go.dev/dl/go1.25.0.linux-arm64.tar.gz | tar -xz -C /usr/local
    export PATH=$PATH:/usr/local/go/bin
  fi
  go get github.com/tdewolff/minify/v2@v2.20.19 2>/dev/null || true
  go build -ldflags="-s -w" -o pocketweb_server colab_server.go
}
chmod +x pocketweb_server

# 3. Cria serviço systemd para iniciar automaticamente
echo "⚙️  Configurando serviço systemd..."
cat > /etc/systemd/system/pocketweb.service << 'EOF'
[Unit]
Description=PocketWeb Server - Ultra-Light Web Packager
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/opt/pocketweb
ExecStart=/opt/pocketweb/pocketweb_server
Restart=always
RestartSec=5
Environment=PORT=8080

[Install]
WantedBy=multi-user.target
EOF

# 4. Instala e instala o Cloudflare Tunnel
echo "🌐 Instalando cloudflared..."
if ! command -v cloudflared &>/dev/null; then
  curl -fL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-arm64 -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi

# 5. Cria serviço systemd para o Cloudflare Tunnel
cat > /etc/systemd/system/cloudflared-pocketweb.service << 'EOF'
[Unit]
Description=Cloudflare Tunnel - PocketWeb
After=network.target pocketweb.service

[Service]
Type=simple
User=nobody
ExecStart=/usr/local/bin/cloudflared tunnel run --token COLE_SEU_TOKEN_AQUI
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# 6. Ativa tudo
systemctl daemon-reload
systemctl enable pocketweb
systemctl start pocketweb
sleep 2

echo ""
echo "================================================"
echo "  ✅ POCKETWEB INSTALADO COM SUCESSO!"
echo "================================================"
echo ""
echo "Status do servidor:"
systemctl status pocketweb --no-pager | head -5
echo ""
echo "Teste rápido de saúde:"
curl -s http://localhost:8080/health && echo ""
echo ""
echo "PRÓXIMO PASSO:"
echo "  Edite o arquivo /etc/systemd/system/cloudflared-pocketweb.service"
echo "  e substitua COLE_SEU_TOKEN_AQUI pelo seu token da Cloudflare:"
echo ""
echo "  sudo nano /etc/systemd/system/cloudflared-pocketweb.service"
echo ""
echo "  Depois execute:"
echo "  sudo systemctl enable cloudflared-pocketweb"
echo "  sudo systemctl start cloudflared-pocketweb"
