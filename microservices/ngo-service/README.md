# ngo-service — build e teste local  

Python 3.11 + Flask + gunicorn. Porta `8081`. Persiste em PostgreSQL (`ngo_db`).

**Importante**: o serviço conecta no Postgres **no startup** (`SimpleConnectionPool` em `app.py`) e derruba o processo (`sys.exit(1)`) se o banco não estiver acessível naquele momento — não sobe isolado, sem banco.

## Pré-requisitos

- Docker + Docker Compose plugin
- Rodar os comandos a partir da **raiz do repositório** (onde está o `docker-compose.yaml`)

## Passo a passo (via Docker Compose — recomendado)

Sobe só o `ngo-service` e sua dependência (`ngo-pg-db`), sem subir `donation-service`/`volunteer-service`/`localstack`:

```bash
# na raiz do repositório
docker compose up -d --build ngo-pg-db ngo-service-app
```

Acompanhar a subida (reinicia sozinho em loop até o Postgres passar no healthcheck — `restart: on-failure:5` cobre isso):

```bash
docker compose logs -f ngo-service-app
```

Testar:

```bash
curl http://localhost:8081/health

# Lista ONGs (seed já vem em db/init.sql)
curl http://localhost:8081/ngos

# Cria ONG
curl -X POST http://localhost:8081/ngos \
  -H "Content-Type: application/json" \
  -d '{"name":"ONG Exemplo","email":"contato@ongexemplo.org","cause":"Educação","city":"São Paulo"}'
```

Códigos de erro esperados: `400` se faltar campo obrigatório, `409` se o `email` já estiver cadastrado (constraint `UNIQUE`).

Derrubar ao final:

```bash
docker compose down -v ngo-pg-db ngo-service-app
```

## Alternativa: build isolado da imagem (`docker build`/`docker run` puro)

Só faz sentido depois que `ngo-pg-db` já estiver de pé (subir com o comando acima ao menos uma vez, ou `docker compose up -d ngo-pg-db`) — o `docker run` abaixo se conecta na rede que o Compose já criou.

```bash
cd microservices/ngo-service
docker build -t ngo-service-app:local .

docker run --rm -p 8081:8081 \
  --network fiap-tech-challenge-fase-5-services_back-tier-private \
  --env-file config.env \
  ngo-service-app:local
```

> Nome da rede confirmado com `docker network ls` (padrão do Compose: `<nome-do-diretório-do-repo>_back-tier-private`).

## Variáveis de ambiente (`config.env`)

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP (padrão `8081`) |
| `DATABASE_URL` | String de conexão Postgres (`postgres://ngo:ngo@ngo-pg-db:5432/ngo_db?sslmode=disable`) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | Credenciais também usadas pelo container `ngo-pg-db` (init do banco) |
| `OTEL_*` | OpenTelemetry — desligado por padrão (`OTEL_TRACES_EXPORTER=none`/`OTEL_METRICS_EXPORTER=none`); trocar pra `otlp` pra habilitar contra um coletor OTLP local |

## Troubleshooting

- **Container reiniciando em loop**: Postgres (`ngo-pg-db`) ainda não passou no healthcheck — checar `docker compose logs ngo-pg-db`.
- **Build falhando no `pip install`**: usar Python 3.11 (já é o que o `Dockerfile` usa) — 3.12 não tem wheel pra `psycopg2-binary==2.9.5`.
