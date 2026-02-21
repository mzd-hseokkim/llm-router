# 01. 멀티테넌시

## 목표
Organization > Team > User > Key의 4단계 계층 구조를 완전히 구현한다. 각 계층에서 독립적인 예산, 요청률 제한, 모델 접근 제어를 적용하고, 계층 간 위임 관리(delegation)를 지원한다.

---

## 요구사항 상세

### 계층 구조
```
Organization (조직)
├── Team A
│   ├── User 1 → Virtual Key 1, Key 2
│   └── User 2 → Virtual Key 3
└── Team B
    └── User 3 → Virtual Key 4
```

### DB 스키마 (완전한 계층)
```sql
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,  -- URL friendly identifier
    settings    JSONB DEFAULT '{}',
    is_active   BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE teams (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) NOT NULL,
    settings    JSONB DEFAULT '{}',  -- 팀별 가드레일, 캐시 설정 등
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(org_id, slug)
);

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    name            VARCHAR(255),
    password_hash   VARCHAR(255),  -- 선택적 (SSO 사용 시 불필요)
    is_active       BOOLEAN DEFAULT true,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE team_members (
    team_id     UUID REFERENCES teams(id),
    user_id     UUID REFERENCES users(id),
    role        VARCHAR(50) NOT NULL DEFAULT 'developer',  -- RBAC
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

-- virtual_keys 테이블에 org_id, team_id, user_id 외래키 추가
```

### 계층별 설정 상속
각 계층의 설정은 상위 계층에서 상속, 하위 계층이 오버라이드:
```
Organization 설정
  ↓ (상속)
Team 설정 (오버라이드 가능)
  ↓ (상속)
User 설정 (오버라이드 가능)
  ↓ (상속)
Virtual Key 설정 (최종 적용)
```

**오버라이드 규칙**: 하위 설정이 상위 제한보다 더 엄격하거나 같아야 함 (완화 불가)
```go
// 조직 RPM=1000, 팀 RPM=500 → 팀은 최대 500 (1000으로 완화 불가)
// 조직 허용모델=[A,B,C], 팀 허용모델=[A] → 팀은 [A]만 사용 가능
func mergeSettings(org, team OrgSettings) TeamEffectiveSettings {
    return TeamEffectiveSettings{
        RPMLimit:      min(org.RPMLimit, team.RPMLimit),
        AllowedModels: intersection(org.AllowedModels, team.AllowedModels),
    }
}
```

### 계층별 사용량 집계
```sql
-- 조직, 팀, 사용자별 집계 뷰
CREATE VIEW org_usage AS
SELECT
    org_id,
    DATE_TRUNC('day', timestamp) as day,
    SUM(total_tokens) as tokens,
    SUM(cost_usd) as cost
FROM request_logs
GROUP BY org_id, day;
```

### 격리 보장
- 팀 A의 키는 팀 B의 데이터 접근 불가
- 팀 Admin은 자신의 팀 설정만 수정 가능
- 모든 쿼리에 `org_id`/`team_id` 필터 강제 적용 (Row Level Security)

### PostgreSQL Row Level Security (RLS)
```sql
ALTER TABLE virtual_keys ENABLE ROW LEVEL SECURITY;

CREATE POLICY team_isolation ON virtual_keys
    USING (team_id = current_setting('app.current_team_id')::uuid);
```

### 위임 관리
- Org Admin → Team Admin 지정
- Team Admin → 팀 내 키 생성, 멤버 관리, 예산/사용량 조회 가능
- 위임 범위는 자신의 권한 이하로만 가능

---

## 기술 설계 포인트

- **테넌트 컨텍스트**: 모든 요청에 `org_id`, `team_id` 컨텍스트 주입
- **쿼리 필터 강제**: 미들웨어에서 DB 세션 변수 설정으로 RLS 자동 적용
- **크로스 테넌트 방지**: 감사 로그에서 크로스 테넌트 접근 시도 감지

---

## 의존성

- `phase2-stability/06-admin-api.md` 완료
- `phase3-enterprise/02-rbac.md` 완료

---

## 완료 기준

- [ ] 4단계 계층 구조 CRUD API 정상 동작
- [ ] 팀 격리: 팀 A의 키로 팀 B 데이터 접근 시 403 반환
- [ ] 계층별 설정 상속 및 오버라이드 정확성 테스트
- [ ] 계층별 사용량 집계 정확도 확인
- [ ] Team Admin 권한 범위 제한 테스트

---

## 예상 산출물

- `internal/tenant/` (디렉토리)
  - `context.go`, `middleware.go`, `service.go`
- `internal/store/postgres/tenant_store.go`
- `migrations/008_multi_tenancy.sql`
- `internal/gateway/middleware/tenant.go`
- `internal/tenant/service_test.go`
