# 05. 비용 추적 및 토큰 카운팅

## 목표
LLM API 요청별 정확한 비용을 계산하고 추적한다. 모델별 토큰 단가 테이블을 관리하고, Provider 응답의 토큰 사용량 또는 자체 카운터를 사용하여 정확한 비용을 산출한다. 다양한 집계 단위로 비용 분석을 제공한다.

---

## 요구사항 상세

### 모델 가격 테이블
```yaml
# config/models.yaml
models:
  - id: "openai/gpt-4o"
    provider: "openai"
    model_name: "gpt-4o"
    pricing:
      input_per_million_tokens: 2.50    # USD
      output_per_million_tokens: 10.00
      context_window: 128000
      currency: "USD"
      effective_from: "2024-05-13"

  - id: "anthropic/claude-opus-4-20250514"
    provider: "anthropic"
    model_name: "claude-opus-4-20250514"
    pricing:
      input_per_million_tokens: 15.00
      output_per_million_tokens: 75.00
      cached_input_per_million_tokens: 1.50  # Prompt Caching
      cache_write_per_million_tokens: 3.75

  - id: "google/gemini-2.0-flash"
    provider: "gemini"
    model_name: "gemini-2.0-flash"
    pricing:
      input_per_million_tokens: 0.075
      output_per_million_tokens: 0.30
      # 컨텍스트 크기에 따른 차등 요금 지원
```

### 비용 계산
```go
type CostCalculator struct {
    models map[string]*ModelPricing
}

func (c *CostCalculator) Calculate(model string, promptTokens, completionTokens int) float64 {
    pricing, ok := c.models[model]
    if !ok {
        return 0 // 모델 정보 없음
    }
    inputCost := float64(promptTokens) / 1_000_000 * pricing.InputPerMillion
    outputCost := float64(completionTokens) / 1_000_000 * pricing.OutputPerMillion
    return inputCost + outputCost
}
```

### 토큰 카운팅 전략 (우선순위)
1. **Provider 응답 `usage` 필드** (최우선): 가장 정확한 값
   - OpenAI: `usage.prompt_tokens`, `usage.completion_tokens`
   - Anthropic: `usage.input_tokens`, `usage.output_tokens`
   - Gemini: `usageMetadata.promptTokenCount`, `usageMetadata.candidatesTokenCount`

2. **자체 토크나이저** (fallback):
   - OpenAI 모델: `tiktoken` (go binding)
   - Anthropic 모델: Anthropic tokenizer 또는 근사 알고리즘
   - 일반: 문자 수 / 4 (대략적 근사, ±10%)

3. **스트리밍 응답 토큰 추적**:
   - `stream_options: {"include_usage": true}` 주입하여 마지막 청크에서 usage 추출
   - 미제공 시 자체 카운팅으로 폴백

### 집계 테이블 (일별 요약)
```sql
CREATE TABLE daily_usage (
    date                DATE NOT NULL,
    model               VARCHAR(200) NOT NULL,
    provider            VARCHAR(50) NOT NULL,
    virtual_key_id      UUID,
    user_id             UUID,
    team_id             UUID,
    org_id              UUID,
    request_count       INTEGER DEFAULT 0,
    prompt_tokens       BIGINT DEFAULT 0,
    completion_tokens   BIGINT DEFAULT 0,
    total_tokens        BIGINT DEFAULT 0,
    cost_usd            DECIMAL(14,8) DEFAULT 0,
    cache_hit_count     INTEGER DEFAULT 0,
    error_count         INTEGER DEFAULT 0,
    PRIMARY KEY (date, model, COALESCE(virtual_key_id, '00000000-0000-0000-0000-000000000000'))
);

CREATE INDEX idx_daily_usage_date ON daily_usage(date DESC);
CREATE INDEX idx_daily_usage_team ON daily_usage(team_id, date DESC);
```

### 비용 분석 API
```
GET /admin/usage/summary?period=monthly&entity=team&entity_id=uuid
→ {
    "total_requests": 50000,
    "total_tokens": 10000000,
    "total_cost_usd": 150.50,
    "by_model": [
      {"model": "openai/gpt-4o", "requests": 30000, "cost_usd": 100.00},
      {"model": "anthropic/claude-sonnet-4-20250514", "requests": 20000, "cost_usd": 50.50}
    ],
    "by_day": [...]
  }

GET /admin/usage/top-spenders?period=monthly&limit=10
```

### 비용 예측 및 알림
- 현재 소비 속도 기반 월말 예상 비용 계산
- 예상 비용이 예산의 80% 초과 시 알림 (Phase 3에서 구현)

### 마크업(Markup) 기능
```go
// 재판매 시나리오: Provider 원가에 마크업 적용
type MarkupConfig struct {
    Percentage float64  // 20 = 20% 마크업
    FixedUSD   float64  // 고정 추가 비용
}
```

---

## 기술 설계 포인트

- **배치 집계**: 실시간 개별 INSERT 대신 1분 배치로 daily_usage 업데이트
- **float 정밀도**: 비용 계산은 `decimal` 패키지 사용 (부동소수점 오차 방지)
- **토큰 카운팅 캐시**: 동일 메시지 반복 시 토크나이저 결과 캐싱

---

## 의존성

- `phase1-mvp/07-request-logging.md` 완료 (토큰 데이터 소스)
- `phase2-stability/04-budget-management.md` 완료 (비용 → 예산 차감)

---

## 완료 기준

- [ ] 각 모델의 토큰 단가가 정확히 적용된 비용 계산 확인
- [ ] Provider usage 필드 없을 때 자체 카운팅 폴백 동작 확인
- [ ] 일별 집계 테이블이 정확한 합계를 가짐
- [ ] 비용 분석 API 응답 정확도 확인
- [ ] 100만 요청 처리 후 비용 집계 성능 (< 1s 응답)

---

## 예상 산출물

- `internal/cost/calculator.go`
- `internal/cost/tokenizer.go`
- `internal/cost/aggregator.go`
- `internal/store/postgres/usage_store.go`
- `config/models.yaml` (업데이트)
- `migrations/006_create_daily_usage.sql`
- `internal/gateway/handler/admin_usage.go`
- `internal/cost/calculator_test.go`
