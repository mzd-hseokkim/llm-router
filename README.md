# LLM Router Gateway

다양한 LLM Provider(OpenAI, Anthropic, Gemini 등)를 단일 OpenAI-compatible 엔드포인트로 통합하는 API Gateway.

클라이언트는 `base_url`만 변경하면 기존 OpenAI SDK를 그대로 사용할 수 있으며, Gateway가 라우팅·인증·캐싱·비용 관리·가드레일·멀티테넌시·관측성을 중앙에서 처리한다.

---

## 목차

- [주요 기능](#주요-기능)
- [사전 요구사항](#사전-요구사항)
- [빠른 시작](#빠른-시작)
- [Provider API Key 설정](#provider-api-key-설정)
- [Virtual Key 발급 및 사용](#virtual-key-발급-및-사용)
- [Admin 대시보드](#admin-대시보드)
- [Admin API 주요 기능](#admin-api-주요-기능)
- [빌드 및 테스트](#빌드-및-테스트)

---

## 주요 기능

| 기능 | 설명 |
|---|---|
| **Zero-Exposure 키 보안** | Provider API 키를 AES-256-GCM으로 암호화. 클라이언트에는 가상 키(Virtual Key)만 노출 |
| **자동 폴백 & 서킷 브레이커** | 가중 로드 밸런싱, Provider 장애 시 자동 전환. 이상 Provider는 서킷 브레이커로 격리 |
| **비용 제어 & 차지백** | 팀/키별 Soft/Hard 예산 한도, 실시간 비용 추적, 자동 차지백 리포트 |
| **정확 매칭 캐싱** | temperature=0 요청을 Redis에 캐싱. 동일 프롬프트는 모델 호출 없이 즉시 응답 |
| **시맨틱 캐싱** | 임베딩 + pgvector 코사인 유사도 검색으로 의미상 유사한 요청도 캐시 히트 |
| **가드레일** | PII 마스킹, 프롬프트 인젝션 차단, 콘텐츠 필터 — 토큰 소모 전 게이트웨이 레이어에서 처리 |
| **멀티테넌시 & RBAC** | Org > Team > User > Key 4단계 계층 구조, 역할 기반 접근 제어 |
| **OAuth/SSO** | Google·GitHub·Okta·커스텀 OIDC 인증 지원 |
| **고급 라우팅** | 메타데이터, 컨텍스트 길이, 시간대 조건 기반 라우팅 규칙 |
| **ML 기반 지능형 라우팅** | 비용-품질 최적화 자동 Provider 선택 |
| **A/B 테스트** | Provider 간 트래픽 분할, 통계 분석, 자동 승자 전환 |
| **프롬프트 관리** | 버전 관리, 템플릿, 팀 간 공유 |
| **완전한 관측성** | OpenTelemetry 트레이싱, Prometheus 메트릭, 불변 감사 로그, Slack/이메일/Webhook 알림 |
| **MCP Gateway** | Model Context Protocol 중앙 허브 |
| **자체 호스팅 모델** | Ollama·vLLM·TGI 로컬 모델 연동 |
| **데이터 레지던시** | GDPR·HIPAA 지역 제한 강제 라우팅 |
| **온프레미스 배포** | Helm Chart, 단일 바이너리, 에어갭 지원 |

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
| Ollama | `ollama/llama3`, `ollama/mistral` |
| vLLM | `vllm/llama-3.1-8b` |

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
| 가드레일 | `/guardrails` | PII 마스킹·프롬프트 인젝션·콘텐츠 필터 런타임 설정 |
| 조직/팀 관리 | `/orgs` | Org > Team > User 계층 관리 |
| A/B 테스트 | `/ab-tests` | Provider 간 트래픽 분할 및 통계 |
| 프롬프트 관리 | `/prompts` | 프롬프트 버전 관리·템플릿·팀 공유 |
| MCP Gateway | `/mcp` | Model Context Protocol 서버 등록·관리 |
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

# 서킷 브레이커 상태
curl http://localhost:8080/admin/circuit-breakers \
  -H "Authorization: Bearer admin123"
```

### 가드레일 설정

```bash
# 가드레일 정책 조회
curl http://localhost:8080/admin/guardrails \
  -H "Authorization: Bearer admin123"

# 가드레일 정책 업데이트 (런타임 즉시 반영)
curl -X PUT http://localhost:8080/admin/guardrails \
  -H "Authorization: Bearer admin123" \
  -H "Content-Type: application/json" \
  -d '{"guardrail_type": "pii_masking", "enabled": true, "config": {}}'
```

### 예산 관리

```bash
# 예산 생성
curl -X POST http://localhost:8080/admin/budgets \
  -H "Authorization: Bearer admin123" \
  -H "Content-Type: application/json" \
  -d '{"entity_type": "key", "entity_id": "<key_id>", "amount": 100.00, "period": "monthly"}'
```

### 사용량 통계

```bash
# 사용량 요약
curl "http://localhost:8080/admin/usage/summary?entity_type=key&entity_id=<key_id>" \
  -H "Authorization: Bearer admin123"

# 전체 사용량
curl "http://localhost:8080/admin/usage?period=day" \
  -H "Authorization: Bearer admin123"
```

### 감사 로그

```bash
curl http://localhost:8080/admin/audit-logs \
  -H "Authorization: Bearer admin123"
```

### OpenAPI 문서

서버 실행 중 브라우저에서 **http://localhost:8080/docs** 접속 (Swagger UI).

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
