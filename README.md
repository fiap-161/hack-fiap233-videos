# hack-fiap233-videos

Microsserviço de vídeos em Go (upload, processamento assíncrono, status e download).

## Arquitetura (Hexagonal / Ports and Adapters)

O código está organizado em **arquitetura hexagonal**: o núcleo (domínio + casos de uso) não depende de detalhes de infraestrutura; os adapters implementam os ports (interfaces).

```
internal/
  domain/          # Entidade Video (núcleo)
  application/      # Ports (VideoRepository, HealthChecker) + VideoService (use cases)
  adapter/
    driver/http/   # Adapter de entrada: handlers HTTP, middleware (X-User-Id)
    driven/postgres/ # Adapter de saída: persistência (VideoRepository, schema)
main.go            # Wiring: db → repo → service → handler; rotas
```

- **Ports:** definidos em `application/ports.go`; o serviço depende apenas deles.
- **Adapters driven:** Postgres implementa `VideoRepository` e `HealthChecker`; no futuro, fila (RabbitMQ) e storage (S3) serão outros adapters driven.
- **Adapter driver:** HTTP traduz request/response e chama o use case; middleware injeta identidade (API Gateway).

### Estrutura de pastas
```
hack-fiap233-videos/
├── main.go                    # Wiring: DB → repo → service → handler; rotas
└── internal/
    ├── domain/
    │   └── video.go           # Entidade Video (núcleo, sem deps externas)
    ├── application/
    │   ├── ports.go            # Ports: VideoRepository, HealthChecker
    │   └── service.go         # Use cases: ListByUser, CreateVideo
    └── adapter/
        ├── driver/
        │   └── http/           # Adapter de entrada (HTTP)
        │       ├── context.go  # UserID/email no context
        │       ├── middleware.go
        │       └── handler.go  # Handler → service; DTOs JSON
        └── driven/
            └── postgres/      # Adapter de saída (persistência)
                ├── video_repository.go
                ├── health.go
                └── schema.go   # CreateTableIfNotExists
```

## Modelo de domínio (Fase 1)

A tabela `videos` possui: `id`, `user_id`, `title`, `description`, `status` (pending | processing | completed | failed), `storage_key`, `result_zip_path`, `error_message`, `created_at`, `updated_at`. Todas as operações são filtradas por `user_id`, obtido do header **`X-User-Id`** repassado pelo API Gateway (Lambda Authorizer).

## Migrations

O schema do banco é versionado na **infra**: `hack-fiap233-infra/migrations/videos/001_initial_schema.sql` (schema final; quando o DB é recriado do zero, só essa migration é aplicada). Aplicar com `./scripts/run_migrations.sh` (variáveis `VIDEOS_DB_*` ou `MIGRATE_VIDEOS_SECRET`). Em ambiente local, o serviço cria a tabela automaticamente no startup (`postgres.CreateTableIfNotExists`).

## Autorização (Fase 2 — identidade via API Gateway)

O serviço não valida JWT nem acessa `JWT_SECRET`. A validação é feita no **API Gateway** (Lambda Authorizer). O Vídeos apenas lê os headers repassados pelo Gateway:

- **`X-User-Id`** — obrigatório em todas as rotas exceto `/videos/health`; ausente ou inválido → **401 Unauthorized**
- **`X-User-Email`** — opcional; disponível no context para uso futuro (ex.: notificação SNS)

Todas as rotas sob `/videos/` (exceto `health`) passam pelo middleware `requireUserID`, que rejeita com 401 se `X-User-Id` estiver ausente ou inválido. Em produção o Gateway já protege as rotas com JWT; requisições que chegarem sem esses headers (ex.: chamada direta ao serviço) são rejeitadas.

## Endpoints

| Método | Rota | Descrição |
|---|---|---|
| GET | `/videos/health` | Health check + status do banco (não exige autenticação) |
| GET | `/videos/` | Listar vídeos do usuário (exige header `X-User-Id`) |
| POST | `/videos/` | Criar vídeo (só metadado: `title`, `description`; exige header `X-User-Id`) |

Rotas protegidas: sem `X-User-Id` válido a API retorna **401 Unauthorized**.

**Nota:** O envio do arquivo de vídeo (upload) será feito no endpoint `POST /videos/upload` . O `POST /videos/` atual apenas cria o registro no banco (título e descrição) com status `pending`.

---

## Testar em ambiente local (sem AWS)

Para baixar o repositório e rodar o serviço na sua máquina **sem usar a infraestrutura da AWS** (EKS, RDS, etc.):

### 1. Subir o Postgres

Na raiz do repositório `hack-fiap233-videos`:

```bash
docker compose -f docker-compose.local.yml up -d
```

Isso sobe um PostgreSQL com o banco `videosdb` na porta **5432**.

### 2. Variáveis de ambiente e rodar o serviço

```bash
export PORT=8080
export DB_HOST=localhost
export DB_PORT=5432
export DB_USERNAME=dbadmin
export DB_PASSWORD=localdev
export DB_NAME=videosdb
export DB_SSLMODE=disable

go run main.go
```

### 3. Testar

```bash
# Health
curl http://localhost:8080/videos/health

# Criar vídeo (X-User-Id obrigatório)
curl -X POST http://localhost:8080/videos/ -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"title":"Meu vídeo","description":"Descrição do vídeo"}'

# Listar vídeos (header X-User-Id obrigatório; em produção vem do API Gateway)
curl -H "X-User-Id: 1" http://localhost:8080/videos/
```

Para derrubar o Postgres: `docker compose -f docker-compose.local.yml down`.

### Pré-requisitos

- [Go](https://go.dev/dl/) 1.21+
- [Docker](https://docs.docker.com/get-docker/) (para o Postgres local)

---

## Deploy (AWS)

O deploy é automático via GitHub Actions. Qualquer push na `main` executa:

1. Build da imagem Docker
2. Push para o ECR
3. Deploy no cluster EKS

### Secrets necessárias no GitHub

| Secret | Descrição |
|---|---|
| `AWS_ACCESS_KEY_ID` | Access Key da AWS Academy |
| `AWS_SECRET_ACCESS_KEY` | Secret Key da AWS Academy |
| `AWS_SESSION_TOKEN` | Session Token da AWS Academy |
