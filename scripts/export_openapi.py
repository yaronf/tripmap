#!/usr/bin/env python3
"""Export ChatGPT-friendly OpenAPI YAML from internal/httpserver/openapi.go."""
from pathlib import Path
import sys

endpoint = sys.argv[1] if len(sys.argv) > 1 else "localhost:8080"
base = endpoint if endpoint.startswith("http") else f"https://{endpoint}"

root = Path(__file__).resolve().parents[1]
p = root / "internal/httpserver/openapi.go"
text = p.read_text()
marker = "const openAPIDoc = `"
start = text.index(marker) + len(marker)
end = text.index("`", start)
doc = text[start:end].replace("{{BASE_URL}}", base)

out_dir = root / "tmp"
out_dir.mkdir(exist_ok=True)
out = out_dir / "openapi-chatgpt.yaml"
out.write_text(doc)

assert "openapi: 3.1.0" in doc
assert "#/components/parameters" not in doc
assert "schemas:" in doc
assert "Idempotency-Key" in doc
print(out)
for line in doc.splitlines()[:12]:
    print(line)
print("OK")
