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
- **Adapters driven:** Postgres (`VideoRepository`, `HealthChecker`), storage filesystem (`Storage`), fila RabbitMQ (`VideoQueue`), notificador noop (`FailureNotifier`; em prod trocar por SNS), processador ffmpeg (`VideoProcessor`).
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
            ├── postgres/      # Persistência (VideoRepository, schema)
            ├── storage/       # Storage (filesystem; em prod S3/MinIO)
            ├── queue/         # Fila (RabbitMQ video.process + DLQ)
            ├── notifier/      # Notificação de falha (noop; em prod SNS)
            └── processor/     # Processamento vídeo → frames → ZIP (ffmpeg)
cmd/
  worker/           # Binário que consome a fila e processa os jobs
```

## Modelo de domínio

A tabela `videos` possui: `id`, `user_id`, `title`, `description`, `status` (pending | processing | completed | failed), `storage_key`, `result_zip_path`, `error_message`, `created_at`, `updated_at`. Todas as operações são filtradas por `user_id`, obtido do header **`X-User-Id`** repassado pelo API Gateway (Lambda Authorizer).

## Migrations

O schema do banco é versionado na **infra**: `hack-fiap233-infra/migrations/videos/001_initial_schema.sql` (schema final; quando o DB é recriado do zero, só essa migration é aplicada). Aplicar com `./scripts/run_migrations.sh` (variáveis `VIDEOS_DB_*` ou `MIGRATE_VIDEOS_SECRET`). Em ambiente local, o serviço cria a tabela automaticamente no startup (`postgres.CreateTableIfNotExists`).

## Autorização (via API Gateway)

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
| POST | `/videos/upload` | Upload de vídeo (multipart: `file` + opcionais `title`, `description`); grava no storage, cria registro `pending` e publica job na fila |
| GET | `/videos/:id` | Detalhe do vídeo (status e metadados); só retorna se for do usuário (`X-User-Id`) |
| GET | `/videos/:id/download` | Download do ZIP de resultado; só se o vídeo for do usuário e status `completed` (retorna 400 se ainda pending/processing/failed) |

Rotas protegidas: sem `X-User-Id` válido a API retorna **401 Unauthorized**.

---

## Testar em ambiente local (sem AWS)

Para baixar o repositório e rodar o serviço na sua máquina **sem usar a infraestrutura da AWS** (EKS, RDS, etc.):

### 1. Subir Postgres e RabbitMQ

Na raiz do repositório `hack-fiap233-videos`:

```bash
docker compose -f docker-compose.local.yml up -d
```

Isso sobe PostgreSQL (`videosdb` na porta 5432) e RabbitMQ (AMQP 5672, management 15672).

### 2. Variáveis de ambiente e rodar o serviço (API)

```bash
export PORT=8080
export DB_HOST=localhost
export DB_PORT=5432
export DB_USERNAME=dbadmin
export DB_PASSWORD=localdev
export DB_NAME=videosdb
export DB_SSLMODE=disable
export STORAGE_BASE_PATH=./data
export AMQP_URL=amqp://guest:guest@localhost:5672/
export QUEUE_NAME=video.process
export QUEUE_DLQ=video.process.dlq

go run main.go
```

Se `AMQP_URL` não for definido, o serviço sobe sem fila (upload grava no storage e cria o vídeo, mas não enfileira job).

### 3. Rodar o worker (processamento assíncrono)

Instale o **ffmpeg** (ver pré-requisitos abaixo) para o ZIP conter os frames extraídos; sem ele o download traz só um readme placeholder.

Em outro terminal, com as mesmas variáveis de DB e storage (e obrigatório `AMQP_URL`):

```bash
export DB_HOST=localhost
export DB_PORT=5432
export DB_USERNAME=dbadmin
export DB_PASSWORD=localdev
export DB_NAME=videosdb
export DB_SSLMODE=disable
export STORAGE_BASE_PATH=./data
export AMQP_URL=amqp://guest:guest@localhost:5672/

go run ./cmd/worker
```

O worker consome a fila `video.process`, baixa o vídeo do storage, extrai frames (ffmpeg) e gera um ZIP; atualiza o status para `completed` ou `failed`. Em falha, o notificador (noop em dev) loga; em produção pode publicar no SNS.

### 4. Testar

```bash
# Health
curl http://localhost:8080/videos/health

# Upload de vídeo (multipart; X-User-Id e X-User-Email recomendados)
curl -X POST http://localhost:8080/videos/upload \
  -H "X-User-Id: 1" -H "X-User-Email: user@example.com" \
  -F "file=@/caminho/para/video.mp4" -F "title=Meu vídeo" -F "description=Teste"

# Listar vídeos (status pending/processing/completed/failed)
curl -H "X-User-Id: 1" http://localhost:8080/videos/

# Detalhe de um vídeo (substitua 1 pelo id)
curl -H "X-User-Id: 1" http://localhost:8080/videos/1

# Download do ZIP (só quando status for completed)
# URL antes do -o para importar corretamente no Bruno/Postman
curl -H "X-User-Id: 1" "http://localhost:8080/videos/1/download" -o resultado.zip
```

Para derrubar: `docker compose -f docker-compose.local.yml down`.

### Pré-requisitos

- [Go](https://go.dev/dl/) 1.21+
- [Docker](https://docs.docker.com/get-docker/) (para o Postgres local)
- **ffmpeg** — usado pelo worker para extrair frames do vídeo e gerar o ZIP. Sem ele, o download traz só um placeholder ("Frames could not be extracted...").
  - **macOS:** `brew install ffmpeg`
  - **Ubuntu/Debian:** `sudo apt install ffmpeg`
  - Confirme no terminal: `ffmpeg -version`

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
