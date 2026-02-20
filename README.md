# LLM Router Gateway

다양한 LLM Provider(OpenAI, Anthropic, Gemini 등)를 단일 OpenAI-compatible 엔드포인트로 통합하는 API Gateway.

## 로컬 개발 시작

### 사전 요구사항

- Go 1.22+
- Docker & Docker Compose

### 1. 인프라 기동

```bash
make docker-up
```

PostgreSQL(5432), Redis(6379), Adminer(8090)가 기동됩니다.

### 2. 서버 실행

```bash
make run
```

또는 hot reload (air 설치 필요):

```bash
air
```

### 3. 동작 확인

```bash
curl http://localhost:8080/ping
# {"status":"ok"}
```

### 4. 인프라 종료

```bash
make docker-down
```

## 빌드

```bash
make build
# → bin/gateway
```

## 테스트

```bash
make test
```

## 환경변수

`.env.example`을 `.env`로 복사 후 수정:

```bash
cp .env.example .env
```

모든 환경변수는 `LLM_ROUTER_` prefix를 사용하며, `config/config.yaml` 값을 오버라이드합니다.
