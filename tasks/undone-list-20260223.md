# 미구현 기능 목록 (2026-02-23 기준)

> 실제 코드베이스 분석 기반. 백엔드 API는 대부분 구현되어 있으나, Admin UI 페이지와 일부 Gateway API가 미완성 상태.

---

## 1. Gateway API — 미구현

### 1-1. Text Completion (`/v1/completions`)

- **현황**: `internal/gateway/handler/completions.go` 파일 존재하나 TODO 상태, 라우터에 미등록
- **필요 작업**:
  - Non-streaming / Streaming 처리 로직 구현
  - 각 Provider 어댑터에 Completion 포맷 변환 추가 (현재 Chat 포맷만 존재)
  - 라우터 등록 (`/v1/completions`)
  - E2E 테스트 추가

### 1-2. Embeddings (`/v1/embeddings`)

- **현황**: `internal/gateway/handler/embeddings.go` 골격만 존재, 실제 처리 로직 미구현
- **필요 작업**:
  - Provider별 임베딩 API 호출 구현 (OpenAI, Gemini, Cohere 지원)
  - 임베딩 결과 캐싱 연동 (exact-match)
  - 비용 추적 (임베딩 토큰 단가 테이블)
  - 라우터 등록 (`/v1/embeddings`)
  - E2E 테스트 추가

---

## 2. Admin UI — 미구현 페이지

> 현재 구현된 페이지: `dashboard`, `keys`, `providers`, `orgs`, `guardrails`, `logs`, `usage`, `playground` (8개)
> 백엔드 API는 모두 존재함. UI만 없는 상태.

### 2-1. 라우팅 규칙 관리 (`/routing`)

- **백엔드**: `GET/PUT /admin/routing`, `GET/POST/PUT/DELETE /admin/routing/rules`, `POST /admin/routing/rules/dry-run`, `POST /admin/routing/reload`
- **필요 작업**:
  - 기본 라우팅 설정 편집 UI (기본 Provider, 폴백 체인)
  - 고급 규칙 목록/생성/편집/삭제 (조건: 모델명, 컨텍스트 길이, 시간대, 메타데이터)
  - DryRun 테스트 패널
  - api.ts에 `routing`, `routingRules` 객체 추가

### 2-2. 예산 관리 (`/budgets`)

- **백엔드**: `GET/POST /admin/budgets`, `GET/PUT/DELETE /admin/budgets/:id`, `POST /admin/budgets/:id/reset`
- **필요 작업**:
  - 예산 목록 (entity_type/entity_id별 현재 사용량 + 한도 바 차트)
  - 예산 생성/편집 폼 (Soft/Hard 한도, 기간 설정)
  - 수동 리셋 버튼
  - api.ts에 `budgets` 객체 추가

### 2-3. A/B 테스트 (`/ab-tests`)

- **백엔드**: `GET/POST /admin/ab-tests`, `GET/PUT/DELETE /admin/ab-tests/:id`, `GET /admin/ab-tests/:id/results`, `POST /admin/ab-tests/:id/conclude`
- **필요 작업**:
  - 실험 목록 (상태: draft/running/concluded)
  - 실험 생성 폼 (트래픽 비율, 비교 Provider 설정)
  - 결과 뷰 (응답시간, 비용, 에러율 비교 차트)
  - 승자 전환 버튼
  - api.ts에 `abTests` 객체 추가

### 2-4. 프롬프트 관리 (`/prompts`)

- **백엔드**: `GET/POST /admin/prompts`, `GET/PUT/DELETE /admin/prompts/:id`, `POST /admin/prompts/:id/versions`, `GET /admin/prompts/:id/versions`
- **필요 작업**:
  - 프롬프트 목록 (이름, 버전, 팀 소속)
  - 프롬프트 에디터 (마크다운 + 변수 {{placeholder}} 하이라이팅)
  - 버전 히스토리 뷰 (diff 비교)
  - api.ts에 `prompts` 객체 추가

### 2-5. 감사 로그 (`/audit`)

- **백엔드**: `GET /admin/audit-logs` (RLS: `SET LOCAL app.current_org_id` 필요)
- **필요 작업**:
  - 감사 로그 테이블 (actor, action, resource, timestamp, IP)
  - 필터링 (날짜 범위, action 타입, actor)
  - 내보내기 (CSV/JSON)
  - api.ts에 `auditLogs` 객체 추가

### 2-6. 알림 설정 (`/alerts`)

- **백엔드**: `GET/PUT /admin/alerts/config`, `POST /admin/alerts/test`, `GET /admin/alerts/history`
- **필요 작업**:
  - Slack/Email/Webhook 채널 설정 폼
  - 알림 조건 편집 (예산 임박 %, 에러율 임계값, 응답시간 임계값)
  - 테스트 발송 버튼
  - 최근 발송 히스토리 목록
  - api.ts에 `alerts` 객체 추가

