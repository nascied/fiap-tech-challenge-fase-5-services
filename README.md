# 🤝 Solidary Tech — Plataforma de Microserviços

[![Docker](https://img.shields.io/badge/Docker-Container-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://www.docker.com/)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)
[![Python](https://img.shields.io/badge/Python-3.11-3776AB?style=for-the-badge&logo=python&logoColor=white)](https://www.python.org/)
[![AWS](https://img.shields.io/badge/AWS-DynamoDB/SQS-232F3E?style=for-the-badge&logo=amazonaws&logoColor=white)](https://aws.amazon.com/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)](https://www.postgresql.org/)

> ⚠️ Projeto desenvolvido como parte do Tech Challenge Fase 5 da Pós-Tech FIAP em Arquitetura Cloud e DevOps.

---

# 📋 Sobre o Projeto

**Solidary Tech** é uma plataforma que conecta ONGs, doadores e voluntários. O backend é composto por microserviços independentes, cada um responsável por um domínio:

- cadastro e consulta de ONGs
- registro de doações, com publicação assíncrona de eventos
- registro e consulta de voluntários por ONG

A solução foi construída priorizando:

- isolamento entre domínios (cada serviço com seu próprio banco/armazenamento)
- comunicação assíncrona via fila de mensagens
- ambiente local reproduzível via Docker Compose, simulando os recursos AWS com LocalStack
- portabilidade da configuração (12-factor) para rodar sem alterações de código em um cluster Kubernetes (Amazon EKS)

---

# 🧱 Arquitetura

```text
microservices/
├── ngo-service/         Python 3.11 (Flask)  → cadastro de ONGs        → PostgreSQL
├── donation-service/    Go 1.22              → registro de doações    → PostgreSQL + SQS
└── volunteer-service/   Python 3.11 (Flask)  → registro de voluntários → DynamoDB
```

```text
                     ┌──────────────────┐
                     │   ngo-service    │──── ngo-pg-db (PostgreSQL)
                     │     :8081        │
                     └──────────────────┘

                     ┌──────────────────┐
                     │ donation-service │──── donation-pg-db (PostgreSQL)
                     │     :8082        │──── SQS: fila "solidary-donations"
                     └──────────────────┘

                     ┌──────────────────┐
                     │ volunteer-service│──── DynamoDB: "SolidaryTechVolunteers"
                     │     :8083        │
                     └──────────────────┘

         SQS e DynamoDB são fornecidos localmente pelo LocalStack
         e substituídos pelos serviços reais da AWS em produção (EKS).
```

---

# ⚙️ Serviços do Projeto

## 1. `ngo-service` — Cadastro de ONGs

| | |
|---|---|
| Linguagem | Python 3.11 + Flask + gunicorn |
| Porta | `8081` |
| Persistência | PostgreSQL (`ngo-pg-db`) |
| Dockerfile | [`microservices/ngo-service/Dockerfile`](microservices/ngo-service/Dockerfile) |

**Endpoints**

| Método | Rota | Sucesso | Erros |
|---|---|---|---|
| `GET` | `/health` | `200` | — |
| `GET` | `/ngos` | `200` — lista todas as ONGs cadastradas | — |
| `POST` | `/ngos` | `201` — body: `name`, `email`, `cause`, `city` | `400` campos obrigatórios ausentes · `409` e-mail já cadastrado (constraint `UNIQUE` em `email`) |

**Variáveis de ambiente** ([`config.env`](microservices/ngo-service/config.env))

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP do serviço |
| `DATABASE_URL` | DSN de conexão com o PostgreSQL |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | Credenciais usadas também para inicializar o container `ngo-pg-db` |

O schema e o seed inicial (duas ONGs de exemplo) estão em [`db/init.sql`](microservices/ngo-service/db/init.sql).

---

## 2. `donation-service` — Registro de Doações

| | |
|---|---|
| Linguagem | Go 1.22 |
| Porta | `8082` |
| Persistência | PostgreSQL (`donation-pg-db`) |
| Mensageria | AWS SQS (fila `solidary-donations`) — publica um evento a cada doação criada |
| Dockerfile | [`microservices/donation-service/Dockerfile`](microservices/donation-service/Dockerfile) |

**Endpoints**

| Método | Rota | Sucesso | Erros |
|---|---|---|---|
| `GET` | `/health` | `200` | — |
| `GET` | `/donations` | `200` — lista todas as doações | — |
| `POST` | `/donations` | `201` — body: `ngo_id`, `amount`, `donor_name` (`status` é sempre forçado para `"APPROVED"`; se o SQS estiver configurado, publica o evento de forma assíncrona) | `400` payload JSON inválido · `500` erro ao persistir |

**Variáveis de ambiente** ([`config.env`](microservices/donation-service/config.env))

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP do serviço |
| `DATABASE_URL` | DSN de conexão com o PostgreSQL |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | Credenciais também usadas pelo container `donation-pg-db` |
| `AWS_REGION` | Região AWS (real ou simulada) |
| `AWS_SQS_URL` | URL da fila SQS |
| `AWS_ENDPOINT_URL_SQS` | Endpoint customizado do SQS — **usar apenas em dev**, aponta pro LocalStack. Ausente/vazio em produção, o SDK usa o endpoint real da AWS |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Credenciais AWS |

> A integração com SQS é opcional por design: se `AWS_SQS_URL` ou `AWS_REGION` não estiverem definidas, o serviço sobe normalmente e só não publica eventos.

Schema em [`db/init.sql`](microservices/donation-service/db/init.sql). Script que cria a fila no LocalStack: [`localstack/init/init-sqs.sh`](localstack/init/init-sqs.sh).

---

## 3. `volunteer-service` — Registro de Voluntários

| | |
|---|---|
| Linguagem | Python 3.11 + Flask + gunicorn |
| Porta | `8083` |
| Persistência | AWS DynamoDB (tabela `SolidaryTechVolunteers`) |
| Dockerfile | [`microservices/volunteer-service/Dockerfile`](microservices/volunteer-service/Dockerfile) |

**Endpoints**

| Método | Rota | Sucesso | Erros |
|---|---|---|---|
| `GET` | `/health` | `200` | — |
| `POST` | `/volunteers` | `201` — body: `name`, `email`, `ngo_id` | `400` campos obrigatórios ausentes · `500` erro no DynamoDB |
| `GET` | `/volunteers/<ngo_id:int>` | `200` — lista voluntários da ONG | `404` se `ngo_id` não for um inteiro (o converter `<int:ngo_id>` do Flask rejeita a rota antes de chegar no handler) |

**Variáveis de ambiente** ([`config.env`](microservices/volunteer-service/config.env))

| Variável | Descrição |
|---|---|
| `PORT` | Porta HTTP do serviço |
| `AWS_REGION` | Região AWS (real ou simulada) |
| `AWS_DYNAMODB_TABLE` | Nome da tabela DynamoDB |
| `AWS_ENDPOINT_URL_DYNAMODB` | Endpoint customizado do DynamoDB — **usar apenas em dev**, aponta pro LocalStack. Ausente/vazio em produção, o SDK usa o endpoint real da AWS |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Credenciais AWS |

Script que cria a tabela no LocalStack: [`localstack/init/init-dynamodb.sh`](localstack/init/init-dynamodb.sh).

---

# 🐳 Rodando o Projeto Localmente com Docker Compose

## Pré-requisitos

- Docker e Docker Compose instalados
- Portas livres na sua máquina: `8080`–`8083`, `4566`, `5433`, `5434`

## Passo a passo

### 1. Ficar na raiz do projeto

```bash
cd fiap-tech-challenge-fase-5-services
```

### 2. Subir tudo com build

```bash
docker compose up -d --build
```

- `-d`: roda em background (detached)
- `--build`: builda as imagens dos 3 serviços a partir dos Dockerfiles

Na primeira vez isso baixa as imagens base (`postgres:16-alpine`, `localstack:4.4.0`, `python:3.11-slim`, `golang:1.22.12-alpine3.20`, `dpage/pgadmin4`) — pode levar alguns minutos.

### 3. Acompanhar a subida

```bash
docker compose ps -a
```

Espere todos os serviços ficarem `Up (healthy)`. A ordem de dependência garante que os bancos e o LocalStack sobem (e passam no healthcheck) antes dos serviços de aplicação.

Para acompanhar logs em tempo real:

```bash
docker compose logs -f                    # de tudo
docker compose logs -f donation-service-app  # de um serviço específico
```

### 4. Testar os healthchecks

```bash
curl http://localhost:8081/health   # ngo-service
curl http://localhost:8082/health   # donation-service
curl http://localhost:8083/health   # volunteer-service
```

### 5. Testar o fluxo completo

```bash
# ONGs (dados de seed já vêm prontos)
curl http://localhost:8081/ngos

# Criar doação (dispara evento pra fila SQS)
curl -X POST http://localhost:8082/donations \
  -H "Content-Type: application/json" \
  -d '{"ngo_id":1,"amount":50,"donor_name":"Seu Nome"}'

curl http://localhost:8082/donations

# Criar voluntário (grava no DynamoDB via LocalStack)
curl -X POST http://localhost:8083/volunteers \
  -H "Content-Type: application/json" \
  -d '{"name":"Voluntario","email":"vol@teste.com","ngo_id":1}'

curl http://localhost:8083/volunteers/1
```

### 6. Extras úteis

| Ferramenta | Acesso |
|---|---|
| pgAdmin | `http://localhost:8080` — login `admin@admin.com` / senha `admin` |
| Listar filas SQS | `docker exec localstack awslocal sqs list-queues` |
| Ler mensagens da fila | `docker exec localstack awslocal sqs receive-message --queue-url http://localhost:4566/000000000000/solidary-donations` |
| Listar tabelas DynamoDB | `docker exec localstack awslocal dynamodb list-tables` |

### 7. Derrubar o ambiente

```bash
docker compose down -v
```

O `-v` também remove os volumes (os dados do Postgres/LocalStack são zerados). Sem `-v`, os dados persistem para a próxima subida.

---

# 🧪 Testando com Postman / Bruno

A collection [`postman/postech-fase5-solidarytech.postman_collection.json`](postman/postech-fase5-solidarytech.postman_collection.json) cobre todas as rotas listadas acima (formato Postman Collection v2.1, importável direto no Postman ou no Bruno).

- **Postman**: Import → File → selecione o `.json`
- **Bruno**: Import Collection → Postman Collection → selecione o `.json`

As variáveis de collection `ngoBaseUrl`, `donationBaseUrl` e `volunteerBaseUrl` já vêm configuradas para `localhost` — basta trocá-las (ou criar um Environment) para apontar para outro ambiente, como o cluster EKS.

---

# 🔭 Observabilidade — Tracing Distribuído

Os três serviços são instrumentados com [OpenTelemetry](https://opentelemetry.io/) (vendor-neutral, exporta via OTLP/HTTP) — **não é código específico do Datadog**, por isso não exige credencial nenhuma pra existir no repositório. `OTEL_EXPORTER_OTLP_ENDPOINT` aponta pro `otel-collector` (repo `fiap-tech-challenge-fase-5-observability`), não direto pro Datadog Agent — é o Collector quem roteia depois: traces → Datadog, métricas → Prometheus (alimenta os dashboards SRE/Golden Metrics), logs → Loki.

Por padrão a instrumentação fica **desligada** em ambos, mas por mecanismos diferentes — cada stack tem uma forma idiomática de fazer isso, e usei a de cada uma:

| Serviço | Como é instrumentado | Como fica desligado por padrão |
|---|---|---|
| `ngo-service`, `volunteer-service` (Python) | **Auto-instrumentação**, zero mudança em `app.py`. O Dockerfile roda `opentelemetry-bootstrap -a install` (detecta Flask/psycopg2/boto3 já instalados e instala o pacote de instrumentação certo pra cada um) e troca o `CMD` para `opentelemetry-instrument gunicorn ...`, que faz o monkey-patch das bibliotecas antes da aplicação subir. | `OTEL_TRACES_EXPORTER=none` no `config.env`. **Importante**: só comentar `OTEL_EXPORTER_OTLP_ENDPOINT` não basta — o SDK Python usa `localhost:4318` como padrão e fica tentando exportar (e falhando) a cada batch, poluindo o log. O interruptor real é `OTEL_TRACES_EXPORTER`. |
| `donation-service` (Go) | **Setup manual** (Go não tem auto-instrumentação via bytecode weaving). `main.go` inicializa o SDK (`initTracing`), envolve os handlers HTTP com `otelhttp.NewHandler` e cria spans manuais em volta da query no Postgres e da publicação no SQS — o span da goroutine assíncrona (`sendNotificationEvent`) usa `context.WithoutCancel` pra manter a correlação (mesmo `trace_id`) sem herdar o cancelamento da requisição HTTP já respondida. | `initTracing()` checa `OTEL_EXPORTER_OTLP_ENDPOINT` explicitamente e retorna um shutdown no-op se estiver vazia — não tenta conectar em lugar nenhum. Comentar a variável já é suficiente aqui. |

**Para habilitar, apontando pro `otel-collector`**:
- **Go** (`donation-service`): descomente as 2 linhas no fim do `config.env` (`OTEL_EXPORTER_OTLP_ENDPOINT` e `OTEL_SEMCONV_STABILITY_OPT_IN`).
- **Python** (`ngo-service`/`volunteer-service`): troque `OTEL_TRACES_EXPORTER=none`/`OTEL_METRICS_EXPORTER=none` para `otlp` — a `OTEL_EXPORTER_OTLP_ENDPOINT` já está preenchida, só é ignorada enquanto os exporters estiverem `none`.

**Variáveis de ambiente** (já presentes nos `config.env`):

| Variável | Efeito |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Endpoint OTLP/HTTP do `otel-collector` (ex.: `http://otel-collector.monitoring.svc.cluster.local:4318`). |
| `OTEL_SERVICE_NAME` (só Python) | Nome do serviço nos traces. No Go isso é fixado em código (`semconv.ServiceName("donation-service")`). |
| `OTEL_TRACES_EXPORTER` / `OTEL_METRICS_EXPORTER` (só Python) | `none` (padrão, desligado) / `otlp` (habilitado) — os interruptores reais da instrumentação Python; o Go usa só a presença/ausência do endpoint (ver tabela acima). |
| `OTEL_LOGS_EXPORTER` (só Python) | `none` — fora de escopo (logs vão pro Loki via coletor de logs do cluster, não via OTel). |
| `OTEL_SEMCONV_STABILITY_OPT_IN` | `http` — sem isso, tanto a instrumentação WSGI/Flask (Python) quanto o `otelhttp` (Go, a partir da v0.55.0) emitem só a convenção semântica **antiga** (`http.server.duration`), não a nova (`http.server.request.duration`) que os dashboards SRE do repo `observability` consultam. Bug real encontrado numa sessão posterior — nenhum dos 3 serviços tinha essa variável setada. |

**Validado localmente** (não faz parte do `docker-compose.yaml` do projeto, foi só pra conferir que os traces realmente saem): subindo um `jaeger` (`jaegertracing/all-in-one`) temporário e apontando `OTEL_EXPORTER_OTLP_ENDPOINT` pra ele, um `POST /donations` gerou um trace com os spans `donations` → `db.insert_donation` e `sqs.publish_donation_event` aninhados, e o `volunteer-service` mostrou `DynamoDB.PutItem` aninhado sob `POST /volunteers` — confirmando que a instrumentação está funcional de ponta a ponta antes de existir credencial do Datadog.

---

# ⚙️ CI/CD — GitHub Actions

Um workflow por microsserviço (`.github/workflows/ci-<serviço>.yaml`), disparado em `pull_request` e `push` pra `main` filtrado por `paths` (só roda o pipeline do serviço que mudou). Cada um tem 4 jobs em sequência:

1. **Lint e qualidade de código** — `go vet`/`go build`/`go test`/`golangci-lint` (Go) ou `py_compile`/`ruff` (Python).
2. **Segurança (SAST + SCA)** — Trivy (scan de filesystem), `govulncheck`+`gosec` (Go) ou `pip-audit`+`bandit` (Python). Os relatórios são publicados como artifact da run; **não travam a pipeline** (mesmo achado pré-existente no código não bloqueia habilitar CI/CD — dar visibilidade primeiro).
3. **Build e publicação da imagem Docker** — Hadolint, build da imagem, Trivy (scan da imagem). Em `pull_request` a imagem só é validada localmente (`push: false`); em `push` pra `main` ela é publicada no Amazon ECR (repositórios `fiap-tc-f5-{ngo,donation,volunteer}-reg`, criados pelo `module.ecr` do repositório `infra`) com a tag = short SHA do commit.
4. **Atualiza imagem no GitOps** (só em `push`) — chama o workflow reutilizável `update-image.yaml` do repositório [`fiap-tech-challenge-fase-5-gitops`](https://github.com/nascied/fiap-tech-challenge-fase-5-gitops), que atualiza `image.repository`/`image.tag` no `values.yaml` do chart Helm correspondente e comita — o ArgoCD sincroniza a partir daí.

**Diferença deliberada em relação ao modelo do Tech Challenge anterior** (`fiap-tech-challenge-fase-3-services`): lá o pipeline publicava imagem e atualizava o GitOps em qualquer `pull_request`, mesmo sem merge — aqui isso só acontece em `push` pra `main` (pós-merge); PR aberto só valida lint/segurança/build.

**A URL do registry ECR não é um secret fixo** — cada pipeline resolve dinamicamente via `aws-actions/amazon-ecr-login` (`steps.ecrlogin.outputs.registry`) depois de autenticar, então continua funcionando mesmo trocando de conta AWS Academy entre sessões (sem precisar repopular um secret depois de cada `terraform apply`, como no modelo anterior).

### Secrets necessários (`Settings > Secrets and variables > Actions`)

| Secret | Uso |
|---|---|
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` | Credenciais de sessão da AWS Academy (mesmo padrão do repositório `infra`) — só usadas no job de build, pra login/push no ECR. Sessão expira em ~4h, precisa ser reenviada periodicamente. |
| `GITOPS_PUSH_TOKEN` | Token com permissão de push no repositório `fiap-tech-challenge-fase-5-gitops`, usado pelo workflow reutilizável `update-image.yaml`. |

**Nunca executado numa run real do GitHub Actions** — validado localmente: os 3 `Dockerfile` buildam com sucesso (`docker build`), `ruff`/`bandit` rodam contra o código real dos 2 serviços Python (achados de estilo/`try/except Exception` genérico existentes, no relatório mas não bloqueantes), e `go mod tidy`/`go vet`/`go build` do `donation-service` validados dentro de um container `golang:1.22.12-alpine3.20` (mesma imagem do `Dockerfile` e do workflow — sandbox local só tinha Go 1.18, insuficiente pro `go.mod`, daí rodar dentro do container).

---

# 🗂️ Estrutura do Projeto

```text
.
├── .github/
│   └── workflows/                # CI/CD por microsserviço (ver seção acima)
│       ├── ci-ngo-service.yaml
│       ├── ci-donation-service.yaml
│       └── ci-volunteer-service.yaml
├── docker-compose.yaml
├── localstack/
│   └── init/                     # scripts executados no boot do LocalStack
│       ├── init-dynamodb.sh      # cria a tabela SolidaryTechVolunteers
│       └── init-sqs.sh           # cria a fila solidary-donations
├── postman/
│   └── postech-fase5-solidarytech.postman_collection.json
└── microservices/
    ├── ngo-service/
    │   ├── app.py
    │   ├── requirements.txt
    │   ├── Dockerfile
    │   ├── config.env
    │   └── db/init.sql
    ├── donation-service/
    │   ├── main.go
    │   ├── go.mod / go.sum
    │   ├── Dockerfile
    │   ├── config.env
    │   └── db/init.sql
    └── volunteer-service/
        ├── app.py
        ├── requirements.txt
        ├── Dockerfile
        └── config.env
```

---

# ☁️ Tecnologias Utilizadas

| Tecnologia | Finalidade |
|---|---|
| Go | Microserviço `donation-service` |
| Python / Flask | Microserviços `ngo-service` e `volunteer-service` |
| Docker / Docker Compose | Containerização e orquestração local |
| PostgreSQL | Persistência relacional (ONGs e doações) |
| AWS DynamoDB | Persistência NoSQL (voluntários) |
| AWS SQS | Mensageria assíncrona (eventos de doação) |
| LocalStack | Simulação local dos serviços AWS |
| pgAdmin | Administração visual dos bancos PostgreSQL |
| Amazon EKS | Orquestração Kubernetes (produção) |

---

# 📌 Status do Projeto

| Status | Item |
|---|---|
| ✅ | 3 microserviços implementados (`ngo-service`, `donation-service`, `volunteer-service`) |
| ✅ | Dockerfiles com boas práticas (multi-stage, usuário não-root, healthcheck) |
| ✅ | Ambiente local completo via Docker Compose (Postgres + LocalStack + pgAdmin) |
| ✅ | Integração validada ponta a ponta (REST → banco → mensageria) |
| ⚠️ | Pipeline CI/CD (build, testes, quality gates, security scan) — implementada (`.github/workflows/`), **nunca executada numa run real do GitHub Actions** |
| ⚠️ | Publicação das imagens em registry (Amazon ECR) — pipeline pronta pra publicar, depende de secrets AWS válidos + run real |
| ⏳ | Deploy no Amazon EKS |

---

# 👨‍💻 Autor

**Edson Nascimento**
Pós-Tech FIAP — Arquitetura Cloud e DevOps
Tech Challenge Fase 5

---

# 📄 Licença

Projeto desenvolvido para fins educacionais como parte da Pós-Tech FIAP.
