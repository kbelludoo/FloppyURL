# Script de Deploy do Agent Go para VPS (PowerShell)
param(
    [Parameter(Mandatory=$false)]
    [string]$VpsHost = "ubuntu@SEU_IP_VPS"
)

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "🚀 Compilando e Enviando Agent Go para a VPS..." -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

# 1. Compilar para Linux
Write-Host "[1/3] Compilando agent-go-linux..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o agent-go-linux .

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Erro na compilação do agent-go-linux!" -ForegroundColor Red
    exit 1
}

# 2. Upload para a VPS
Write-Host "[2/3] Enviando binário e .env para $VpsHost..." -ForegroundColor Yellow
scp agent-go-linux .env "${VpsHost}:/home/ubuntu/agent-go/"
scp scripts/agent-go.service "${VpsHost}:/home/ubuntu/agent-go/"

# 3. Reiniciar serviço na VPS
Write-Host "[3/3] Reiniciando serviço na VPS..." -ForegroundColor Yellow
ssh $VpsHost "sudo cp /home/ubuntu/agent-go/agent-go.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl restart agent-go && sudo systemctl status agent-go --no-pager"

Write-Host "==========================================================" -ForegroundColor Green
Write-Host "✅ Deploy Concluído com Sucesso!" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Green
