# 02. 가중 로드 밸런싱

## 목표
복수의 Provider 또는 동일 Provider의 복수 키에 대해 가중치 기반 트래픽 분산을 구현한다. 실시간 레이턴시 측정을 통해 동적으로 라우팅을 조정하는 적응형 로드 밸런싱도 지원한다.

---

## 요구사항 상세

### 로드 밸런싱 전략

**1. 가중 랜덤 (Weighted Random)**
```yaml
targets:
  - provider: openai
    model: gpt-4o
    weight: 70
  - provider: azure
    model: gpt-4o
    weight: 30
```
- weight 합계에 비례하여 트래픽 분산
- 구현: Weighted Reservoir Sampling (O(1))

**2. 라운드로빈 (Round-Robin)**
- 순서대로 균등 분배
- 가중치 기반 확장: weight=2인 타겟은 2회마다 1회 추가 선택

**3. 최소 레이턴시 (Least Latency)**
- EWMA(지수 가중 이동 평균) 기반 레이턴시 추적
- 최근 100개 요청의 P50 레이턴시 기준
- 레이턴시 낮은 Provider 우선 선택

**4. 최소 비용 (Least Cost)**
- 모델별 토큰 단가 기반 비용 추정
- 동일 품질 전제 시 비용 최저 Provider 선택

### EWMA 레이턴시 추적
```go
type LatencyTracker struct {
    mu      sync.RWMutex
    ewma    map[string]float64  // provider → ms
    alpha   float64             // 평활 계수 (기본 0.1)
}

func (t *LatencyTracker) Record(provider string, latencyMs float64) {
    t.mu.Lock()
    defer t.mu.Unlock()
    prev := t.ewma[provider]
    t.ewma[provider] = t.alpha*latencyMs + (1-t.alpha)*prev
}
```

### 라우팅 설정 구조
```yaml
routes:
  - match:
      model_prefix: "openai/"
    strategy: weighted_random
    targets:
      - provider: openai
        model: "{model}"   # 요청 모델 그대로 사용
        weight: 70
      - provider: azure_openai
        model: "{model}"
        weight: 30

  - match:
      model: "anthropic/claude-opus-4-20250514"
    strategy: least_latency
    targets:
      - provider: anthropic
        region: us-east-1
        weight: 100
      - provider: anthropic
        region: us-west-2
        weight: 100
```

### 로드 밸런서 상태 관리
- **전역 상태**: Redis (분산 환경 공유)
- **로컬 캐시**: 인메모리 5초 캐싱으로 Redis 부하 최소화
- **원자적 업데이트**: 라운드로빈 카운터는 Redis INCR 사용

### 동적 가중치 조정
- 에러율 높은 Provider 가중치 자동 감소
- 레이턴시 급증 Provider 가중치 감소
- 수동 가중치 조정 API 제공

### 관리 API
```
GET /admin/load-balancer/stats      # Provider별 트래픽 분배 현황
GET /admin/load-balancer/latency    # Provider별 레이턴시 통계
PUT /admin/load-balancer/weights    # 가중치 수동 조정
```

---

## 기술 설계 포인트

- **인터페이스 설계**: `LoadBalancer` 인터페이스로 전략 교체 가능
- **동시성 안전**: 레이턴시 추적 시 `sync.RWMutex` 사용
- **히스테리시스**: 가중치 변경이 너무 빈번하지 않도록 최소 변경 간격 설정
- **점진적 전환**: 새 Provider 추가 시 가중치 0 → 점진적 증가

---

## 의존성

- `phase1-mvp/03-provider-adapters.md` 완료
- `phase2-stability/01-failover-fallback.md` 완료
- Redis 연결

---

## 완료 기준

- [ ] 70/30 가중치 설정 시 1000 요청 중 약 700/300 분배 확인
- [ ] 레이턴시 기반 라우팅에서 빠른 Provider 우선 선택 확인
- [ ] 에러율 높은 Provider 가중치 자동 감소 확인
- [ ] 동시 요청 1000개 시 레이스 컨디션 없음 확인

---

## 예상 산출물

- `internal/gateway/router/load_balancer.go`
- `internal/gateway/router/strategies/weighted_random.go`
- `internal/gateway/router/strategies/least_latency.go`
- `internal/gateway/router/strategies/round_robin.go`
- `internal/telemetry/latency_tracker.go`
- `internal/gateway/handler/admin_load_balancer.go`
- `internal/gateway/router/load_balancer_test.go`
