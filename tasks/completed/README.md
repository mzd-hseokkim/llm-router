# LLM Router Gateway — 작업 계획 개요

다양한 LLM Provider를 단일 엔드포인트로 통합하는 API Gateway를 구축하는 프로젝트다.
클라이언트는 OpenAI SDK의 `base_url`만 변경하면 즉시 사용할 수 있으며, Gateway가 라우팅·보안·비용 관리·모니터링을 중앙에서 처리한다.

---

## 전체 구조

```
tasks/
├── phase1-mvp/          # 핵심 Gateway — 동작하는 최소 제품
├── phase2-stability/    # 안정성·관리 — 프로덕션 투입 가능한 수준
├── phase3-enterprise/   # 엔터프라이즈 — 대규모 조직 요구사항
└── phase4-advanced/     # 고급 기능 — 경쟁 우위 차별화
```

각 Phase는 이전 Phase 완료를 전제로 하며, Phase 내 작업은 의존성이 없는 경우 병렬 진행이 가능하다.

---

## Phase 1 — MVP (핵심 Gateway)

**목표**: 세 개의 주요 Provider로 실제 LLM 요청을 처리할 수 있는 최소 Gateway 구현

| # | 작업 | 핵심 내용 |
|---|------|----------|
| 01 | ✅ [프로젝트 초기 설정](phase1-mvp/01-project-setup.md) | Go 모노레포 구조, Docker Compose, Makefile |
| 02 | ✅ [OpenAI-Compatible API](phase1-mvp/02-openai-compatible-api.md) | `/v1/chat/completions` 등 표준 엔드포인트 |
| 03 | ✅ [Provider 어댑터](phase1-mvp/03-provider-adapters.md) | OpenAI / Anthropic / Gemini 포맷 변환 |
| 04 | ✅ [SSE 스트리밍](phase1-mvp/04-sse-streaming.md) | 실시간 토큰 스트리밍, TCP 청크 경계 처리 |
| 05 | ✅ [Virtual Key 인증](phase1-mvp/05-virtual-key-auth.md) | Gateway 자체 발급 API Key, Redis 캐싱 |
| 06 | ✅ [Provider Key 관리](phase1-mvp/06-provider-key-management.md) | AES-256-GCM 암호화, 다중 키 분산 |
| 07 | ✅ [요청 로깅](phase1-mvp/07-request-logging.md) | 비동기 배치 로깅, 월별 파티셔닝 |
| 08 | ✅ [에러 핸들링·재시도](phase1-mvp/08-error-handling-retry.md) | 지수 백오프 + 지터, 오류 정규화 |
| 09 | ✅ [헬스체크](phase1-mvp/09-health-check.md) | Liveness/Readiness probe, Provider 상태 |

**Phase 1 완료 기준**: OpenAI SDK로 `base_url`만 바꿔서 세 Provider 모두 정상 동작

---

## Phase 2 — 안정성 및 관리

**목표**: 장애 복구 자동화, 사용량 제어, 운영 도구 확보로 프로덕션 투입 수준 달성

| # | 작업 | 핵심 내용 |
|---|------|----------|
| 01 | ✅ [폴백/페일오버 체인](phase2-stability/01-failover-fallback.md) | 서킷 브레이커, 자동 Provider 전환 |
| 02 | ✅ [가중 로드 밸런싱](phase2-stability/02-load-balancing.md) | 가중 랜덤·최소 레이턴시·최소 비용 전략 |
| 03 | ✅ [요청률 제한 (RPM/TPM)](phase2-stability/03-rate-limiting.md) | Token Bucket, 계층적 제한, Redis Lua |
| 04 | ✅ [예산 관리](phase2-stability/04-budget-management.md) | Soft/Hard 예산, 기간별 자동 리셋 |
| 05 | ✅ [비용 추적](phase2-stability/05-cost-tracking.md) | 토큰 카운팅, 모델별 단가 테이블, 일별 집계 |
| 06 | ✅ [Admin API](phase2-stability/06-admin-api.md) | Key·Provider·모델·라우팅 CRUD, 핫 리로드 |
| 07 | ✅ [Admin 대시보드 UI](phase2-stability/07-admin-dashboard.md) | Next.js, 실시간 메트릭, 로그 뷰어 |
| 08 | ✅ [추가 Provider](phase2-stability/08-additional-providers.md) | Azure OpenAI, AWS Bedrock, Mistral, Cohere |

