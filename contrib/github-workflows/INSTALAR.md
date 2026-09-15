# Instalar os workflows do GitHub Actions

O token deste ambiente não tem permissão `workflows`, então os arquivos
ficam aqui. Para ativá-los, mova para `.github/workflows/`:

```bash
mkdir -p .github/workflows
git mv contrib/github-workflows/ci.yml .github/workflows/ci.yml
git mv contrib/github-workflows/deploy-pages.yml .github/workflows/deploy-pages.yml
git commit -m "ci: ativa workflows (ci + deploy-pages)"
git push
```

- `ci.yml` — roda em push/PR: gofmt, go vet, go test, build do kernel WASM,
  verificação dos checksums do Brotli vendorizado e E2E headless
  (inclui cenário negativo de disco corrompido).
- `deploy-pages.yml` — compila o WASM e publica o bootloader no GitHub Pages.
  Requer Settings → Pages → Source: "GitHub Actions".
