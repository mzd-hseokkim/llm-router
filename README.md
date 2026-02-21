# LLM Router Gateway

다양한 LLM Provider(OpenAI, Anthropic, Gemini 등)를 단일 OpenAI-compatible 엔드포인트로 통합하는 API Gateway.

클라이언트는 `base_url`만 변경하면 기존 OpenAI SDK를 그대로 사용할 수 있으며, Gateway가 라우팅·인증·비용 관리·모니터링을 중앙에서 처리한다.

---

## 목차

- [사전 요구사항](#사전-요구사항)
- [빠른 시작](#빠른-시작)
- [Provider API Key 설정](#provider-api-key-설정)
- [Virtual Key 발급 및 사용](#virtual-key-발급-및-사용)
- [Admin 대시보드](#admin-대시보드)
- [Admin API 주요 기능](#admin-api-주요-기능)
- [빌드 및 테스트](#빌드-및-테스트)

---

## 사전 요구사항

- **Go 1.22+**
- **Docker & Docker Compose**
- **Node.js 18+** (Admin UI 사용 시)
- **goose** (DB 마이그레이션)
  ```bash
  go install github.com/pressly/goose/v3/cmd/goose@latest
  ```

---

## 빠른 시작

### 1. 인프라 기동

PostgreSQL(포트 5433)과 Redis(포트 6380)를 Docker로 실행한다.

```bash
make docker-up
```

### 2. DB 마이그레이션

```bash
goose -dir migrations postgres \
  "postgres://llmrouter:llmrouter@localhost:5433/llmrouter?sslmode=disable" up
```

### 3. Gateway 서버 실행

```bash
make run
```

또는 환경변수를 `.env.local`에 저장해 두고 스크립트로 실행:

```bash
bash scripts/start.sh
```

서버가 기동되면 헬스체크로 확인:

```bash
curl http://localhost:8080/ping
# {"status":"ok"}
```

### 4. 인프라 종료

```bash
make docker-down
```

---

## Provider API Key 설정

`config/config.yaml`에 직접 입력하거나, **환경변수(권장)**로 설정한다.

```bash
# .env.local 파일 생성 (git 미추적)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
GEMINI_API_KEY=AIza...
```

`make run` 또는 `scripts/start.sh`는 `.env.local`을 자동으로 로드한다.

지원 Provider:

| Provider | 모델 예시 |
|---|---|
| OpenAI | `gpt-4o`, `gpt-4o-mini` |
| Anthropic | `claude-opus-4-6`, `claude-sonnet-4-6` |
| Gemini | `gemini-1.5-pro`, `gemini-1.5-flash` |
| Azure OpenAI | `azure/gpt-4o` |
| AWS Bedrock | `bedrock/claude-3-5-sonnet` |
| Mistral | `mistral/mistral-large` |
| Cohere | `cohere/command-r-plus` |

---

## Virtual Key 발급 및 사용

Gateway는 자체 Virtual Key를 발급한다. 클라이언트는 Provider 키 대신 이 키를 사용한다.

### Virtual Key 발급

마스터 키(`admin123`, `config/config.yaml`의 `gateway.master_key`)로 발급한다.

```bash
curl -s -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer admin123" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-app-key",
    "rpm_limit": 60,
    "tpm_limit": 100000
  }' | jq
```

응답에서 `key` 필드(`vk-...`)를 복사한다.

### curl로 LLM 호출

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer vk-..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [{"role": "user", "content": "안녕!"}]
  }' | jq
```

### Python OpenAI SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="vk-..."
)

response = client.chat.completions.create(
    model="claude-sonnet-4-6",
    messages=[{"role": "user", "content": "안녕!"}]
)
print(response.choices[0].message.content)
```

### 스트리밍

```python
with client.chat.completions.stream(
    model="gpt-4o",
    messages=[{"role": "user", "content": "스트리밍 테스트"}]
) as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)
```

### 모델 목록 조회

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer vk-..."
```

---

## Admin 대시보드

Gateway 운영·모니터링·설정을 위한 웹 UI다.

### 실행

```bash
cd admin-ui
npm install
npm run dev
```

브라우저에서 **http://localhost:3001** 접속.

### 로그인

| 항목 | 값 |
|---|---|
| 로그인 방식 | Master Key |
| Key | `admin123` (`config/config.yaml`의 `gateway.master_key`) |

> OAuth(Google, GitHub, Okta, Azure AD) 로그인도 지원한다. `config/config.yaml`에서 OAuth provider를 설정하면 로그인 화면에 버튼이 표시된다.

### 주요 화면

| 화면 | 경로 | 설명 |
|---|---|---|
| 대시보드 | `/dashboard` | 실시간 요청 수·비용·레이턴시·에러율 |
| API Keys | `/dashboard/keys` | Virtual Key 생성·삭제·한도 설정 |
| Providers | `/dashboard/providers` | Provider 키 등록·활성화·우선순위 |
| 라우팅 규칙 | `/dashboard/routing` | 조건부 라우팅 규칙 설정 |
| 사용량 로그 | `/dashboard/logs` | 요청별 상세 로그·토큰 수·비용 |
| 예산 관리 | `/dashboard/budgets` | 팀·키별 월간 예산 설정 |
| 감사 로그 | `/dashboard/audit` | 불변 감사 추적 |
| 알림 | `/dashboard/alerts` | Slack·Email·Webhook 알림 설정 |

---

## Admin API 주요 기능

모든 Admin API는 `Authorization: Bearer <master_key>` 헤더가 필요하다.

### Virtual Key 관리

```bash
# 전체 조회
curl http://localhost:8080/admin/keys \
  -H "Authorization: Bearer admin123"

# 생성
curl -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer admin123" \
  -H "Content-Type: application/json" \
  -d '{"name": "team-a-key", "rpm_limit": 100, "tpm_limit": 500000}'

# 삭제
curl -X DELETE http://localhost:8080/admin/keys/{key_id} \
  -H "Authorization: Bearer admin123"
```

### Provider 관리

```bash
# Provider 목록
curl http://localhost:8080/admin/providers \
  -H "Authorization: Bearer admin123"

# Provider 상태 (헬스체크)
curl http://localhost:8080/health/providers \
  -H "Authorization: Bearer admin123"
```

### 사용량 통계

```bash
# 전체 사용량
curl "http://localhost:8080/admin/usage?period=day" \
  -H "Authorization: Bearer admin123"
```

---

## 빌드 및 테스트

```bash
# 바이너리 빌드
make build
# → bin/gateway (또는 Windows: gateway.exe)

# 유닛 테스트
make test

# E2E 스모크 테스트 (서버 실행 중 상태에서)
make e2e-smoke

# 전체 E2E 테스트
make e2e
```

---

## 환경변수 레퍼런스

| 변수 | 기본값 | 설명 |
|---|---|---|
| `OPENAI_API_KEY` | — | OpenAI API Key |
| `ANTHROPIC_API_KEY` | — | Anthropic API Key |
| `GEMINI_API_KEY` | — | Google Gemini API Key |
| `MASTER_KEY` | `admin123` | Admin API 접근 키 |
| `ENCRYPTION_KEY` | — | DB 저장 키 암호화용 32바이트 base64 |
| `DATABASE_URL` | `postgres://llmrouter:llmrouter@localhost:5433/llmrouter?sslmode=disable` | PostgreSQL 연결 문자열 |
| `REDIS_ADDR` | `localhost:6380` | Redis 주소 |

`.env.local` 파일에 저장하면 `make run`·`scripts/start.sh` 실행 시 자동 로드된다.

> **주의**: `config/config.yaml`의 API Key나 Master Key는 예시값이다. 프로덕션에서는 반드시 환경변수로 주입하고 실제 값을 커밋하지 않는다.