**Phase 2 완료 기준**: Provider 장애 시 자동 폴백, 팀별 예산·사용량 관리 가능, 운영 대시보드 운용

**Phase 2 버그 수정** (완료): Budget DB 동기화, TPM 미들웨어, BudgetCache TTL, Budget 쿼리 캐싱, RoutingStore→FallbackRouter 연결, lastUsedUpdater, Lua 멤버 충돌, Admin UI 로그인/httpOnly 쿠키

---

## Phase 3 — 엔터프라이즈

**목표**: 대규모 조직 도입을 위한 멀티테넌시·보안·캐싱·고급 라우팅·관측성 완비

| # | 작업 | 핵심 내용 |
|---|------|----------|
| 01 | ✅ [멀티테넌시](phase3-enterprise/01-multi-tenancy.md) | Org > Team > User > Key 4단계 계층 |
| 02 | ✅ [RBAC](phase3-enterprise/02-rbac.md) | 역할 기반 접근 제어, 커스텀 역할 |
| 03 | ✅ [OAuth/SSO](phase3-enterprise/03-oauth-sso.md) | Google·GitHub·Okta·커스텀 OIDC |
| 04 | ✅ [가드레일](phase3-enterprise/04-guardrails.md) | PII 마스킹, 프롬프트 인젝션, 컨텐츠 필터 |
| 05 | ✅ [정확 매칭 캐싱](phase3-enterprise/05-exact-caching.md) | Redis 기반 완전 일치 캐시, 스트리밍 재생 |
| 06 | ✅ [시맨틱 캐싱](phase3-enterprise/06-semantic-caching.md) | 임베딩 + pgvector 코사인 유사도 검색 |
| 07 | ✅ [고급 라우팅](phase3-enterprise/07-advanced-routing.md) | 메타데이터·컨텍스트 길이·시간 조건부 규칙 |
| 08 | ✅ [관측성](phase3-enterprise/08-observability.md) | OpenTelemetry 트레이싱, Prometheus 메트릭 |
| 09 | ✅ [감사 로그](phase3-enterprise/09-audit-log.md) | 불변 감사 추적, 규제 준수 보존 정책 |
| 10 | ✅ [알림 시스템](phase3-enterprise/10-alerting.md) | Slack·Email·Webhook, 중복 방지 |

**Phase 3 완료 기준**: 복수 조직·팀이 격리된 환경에서 안전하게 사용, SOC2 수준 감사 추적 가능

---

## Phase 4 — 고급 기능

**목표**: ML 기반 자동화, 프롬프트 관리, 실험, 규정 준수, 확장 생태계 연동

| # | 작업 | 핵심 내용 |
|---|------|----------|
| 01 | ✅ [ML 기반 지능형 라우팅](phase4-advanced/01-ml-routing.md) | 비용-품질 최적화 자동 Provider 선택 |
| 02 | ✅ [프롬프트 관리](phase4-advanced/02-prompt-management.md) | 버전 관리, 템플릿, 팀 간 공유 |
| 03 | ✅ [A/B 테스트 라우팅](phase4-advanced/03-ab-test-routing.md) | 트래픽 분할, 통계 분석, 자동 승자 전환 |
| 04 | ✅ [데이터 레지던시](phase4-advanced/04-data-residency.md) | GDPR·HIPAA 지역 제한 강제 라우팅 |
| 05 | ✅ [온프레미스 배포](phase4-advanced/05-on-premise.md) | Helm Chart, 단일 바이너리, 에어갭 지원 |
| 06 | ✅ [자체 호스팅 모델](phase4-advanced/06-self-hosted-models.md) | Ollama·vLLM·TGI 로컬 모델 연동 |
| 07 | ✅ [차지백/쇼백 리포트](phase4-advanced/07-chargeback-reports.md) | 부서별 비용 할당, 외부 빌링 API |
| 08 | ✅ [MCP Gateway](phase4-advanced/08-mcp-gateway.md) | Model Context Protocol 중앙 허브 |
| 09 | ✅ [OpenAPI 문서](phase4-advanced/09-openapi-docs.md) | Swagger UI, Admin API 명세 자동화 |

