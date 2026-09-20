package main

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// newTestApp cria um App com um *sql.DB conectado a um driver mock (sem
// Postgres real) — SqsSvc fica nil de propósito: os testes cobrem o
// contrato HTTP/DB do handler, não a integração real com SQS (isso exigiria
// abstrair *sqs.SQS atrás de uma interface só para teste, fora do escopo
// desta rodada). Como a.SqsSvc != nil é o gate para despachar o evento
// (main.go:225), deixar nil garante que esse branch nunca executa nos testes.
func newTestApp(t *testing.T) (*App, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("erro ao criar sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &App{DB: db, SqsSvc: nil}, mock
}

func TestHealthHandler(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	app.HealthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body não é JSON válido: %v", err)
	}
	if body["status"] != "ok" || body["service"] != "donation-service" {
		t.Fatalf("body inesperado: %v", body)
	}
}

func TestDonationHandler_Post_InvalidJSON(t *testing.T) {
	app, _ := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/donations", strings.NewReader("{not-json"))
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDonationHandler_Post_Success(t *testing.T) {
	app, mock := newTestApp(t)

	now := time.Now()
	mock.ExpectQuery("INSERT INTO donations").
		WithArgs(1, 50.0, "Doador Teste", "APPROVED").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(42, now))

	body := `{"ngo_id":1,"amount":50,"donor_name":"Doador Teste"}`
	req := httptest.NewRequest(http.MethodPost, "/donations", strings.NewReader(body))
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got Donation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body não é JSON válido: %v", err)
	}
	if got.ID != 42 || got.Status != "APPROVED" || got.NgoID != 1 {
		t.Fatalf("donation retornada inesperada: %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestDonationHandler_Post_DBError(t *testing.T) {
	app, mock := newTestApp(t)

	mock.ExpectQuery("INSERT INTO donations").
		WillReturnError(driver.ErrBadConn)

	body := `{"ngo_id":1,"amount":50,"donor_name":"Doador Teste"}`
	req := httptest.NewRequest(http.MethodPost, "/donations", strings.NewReader(body))
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestDonationHandler_Get_Success(t *testing.T) {
	app, mock := newTestApp(t)

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "ngo_id", "amount", "donor_name", "status", "created_at"}).
		AddRow(2, 1, 30.0, "Doador B", "APPROVED", now).
		AddRow(1, 1, 50.0, "Doador A", "APPROVED", now)
	mock.ExpectQuery("SELECT id, ngo_id, amount, donor_name, status, created_at FROM donations").
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/donations", nil)
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []Donation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body não é JSON válido: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperado 2 doações, veio %d", len(got))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectativas do mock não atendidas: %v", err)
	}
}

func TestDonationHandler_Get_DBError(t *testing.T) {
	app, mock := newTestApp(t)

	mock.ExpectQuery("SELECT id, ngo_id, amount, donor_name, status, created_at FROM donations").
		WillReturnError(driver.ErrBadConn)

	req := httptest.NewRequest(http.MethodGet, "/donations", nil)
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestDonationHandler_MethodNotAllowed(t *testing.T) {
	app, _ := newTestApp(t)
	req := httptest.NewRequest(http.MethodDelete, "/donations", nil)
	rec := httptest.NewRecorder()

	app.DonationHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://otel-collector:4318":  "otel-collector:4318",
		"https://otel-collector:4318": "otel-collector:4318",
		"otel-collector:4318":         "otel-collector:4318",
	}
	for input, want := range cases {
		if got := normalizeEndpoint(input); got != want {
			t.Errorf("normalizeEndpoint(%q) = %q, esperado %q", input, got, want)
		}
	}
}

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("DONATION_TEST_VAR", "valor-setado")
	if got := envOrDefault("DONATION_TEST_VAR", "default"); got != "valor-setado" {
		t.Errorf("envOrDefault com var setada = %q, esperado %q", got, "valor-setado")
	}

	if got := envOrDefault("DONATION_TEST_VAR_AUSENTE", "default"); got != "default" {
		t.Errorf("envOrDefault sem var = %q, esperado %q", got, "default")
	}
}
