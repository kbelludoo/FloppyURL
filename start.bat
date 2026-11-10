@echo off
echo ============================================================
echo   INICIANDO AGENTE IA EM GO (NVIDIA NIM / KIMI-K3)
echo ============================================================

if not exist .env (
    echo [ERRO] Arquivo .env nao encontrado!
    pause
    exit /b 1
)

for /f "usebackq tokens=1,* delims==" %%A in (.env) do (
    if not "%%A"=="" if not "%%A:~0,1%"=="#" (
        set "%%A=%%B"
    )
)

agent-go.exe -serve 8888 -web-dir Website -agent -router
pause