### 2-7. 차지백/쇼백 리포트 (`/reports`)

- **백엔드**: `GET /admin/reports`, `POST /admin/reports/generate`, `GET /admin/reports/:id`
- **필요 작업**:
  - 기간별 팀/키별 비용 집계 테이블
  - 리포트 생성 트리거 (수동/자동 예약)
  - CSV/PDF 내보내기
  - api.ts에 `reports` 객체 추가

### 2-8. MCP Gateway 관리 (`/mcp`)

- **백엔드**: `GET/POST /admin/mcp/servers`, `GET/PUT/DELETE /admin/mcp/servers/:id`, `GET/POST /admin/mcp/policies`
- **필요 작업**:
  - 등록된 MCP 서버 목록 (WebSocket/Stdio/SSE 전송 타입 표시)
  - 서버 추가/편집 폼 (endpoint, transport, 인증 설정)
  - MCP 정책 편집 (허용 툴/리소스 목록)
  - api.ts에 `mcp` 객체 추가

---

## 3. Admin UI — 기존 페이지 기능 보완

### 3-1. Dashboard 페이지 보완

- 현재: 통계 카드 + 기본 차트
- 부재 기능:
  - Circuit Breaker 상태 요약 위젯 (Provider별 OPEN/CLOSED)
  - 활성 예산 초과 임박 경고 배너
  - 오늘의 캐시 히트율 지표

### 3-2. Sidebar 네비게이션 보완

- `Sidebar.tsx`에 새 페이지 링크 추가 필요:
  - Routing (라우팅 규칙)
  - Budgets (예산 관리)
  - A/B Tests
  - Prompts
  - Audit (감사 로그)
  - Alerts (알림)
  - Reports (리포트)
  - MCP

### 3-3. Keys 페이지 — Circuit Breaker 연동

- 현재: Virtual Key CRUD
- 추가 필요: `GET/POST /admin/circuit-breakers` 연동 → Provider별 서킷 브레이커 상태/리셋 UI

---

## 4. Admin UI API 클라이언트 (`lib/api.ts`) 미구현 객체

현재 구현: `keys`, `providers`, `providerKeys`, `logs`, `usage`, `orgs`, `teams`, `users`, `guardrails`

추가 필요:

| 객체명 | 대응 백엔드 경로 |
|---|---|
| `budgets` | `/admin/budgets/*` |
| `abTests` | `/admin/ab-tests/*` |
| `prompts` | `/admin/prompts/*` |
| `routing` | `/admin/routing`, `/admin/routing/reload` |
| `routingRules` | `/admin/routing/rules/*` |
| `auditLogs` | `/admin/audit-logs` |
| `alerts` | `/admin/alerts/*` |
| `reports` | `/admin/reports/*` |
| `mcp` | `/admin/mcp/*` |
| `circuitBreakers` | `/admin/circuit-breakers/*` |
| `rateLimits` | `/admin/rate-limits/*` |
| `residency` | `/admin/residency/*` |

---

## 5. 테스트 커버리지 — 보완 필요

### 5-1. Go E2E 테스트

- Embeddings 엔드포인트 테스트 (`/v1/embeddings`) — 구현 후 추가 필요
- Text Completion 테스트 (`/v1/completions`) — 구현 후 추가 필요
- A/B 테스트 시나리오 (트래픽 분할 동작 확인)
- Semantic Caching 시나리오 (제거됐으나 재추가 시)

### 5-2. Admin UI 테스트

- 현재 admin-ui에 테스트 파일 없음
- 신규 페이지 추가 시 컴포넌트 단위 테스트 권장 (vitest + testing-library)

---

## 6. 기타

### 6-1. Grok Provider 문서화

- `internal/provider/grok/` 구현 존재하나 README 및 config.yaml 예시에 미포함
- Provider 표에 Grok 추가 필요

### 6-2. Semantic Caching 재도입 여부

- migration 010에서 생성, migration 019에서 제거됨
- 임베딩 기반 유사도 검색 기능 재도입을 원할 경우 별도 검토 필요
- pgvector 확장은 DB에 이미 설치 가능

---

## 우선순위 제안

| 우선순위 | 항목 | 이유 |
|---|---|---|
| 높음 | Admin UI 미구현 8개 페이지 | 백엔드 이미 완성, UI만 없어서 운영 불가 |
| 높음 | `lib/api.ts` 클라이언트 보완 | UI 페이지 구현의 전제 조건 |
| 중간 | Embeddings API 구현 | 시맨틱 캐싱, RAG 파이프라인 연동에 필요 |
| 중간 | Text Completion API 구현 | 레거시 OpenAI 호환 클라이언트 지원 |
| 낮음 | Admin UI 기존 페이지 보완 | 없어도 운영 가능 |
| 낮음 | Grok 문서화 | 코드 있음, 문서만 없음 |
