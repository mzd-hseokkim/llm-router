# 01. 폴백/페일오버 체인

## 목표
Provider 장애 발생 시 사전 정의된 폴백 체인에 따라 자동으로 대체 Provider로 전환하는 페일오버 메커니즘을 구현한다. 서킷 브레이커 패턴을 적용하여 장애 Provider로의 반복적인 요청을 방지한다.

---

## 요구사항 상세

### 폴백 체인 설정
```yaml
# config/routing.yaml
routing:
  fallback_chains:
    - name: "default"
      chain:
        - provider: openai
          model: gpt-4o
        - provider: anthropic
          model: claude-sonnet-4-20250514
        - provider: gemini
          model: gemini-2.0-flash
      retry_on: [429, 500, 502, 503, 504]
      timeout_ms: 30000
```

### 폴백 트리거 조건
- HTTP 429 (Rate Limited)
- HTTP 500, 502, 503, 504 (서버 오류)
- 연결 타임아웃
- DNS 해석 실패
- TLS 핸드셰이크 실패
- **제외**: 4xx 클라이언트 오류 (400, 401, 403, 404, 413, 422)

### 서킷 브레이커 패턴
```go
type CircuitBreaker struct {
    State           CircuitState    // Closed, Open, HalfOpen
    FailureThreshold int            // 실패 임계값 (기본: 5회)
    SuccessThreshold int            // 복구 임계값 (기본: 2회)
    Timeout         time.Duration   // Open 상태 유지 시간 (기본: 60s)
}
```

**상태 전환**
```
Closed (정상)
  → 연속 N회 실패 → Open (차단)

Open (차단)
  → Timeout 경과 → HalfOpen (시험 허용)

HalfOpen (시험)
  → M회 성공 → Closed (복구)
  → 1회 실패 → Open (재차단)
```

**서킷 브레이커 상태 저장**: Redis (분산 환경에서 공유)
```
circuit:{provider}:state = "open"
circuit:{provider}:failures = 5
circuit:{provider}:last_failure = timestamp
```

### 폴백 실행 로직
```go
func (r *Router) ExecuteWithFallback(ctx context.Context, req Request, chain []Target) (Response, error) {
    var lastErr error
    for _, target := range chain {
        // 서킷 브레이커 확인
        if r.circuitBreaker.IsOpen(target.Provider) {
            continue // 다음 폴백으로
        }

        resp, err := r.execute(ctx, req, target)
        if err == nil {
            r.circuitBreaker.RecordSuccess(target.Provider)
            return resp, nil
        }

        if shouldFallback(err) {
            r.circuitBreaker.RecordFailure(target.Provider)
            lastErr = err
            continue // 다음 폴백으로
        }

        return nil, err // 재시도 불가 오류는 즉시 반환
    }
    return nil, fmt.Errorf("all fallbacks exhausted: %w", lastErr)
}
```

### 모델 호환성 매핑
폴백 시 다른 모델로 전환할 때 호환성 확인:
- 컨텍스트 길이 호환 여부
- Tool Calling 지원 여부
- Vision 지원 여부
- 호환 불가 시 적절한 오류 반환

### 쿨다운 기간
- Provider 장애 감지 후 일정 시간 라우팅 제외
- 기본 쿨다운: 60초 (설정 가능)
- 점진적 복구: HalfOpen → 트래픽 10% → 50% → 100%

### 폴백 이벤트 기록
```json
{
  "event": "fallback_triggered",
  "original_provider": "openai",
  "original_model": "gpt-4o",
  "fallback_provider": "anthropic",
  "fallback_model": "claude-sonnet-4-20250514",
  "reason": "provider_error",
  "http_status": 503,
  "attempt": 1
}
```

### 관리 API
```
GET /admin/circuit-breakers          # 전체 서킷 브레이커 상태
POST /admin/circuit-breakers/:provider/reset  # 수동 초기화
GET /admin/providers/status          # Provider 가용성 상태
```

---

## 기술 설계 포인트

- **전략 패턴**: `FallbackStrategy` 인터페이스로 체인, 라운드로빈 등 교체 가능
- **Redis 기반 공유 상태**: 멀티 인스턴스 환경에서 서킷 브레이커 상태 공유
- **컨텍스트 전파**: 폴백 시 원본 요청 컨텍스트 재사용 (만료 시간 포함)
- **응답 캐싱과 조합**: 폴백 전 캐시 확인으로 불필요한 폴백 감소

---

## 의존성

- `phase1-mvp/08-error-handling-retry.md` 완료
- `phase1-mvp/03-provider-adapters.md` 완료
- Redis 연결 (circuit breaker 상태 저장)

---

## 완료 기준

- [ ] Provider A 장애 시 Provider B로 자동 전환 확인
- [ ] 서킷 브레이커 Open → HalfOpen → Closed 전환 시나리오 테스트
- [ ] 폴백 이벤트 로그 기록 확인
- [ ] 분산 환경(2 인스턴스)에서 서킷 브레이커 공유 상태 확인
- [ ] 모든 폴백 소진 시 명확한 오류 반환 확인
- [ ] 쿨다운 기간 동작 확인

---

## 예상 산출물

- `internal/gateway/router/fallback.go`
- `internal/gateway/router/circuit_breaker.go`
- `internal/store/redis/circuit_breaker_store.go`
- `config/routing.yaml` (예시)
- `internal/gateway/handler/admin_circuit_breaker.go`
- `internal/gateway/router/fallback_test.go`
- `internal/gateway/router/circuit_breaker_test.go`
