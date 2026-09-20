# volunteer-service — build e teste local

Python 3.11 + Flask + gunicorn. Porta `8083`. Persiste em DynamoDB (`SolidaryTechVolunteers`, via LocalStack em ambiente local).

**Nota**: diferente de `ngo-service`/`donation-service`, este serviço conecta no DynamoDB de forma **preguiçosa** (só na primeira requisição) — o container sobe mesmo sem o LocalStack no ar, mas qualquer chamada a `/volunteers` falha (`500`) até o LocalStack estar disponível.

## Pré-requisitos

- Docker + Docker Compose plugin
- Rodar os comandos a partir da **raiz do repositório** (onde está o `docker-compose.yaml`)

## Passo a passo (via Docker Compose — recomendado)

Sobe só o `volunteer-service` e sua dependência (`localstack`), sem subir `ngo-service`/`donation-service`:

```bash
# na raiz do repositório
docker compose up -d --build localstack volunteer-service-app
```

Acompanhar a subida:

```bash
docker compose logs -f volunteer-service-app
```

Testar:

```bash
curl http://localhost:8083/health

# Criar voluntário (grava no DynamoDB via LocalStack)
curl -X POST http://localhost:8083/volunteers \
  -H "Content-Type: application/json" \
  -d '{"name":"Voluntario","email":"vol@teste.com","ngo_id":1}'

# Listar voluntários de uma ONG
curl http://localhost:8083/volunteers/1
```

Códigos de erro esperados: `400` campos obrigatórios ausentes, `500` erro no DynamoDB, `404` se `ngo_id` não for um inteiro (o converter `<int:ngo_id>` do Flask rejeita a rota antes de chegar no handler).

Derrubar ao final:

```bash
docker compose down -v localstack volunteer-service-app
```

## Alternativa: build isolado da imagem (`docker build`/`docker run` puro)

Só faz sentido depois que `localstack` já estiver de pé (subir com o comando acima ao menos uma vez, ou `docker compose up -d localstack`) — o `docker run` abaixo se conecta na rede que o Compose já criou.

```bash
cd microservices/volunteer-service
docker build -t volunteer-service-app:local .

docker run --rm -p 8083:8083 \
  --network fiap-tech-challenge-fase-5-services_back-tier-private \
  --env-file config.env \
  volunteer-service-app:local
```

> Nome da rede confirmado com `docker network ls` (padrão do Compose: `<nome-do-diretório-do-repo>_back-tier-private`).

## Variáveis de ambiente (`config.env`)

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP (padrão `8083`) |
| `AWS_REGION` / `AWS_DYNAMODB_TABLE` / `AWS_ENDPOINT_URL_DYNAMODB` | Tabela DynamoDB via LocalStack — criada por `localstack/init/init-dynamodb.sh` |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Credenciais dummy (`test`/`test`), só pro SDK aceitar apontar pro LocalStack |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Desligado por padrão (`OTEL_TRACES_EXPORTER=none`/`OTEL_METRICS_EXPORTER=none`); trocar pra `otlp` pra habilitar contra um coletor OTLP local |

## Troubleshooting

- **`/volunteers` retornando `500`**: `localstack` ainda não terminou de subir, ou `init-dynamodb.sh` ainda não rodou — checar `docker compose logs localstack`.
- **Build falhando no `pip install`**: usar Python 3.11 (já é o que o `Dockerfile` usa).
