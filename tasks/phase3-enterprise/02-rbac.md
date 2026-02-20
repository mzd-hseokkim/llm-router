# 02. RBAC (역할 기반 접근 제어)

## 목표
Gateway의 모든 관리 기능과 API 접근을 역할(Role) 기반으로 제어한다. 사전 정의된 역할 외에 커스텀 역할 생성을 지원하고, 모델 수준까지 세분화된 권한 제어를 구현한다.

---

## 요구사항 상세

### 기본 역할 정의
| 역할 | 범위 | 권한 |
|------|------|------|
| `super_admin` | 전체 시스템 | 모든 작업 |
| `org_admin` | 조직 내 | 팀/사용자 관리, 예산 설정, 사용량 조회 |
| `team_admin` | 팀 내 | 키 생성/관리, 팀원 추가/제거, 팀 사용량 조회 |
| `developer` | 자신의 키 | 모델 사용, 자신의 사용량/비용 조회 |
| `viewer` | 읽기 전용 | 자신의 사용량 조회만 가능 |

### 권한(Permission) 목록
```go
const (
    // 키 관련
    PermCreateKey   Permission = "keys:create"
    PermReadKey     Permission = "keys:read"
    PermUpdateKey   Permission = "keys:update"
    PermDeleteKey   Permission = "keys:delete"

    // 팀/사용자 관련
    PermManageTeam  Permission = "teams:manage"
    PermManageUsers Permission = "users:manage"

    // 예산/사용량
    PermReadUsage   Permission = "usage:read"
    PermSetBudget   Permission = "budget:set"

    // Provider/모델 관련 (super_admin만)
    PermManageProviders Permission = "providers:manage"
    PermManageModels    Permission = "models:manage"

    // 시스템
    PermManageSystem Permission = "system:manage"
)
```

### 역할-권한 매핑 (기본값)
```go
var DefaultRolePermissions = map[Role][]Permission{
    "super_admin": {ALL_PERMISSIONS},
    "org_admin": {
        PermCreateKey, PermReadKey, PermUpdateKey, PermDeleteKey,
        PermManageTeam, PermManageUsers,
        PermReadUsage, PermSetBudget,
    },
    "team_admin": {
        PermCreateKey, PermReadKey, PermUpdateKey, PermDeleteKey,
        PermManageTeam,
        PermReadUsage,
    },
    "developer": {PermReadKey, PermReadUsage},
    "viewer":    {PermReadUsage},
}
```

### DB 스키마
```sql
CREATE TABLE roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID REFERENCES organizations(id),  -- NULL = 시스템 역할
    name        VARCHAR(50) NOT NULL,
    description TEXT,
    is_system   BOOLEAN DEFAULT false,  -- 시스템 기본 역할 (수정 불가)
    permissions JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE user_roles (
    user_id     UUID REFERENCES users(id),
    role_id     UUID REFERENCES roles(id),
    org_id      UUID REFERENCES organizations(id),
    team_id     UUID REFERENCES teams(id),  -- 팀 스코프 역할
    assigned_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'))
);
```

### RBAC 미들웨어
```go
func RequirePermission(perm Permission) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            user := auth.UserFromContext(r.Context())
            if !user.HasPermission(perm) {
                http.Error(w, "Forbidden", http.StatusForbidden)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// 라우터에 권한 적용
router.Post("/admin/keys", RequirePermission(PermCreateKey), handler.CreateKey)
router.Delete("/admin/keys/:id", RequirePermission(PermDeleteKey), handler.DeleteKey)
```

### 모델 수준 권한
```yaml
# 역할별 모델 접근 제어
model_access:
  developer:
    allowed: ["openai/gpt-4o-mini", "anthropic/claude-haiku-3-5"]
    blocked: ["openai/gpt-4o", "anthropic/claude-opus-4-20250514"]
  org_admin:
    allowed: ["*"]  # 전체 허용
```

### 커스텀 역할
- Org Admin이 자신의 조직 내에서 커스텀 역할 생성 가능
- 커스텀 역할은 기본 역할의 서브셋만 허용 (권한 상승 불가)
- UI에서 체크박스로 권한 선택

### 권한 캐싱
- 사용자 역할/권한을 Redis에 캐싱 (5분 TTL)
- 역할 변경 시 캐시 즉시 무효화

---

## 기술 설계 포인트

- **권한 체크 중앙화**: 모든 권한 체크는 `auth.Authorizer` 를 통해 단일 지점에서 처리
- **캐싱**: 역할 조회는 캐싱, 권한 체크는 인메모리에서 즉시 처리
- **감사 로그**: 권한 체크 실패는 보안 이벤트로 기록

---

## 의존성

- `phase3-enterprise/01-multi-tenancy.md` 완료
- `phase1-mvp/05-virtual-key-auth.md` 완료

---

## 완료 기준

- [ ] 5개 기본 역할의 권한 매핑 테스트 통과
- [ ] `developer` 역할로 Provider 관리 API 접근 시 403 반환
- [ ] 커스텀 역할 생성/적용 동작 확인
- [ ] 모델 수준 권한 제어 동작 확인
- [ ] 역할 변경 후 캐시 즉시 무효화 확인

---

## 예상 산출물

- `internal/auth/rbac/role.go`
- `internal/auth/rbac/permission.go`
- `internal/auth/rbac/authorizer.go`
- `internal/auth/rbac/middleware.go`
- `internal/store/postgres/role_store.go`
- `migrations/009_create_roles.sql`
- `internal/gateway/handler/admin/roles.go`
- `internal/auth/rbac/authorizer_test.go`