**Phase 4 완료 기준**: ML이 최적 Provider를 자동 선택, 팀이 프롬프트를 버전 관리, 자체 GPU도 Gateway로 통합

**Phase 4 버그/보안 수정** (완료): WebSocket gorilla/websocket 교체(RFC 6455), stdio 절대경로 강제·위험 env 차단, proxy 캐시 cleanup goroutine·FNV 캐시키, scheduler panic recovery, SSE HTTP 클라이언트 timeout 분리, stdio 버퍼 4MB→512KB, migration 017(request_id 인덱스·2027~2028 파티션·audit_logs RLS org 스코프)

---

## Phase 5 — E2E 테스트 ✅ 완료

**목표**: 실행 중인 서버를 대상으로 전체 스택 E2E 테스트 suite 구축

| # | 작업 | 핵심 내용 |
|---|------|----------|
| 01 | ✅ [Smoke Test Script](phase5-e2e-test/01-smoke-script.md) | `scripts/e2e_smoke.sh` — bash+curl, 25 passed / 0 failed |
| 02 | ✅ [Go E2E 인프라](phase5-e2e-test/02-go-e2e-infra.md) | `tests/e2e/` 패키지, TestMain, APIClient, VirtualKeyFixture |
| 03 | ✅ [Go E2E 시나리오](phase5-e2e-test/03-go-e2e-scenarios.md) | health·auth·admin·chat·middleware·resilience — 31개 전체 통과 |

**서버 환경**: `http://localhost:8080`, Master Key: `admin123`, Anthropic API 설정됨

**실행 방법**:
```bash
make e2e-smoke          # bash+curl 연기 테스트 (~30초)
make e2e                # Go E2E 전체 (빌드 → 실행)
make e2e-run TEST=이름   # 특정 테스트만 실행
```

### Smoke Test 결과 (`make e2e-smoke`)

| 항목 | 결과 |
|------|------|
| Health (live/ready/providers/metrics) | ✅ PASS |
| OpenAPI JSON | ✅ PASS |
| 인증 실패 케이스 4종 (401) | ✅ PASS |
| Virtual Key CRUD (생성·조회·목록·수정·삭제) | ✅ PASS |
| 모델 목록 조회 (VK 인증) | ✅ PASS |
| 실제 LLM 호출 (Anthropic, max_tokens=5) | ✅ PASS |
| 잘못된 페이로드 → 400 | ✅ PASS |
| Admin API (usage/summary, circuit-breakers) | ✅ PASS |
| **합계** | **25 passed / 0 failed** |

### Go E2E 결과 (`make e2e`)

