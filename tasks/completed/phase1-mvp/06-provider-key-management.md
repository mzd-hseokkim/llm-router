# 06. Provider API Key 중앙 관리

## 목표
LLM Provider API Key를 Gateway에서 중앙 관리한다. 클라이언트에게 Provider Key를 노출하지 않고, Gateway가 요청 시 적절한 Provider Key를 선택하여 사용한다. 다중 키 등록, 암호화 저장, 키 그룹 관리를 지원한다.

---

## 요구사항 상세

### Provider Key 스키마
```sql
CREATE TABLE provider_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        VARCHAR(50) NOT NULL,    -- 'openai', 'anthropic', 'gemini'
    key_alias       VARCHAR(100) NOT NULL,   -- 사람이 읽기 쉬운 이름
    encrypted_key   BYTEA NOT NULL,          -- AES-256-GCM 암호화
    key_preview     VARCHAR(20),             -- 마지막 4자 (예: ...xyzw)

    -- 그룹화
    group_name      VARCHAR(100),            -- 'production', 'staging', 'team-a'
    tags            TEXT[],

    -- 상태
    is_active       BOOLEAN DEFAULT true,
    weight          INTEGER DEFAULT 100,     -- 로드 밸런싱 가중치

    -- 할당량 추적 (Phase 2에서 활용)
    monthly_budget_usd DECIMAL(12,4),
    current_month_spend DECIMAL(12,4) DEFAULT 0,

    -- 메타데이터
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ,
    use_count       BIGINT DEFAULT 0
);

CREATE INDEX idx_provider_keys_provider ON provider_keys(provider) WHERE is_active = true;
CREATE INDEX idx_provider_keys_group ON provider_keys(provider, group_name) WHERE is_active = true;
```

### 암호화 방식
- **알고리즘**: AES-256-GCM (인증 암호화)
- **키 관리**: 환경변수 `ENCRYPTION_KEY` (32바이트, base64 인코딩)
- **저장 포맷**: `nonce(12B) || ciphertext || tag(16B)` → base64 → BYTEA
- **키 순환**: 새 암호화 키로 재암호화 지원 (`/admin/rotate-encryption-key`)

### Provider Key 선택 전략
```go
type KeySelector interface {
    // provider 이름과 선택적 group으로 키 선택
    Select(ctx context.Context, provider, group string) (*ProviderKey, error)
}
```

**기본 전략 (Round-Robin with Weight)**
```go
// 가중치 기반 랜덤 선택
// weight=200인 키는 weight=100인 키보다 2배 더 많이 선택됨
func (s *WeightedSelector) Select(provider, group string) (*ProviderKey, error) {
    keys := s.getActiveKeys(provider, group)
    return weightedRandom(keys)
}
```

**단일 키 환경**: 등록된 키가 1개면 그것만 사용

### Admin API (Provider Key 관리)
```
# Provider Key 등록
POST /admin/provider-keys
{
  "provider": "openai",
  "key_alias": "openai-primary",
  "api_key": "sk-...",          // 암호화 후 저장
  "group_name": "production",
  "weight": 100
}

# Provider Key 목록 (마스킹된 상태)
GET /admin/provider-keys
GET /admin/provider-keys?provider=openai

# Provider Key 수정 (키 값 제외 속성)
PUT /admin/provider-keys/:id
{ "weight": 200, "is_active": false }

# Provider Key 삭제
DELETE /admin/provider-keys/:id

# Provider Key 값 업데이트 (키 순환)
PUT /admin/provider-keys/:id/rotate
{ "new_api_key": "sk-..." }
```

**응답에서 키 마스킹**: `api_key` 필드 절대 반환 금지, `key_preview: "...xyzw"` 만 노출

### 키 캐싱 (메모리)
- 활성 Provider Key를 메모리에 캐싱 (5분 TTL)
- DB 수정 이벤트 시 캐시 즉시 갱신 (pub/sub 또는 polling)
- 서비스 재시작 없이 키 추가/수정 반영

### 설정 파일 기반 Provider 등록 (대안)
```yaml
# config/config.yaml
providers:
  - name: openai
    api_key: ${OPENAI_API_KEY}      # 환경변수 참조
    base_url: https://api.openai.com/v1
  - name: anthropic
    api_key: ${ANTHROPIC_API_KEY}
  - name: gemini
    api_key: ${GEMINI_API_KEY}
```
- DB와 설정 파일 모두 지원, DB 설정이 우선
- 개발 환경에서 빠른 시작을 위한 설정 파일 방식

### Provider 상태 모니터링
- 키별 에러율 추적 (Redis)
- 에러율 임계값(30%) 초과 시 해당 키 일시 비활성화
- 헬스체크 통과 후 자동 재활성화

---

## 기술 설계 포인트

- **Zero-Exposure 원칙**: Provider Key는 요청 처리 시점에만 메모리에 존재, 로그에 절대 기록 금지
- **키 선택 캐시**: 동일 Provider 요청 시 매번 DB 조회 없이 인메모리 캐시 활용
- **원자적 키 순환**: 새 키 검증 성공 후 기존 키 비활성화 (atomic swap)
- **멀티 키 대기열**: 키가 여러 개인 경우 에러율 낮은 키 우선 선택

---

## 의존성

- `01-project-setup.md` 완료 (DB, 암호화 설정)
- `03-provider-adapters.md` 완료 (Provider별 키 사용 방식)
- `05-virtual-key-auth.md` 완료 (Admin 인증)

---

## 완료 기준

- [ ] Provider Key 등록/수정/삭제 API 정상 동작
- [ ] 등록된 Provider Key 사용하여 실제 Provider API 호출 성공
- [ ] DB에 키 원문 저장 안 됨 (암호화 확인)
- [ ] 응답에서 키 원문 미노출 확인
- [ ] 키 2개 등록 시 라운드로빈 동작 확인
- [ ] 환경변수 기반 키 설정도 동작 확인
- [ ] 키 비활성화 후 해당 키 사용 안 됨 확인

---

## 예상 산출물

- `internal/provider/key_manager.go`
- `internal/provider/key_selector.go`
- `internal/store/postgres/provider_key_store.go`
- `internal/crypto/aes.go`
- `migrations/003_create_provider_keys.sql`
- `internal/gateway/handler/admin_provider_keys.go`
- `internal/provider/key_manager_test.go`
