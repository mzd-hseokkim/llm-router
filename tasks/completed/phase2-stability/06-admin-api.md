# 06. Admin API

## 목표
Gateway의 전반적인 설정을 관리하는 RESTful Admin API를 구현한다. Virtual Key, Provider/모델, 라우팅 규칙, 사용자/팀 CRUD와 사용량 조회 API를 제공한다. 재시작 없이 설정 변경을 적용하는 핫 리로드를 지원한다.

---

## 요구사항 상세

### API 인증
- `Authorization: Bearer {master_key}` 또는 `Authorization: Bearer {admin_virtual_key}`
- Admin API는 별도 포트(기본 8081) 또는 `/admin` 경로 prefix로 분리
- IP 화이트리스트 옵션 (설정 가능)

### Virtual Key 관리
```
POST   /admin/keys                    # 키 생성
GET    /admin/keys                    # 키 목록 (페이지네이션)
GET    /admin/keys/:id                # 키 상세 + 사용량
PUT    /admin/keys/:id                # 키 수정
DELETE /admin/keys/:id                # 키 비활성화 (소프트 삭제)
POST   /admin/keys/:id/regenerate     # 키 재생성 (rotation)
GET    /admin/keys/:id/usage          # 사용량 통계
```

### Provider/모델 관리
```
POST   /admin/providers               # Provider 등록
GET    /admin/providers               # Provider 목록
PUT    /admin/providers/:id           # Provider 수정
DELETE /admin/providers/:id           # Provider 삭제
GET    /admin/providers/:id/health    # Provider 헬스 상태

POST   /admin/provider-keys           # Provider API Key 등록
GET    /admin/provider-keys           # Provider Key 목록 (마스킹)
PUT    /admin/provider-keys/:id       # Provider Key 수정
DELETE /admin/provider-keys/:id       # Provider Key 삭제
POST   /admin/provider-keys/:id/rotate  # Provider Key 순환

GET    /admin/models                  # 지원 모델 목록
POST   /admin/models                  # 모델 추가
PUT    /admin/models/:id              # 모델 정보 수정 (가격 등)
DELETE /admin/models/:id              # 모델 제거
```

### 라우팅 설정 관리
```
GET    /admin/routing                 # 현재 라우팅 설정 조회
PUT    /admin/routing                 # 전체 라우팅 설정 교체
PATCH  /admin/routing/rules/:id       # 특정 라우팅 규칙 수정
POST   /admin/routing/reload          # 설정 핫 리로드 트리거
```

### 사용자/팀/조직 관리
```
POST   /admin/organizations           # 조직 생성
GET    /admin/organizations           # 조직 목록
GET    /admin/organizations/:id       # 조직 상세
PUT    /admin/organizations/:id       # 조직 수정

POST   /admin/teams                   # 팀 생성
GET    /admin/teams                   # 팀 목록
GET    /admin/teams/:id               # 팀 상세
PUT    /admin/teams/:id               # 팀 수정
POST   /admin/teams/:id/members       # 팀원 추가
DELETE /admin/teams/:id/members/:uid  # 팀원 제거

POST   /admin/users                   # 사용자 생성
GET    /admin/users                   # 사용자 목록
GET    /admin/users/:id               # 사용자 상세
PUT    /admin/users/:id               # 사용자 수정
```

### 사용량/비용 조회
```
GET /admin/usage/summary
    ?period=daily|weekly|monthly
    &entity_type=key|user|team|org
    &entity_id=uuid
    &model=anthropic/claude-sonnet-4-20250514
    &from=2026-01-01&to=2026-01-31

GET /admin/usage/top-spenders?limit=10&period=monthly
GET /admin/logs?key_id=uuid&from=...&to=...&limit=100&page=1
GET /admin/logs/:request_id               # 특정 요청 상세
```

### 응답 페이지네이션
```json
{
  "data": [...],
  "pagination": {
    "total": 500,
    "page": 1,
    "per_page": 20,
    "total_pages": 25,
    "next_cursor": "uuid"
  }
}
```

### 핫 리로드 메커니즘
```go
// 설정 변경 이벤트 발행
type ConfigEvent struct {
    Type    string   // "routing_updated", "key_created", "model_added"
    Payload interface{}
}

// 각 컴포넌트가 이벤트 구독 후 설정 갱신
func (r *Router) OnConfigEvent(event ConfigEvent) {
    if event.Type == "routing_updated" {
        r.reloadRoutes()
    }
}
```
- Redis Pub/Sub 또는 인메모리 이벤트 버스
- 재시작 없이 라우팅 규칙, 모델 가격, 제한값 변경 가능

### OpenAPI 스펙 자동 생성
- `GET /admin/openapi.json` — OpenAPI 3.0 스펙
- `GET /admin/docs` — Swagger UI

---

## 기술 설계 포인트

- **CQRS 경향**: 쿼리(조회)와 커맨드(수정)를 분리하여 읽기 성능 최적화
- **낙관적 잠금**: 동시 수정 충돌 방지를 위한 `updated_at` 기반 ETag
- **감사 로그**: 모든 관리 작업은 감사 로그 기록 (누가, 무엇을, 언제)
- **에러 응답 일관성**: RFC 7807 Problem Details 형식

---

## 의존성

- `phase1-mvp/05-virtual-key-auth.md` 완료
- `phase1-mvp/06-provider-key-management.md` 완료
- `phase2-stability/03-rate-limiting.md` 완료
- `phase2-stability/04-budget-management.md` 완료

---

## 완료 기준

- [ ] 모든 CRUD 엔드포인트 정상 동작 확인
- [ ] 라우팅 설정 변경 후 재시작 없이 즉시 적용 확인
- [ ] 페이지네이션 동작 확인
- [ ] OpenAPI 스펙 자동 생성 확인
- [ ] 관리 작업이 감사 로그에 기록됨 확인
- [ ] 인증 없이 Admin API 접근 시 401 반환 확인

---

## 예상 산출물

- `internal/gateway/handler/admin/` (디렉토리)
  - `keys.go`, `providers.go`, `models.go`, `routing.go`
  - `users.go`, `teams.go`, `organizations.go`
  - `usage.go`, `logs.go`
- `internal/config/hot_reload.go`
- `internal/admin/openapi.go`
- `migrations/007_create_organizations_teams.sql`
