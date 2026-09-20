# donation-service — build e teste local 

Go 1.22 (build multi-stage). Porta `8082`. Persiste em PostgreSQL (`donation_db`) e publica eventos na fila SQS `solidary-donations` (via LocalStack em ambiente local). **Hot Path / Tier 0** do projeto.

**Importante**: o serviço conecta no Postgres **no startup** (`sql.Open`+`db.Ping()` em `main.go`) e derruba o processo (`log.Fatalf`) se o banco não estiver acessível naquele momento — não sobe isolado, sem banco.

## Pré-requisitos

- Docker + Docker Compose plugin
- Rodar os comandos a partir da **raiz do repositório** (onde está o `docker-compose.yaml`)

## Passo a passo (via Docker Compose — recomendado)

Sobe só o `donation-service` e suas dependências (`donation-pg-db` + `localstack`, necessário pro SQS), sem subir `ngo-service`/`volunteer-service`:

```bash
# na raiz do repositório
docker compose up -d --build donation-pg-db localstack donation-service-app
```

Acompanhar a subida (reinicia sozinho em loop até o Postgres passar no healthcheck — `restart: on-failure:5` cobre isso):

```bash
docker compose logs -f donation-service-app
```

Testar:

```bash
curl http://localhost:8082/health

# Criar doação (status é sempre forçado para "APPROVED"; publica evento no SQS de forma assíncrona)
curl -X POST http://localhost:8082/donations \
  -H "Content-Type: application/json" \
  -d '{"ngo_id":1,"amount":50,"donor_name":"Seu Nome"}'

# Listar doações
curl http://localhost:8082/donations
```

Códigos de erro esperados: `400` payload JSON inválido, `500` erro ao persistir.

Derrubar ao final:

```bash
docker compose down -v donation-pg-db localstack donation-service-app
```

## Alternativa: build isolado da imagem (`docker build`/`docker run` puro)

Só faz sentido depois que `donation-pg-db`/`localstack` já estiverem de pé (subir com o comando acima ao menos uma vez, ou `docker compose up -d donation-pg-db localstack`) — o `docker run` abaixo se conecta na rede que o Compose já criou.

```bash
cd microservices/donation-service
docker build -t donation-service-app:local .

docker run --rm -p 8082:8082 \
  --network fiap-tech-challenge-fase-5-services_back-tier-private \
  --env-file config.env \
  donation-service-app:local
```

> Nome da rede confirmado com `docker network ls` (padrão do Compose: `<nome-do-diretório-do-repo>_back-tier-private`).

## Variáveis de ambiente (`config.env`)

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP (padrão `8082`) |
| `DATABASE_URL` | String de conexão Postgres (`postgres://donation:donation@donation-pg-db:5432/donation_db?sslmode=disable`) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | Credenciais também usadas pelo container `donation-pg-db` (init do banco) |
| `AWS_REGION` / `AWS_SQS_URL` / `AWS_ENDPOINT_URL_SQS` | Fila SQS via LocalStack — criada por `localstack/init/init-sqs.sh` |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Credenciais dummy (`test`/`test`), só pro SDK aceitar apontar pro LocalStack |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Comentado por padrão — desligado (`initTracing()`/`initMetrics()` fazem no-op sem ela). Descomentar pra apontar pra um coletor OTLP local |

## Troubleshooting

- **Container reiniciando em loop**: Postgres (`donation-pg-db`) ainda não passou no healthcheck, ou `localstack` ainda não terminou de subir — checar `docker compose logs donation-pg-db localstack`.
- **`POST /donations` funciona mas evento não chega na fila**: confirmar que `localstack/init/init-sqs.sh` rodou (`docker compose logs localstack | grep solidary-donations`) — só roda depois do healthcheck do LocalStack passar.
- **Build falhando**: usar `golang:1.22.12-alpine3.20` (já é a imagem do `Dockerfile`) — versões de `otelhttp`/`otel` no `go.mod` foram pinadas nessa versão de Go de propósito.
