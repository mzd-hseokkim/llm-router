# 07. 고급 라우팅

## 목표
단순 모델명 기반 라우팅을 넘어 지연시간, 비용, 조건부 메타데이터, 사용자 티어 등 다양한 요소를 고려한 지능적 라우팅을 구현한다. 라우팅 규칙은 실시간으로 변경 가능하며 코드 재배포 없이 적용된다.

---

## 요구사항 상세

### 라우팅 규칙 구조 (YAML)
```yaml
routes:
  # 규칙 1: 특정 메타데이터 기반
  - name: "premium-users"
    priority: 100
    match:
      metadata:
        user_tier: "premium"
    strategy: direct
    target:
      provider: anthropic
      model: claude-opus-4-20250514

  # 규칙 2: 모델 프리픽스 + 조직 기반
  - name: "team-a-openai"
    priority: 90
    match:
      model_prefix: "openai/"
      team_id: "team-uuid-123"
    strategy: weighted
    targets:
      - provider: openai
        model: "{model}"
        weight: 70
      - provider: azure
        model: "{model}"
        weight: 30

  # 규칙 3: 컨텍스트 길이 기반 라우팅
  - name: "long-context"
    priority: 80
    match:
      min_context_tokens: 100000
    strategy: direct
    target:
      provider: anthropic
      model: claude-opus-4-20250514  # 200K 컨텍스트 지원

  # 기본 규칙 (최저 우선순위)
  - name: "default"
    priority: 0
    match: {}
    strategy: least_cost
    targets:
      - provider: openai
        model: gpt-4o
      - provider: anthropic
        model: claude-sonnet-4-20250514
```

### 매칭 조건 타입
```go
type RouteMatch struct {
    // 모델 관련
    Model        string   // 정확 매칭
    ModelPrefix  string   // 프리픽스 매칭
    ModelRegex   string   // 정규식 매칭

    // 인증 컨텍스트
    KeyID        string
    UserID       string
    TeamID       string
    OrgID        string

    // 요청 메타데이터
    Metadata     map[string]string  // 키-값 매칭

    // 요청 특성
    MinContextTokens int   // 최소 컨텍스트 길이
    MaxContextTokens int
    HasTools         bool  // tool calling 사용 여부
    HasVision        bool  // 이미지 포함 여부

    // 시간 조건
    TimeRange   *TimeRange  // 특정 시간대 라우팅 (예: 새벽에는 저비용 모델)
}
```

### 라우팅 전략
```go
type Strategy string

const (
    StrategyDirect        Strategy = "direct"         // 단일 타겟
    StrategyWeighted      Strategy = "weighted"        // 가중 랜덤
    StrategyLeastLatency  Strategy = "least_latency"   // 최소 레이턴시
    StrategyLeastCost     Strategy = "least_cost"      // 최소 비용
    StrategyQualityFirst  Strategy = "quality_first"   // 품질 최우선
    StrategyFailover      Strategy = "failover"        // 순차 폴백
)
```

### 라우팅 엔진 구현
```go
type Router struct {
    rules []*RouteRule  // 우선순위 정렬된 규칙 목록
    mu    sync.RWMutex  // 핫 리로드용 락
}

func (r *Router) Route(ctx context.Context, req *ChatRequest) (*Target, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    for _, rule := range r.rules {
        if rule.Matches(ctx, req) {
            return rule.Strategy.Select(ctx, req, rule.Targets)
        }
    }
    return nil, ErrNoMatchingRoute
}
```

### 동적 비용 기반 라우팅
```go
func (s *LeastCostStrategy) Select(ctx context.Context, req *ChatRequest, targets []Target) (*Target, error) {
    estimatedTokens := estimateTokens(req.Messages)

    var cheapest *Target
    var lowestCost float64 = math.MaxFloat64

    for _, t := range targets {
        pricing := s.pricingRegistry.Get(t.Provider, t.Model)
        cost := pricing.EstimateCost(estimatedTokens, estimatedTokens/2) // input:output = 2:1 가정

        if cost < lowestCost {
            lowestCost = cost
            cheapest = &t
        }
    }
    return cheapest, nil
}
```

### 핫 리로드
```go
// 파일 변경 감지 (fsnotify) 또는 DB 변경 이벤트
func (r *Router) Watch(ctx context.Context, configSource ConfigSource) {
    for {
        select {
        case event := <-configSource.Changes():
            newRules := parseRules(event.Config)
            r.mu.Lock()
            r.rules = newRules
            r.mu.Unlock()
            log.Info("routing rules reloaded", "rule_count", len(newRules))
        case <-ctx.Done():
            return
        }
    }
}
```

### 라우팅 결정 로깅
```json
{
  "event": "routing_decision",
  "request_id": "req_xxx",
  "matched_rule": "premium-users",
  "strategy": "direct",
  "selected_target": {
    "provider": "anthropic",
    "model": "claude-opus-4-20250514"
  },
  "reason": "user_tier=premium matched rule priority=100"
}
```

### 관리 API
```
GET  /admin/routing/rules              # 현재 라우팅 규칙 조회
POST /admin/routing/rules              # 규칙 추가
PUT  /admin/routing/rules/:id          # 규칙 수정
DELETE /admin/routing/rules/:id        # 규칙 삭제
POST /admin/routing/test               # 라우팅 결정 시뮬레이션 (dry-run)
POST /admin/routing/reload             # 강제 리로드
```

### 라우팅 시뮬레이터 (dry-run)
```
POST /admin/routing/test
{
  "model": "openai/gpt-4o",
  "metadata": {"user_tier": "premium"},
  "team_id": "uuid",
  "estimated_tokens": 5000
}

→ {
    "matched_rule": "premium-users",
    "selected_target": {"provider": "anthropic", "model": "claude-opus-4-20250514"},
    "estimated_cost_usd": 0.015
  }
```

---

## 기술 설계 포인트

- **규칙 우선순위**: 높은 priority 숫자가 먼저 평가 (100 > 90 > 0)
- **규칙 캐싱**: 동일 요청 패턴의 라우팅 결과 단기 캐싱 (1초)
- **컴파일된 정규식**: 시작 시 모든 regex 사전 컴파일
- **원자적 교체**: 규칙 리로드 시 전체 규칙 목록을 원자적으로 교체

---

## 의존성

- `phase2-stability/02-load-balancing.md` 완료
- `phase2-stability/01-failover-fallback.md` 완료
- `phase2-stability/06-admin-api.md` 완료

---

## 완료 기준

- [ ] 우선순위 기반 규칙 매칭 정확도 테스트
- [ ] 메타데이터 조건 매칭 테스트
- [ ] 핫 리로드 시 진행 중인 요청 영향 없음 확인
- [ ] dry-run API로 라우팅 결정 미리 확인 가능
- [ ] 규칙 미매칭 시 기본 규칙 적용 확인

---

## 예상 산출물

- `internal/gateway/router/engine.go`
- `internal/gateway/router/rule.go`
- `internal/gateway/router/matcher.go`
- `internal/gateway/router/strategies/` (전략 구현체)
- `internal/gateway/handler/admin/routing.go`
- `config/routing.yaml` (예시 업데이트)
- `internal/gateway/router/engine_test.go`
- `internal/gateway/router/matcher_test.go`