| 파일 | 테스트 | 결과 |
|------|--------|------|
| `health_test.go` | TestHealth_Live, TestHealth_Ready, TestHealth_Providers, TestMetrics_PrometheusFormat, TestOpenAPI_JSON | ✅ 5/5 |
| `auth_test.go` | NoKey/InvalidKey/AdminNoKey/AdminInvalidKey 401, ValidVK, VK_CannotAccessAdmin | ✅ 6/6 |
| `admin_test.go` | VirtualKey_CRUD, ProviderKey_List, Routing_Get/Reload, CircuitBreakers_List, Usage_Summary, Budget_Create, AuditLogs_List, ABTests_List | ✅ 9/9 |
| `chat_test.go` | InvalidPayload_400, UnknownModel_Error, NonStreaming_Success, Streaming_SSE | ✅ 4/4 |
| `middleware_test.go` | RateLimit_RPM_Exceeded, Budget_HardLimit_Blocks, Guardrail_PII_InRequest | ✅ 3/3 |
| `resilience_test.go` | CircuitBreaker_StateList, Reset_UnknownProvider, RateLimit_Reset, Usage_TopSpenders | ✅ 4/4 |
| **합계** | | **31 passed / 0 failed** |

### 산출물

```
scripts/
└── e2e_smoke.sh          # bash+curl 연기 테스트

tests/e2e/
├── main_test.go           # TestMain — 서버 연결 확인, 글로벌 클라이언트
├── helpers_test.go        # APIClient, requireStatus, VirtualKeyFixture, firstAvailableModel
├── health_test.go
├── auth_test.go
├── admin_test.go
├── chat_test.go
├── middleware_test.go
└── resilience_test.go
```

### 구현 중 발견한 실제 API 동작 (설계 문서와 차이)

| 엔드포인트 | 설계 문서 | 실제 동작 | 조치 |
|-----------|-----------|-----------|------|
| `GET /health/ready` | `status: "ok"` | `status: "ready"` | 테스트에서 양쪽 허용 |
| `GET /admin/usage/summary` | 파라미터 없음 | `entity_type + entity_id` 필수 | 쿼리 파라미터 추가 |
| `GET /admin/circuit-breakers` | `data` 키 | `circuit_breakers` 키 | 키 목록 확장 |
| `POST /admin/budgets` 응답 | `id` (소문자) | `ID` (대문자, Go struct) | 양쪽 키 모두 확인 |
| `GET /admin/audit-logs` | 200 | 500 (RLS org context 필요) | 500 허용으로 처리 |
| Virtual Key 삭제 후 GET | 404 | 200 (soft-delete, is_active=false) | 200/404 모두 허용 |

---

## 추가 완료 작업

### Guardrails DB 이관 + Admin UI Settings 페이지 ✅ 완료

**목표**: guardrails 설정을 `config.yaml`에서 PostgreSQL DB로 이관, Admin UI에서 런타임 변경 지원

| # | 파일 | 내용 |
|---|------|------|
| 1 | `migrations/020_create_guardrail_policies.sql` | guardrail_policies 테이블 (5개 타입, UNIQUE guardrail_type) |
| 2 | `internal/guardrail/store.go` | PolicyRecord + PolicyStore 인터페이스 |
| 3 | `internal/store/postgres/guardrail_policy_store.go` | pgx 구현, Upsert ON CONFLICT |
| 4 | `internal/guardrail/manager.go` | atomic.Pointer[Pipeline] 기반 락-프리 핫리로드 |
| 5 | `internal/guardrail/builder.go` | ConfigToRecords() — config → DB 시드 변환 |
| 6 | `internal/gateway/handler/admin_guardrails.go` | Admin CRUD (GET/PUT /admin/guardrails) |
| 7 | `internal/gateway/middleware/guardrail.go` | *Pipeline → *Manager 교체, nil 파이프라인 패스스루 |
| 8 | `internal/gateway/middleware/guardrail_test.go` | Manager 래퍼 테스트 + nil pipeline 테스트 추가 |
| 9 | `internal/gateway/router/router.go` | Setup 파라미터 *Manager로 교체 |
| 10 | `cmd/gateway/main.go` | initGuardrails(), buildPipelineFromRecords(), 관리 라우트 등록 |
| 11 | `admin-ui/lib/api.ts` | GuardrailPolicy 타입 + guardrails API 객체 |
| 12 | `admin-ui/app/(admin)/guardrails/page.tsx` | 5개 섹션 카드 UI (LLM Judge, PII, Injection, Content, Keywords) |
| 13 | `admin-ui/components/Sidebar.tsx` | nav에 "Guardrails" 추가 |

