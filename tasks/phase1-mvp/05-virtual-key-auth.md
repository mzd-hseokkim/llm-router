# 05. Virtual Key 기반 인증

## 목표
Gateway 자체 발급 API Key(Virtual Key) 시스템을 구현한다. 클라이언트는 Virtual Key를 사용하여 인증하며, Gateway는 이를 검증하고 해당 키에 연결된 권한, 예산, 모델 접근 정책을 적용한다. Provider API Key는 클라이언트에게 노출되지 않는다.

---

## 요구사항 상세

### Virtual Key 스키마
```sql
CREATE TABLE virtual_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key_hash    VARCHAR(64) NOT NULL UNIQUE,  -- SHA-256 해시
    key_prefix  VARCHAR(10) NOT NULL,          -- sk-xxxx (식별용)
    name        VARCHAR(255),
    user_id     UUID REFERENCES users(id),
    team_id     UUID REFERENCES teams(id),
    org_id      UUID REFERENCES organizations(id),

    -- 제한
    expires_at       TIMESTAMPTZ,
    budget_usd       DECIMAL(12,4),           -- NULL = 무제한
    rpm_limit        INTEGER,                 -- NULL = 무제한
    tpm_limit        INTEGER,                 -- NULL = 무제한

    -- 모델 접근 제어
    allowed_models   TEXT[],                  -- NULL = 전체 허용
    blocked_models   TEXT[],

    -- 메타데이터
    metadata    JSONB DEFAULT '{}',

    -- 상태
    is_active   BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);
```

### Key 포맷
- **형식**: `sk-{base62_random_40chars}` (OpenAI 호환 외관)
- **저장**: SHA-256 해시만 저장, 원문 키는 생성 시 1회만 반환
- **식별 프리픽스**: `sk-xx` (처음 6자) 으로 조회 최적화
- **총 길이**: 46자 (`sk-` + 43자)

### Key 생성 API
```
POST /admin/keys
Authorization: Bearer {master_key}

{
  "name": "Production API Key",
  "user_id": "uuid",
  "team_id": "uuid",
  "expires_at": "2026-12-31T00:00:00Z",
  "budget_usd": 100.0,
  "rpm_limit": 1000,
  "tpm_limit": 100000,
  "allowed_models": ["anthropic/claude-sonnet-4-20250514", "openai/gpt-4o"],
  "metadata": {"project": "my-app", "env": "production"}
}

응답:
{
  "id": "uuid",
  "key": "sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",  // 1회만 반환
  "key_prefix": "sk-xxxx",
  "name": "Production API Key",
  "created_at": "2026-01-01T00:00:00Z"
}
```

### 인증 미들웨어 로직
```
1. Authorization: Bearer {key} 헤더 추출
2. key 프리픽스로 DB 후보 레코드 조회 (인덱스 활용)
3. SHA-256(key) 계산 후 저장된 해시와 비교
4. 유효성 검사:
   a. is_active = true
   b. expires_at > NOW() (또는 NULL)
   c. 모델 접근 허용 (allowed_models 체크)
5. 검증 결과를 요청 컨텍스트에 주입
6. last_used_at 업데이트 (비동기, 성능 영향 최소화)
```

### 인증 캐싱 (Redis)
- 키 검증 결과를 Redis에 1분 TTL로 캐싱
- 캐시 키: `vk:auth:{key_hash}`
- 키 폐기/수정 시 캐시 즉시 무효화
- 캐시 히트 시 DB 조회 없이 즉시 처리

### Master Key / Admin Key
- **Master Key**: 시스템 초기화 시 환경변수로 설정 (`MASTER_KEY`)
  - Admin API에 대한 완전한 접근 권한
  - DB에 저장되지 않음
- **Admin Key**: DB 등록된 Virtual Key 중 `is_admin: true` 플래그를 가진 키
  - 조직/팀 범위 내의 관리 작업 허용

### 모델 접근 제어
```go
func (k *VirtualKey) CanAccessModel(model string) error {
    // blocked_models 우선 체크
    if slices.Contains(k.BlockedModels, model) {
        return ErrModelBlocked
    }
    // allowed_models가 설정된 경우 화이트리스트 체크
    if len(k.AllowedModels) > 0 && !slices.Contains(k.AllowedModels, model) {
        return ErrModelNotAllowed
    }
    return nil
}
```

### 보안 요구사항
- 키 원문은 생성 시 1회만 반환, 이후 조회 불가
- DB에는 해시만 저장 (bcrypt 불필요, SHA-256으로 충분 - 랜덤 키이므로)
- 타이밍 공격 방지: `crypto/subtle.ConstantTimeCompare` 사용
- 인증 실패 로그에 원문 키 절대 포함 금지

---

## 기술 설계 포인트

- **컨텍스트 키 타입**: 타입 안전한 컨텍스트 키 사용 (string 금지)
  ```go
  type contextKey int
  const virtualKeyCtxKey contextKey = 0
  ```
- **비동기 last_used_at 업데이트**: 채널 기반 배치 업데이트로 DB 부하 최소화
- **키 프리픽스 인덱스**: `key_prefix` 컬럼에 인덱스로 해시 비교 대상 최소화
- **Timing-safe 비교**: 해시 비교는 항상 고정 시간 비교 사용

---

## 의존성

- `01-project-setup.md` 완료 (DB, Redis 설정)
- `02-openai-compatible-api.md`의 미들웨어 체인

---

## 완료 기준

- [ ] Virtual Key 생성/조회/수정/폐기 API 정상 동작
- [ ] 유효한 Virtual Key로 `/v1/chat/completions` 요청 성공
- [ ] 만료된 키로 요청 시 401 반환
- [ ] 허용되지 않은 모델 요청 시 403 반환
- [ ] Redis 캐싱으로 동일 키 2회 요청 시 DB 조회 없음 확인
- [ ] 키 폐기 후 캐시 무효화 즉시 적용 확인
- [ ] 타이밍 공격 저항성 테스트 (응답 시간 편차 < 1ms)
- [ ] 인증 실패 로그에 키 원문 미포함 확인

---

## 예상 산출물

- `internal/auth/virtual_key.go`
- `internal/auth/middleware.go`
- `internal/auth/cache.go`
- `internal/store/postgres/virtual_key_store.go`
- `migrations/001_create_virtual_keys.sql`
- `migrations/002_create_users_teams.sql`
- `internal/gateway/handler/admin_keys.go`
- `internal/auth/virtual_key_test.go`
