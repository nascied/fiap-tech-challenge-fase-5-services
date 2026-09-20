"""
Testes unitários do volunteer-service.

Diferente do ngo-service, app.py NÃO faz chamada de rede no import — boto3.resource(...)
e dynamodb.Table(...) só resolvem clientes/nomes localmente, a conexão real só
acontece em put_item/scan. Por isso dá pra mockar o DynamoDB inteiro com `moto`
em vez de mockar o SDK manualmente: criamos a tabela de teste com o schema real
(partition key volunteer_id, ver CLAUDE.md/módulo Terraform dynamodb) dentro do
contexto mockado, e só então importamos app.py — assim table.put_item/scan das
rotas batem contra a tabela fake do moto, não uma tabela real.

moto==4.2.14 (pinado por compatibilidade com boto3==1.26.50, ver requirements-test.txt)
usa decorators por serviço (mock_dynamodb) — o decorator unificado mock_aws só
existe a partir do moto 5.x, que exige boto3 >= 1.28.
"""
import sys
import importlib

import boto3
import pytest
from moto import mock_dynamodb


@pytest.fixture
def app_module(monkeypatch):
    monkeypatch.setenv("AWS_REGION", "us-east-1")
    monkeypatch.setenv("AWS_DYNAMODB_TABLE", "SolidaryTechVolunteersTest")
    monkeypatch.delenv("AWS_ENDPOINT_URL_DYNAMODB", raising=False)  # moto não intercepta corretamente com endpoint custom

    mock = mock_dynamodb()
    mock.start()

    client = boto3.client("dynamodb", region_name="us-east-1")
    client.create_table(
        TableName="SolidaryTechVolunteersTest",
        KeySchema=[{"AttributeName": "volunteer_id", "KeyType": "HASH"}],
        AttributeDefinitions=[{"AttributeName": "volunteer_id", "AttributeType": "S"}],
        BillingMode="PAY_PER_REQUEST",
    )

    sys.modules.pop("app", None)
    import app as module
    importlib.reload(module)
    module.app.testing = True

    yield module

    sys.modules.pop("app", None)
    mock.stop()


@pytest.fixture
def client(app_module):
    return app_module.app.test_client()


def test_health(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.get_json() == {"status": "ok", "service": "volunteer-service"}


def test_register_volunteer_missing_fields(client):
    resp = client.post("/volunteers", json={"name": "Sem email nem ngo_id"})
    assert resp.status_code == 400
    assert "error" in resp.get_json()


def test_register_volunteer_success(client):
    resp = client.post(
        "/volunteers",
        json={"name": "Voluntario Teste", "email": "vol@teste.com", "ngo_id": 1},
    )

    assert resp.status_code == 201
    body = resp.get_json()
    assert body["name"] == "Voluntario Teste"
    assert body["ngo_id"] == 1
    assert "volunteer_id" in body and len(body["volunteer_id"]) == 36  # formato uuid4


def test_register_volunteer_dynamodb_error(client, app_module, monkeypatch):
    def boom(*args, **kwargs):
        raise RuntimeError("falha simulada no DynamoDB")

    monkeypatch.setattr(app_module.table, "put_item", boom)

    resp = client.post(
        "/volunteers",
        json={"name": "Voluntario Erro", "email": "erro@teste.com", "ngo_id": 1},
    )

    assert resp.status_code == 500


def test_get_volunteers_by_ngo_success(client):
    client.post("/volunteers", json={"name": "Vol NGO 1 - A", "email": "a@teste.com", "ngo_id": 1})
    client.post("/volunteers", json={"name": "Vol NGO 1 - B", "email": "b@teste.com", "ngo_id": 1})
    client.post("/volunteers", json={"name": "Vol NGO 2", "email": "c@teste.com", "ngo_id": 2})

    resp = client.get("/volunteers/1")

    assert resp.status_code == 200
    body = resp.get_json()
    assert len(body) == 2
    # ngo_id vem como Decimal do DynamoDB (table.scan) e o jsonify do Flask
    # 2.2.2 serializa Decimal como str(o), não como número — contrato real
    # da API hoje é "1" (string), não 1 (int). Não é um artefato do moto.
    assert all(v["ngo_id"] == "1" for v in body)


def test_get_volunteers_by_ngo_invalid_id(client):
    resp = client.get("/volunteers/abc")
    assert resp.status_code == 404  # rejeitado pelo conversor <int:ngo_id> do Flask, antes do handler


def test_get_volunteers_by_ngo_error(client, app_module, monkeypatch):
    def boom(*args, **kwargs):
        raise RuntimeError("falha simulada no DynamoDB")

    monkeypatch.setattr(app_module.table, "scan", boom)

    resp = client.get("/volunteers/1")

    assert resp.status_code == 500
