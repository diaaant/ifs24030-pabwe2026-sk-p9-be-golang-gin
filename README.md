# Delcom Blog — Backend (Golang Gin)

REST API blog multi-tier dengan AI Blog Generator terintegrasi Search Engine.
Clean Architecture + Feature-First, 100% statement coverage.

## Menjalankan

```bash
psql -U postgres -c "CREATE DATABASE pabwe_p9_blog;"
go mod tidy                  # membuat go.sum
go run cmd/server/main.go    # http://localhost:8080
```

Swagger UI: http://localhost:8080/swagger/index.html — Spesifikasi: `/api.json`

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

## Deploy (variabel lingkungan platform)

Biner membaca variabel dari lingkungan (berkas `.env` tidak ikut ke server):
`APP_PORT` atau `PORT` (default 8080), `JWT_SECRET` atau `APP_KEY`, `DB_*`, `OPENAI_*`,
`SEARCH_ENGINE_ENDPOINT`. Database target dibuat otomatis bila belum ada.

## Pilihan database (PostgreSQL atau MySQL/MariaDB)

Driver dipilih dari `DB_CONNECTION`: `pgsql` (default) atau `mysql`/`mariadb`.
Variabel lain sama: `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD`.
Untuk database terkelola (MySQL) platform menyediakan databasenya; backend hanya membuat tabel
dan akun bawaan secara otomatis saat start.
