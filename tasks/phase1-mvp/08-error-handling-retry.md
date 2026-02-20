# 08. 에러 핸들링 및 재시도

## 목표
Provider API 호출 실패 시 클라이언트에게 명확하고 일관된 오류 응답을 제공하고, 일시적 장애에 대해 지수 백오프 + 지터 기반 자동 재시도를 수행한다. 클라이언트 오류(4xx)와 서버/네트워크 오류(5xx, 타임아웃)를 명확히 구분하여 처리한다.

---

## 요구사항 상세

### 오류 분류 체계
```go
type GatewayErrorCode string

const (
    // 클라이언트 오류 (재시도 안 함)
    ErrInvalidRequest    GatewayErrorCode = "invalid_request_error"
    ErrAuthFailed        GatewayErrorCode = "authentication_error"
    ErrPermissionDenied  GatewayErrorCode = "permission_error"
    ErrModelNotFound     GatewayErrorCode = "model_not_found"
    ErrContextTooLong    GatewayErrorCode = "context_length_exceeded"

    // 재시도 가능한 오류
    ErrRateLimited       GatewayErrorCode = "rate_limit_error"     // 429
    ErrProviderError     GatewayErrorCode = "provider_error"       // 5xx
    ErrTimeout           GatewayErrorCode = "timeout_error"        // 타임아웃
    ErrNetworkError      GatewayErrorCode = "network_error"        // 연결 실패
    ErrOverloaded        GatewayErrorCode = "overloaded_error"     // 503
)
```

### 재시도 정책
```go
type RetryPolicy struct {
    MaxAttempts     int           // 기본: 3
    InitialDelay    time.Duration // 기본: 500ms
    MaxDelay        time.Duration // 기본: 30s
    Multiplier      float64       // 기본: 2.0 (지수 백오프)
    JitterFactor    float64       // 기본: 0.25 (±25% 랜덤)
    RetryOn         []int         // HTTP 상태 코드 목록
}

// 지연 계산: delay = min(InitialDelay * Multiplier^attempt, MaxDelay) * (1 ± jitter)
// 예: 500ms → 1000ms → 2000ms → 4000ms (최대 30s)
```

**재시도 대상 HTTP 상태코드**
- `429 Too Many Requests` — Rate Limited (Retry-After 헤더 존중)
- `500 Internal Server Error` — Provider 내부 오류
- `502 Bad Gateway` — 게이트웨이 오류
- `503 Service Unavailable` — 서비스 불가
- `504 Gateway Timeout` — 타임아웃

**재시도 제외 상태코드** (즉시 클라이언트에 반환)
- `400 Bad Request` — 잘못된 요청
- `401 Unauthorized` — 인증 실패
- `403 Forbidden` — 권한 없음
- `404 Not Found` — 리소스 없음
- `413 Payload Too Large` — 페이로드 초과
- `422 Unprocessable Entity` — 유효성 검사 실패

### Retry-After 헤더 처리
```go
func retryDelay(resp *http.Response, attempt int, policy RetryPolicy) time.Duration {
    // Provider가 Retry-After 헤더 제공 시 존중
    if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
        if d, err := parseRetryAfter(retryAfter); err == nil {
            return min(d, policy.MaxDelay)
        }
    }
    return calculateBackoff(attempt, policy)
}
```

### 오류 응답 포맷 (OpenAI 호환)
```json
{
  "error": {
    "message": "The model is currently overloaded. Please retry after 10 seconds.",
    "type": "overloaded_error",
    "code": "provider_overloaded",
    "param": null
  }
}
```

**HTTP 상태 코드 매핑**
| Gateway Error | HTTP Status |
|---------------|-------------|
| `invalid_request_error` | 400 |
| `authentication_error` | 401 |
| `permission_error` | 403 |
| `model_not_found` | 404 |
| `rate_limit_error` | 429 |
| `provider_error` | 502 |
| `timeout_error` | 504 |
| `network_error` | 503 |

### Provider 오류 정규화
각 Provider의 오류 응답을 공통 `GatewayError` 타입으로 변환:

**OpenAI 오류 → GatewayError**
```go
// openai: {"error": {"type": "invalid_request_error", "code": "context_length_exceeded"}}
// → GatewayError{Code: ErrContextTooLong, HTTPStatus: 400}
```

**Anthropic 오류 → GatewayError**
```go
// anthropic: {"type": "error", "error": {"type": "overloaded_error"}}
// → GatewayError{Code: ErrOverloaded, HTTPStatus: 503}
```

**Gemini 오류 → GatewayError**
```go
// gemini: {"error": {"code": 429, "status": "RESOURCE_EXHAUSTED"}}
// → GatewayError{Code: ErrRateLimited, HTTPStatus: 429}
```

### 스트리밍 오류 처리
- 스트리밍 시작 후 오류 발생: SSE 에러 이벤트 전송 후 스트림 종료
  ```
  data: {"error": {"message": "Stream interrupted", "type": "provider_error"}}
  data: [DONE]
  ```
- 스트리밍 시작 전 오류: 일반 JSON 오류 응답

### 컨텍스트 취소 처리
- 클라이언트 연결 종료 (`ctx.Done()`) → 현재 시도 즉시 취소, 재시도 안 함
- Gateway 셧다운 → graceful drain 중 새 재시도 없음

### 재시도 메트릭 기록
- 재시도 횟수별 카운터
- 재시도 성공/실패 비율
- Provider별 재시도 빈도

---

## 기술 설계 포인트

- **재시도 로직 분리**: 재시도 정책을 Provider 어댑터에서 분리하여 별도 레이어로 구현
- **멱등성 보장**: POST 요청의 재시도는 동일 결과를 보장해야 함 (LLM 요청은 본질적으로 비멱등이므로 주의)
- **재시도 예산**: 전체 재시도 시간이 클라이언트 타임아웃을 넘지 않도록 제어
- **에러 래핑**: `fmt.Errorf("%w", err)` 패턴으로 오류 체인 유지
- **패닉 복구**: HTTP 핸들러에서 패닉 발생 시 500 응답 반환 후 로그 기록

---

## 의존성

- `03-provider-adapters.md` 완료 (Provider별 오류 응답 포맷)
- `02-openai-compatible-api.md` 완료 (오류 응답 구조)

---

## 완료 기준

- [ ] Provider 429 응답 시 Retry-After 헤더 존중하여 재시도
- [ ] 3회 재시도 후 실패 시 클라이언트에 503 반환
- [ ] Provider 400 응답은 즉시 클라이언트에 전달 (재시도 없음)
- [ ] 지수 백오프 + 지터 동작 단위 테스트 통과
- [ ] 클라이언트 연결 종료 시 재시도 즉시 중단
- [ ] 오류 응답이 OpenAI-compatible 포맷인지 확인
- [ ] 각 Provider 오류 정규화 테스트 통과

---

## 예상 산출물

- `internal/gateway/retry/policy.go`
- `internal/gateway/retry/executor.go`
- `internal/gateway/errors/gateway_error.go`
- `internal/provider/openai/errors.go`
- `internal/provider/anthropic/errors.go`
- `internal/provider/gemini/errors.go`
- `internal/gateway/middleware/recovery.go`
- `internal/gateway/retry/executor_test.go`
- `internal/gateway/retry/policy_test.go`
