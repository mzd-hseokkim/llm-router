# 03. 요청률 제한 (RPM/TPM)

## 목표
Virtual Key, 사용자, 팀, 조직 단위로 분당 요청 수(RPM)와 분당 토큰 수(TPM)를 제한한다. Redis Token Bucket 알고리즘을 사용하여 분산 환경에서 일관된 제한을 적용한다.

---

## 요구사항 상세

### 제한 차원
| 차원 | 키 포맷 | 예시 |
|------|---------|------|
| Virtual Key | `rl:key:{key_id}:{metric}:{window}` | rl:key:uuid:rpm:2026010100 |
| User | `rl:user:{user_id}:{metric}:{window}` | |
| Team | `rl:team:{team_id}:{metric}:{window}` | |
| Organization | `rl:org:{org_id}:{metric}:{window}` | |
| Model | `rl:model:{model}:{metric}:{window}` | |
| Global | `rl:global:{metric}:{window}` | |

### 제한 메트릭
- **RPM** (Requests Per Minute): 분당 요청 수
- **TPM** (Tokens Per Minute): 분당 토큰 수 (입력 + 출력)
- **TPM_IN** (Input Tokens Per Minute): 입력 토큰만
- **TPM_OUT** (Output Tokens Per Minute): 출력 토큰만
- **RPS** (Requests Per Second): 초당 요청 수 (버스트 제어)

### Token Bucket 알고리즘 (Redis Lua 스크립트)
```lua
-- Sliding Window Counter (1분 윈도우)
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])  -- 초 단위
local cost = tonumber(ARGV[3])    -- 소비할 단위 (요청=1, 토큰=N)
local now = tonumber(ARGV[4])

-- 오래된 항목 제거
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

-- 현재 윈도우 내 합계
local current = redis.call('ZCARD', key)  -- RPM용
-- 또는 SUM (TPM용)

if current + cost <= limit then
    redis.call('ZADD', key, now, now .. ':' .. math.random())
    redis.call('EXPIRE', key, window + 1)
    return {1, limit - current - cost}  -- {allowed, remaining}
else
    return {0, 0}  -- {denied, remaining}
end
```

### 계층적 제한 적용
```
요청 수신
  ↓
Global RPM 체크
  ↓
Organization RPM/TPM 체크
  ↓
Team RPM/TPM 체크
  ↓
Virtual Key RPM/TPM 체크
  ↓
요청 처리
  ↓
토큰 소비 기록 (응답 완료 후)
```

모든 계층 통과 후 처리, 하나라도 실패 시 429 반환.

### 토큰 사전 소비 (Pre-charge)
- 스트리밍 요청 시 출력 토큰을 미리 알 수 없음
- **방법 A**: 요청 시 입력 토큰만 차감, 응답 완료 후 출력 토큰 추가 차감
- **방법 B**: 요청 시 max_tokens 기준으로 예약, 실제 사용량으로 정산
- Phase 2에서는 방법 A 적용

### 429 응답 포맷
```json
HTTP/1.1 429 Too Many Requests
Retry-After: 30
X-RateLimit-Limit-Requests: 1000
X-RateLimit-Remaining-Requests: 0
X-RateLimit-Reset-Requests: 1704067260

{
  "error": {
    "message": "Rate limit exceeded: 1000 RPM. Retry after 30 seconds.",
    "type": "rate_limit_error",
    "code": "rate_limit_exceeded"
  }
}
```

### 동시 요청 제한
```go
type ConcurrencyLimiter struct {
    semaphore chan struct{}  // 최대 동시 요청 수
}

func (l *ConcurrencyLimiter) Acquire(ctx context.Context) error {
    select {
    case l.semaphore <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    default:
        return ErrTooManyRequests
    }
}
```

### Soft Limit / Hard Limit
- **Soft Limit**: 80% 도달 시 경고 알림 (Phase 3에서 구현)
- **Hard Limit**: 100% 도달 시 요청 차단

### 관리 API
```
GET /admin/rate-limits/:key_id         # 현재 사용량 조회
GET /admin/rate-limits/:key_id/reset   # 사용량 초기화
PUT /admin/rate-limits/:key_id         # 제한값 수정
```

---

## 기술 설계 포인트

- **원자적 Lua 스크립트**: Redis에서 체크 + 업데이트를 원자적으로 실행 (레이스 컨디션 방지)
- **파이프라이닝**: 여러 차원의 체크를 Redis 파이프라인으로 묶어 RTT 최소화
- **로컬 캐싱**: 제한값 조회는 인메모리 캐싱 (1분 TTL), 카운터는 Redis 직접 접근
- **우아한 초과**: 제한 직전까지는 허용하되 초과 즉시 차단 (no burst beyond limit)

---

## 의존성

- `phase1-mvp/05-virtual-key-auth.md` 완료
- `phase1-mvp/07-request-logging.md` 완료 (토큰 집계)
- Redis 연결

---

## 완료 기준

- [ ] RPM 100 설정 시 101번째 요청에서 429 반환 확인
- [ ] TPM 제한이 스트리밍 응답 완료 후 정확히 차감됨
- [ ] 분산 환경(2 인스턴스) 공유 RPM 제한 정확도 테스트
- [ ] 429 응답에 Retry-After, X-RateLimit-* 헤더 포함 확인
- [ ] 계층적 제한 (키 제한 < 팀 제한 < 조직 제한) 우선순위 동작 확인
- [ ] 동시 요청 제한 동작 확인

---

## 예상 산출물

- `internal/ratelimit/limiter.go`
- `internal/ratelimit/redis_limiter.go`
- `internal/ratelimit/hierarchical_limiter.go`
- `internal/ratelimit/concurrency_limiter.go`
- `internal/store/redis/rate_limit_store.go`
- `internal/gateway/middleware/rate_limit.go`
- `internal/gateway/handler/admin_rate_limits.go`
- `internal/ratelimit/redis_limiter_test.go`
