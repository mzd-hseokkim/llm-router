# 02. 프롬프트 관리 (Prompt Management)

## 목표
재사용 가능한 프롬프트 템플릿의 버전 관리, 배포 이력 추적, 팀 간 공유를 지원하는 프롬프트 관리 시스템을 구현한다. A/B 테스트와 품질 메트릭 추적으로 프롬프트 최적화를 가속한다.

---

## 요구사항 상세

### 프롬프트 템플릿 구조
```json
{
  "id": "uuid",
  "name": "customer-support-v2",
  "slug": "customer-support",
  "version": "2.1.0",
  "status": "active",  // draft | active | deprecated | archived
  "template": "You are a helpful customer support agent for {{company_name}}. Your tone is {{tone}}. Always respond in {{language}}.",
  "variables": [
    {"name": "company_name", "type": "string", "required": true},
    {"name": "tone", "type": "enum", "values": ["professional", "casual"], "default": "professional"},
    {"name": "language", "type": "string", "default": "English"}
  ],
  "model": "openai/gpt-4o",
  "parameters": {
    "temperature": 0.7,
    "max_tokens": 1024
  },
  "tags": ["customer-support", "production"],
  "team_id": "uuid",
  "created_by": "user_id",
  "created_at": "2026-01-01T00:00:00Z"
}
```

### 버전 관리
- Semantic Versioning (MAJOR.MINOR.PATCH)
- 변경 사유 기록 (changelog)
- 이전 버전으로 롤백 API
- 버전 간 diff 조회

```
GET /admin/prompts/:slug/versions          # 버전 목록
GET /admin/prompts/:slug/versions/:v       # 특정 버전 조회
GET /admin/prompts/:slug/diff?from=1.0&to=2.0  # 버전 비교
POST /admin/prompts/:slug/rollback/:v      # 롤백
```

### 프롬프트 렌더링 API
```
POST /admin/prompts/:slug/render
{
  "variables": {
    "company_name": "ACME Corp",
    "tone": "professional",
    "language": "Korean"
  }
}

→ {
    "rendered": "You are a helpful customer support agent for ACME Corp. Your tone is professional. Always respond in Korean.",
    "token_count": 32
  }
```

### 프롬프트를 API 요청에 주입
```
POST /v1/chat/completions
{
  "model": "openai/gpt-4o",
  "prompt_slug": "customer-support",  // 프롬프트 주입
  "prompt_variables": {
    "company_name": "ACME",
    "language": "Korean"
  },
  "messages": [
    {"role": "user", "content": "환불이 가능한가요?"}
  ]
}
```

### 프롬프트 메트릭 추적
- 프롬프트별 사용 횟수
- 평균 레이턴시
- 평균 토큰 수 / 비용
- 오류율

```sql
CREATE TABLE prompt_metrics (
    prompt_id       UUID,
    prompt_version  VARCHAR(20),
    date            DATE,
    request_count   INTEGER DEFAULT 0,
    avg_latency_ms  FLOAT,
    avg_tokens      FLOAT,
    total_cost_usd  DECIMAL(14,8),
    error_count     INTEGER DEFAULT 0,
    PRIMARY KEY (prompt_id, prompt_version, date)
);
```

### 프롬프트 허브 (팀 간 공유)
- 공개(public) / 팀 전용(team) / 비공개(private) 가시성
- 조직 내 검색 및 복사(fork)
- 태그 기반 필터링
- 사용 통계 기반 인기 프롬프트 추천

### DB 스키마
```sql
CREATE TABLE prompts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        VARCHAR(100) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    team_id     UUID,
    visibility  VARCHAR(20) DEFAULT 'team',
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE prompt_versions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id   UUID REFERENCES prompts(id),
    version     VARCHAR(20) NOT NULL,
    status      VARCHAR(20) NOT NULL,
    template    TEXT NOT NULL,
    variables   JSONB NOT NULL DEFAULT '[]',
    parameters  JSONB NOT NULL DEFAULT '{}',
    model       VARCHAR(200),
    changelog   TEXT,
    created_by  UUID,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(prompt_id, version)
);
```

---

## 기술 설계 포인트

- **Handlebars/Mustache 템플릿**: `{{variable}}` 구문으로 변수 주입
- **타입 검증**: 변수 타입 및 필수 여부 서버 측 검증
- **프롬프트 캐싱**: 렌더링 결과 캐싱 (동일 변수 조합)
- **비교 뷰**: 버전 간 사이드바이사이드 diff 뷰

---

## 의존성

- `phase2-stability/06-admin-api.md` 완료
- `phase3-enterprise/01-multi-tenancy.md` 완료

---

## 완료 기준

- [ ] 프롬프트 생성/버전 관리/롤백 API 동작 확인
- [ ] 템플릿 변수 주입 및 렌더링 정확성 확인
- [ ] API 요청 시 `prompt_slug`로 프롬프트 자동 주입 확인
- [ ] 버전 diff 조회 정확성 확인
- [ ] 프롬프트 메트릭 수집 확인

---

## 예상 산출물

- `internal/prompt/` (디렉토리)
  - `service.go`, `renderer.go`, `version.go`
- `internal/store/postgres/prompt_store.go`
- `migrations/014_create_prompts.sql`
- `internal/gateway/handler/admin/prompts.go`
- `internal/gateway/middleware/prompt_injection.go`
- `admin-ui/app/dashboard/prompts/page.tsx`
- `internal/prompt/renderer_test.go`
