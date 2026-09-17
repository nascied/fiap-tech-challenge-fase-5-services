package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/joho/godotenv"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

var tracer = otel.Tracer("donation-service")

type Donation struct {
	ID        int       `json:"id"`
	NgoID     int       `json:"ngo_id"`
	Amount    float64   `json:"amount"`
	DonorName string    `json:"donor_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type App struct {
	DB          *sql.DB
	SqsSvc      *sqs.SQS
	SqsQueueURL string
}

func main() {
	_ = godotenv.Load()

	shutdownTracing := initTracing()
	defer shutdownTracing(context.Background())

	shutdownMetrics := initMetrics()
	defer shutdownMetrics(context.Background())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL é obrigatória")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil || db.Ping() != nil {
		log.Fatalf("Erro ao conectar ao banco de dados: %v", err)
	}
	log.Println("Conectado ao PostgreSQL (donation-service).")

	var sqsSvc *sqs.SQS
	queueURL := os.Getenv("AWS_SQS_URL")
	region := os.Getenv("AWS_REGION")
	if queueURL != "" && region != "" {
		awsCfg := &aws.Config{Region: aws.String(region)}
		if endpoint := os.Getenv("AWS_ENDPOINT_URL_SQS"); endpoint != "" {
			awsCfg.Endpoint = aws.String(endpoint)
		}
		sess, _ := session.NewSession(awsCfg)
		sqsSvc = sqs.New(sess)
		log.Println("Integração com AWS SQS ativada.")
	}

	app := &App{DB: db, SqsSvc: sqsSvc, SqsQueueURL: queueURL}

	mux := http.NewServeMux()
	mux.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(app.HealthHandler), "health"))
	mux.Handle("/donations", otelhttp.NewHandler(http.HandlerFunc(app.DonationHandler), "donations"))

	log.Printf("donation-service rodando na porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// buildResource monta os atributos de resource (service.name, deployment.environment
// etc.) compartilhados entre o TracerProvider e o MeterProvider.
func buildResource() *resource.Resource {
	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("donation-service"),
			attribute.String("deployment.environment", envOrDefault("ENVIRONMENT", "local")),
		),
	)
	return res
}

// normalizeEndpoint tira o scheme (http://, https://) de OTEL_EXPORTER_OTLP_ENDPOINT,
// já que os exportadores otlptracehttp/otlpmetrichttp usam WithEndpoint(host:port)
// e adicionam o path padrão (/v1/traces, /v1/metrics) sozinhos.
func normalizeEndpoint(endpoint string) string {
	return strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
}

// initTracing configura o SDK OpenTelemetry com exportador OTLP/HTTP. Se
// OTEL_EXPORTER_OTLP_ENDPOINT não estiver definida, o tracer global fica no-op
// (sem overhead, sem tentar conectar em lugar nenhum) — instrumentação é opt-in.
func initTracing() func(context.Context) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }
	}

	ctx := context.Background()
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(normalizeEndpoint(endpoint)),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Printf("Falha ao iniciar exportador OTLP de traces, desativado: %v", err)
		return func(context.Context) error { return nil }
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(buildResource()),
	)
	otel.SetTracerProvider(tp)

	log.Printf("Tracing OTLP ativado (endpoint=%s)", endpoint)
	return tp.Shutdown
}

// initMetrics configura o SDK de métricas do OpenTelemetry. O otelhttp
// (usado nas rotas em main()) grava automaticamente o histograma
// http.server.request.duration (convenção semântica nova) em todo
// MeterProvider registrado globalmente — é essa métrica que alimenta os
// dashboards de Golden Metrics/SLO no Grafana (repositório observability).
// Isso só funciona a partir do otelhttp v0.55.0 (ver go.mod) com
// OTEL_SEMCONV_STABILITY_OPT_IN=http setada (ver config.env) — versões
// anteriores (ex.: v0.49.0) ignoram essa variável e emitem incondicionalmente
// só a convenção antiga (http.server.duration), deixando os dashboards vazios.
// A v0.54.0 já lê a variável mas tem uma regressão no wrapper de resposta
// (chama WriteHeader duas vezes quando o handler define um status explícito
// antes do corpo — gera "superfluous response.WriteHeader call" em toda
// requisição); só foi corrigida na v0.55.0, que exige Go 1.22 (daí o bump
// do go.mod e do Dockerfile).
// Mesmo opt-in que initTracing: sem OTEL_EXPORTER_OTLP_ENDPOINT, vira no-op.
func initMetrics() func(context.Context) error {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }
	}

	ctx := context.Background()
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(normalizeEndpoint(endpoint)),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		log.Printf("Falha ao iniciar exportador OTLP de métricas, desativado: %v", err)
		return func(context.Context) error { return nil }
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(buildResource()),
	)
	otel.SetMeterProvider(mp)

	log.Printf("Métricas OTLP ativadas (endpoint=%s)", endpoint)
	return mp.Shutdown
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func (a *App) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"donation-service"}`))
}

func (a *App) DonationHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		var d Donation
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, `{"error":"Payload inválido"}`, http.StatusBadRequest)
			return
		}

		d.Status = "APPROVED" // Simulação de gateway de pagamento

		dbCtx, dbSpan := tracer.Start(ctx, "db.insert_donation")
		err := a.DB.QueryRowContext(dbCtx,
			"INSERT INTO donations (ngo_id, amount, donor_name, status) VALUES ($1, $2, $3, $4) RETURNING id, created_at",
			d.NgoID, d.Amount, d.DonorName, d.Status,
		).Scan(&d.ID, &d.CreatedAt)
		dbSpan.End()

		if err != nil {
			log.Printf("Erro ao salvar doação: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		if a.SqsSvc != nil {
			// context.WithoutCancel: mantém trace_id/span_id (correlação) mas
			// desliga do cancelamento da requisição HTTP original, já que isso
			// roda em goroutine separada depois da resposta já ter sido enviada.
			go a.sendNotificationEvent(context.WithoutCancel(ctx), d)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(d)
		return
	}

	if r.Method == http.MethodGet {
		listCtx, listSpan := tracer.Start(ctx, "db.list_donations")
		rows, err := a.DB.QueryContext(listCtx,
			"SELECT id, ngo_id, amount, donor_name, status, created_at FROM donations ORDER BY id DESC")
		listSpan.End()
		if err != nil {
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		donations := []Donation{}
		for rows.Next() {
			var d Donation
			rows.Scan(&d.ID, &d.NgoID, &d.Amount, &d.DonorName, &d.Status, &d.CreatedAt)
			donations = append(donations, d)
		}

		json.NewEncoder(w).Encode(donations)
		return
	}

	http.Error(w, `{"error":"Método não permitido"}`, http.StatusMethodNotAllowed)
}

func (a *App) sendNotificationEvent(ctx context.Context, d Donation) {
	_, span := tracer.Start(ctx, "sqs.publish_donation_event")
	defer span.End()

	body, _ := json.Marshal(d)
	_, err := a.SqsSvc.SendMessage(&sqs.SendMessageInput{
		MessageBody: aws.String(string(body)),
		QueueUrl:    aws.String(a.SqsQueueURL),
	})
	if err != nil {
		span.RecordError(err)
		log.Printf("Falha ao despachar evento SQS: %v", err)
	}
}
