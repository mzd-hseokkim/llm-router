# 09. 감사 로그 (Audit Log)

## 목표
모든 관리 작업과 보안 관련 이벤트를 불변(immutable) 감사 로그로 기록한다. 누가, 언제, 무엇을 변경했는지 추적하며 규제 준수(GDPR, SOC2 등)와 보안 감사를 지원한다.

---

## 요구사항 상세

### 감사 로그 대상 이벤트
**인증/인가**
- 로그인 성공/실패
- 로그아웃
- Virtual Key 인증 실패
- 권한 거부 (403)

**Virtual Key 관리**
- 키 생성, 수정, 삭제, 비활성화
- 키 재생성(rotation)
- 키 권한 변경

**Provider/모델 관리**
- Provider Key 등록, 수정, 삭제
- 모델 가격 변경
- 라우팅 규칙 변경

**사용자/팀/조직 관리**
- 사용자 생성, 수정, 삭제
- 팀 멤버 추가/제거
- 역할 할당/회수
- 예산 설정 변경

**시스템 설정**
- 서킷 브레이커 수동 초기화
- 캐시 강제 삭제
- 설정 핫 리로드

**보안 이벤트**
- 가드레일 트리거 (프롬프트 인젝션 탐지, PII 감지)
- 비정상 접근 패턴 감지

### 감사 로그 스키마
```sql
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type      VARCHAR(100) NOT NULL,    -- 'key.created', 'user.deleted'
    action          VARCHAR(50) NOT NULL,     -- 'create', 'update', 'delete', 'login'
    actor_type      VARCHAR(20) NOT NULL,     -- 'user', 'api_key', 'system'
    actor_id        UUID,                     -- 수행자 ID
    actor_email     VARCHAR(255),             -- 수행자 이메일 (감사 가독성)
    ip_address      INET,
    user_agent      TEXT,
    resource_type   VARCHAR(50),              -- 'virtual_key', 'user', 'team'
    resource_id     UUID,
    resource_name   VARCHAR(255),
    changes         JSONB,                    -- {"before": {...}, "after": {...}}
    metadata        JSONB DEFAULT '{}',
    request_id      VARCHAR(36),
    org_id          UUID,
    team_id         UUID,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (timestamp);

-- 감사 로그는 삭제/수정 금지 (불변성)
-- RLS로 UPDATE, DELETE 차단
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_insert_only ON audit_logs
    FOR SELECT USING (true);
-- INSERT만 허용, UPDATE/DELETE 차단
```

### 이벤트 타입 명명 규칙
```
{resource_type}.{action}
예:
  virtual_key.created
  virtual_key.deleted
  virtual_key.rotated
  user.role_assigned
  team.member_added
  provider_key.rotated
  routing_rule.updated
  budget.exceeded
  auth.login_failed
  guardrail.pii_detected
```

### 변경 사항 기록 (Diff)
```json
{
  "changes": {
    "before": {
      "rpm_limit": 1000,
      "is_active": true
    },
    "after": {
      "rpm_limit": 2000,
      "is_active": true
    },
    "changed_fields": ["rpm_limit"]
  }
}
```

민감 필드(API Key, 비밀번호 등)는 `"[REDACTED]"` 로 마스킹.

### 감사 로그 미들웨어
```go
func AuditMiddleware(auditLog AuditLogger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 요청 처리
            recorder := newResponseRecorder(w)
            next.ServeHTTP(recorder, r)

            // 응답 완료 후 비동기 로깅
            if isAuditableEndpoint(r) {
                go auditLog.Record(context.Background(), AuditEvent{
                    EventType:   inferEventType(r),
                    Actor:       actorFromContext(r.Context()),
                    IPAddress:   realIP(r),
                    StatusCode:  recorder.statusCode,
                    // ...
                })
            }
        })
    }
}
```

### 보존 정책
- 감사 로그: 최소 1년 보존 (SOC2 요구사항)
- 보안 이벤트: 3년 보존
- 자동 아카이빙: S3 등 저비용 스토리지로 이동
- 삭제 금지: DB 레벨 정책으로 DELETE 차단

### 감사 로그 조회 API
```
GET /admin/audit-logs
    ?from=2026-01-01&to=2026-01-31
    &actor_id=uuid
    &event_type=virtual_key.deleted
    &resource_id=uuid
    &limit=100&page=1

GET /admin/audit-logs/security-events  # 보안 이벤트만
GET /admin/audit-logs/export?format=csv  # CSV 내보내기
```

---

## 기술 설계 포인트

- **비동기 기록**: 감사 로그 기록이 요청 처리 지연에 영향 주지 않도록 비동기 처리
- **데이터 무결성**: 로그 변조 감지를 위한 해시 체인 (선택적)
- **민감 데이터 마스킹**: API Key, 비밀번호 등은 감사 로그에서 마스킹

---

## 의존성

- `phase3-enterprise/02-rbac.md` 완료
- `phase3-enterprise/01-multi-tenancy.md` 완료

---

## 완료 기준

- [ ] 모든 관리 API 호출이 감사 로그에 기록됨
- [ ] 보안 이벤트(가드레일 트리거) 기록 확인
- [ ] 감사 로그 수정/삭제 불가 확인
- [ ] 변경 사항 diff 정확하게 기록 확인
- [ ] 감사 로그 CSV 내보내기 동작 확인

---

## 예상 산출물

- `internal/audit/logger.go`
- `internal/audit/events.go`
- `internal/audit/middleware.go`
- `internal/store/postgres/audit_store.go`
- `migrations/012_create_audit_logs.sql`
- `internal/gateway/handler/admin/audit_logs.go`
- `internal/audit/logger_test.go`
