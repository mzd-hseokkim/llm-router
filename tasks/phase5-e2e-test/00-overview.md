# Phase 5 — E2E 테스트

## 목표

실행 중인 Gateway 서버를 대상으로 전체 스택 E2E 테스트 suite를 구축한다.
단위 테스트로 커버되지 않는 HTTP 미들웨어 파이프라인, 인증 흐름, 실제 LLM 응답을 검증한다.

---

## 전제 조건 (서버는 이미 실행 중)

| 항목 | 값 |
|------|-----|
| Gateway URL | `http://localhost:8080` |
| Master Key | `admin123` |
| PostgreSQL | `localhost:5433` (user: llmrouter, pass: llmrouter, db: llmrouter) |
| Redis | `localhost:6380` |
| Anthropic API Key | config/config.yaml에 설정됨 (claude 모델 사용 가능) |
| OpenAI / Gemini | 미설정 (테스트 불가) |

서버 상태 확인:
```bash
curl -s http://localhost:8080/health/live | jq .
```

---

## 접근 방식

### 단계 1 — Smoke Script (즉시 실행 가능)
- 파일: `scripts/e2e_smoke.sh`
- 도구: bash + curl
- 목적: 서버가 제대로 동작하는지 빠르게 확인 (5분 이내)
- LLM 호출: 실제 Anthropic API 사용 (Virtual Key 임시 생성)
- 문서: `tasks/phase5-e2e-test/01-smoke-script.md`

### 단계 2 — Go E2E 테스트 (시나리오별 검증)
- 위치: `tests/e2e/`
- 빌드 태그: `//go:build e2e`
- 목적: 미들웨어 파이프라인, 오류 처리, 엣지 케이스 체계적 검증
- 실행: `go test -v -tags e2e ./tests/e2e/... -timeout 5m`
- 문서: `tasks/phase5-e2e-test/02-go-e2e-infra.md`, `03-go-e2e-scenarios.md`

---

## 예상 산출물

```
scripts/
└── e2e_smoke.sh           # curl 기반 연기 테스트 (Task 01)

tests/
└── e2e/
    ├── main_test.go        # TestMain — 서버 연결 확인
    ├── helpers_test.go     # API 클라이언트, 어설션 헬퍼
    ├── health_test.go      # /health/* 엔드포인트
    ├── auth_test.go        # Virtual Key 인증 / Admin 인증
    ├── admin_test.go       # Admin CRUD (Key, Provider, Routing, Budget)
    ├── chat_test.go        # /v1/chat/completions (스트리밍 포함)
    ├── middleware_test.go  # Rate limit, Budget 초과, Guardrail
    └── resilience_test.go  # Circuit breaker 상태, Fallback 관련
```

Makefile 타겟 추가:
```makefile
e2e-smoke:
	bash scripts/e2e_smoke.sh

e2e:
	go test -v -tags e2e ./tests/e2e/... -timeout 5m
```

---

## 테스트 시나리오 목록 (20개)

| # | 시나리오 | 방식 | 파일 |
|---|----------|------|------|
| 1 | GET /health/live → 200 | smoke + go | health |
| 2 | GET /health/ready → 200 (DB+Redis 연결) | smoke + go | health |
| 3 | GET /health/providers → 200 | smoke + go | health |
| 4 | GET /v1/models (키 없음) → 401 | smoke + go | auth |
| 5 | GET /v1/models (잘못된 키) → 401 | smoke + go | auth |
| 6 | GET /admin/logs (마스터 키 없음) → 401 | smoke + go | auth |
| 7 | Virtual Key CRUD (생성→조회→수정→삭제) | smoke + go | admin |
| 8 | GET /v1/models (유효 Key) → 200 + 목록 | smoke + go | auth |
| 9 | POST /v1/chat/completions (비스트리밍) | smoke + go | chat |
| 10 | POST /v1/chat/completions (SSE 스트리밍) | go | chat |
| 11 | POST /v1/chat/completions (잘못된 페이로드) → 400 | smoke + go | chat |
| 12 | GET /admin/usage/summary → 200 | smoke + go | admin |
| 13 | GET /admin/circuit-breakers → 200 | smoke + go | admin |
| 14 | POST /admin/budgets (생성) → 201 | go | admin |
| 15 | Rate limit 초과 → 429 + Retry-After | go | middleware |
| 16 | Budget 소진 → 차단 | go | middleware |
| 17 | PII 포함 프롬프트 → 마스킹/차단 | go | middleware |
| 18 | GET /metrics → Prometheus 포맷 확인 | smoke + go | health |
| 19 | GET /docs/openapi.json → 200 + 유효 JSON | smoke | health |
| 20 | POST /admin/routing/reload → 200 | go | admin |

---

## 작업 순서

1. `tasks/phase5-e2e-test/01-smoke-script.md` → `scripts/e2e_smoke.sh` 작성 및 실행
2. `tasks/phase5-e2e-test/02-go-e2e-infra.md` → `tests/e2e/` 인프라 파일 작성
3. `tasks/phase5-e2e-test/03-go-e2e-scenarios.md` → 시나리오별 테스트 구현
4. Makefile에 `e2e-smoke`, `e2e` 타겟 추가
5. `tasks/README.md`에 Phase 5 완료 표시

---

## 완료 기준

- [ ] `make e2e-smoke` — 실패 없이 통과
- [ ] `make e2e` — 전체 20개 시나리오 통과
- [ ] LLM 호출(Anthropic) 실제 응답 검증 포함
- [ ] SSE 스트리밍 chunk 수신 검증 포함