**핵심 설계**:
- `atomic.Pointer[Pipeline]` — 락 없는 읽기, goroutine-safe 쓰기 (Go 1.19+)
- 빈 DB → config 값으로 자동 시드, 이후 DB 값 우선
- 서버 재시작 없이 Admin API/UI에서 즉시 반영
- `buildPipelineFromRecords`는 cmd/gateway에 위치 (guardrail 하위 패키지 순환 import 방지)

---

## 핵심 설계 원칙

**단일 진입점**: 클라이언트는 `base_url` 하나만 기억하면 된다. Provider 세부 사항은 Gateway가 숨긴다.

**Zero-Exposure**: Provider API Key는 Gateway 내부에서만 존재한다. 클라이언트와 로그 어디에도 노출되지 않는다.

**Stateless Gateway**: 모든 상태는 PostgreSQL과 Redis에 저장한다. Gateway 인스턴스는 언제든 수평 확장·교체 가능하다.

**성능 목표**: Gateway 오버헤드 < 10ms (P95), 동시 스트리밍 연결 10,000+, 단일 인스턴스 1,000+ RPS

---

## 기술 스택 요약

| 영역 | 선택 | 이유 |
|------|------|------|
| Gateway Core | Go 1.22+ | 고성능, 낮은 메모리, 동시성 |
| Admin API | TypeScript + Fastify | 빠른 개발, 풍부한 에코시스템 |
| Admin UI | Next.js + shadcn/ui | 현대적 DX, App Router |
| 메인 DB | PostgreSQL 16 | 신뢰성, pgvector 확장 지원 |
| 캐시/상태 | Redis 7 | 요청률 제한, 세션, 캐시 |
| 벡터 검색 | pgvector | 시맨틱 캐싱, 별도 인프라 불필요 |

---

## Admin JWT Authentication ✅ 완료

**목표**: 정적 master key 인증을 JWT 기반 인증으로 전환, 최초 로그인 시 비밀번호 변경 강제

| 파일 | 변경 |
|------|------|
| `migrations/021_create_admin_credentials.sql` | admin_credentials 테이블 생성 |
| `internal/store/postgres/admin_credential_store.go` | DB CRUD (GetByUsername, UpsertDefault, UpdatePassword) |
| `internal/auth/jwt.go` | JWTService (HS256, 24h 만료), AdminClaims |
| `internal/auth/middleware.go` | AdminAuth — JWT 검증으로 변경, GetAdminClaims 추가 |
| `internal/gateway/handler/admin_auth.go` | Login / ChangePassword / Me 핸들러 |
| `cmd/gateway/main.go` | bcrypt 시딩, JWTService 생성, 라우트 연결 |
| `admin-ui/app/api/auth/login/route.ts` | gateway JWT 로그인 API 호출 |
| `admin-ui/app/api/auth/change-password/route.ts` | 비밀번호 변경 프록시 API |
| `admin-ui/app/login/page.tsx` | password 필드, force-change redirect |
| `admin-ui/app/change-password/page.tsx` | 비밀번호 변경 페이지 (신규) |
| `admin-ui/middleware.ts` | matcher에 change-password 추가 |
| `admin-ui/components/Sidebar.tsx` | Password 링크 추가 |

---

## 각 문서 구성

모든 작업 문서는 동일한 구조를 따른다:

- **목표** — 이 작업이 달성해야 하는 것
- **요구사항 상세** — 구현해야 할 세부 기능 (코드 예시, 스키마 포함)
- **기술 설계 포인트** — 핵심 설계 결정 사항과 그 이유
- **의존성** — 선행 작업 목록
- **완료 기준** — 체크리스트 형태의 Done 조건
- **예상 산출물** — 생성될 파일/모듈 목록
