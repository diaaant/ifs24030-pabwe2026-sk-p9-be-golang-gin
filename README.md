# Delcom Blog — Backend (Golang Gin)

REST API blog multi-tier dengan AI Blog Generator terintegrasi Search Engine.
Clean Architecture + Feature-First, 100% statement coverage.

## Menjalankan

```bash
psql -U postgres -c "CREATE DATABASE pabwe_p9_blog;"
go mod tidy                  # membuat go.sum
go run cmd/server/main.go    # http://localhost:8081
```

Swagger UI: http://localhost:8081/swagger/index.html — Spesifikasi: `/api.json`

Akun bawaan (seeder): `admin/admin` (admin), `member/member` (member).

## Pengujian

```bash
go test -v -cover ./internal/...
```

## Build

```bash
go build -o server.exe cmd/server/main.go   # Windows
go build -o server cmd/server/main.go       # Linux/macOS
```

Biner membaca `.env` dari direktori aktif.

## Asumsi integrasi

- **Search Engine** (`SEARCH_ENGINE_ENDPOINT`): `GET {endpoint}?q=<topik>` dengan header
  `Authorization: Bearer <OPENAI_KEY>`; respons JSON `{"results":[{title,url,snippet}]}`
  atau `{"data":{"results":[...]}}`. Field `link/content/description` juga dikenali.
- **OpenAI** (`OPENAI_ENDPOINT`): base URL; dipanggil `POST {endpoint}/chat/completions`.
