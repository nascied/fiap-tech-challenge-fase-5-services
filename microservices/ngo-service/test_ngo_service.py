"""
Testes unitários do ngo-service.

app.py conecta no PostgreSQL (SimpleConnectionPool) na CARGA do módulo — se
DATABASE_URL não existir ou o banco não estiver acessível, o processo sai
com sys.exit(1) antes de qualquer rota existir (ver app.py:17-27). Por isso
o fixture abaixo faz o patch de psycopg2.pool.SimpleConnectionPool ANTES de
importar app.py, e reimporta o módulo do zero a cada teste (via
sys.modules.pop) para garantir isolamento entre casos.
"""
import sys
import importlib
from unittest.mock import MagicMock, patch

import psycopg2
import pytest


@pytest.fixture
def app_module(monkeypatch):
    monkeypatch.setenv("DATABASE_URL", "postgres://fake:fake@localhost/fake_db")
    sys.modules.pop("app", None)

    with patch("psycopg2.pool.SimpleConnectionPool") as mock_pool_cls:
        mock_pool = MagicMock()
        mock_pool_cls.return_value = mock_pool

        import app as module
        importlib.reload(module)
        module.app.testing = True
        module.pool = mock_pool  # garante que as rotas usem o mock, não a instância real do pool

        yield module

    sys.modules.pop("app", None)


@pytest.fixture
def client(app_module):
    return app_module.app.test_client()


@pytest.fixture
def mock_cursor(app_module):
    """Configura pool.getconn() -> conn -> conn.cursor() -> cursor mockados."""
    cursor = MagicMock()
    cursor.__enter__.return_value = cursor
    cursor.__exit__.return_value = False

    conn = MagicMock()
    conn.cursor.return_value = cursor

    app_module.pool.getconn.return_value = conn
    return cursor, conn


def test_health(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.get_json() == {"status": "ok", "service": "ngo-service"}


def test_create_ngo_missing_fields(client):
    resp = client.post("/ngos", json={"name": "ONG Sem Email"})
    assert resp.status_code == 400
    assert "error" in resp.get_json()


def test_create_ngo_success(client, mock_cursor):
    cursor, conn = mock_cursor
    cursor.fetchone.return_value = {
        "id": 1,
        "name": "ONG Exemplo",
        "email": "contato@ong.org",
        "cause": "Educação",
        "city": "São Paulo",
    }

    resp = client.post(
        "/ngos",
        json={"name": "ONG Exemplo", "email": "contato@ong.org", "cause": "Educação", "city": "São Paulo"},
    )

    assert resp.status_code == 201
    assert resp.get_json()["id"] == 1
    cursor.execute.assert_called_once()
    conn.commit.assert_called_once()


def test_create_ngo_duplicate_email(client, mock_cursor):
    cursor, conn = mock_cursor
    cursor.execute.side_effect = psycopg2.IntegrityError("duplicate key value violates unique constraint")

    resp = client.post(
        "/ngos",
        json={"name": "ONG Dup", "email": "ja@existe.org", "cause": "Saúde", "city": "Rio"},
    )

    assert resp.status_code == 409
    assert resp.get_json() == {"error": "E-mail já cadastrado"}
    conn.rollback.assert_called_once()


def test_create_ngo_generic_error(client, mock_cursor):
    cursor, conn = mock_cursor
    cursor.execute.side_effect = Exception("erro inesperado de conexão")

    resp = client.post(
        "/ngos",
        json={"name": "ONG Erro", "email": "erro@ong.org", "cause": "Meio Ambiente", "city": "Curitiba"},
    )

    assert resp.status_code == 500
    conn.rollback.assert_called_once()


def test_get_ngos_success(client, mock_cursor):
    cursor, _ = mock_cursor
    cursor.fetchall.return_value = [
        {"id": 2, "name": "ONG B", "email": "b@ong.org", "cause": "Cultura", "city": "SP"},
        {"id": 1, "name": "ONG A", "email": "a@ong.org", "cause": "Educação", "city": "RJ"},
    ]

    resp = client.get("/ngos")

    assert resp.status_code == 200
    body = resp.get_json()
    assert len(body) == 2
    assert body[0]["id"] == 2


def test_get_ngos_error(client, mock_cursor):
    cursor, _ = mock_cursor
    cursor.execute.side_effect = Exception("timeout de conexão")

    resp = client.get("/ngos")

    assert resp.status_code == 500
