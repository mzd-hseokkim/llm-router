# 03. OAuth/SSO 통합

## 목표
기업 환경에서 사용되는 OAuth 2.0 / OIDC 기반 SSO(Single Sign-On) 인증을 지원한다. Google Workspace, GitHub, Okta, Microsoft Entra ID 등 주요 IdP와 연동하여 기업 계정으로 Gateway에 로그인할 수 있게 한다.

---

## 요구사항 상세

### 지원 OAuth Providers
| Provider | 용도 |
|----------|------|
| Google Workspace | 기업 Google 계정 로그인 |
| GitHub | 개발자 GitHub 계정 로그인 |
| Microsoft Entra ID | Microsoft 365 기업 계정 |
| Okta | 엔터프라이즈 IdP |
| 커스텀 OIDC | 자체 IdP 지원 |

### OAuth 2.0 Authorization Code Flow
```
1. 클라이언트 → Gateway: GET /auth/login?provider=google
2. Gateway → 클라이언트: 302 Redirect → Google OAuth URL
3. 사용자 → Google: 로그인 + 동의
4. Google → Gateway: 302 Redirect → /auth/callback?code=xxx
5. Gateway → Google: code를 token으로 교환
6. Gateway → Google: token으로 사용자 정보 조회
7. Gateway: 로컬 사용자 찾기/생성, 세션 발행
8. Gateway → 클라이언트: 302 Redirect → /dashboard (쿠키 설정)
```

### 엔드포인트
```
GET  /auth/login?provider={provider}      # OAuth 로그인 시작
GET  /auth/callback?code=xxx&state=xxx   # OAuth 콜백 처리
POST /auth/logout                         # 로그아웃 (세션 삭제)
GET  /auth/me                            # 현재 사용자 정보
POST /auth/refresh                        # 세션 갱신
GET  /auth/providers                     # 활성화된 OAuth Provider 목록
```

### 세션 관리
```go
type Session struct {
    ID          string    `json:"id"`
    UserID      UUID      `json:"user_id"`
    CreatedAt   time.Time `json:"created_at"`
    ExpiresAt   time.Time `json:"expires_at"`
    OAuthToken  string    `json:"-"`  // 암호화 저장
}
```

- **세션 저장소**: Redis (TTL 24시간)
- **세션 쿠키**: `httpOnly`, `Secure`, `SameSite=Strict`
- **세션 키**: `session:{session_id}` → JSON
- **자동 갱신**: 만료 1시간 전 활성 요청 시 자동 연장

### OIDC 설정 (커스텀 IdP)
```yaml
auth:
  providers:
    - name: okta
      type: oidc
      issuer: https://company.okta.com
      client_id: ${OKTA_CLIENT_ID}
      client_secret: ${OKTA_CLIENT_SECRET}
      scopes: [openid, email, profile, groups]
      # 그룹 기반 역할 매핑
      group_role_mapping:
        "engineering": team_admin
        "all-employees": developer
```

### 그룹 기반 자동 역할 할당
```go
func mapGroupsToRoles(groups []string, mapping map[string]Role) []Role {
    var roles []Role
    for _, group := range groups {
        if role, ok := mapping[group]; ok {
            roles = append(roles, role)
        }
    }
    return roles
}
```

### JIT (Just-in-Time) 프로비저닝
- SSO 첫 로그인 시 사용자 자동 생성
- IdP의 그룹 정보 기반 팀 자동 배정
- 조직 도메인 기반 자동 조직 배정 (예: `@company.com` → company 조직)

### 서비스 계정 (M2M)
```
POST /auth/token
Content-Type: application/x-www-form-urlencoded
grant_type=client_credentials&client_id=xxx&client_secret=xxx

→ {"access_token": "...", "token_type": "Bearer", "expires_in": 3600}
```
- 자동화 환경(CI/CD, 서비스)을 위한 클라이언트 자격증명 플로우
- Virtual Key와 별개로 단기 JWT 토큰 발행

### PKCE 지원
- Public 클라이언트(SPA, 모바일)를 위한 PKCE (Proof Key for Code Exchange)
- `code_challenge`, `code_verifier` 처리

---

## 기술 설계 포인트

- **상태 파라미터**: CSRF 방지를 위한 `state` 파라미터 생성/검증 (Redis 저장)
- **토큰 안전 저장**: OAuth refresh token은 AES-256-GCM으로 암호화 후 저장
- **IdP 추상화**: 각 OAuth Provider를 동일한 인터페이스로 구현
- **라이브러리**: `golang.org/x/oauth2` + `coreos/go-oidc`

---

## 의존성

- `phase3-enterprise/02-rbac.md` 완료
- `phase3-enterprise/01-multi-tenancy.md` 완료

---

## 완료 기준

- [ ] Google OAuth로 로그인하여 대시보드 접근 성공
- [ ] GitHub OAuth 로그인 성공
- [ ] 커스텀 OIDC (Keycloak으로 테스트) 로그인 성공
- [ ] 그룹 기반 역할 자동 할당 동작 확인
- [ ] JIT 프로비저닝으로 신규 사용자 자동 생성 확인
- [ ] 세션 만료 시 재로그인 요청 확인
- [ ] CSRF 방지 (state 파라미터) 테스트

---

## 예상 산출물

- `internal/auth/oauth/` (디렉토리)
  - `provider.go`, `google.go`, `github.go`, `oidc.go`
- `internal/auth/session/session.go`
- `internal/store/redis/session_store.go`
- `internal/gateway/handler/auth.go`
- `migrations/010_create_oauth_sessions.sql`
- `admin-ui/app/login/page.tsx` (OAuth 버튼 UI)
