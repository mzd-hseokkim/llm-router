# 05. 정확 매칭 캐싱 (Exact-Match Cache)

## 목표
동일한 프롬프트, 모델, 파라미터 조합에 대한 LLM 응답을 캐싱하여 반복 요청 시 즉시 반환한다. Provider 비용 절감과 응답 지연시간 단축이 주요 목적이다.

---

## 요구사항 상세

### 캐시 키 생성
```go
type CacheKey struct {
    Model       string
    Messages    []Message
    Temperature float64
    MaxTokens   int
    TopP        float64
    Tools       []Tool   // tool calling 포함 시
    // stream은 캐시 키에 미포함 (캐시된 응답을 스트리밍으로 재생 가능)
}

func (k CacheKey) Hash() string {
    // 결정론적 JSON 직렬화 후 SHA-256 해시
    data, _ := json.Marshal(k)
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

**캐시 키 포함 항목**:
- 모델 식별자
- 전체 messages 배열 (순서 포함)
- `temperature`, `max_tokens`, `top_p` (0이 아닌 경우)
- `tools` 배열 (tool calling 사용 시)
- `response_format` (structured output)

**캐시 키 제외 항목**:
- `stream` (비스트리밍 캐시를 스트리밍으로 재생 가능)
- `user` (캐시 공유를 위해 제외)
- `metadata`
- Gateway 내부 필드

### 캐시 조회 전 조건 체크
캐싱하지 않는 경우:
- `temperature > 0` (비결정론적 출력)
  - 단, 명시적으로 `cache: true` 옵션 제공 시 허용
- `stream_options.include_usage: false`
- 요청에 `Cache-Control: no-cache` 헤더
- 요청에 `x-gateway-no-cache: true` 헤더
- `max_tokens`가 지정되지 않은 경우

### Redis 캐시 구조
```
cache:exact:{hash} → {
    "response": { ... OpenAI 응답 ... },
    "created_at": 1234567890,
    "model": "anthropic/claude-sonnet-4-20250514",
    "prompt_tokens": 100,
    "completion_tokens": 200,
    "cost_usd": 0.00450
}
TTL: 설정 기반 (기본 24시간)
```

### 캐시 저장 (응답 완료 후)
```go
func (c *Cache) Store(ctx context.Context, key string, resp *ChatResponse, ttl time.Duration) error {
    // 응답 크기 제한 (기본 1MB)
    if c.responseSize(resp) > c.maxSize {
        return ErrResponseTooLarge
    }
    data, _ := json.Marshal(resp)
    return c.redis.Set(ctx, "cache:exact:"+key, data, ttl).Err()
}
```

### 캐시 응답 헤더
```
X-Cache: HIT
X-Cache-Key: abc123...
X-Cache-Age: 3600
X-Cached-At: 2026-01-01T00:00:00Z
```

### 스트리밍 캐시 재생
비스트리밍 응답이 캐싱된 경우, 스트리밍 요청 시 청크로 분해하여 전송:
```go
func replayAsStream(w http.ResponseWriter, cached *ChatResponse) {
    // 응답을 word 단위로 분해하여 SSE 이벤트로 전송
    words := strings.Fields(cached.Choices[0].Message.Content)
    for i, word := range words {
        chunk := buildStreamChunk(word, i == len(words)-1)
        writeSSEEvent(w, chunk)
        time.Sleep(1 * time.Millisecond) // 자연스러운 스트리밍 시뮬레이션
    }
}
```

### 캐시 범위 설정
```yaml
cache:
  exact_match:
    enabled: true
    default_ttl: 86400    # 24시간 (초)
    max_ttl: 604800       # 7일 (초)
    max_response_size: 1048576  # 1MB
    cache_temperature_zero_only: true  # temperature=0 요청만 캐시
```

팀별 TTL 오버라이드 가능.

### 캐시 무효화
- **TTL 만료**: 자동 만료
- **수동 무효화**: `DELETE /admin/cache/exact/{hash}` 또는 `DELETE /admin/cache/exact?model=xxx`
- **모델 버전 변경**: 모델 ID 변경 시 캐시 자동 미적중 (캐시 키에 모델 ID 포함)

### 캐시 메트릭
- `cache_requests_total{type="exact", result="hit|miss"}`
- `cache_hit_ratio` (5분 이동 평균)
- `cache_entries_total`
- `cache_memory_bytes`

### 관리 API
```
GET  /admin/cache/stats                    # 캐시 통계 (히트율, 크기)
DELETE /admin/cache/exact                  # 전체 캐시 삭제
DELETE /admin/cache/exact?model={model}    # 특정 모델 캐시 삭제
GET  /admin/cache/exact/{hash}             # 특정 캐시 조회
```

---

## 기술 설계 포인트

- **키 결정론적 직렬화**: JSON 키 순서를 정렬하여 동일 요청이 항상 동일 해시 생성
- **압축**: 큰 응답은 Redis 저장 전 gzip 압축
- **캐시 스탬피드 방지**: 동일 캐시 미스 요청이 동시에 오면 하나만 Provider 호출 (singleflight)
- **캐시 비용 기록**: 캐시 히트 요청도 로그 기록 (cost_usd=0, cache_hit=true)

---

## 의존성

- `phase1-mvp/02-openai-compatible-api.md` 완료
- `phase1-mvp/07-request-logging.md` 완료

---

## 완료 기준

- [ ] 동일 요청 2회 시 두 번째부터 캐시 히트 응답 확인
- [ ] `X-Cache: HIT` 헤더 반환 확인
- [ ] 스트리밍 요청 시 캐시된 응답을 SSE로 재생 확인
- [ ] `temperature > 0` 요청은 캐싱 안 됨 확인
- [ ] TTL 만료 후 캐시 미스 → 새 Provider 요청 확인
- [ ] singleflight로 동시 동일 요청 시 Provider 1회만 호출 확인

---

## 예상 산출물

- `internal/cache/exact/cache.go`
- `internal/cache/exact/key.go`
- `internal/cache/exact/replay.go`
- `internal/store/redis/cache_store.go`
- `internal/gateway/middleware/cache.go`
- `internal/gateway/handler/admin/cache.go`
- `internal/cache/exact/cache_test.go`
- `internal/cache/exact/key_test.go`
