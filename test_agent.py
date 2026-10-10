import urllib.request
import json
import sys

url = "http://localhost:8888/agent"
prompt = "Use a ferramenta browser_navigate com url https://example.com e me diga o que encontrou na pagina."
if len(sys.argv) > 1:
    prompt = sys.argv[1]

payload = json.dumps({"message": prompt}).encode("utf-8")
req = urllib.request.Request(url, data=payload, headers={"Content-Type": "application/json"})

try:
    with urllib.request.urlopen(req, timeout=60) as resp:
        res = json.loads(resp.read().decode("utf-8"))
        print("RESPOSTA DO AGENTE GO:")
        print(res.get("response", res))
except Exception as e:
    print("Erro:", e)
