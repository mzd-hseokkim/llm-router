# 04. 예산 관리

## 목표
Virtual Key, 사용자, 팀, 조직 단위로 LLM 사용 예산 한도를 설정하고 집행한다. Soft Budget(경고)과 Hard Budget(차단) 이중 구조를 통해 과다 지출을 방지하고, 예산 기간(일/주/월) 자동 리셋을 지원한다.

---

## 요구사항 상세

### 예산 설정 구조
```sql
CREATE TABLE budgets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type     VARCHAR(20) NOT NULL,   -- 'key', 'user', 'team', 'org'
    entity_id       UUID NOT NULL,
    period          VARCHAR(10) NOT NULL,   -- 'hourly', 'daily', 'weekly', 'monthly', 'lifetime'
    soft_limit_usd  DECIMAL(12,4),          -- NULL = 미설정
    hard_limit_usd  DECIMAL(12,4),          -- NULL = 무제한
    current_spend   DECIMAL(12,8) DEFAULT 0,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_budgets_entity ON budgets(entity_type, entity_id, period);
```

### 예산 기간 자동 리셋
```go
type PeriodConfig struct {
    Type     string    // "daily", "weekly", "monthly"
    ResetAt  time.Time // 리셋 시각 (UTC)
}

// 기간별 period_end 계산
func nextPeriodEnd(period string, now time.Time) time.Time {
    switch period {
    case "daily":
        return startOfNextDay(now)
    case "weekly":
        return startOfNextMonday(now)
    case "monthly":
        return startOfNextMonth(now)
    }
}
```

스케줄러가 period_end 시점에 current_spend를 0으로 리셋, 새 기간 레코드 생성.

### 예산 체크 및 집행
```go
func (b *BudgetManager) CheckBudget(ctx context.Context, entityID UUID, entityType string) error {
    budget, err := b.store.GetBudget(ctx, entityID, entityType)
    if err != nil {
        return nil // 예산 미설정 = 무제한
    }

    // Hard Limit 체크
    if budget.HardLimitUSD != nil && budget.CurrentSpend >= *budget.HardLimitUSD {
        return ErrBudgetExceeded{
            Current: budget.CurrentSpend,
            Limit:   *budget.HardLimitUSD,
        }
    }

    // Soft Limit 체크 (경고만, 차단 안 함)
    if budget.SoftLimitUSD != nil && budget.CurrentSpend >= *budget.SoftLimitUSD {
        b.notify(ctx, entityID, SoftLimitReached)
    }

    return nil
}
```

### 비용 집계 (요청 완료 후)
```go
func (b *BudgetManager) RecordSpend(ctx context.Context, req *CompletedRequest) error {
    cost := b.costCalc.Calculate(req.Model, req.PromptTokens, req.CompletionTokens)

    // 원자적 업데이트 (Redis 집계 후 주기적으로 DB 동기화)
    b.redisIncr(ctx, req.KeyID, cost)
    b.redisIncr(ctx, req.UserID, cost)
    b.redisIncr(ctx, req.TeamID, cost)
    b.redisIncr(ctx, req.OrgID, cost)

    return nil
}
```

### Redis 기반 실시간 예산 추적
```
budget:{entity_type}:{entity_id}:{period} = "0.00150000"  (float64)
```
- 요청 완료 시 `INCRBYFLOAT` 원자적 업데이트
- DB는 10초마다 배치 동기화 (성능)
- Gateway 재시작 시 DB에서 Redis 재초기화

### 예산 초과 응답
```json
HTTP/1.1 429 Too Many Requests
{
  "error": {
    "message": "Budget exceeded: $100.00 monthly limit reached. Current spend: $100.50",
    "type": "budget_exceeded_error",
    "code": "budget_exceeded",
    "param": {
      "limit_usd": 100.0,
      "current_spend_usd": 100.50,
      "period": "monthly"
    }
  }
}
```

### 계층적 예산 집행
요청 시 모든 계층의 예산을 순서대로 체크:
1. Global 예산
2. Organization 예산
3. Team 예산
4. User 예산
5. Virtual Key 예산

하나라도 초과 시 요청 차단.

### 관리 API
```
GET /admin/budgets/:entity_type/:entity_id    # 예산 조회
POST /admin/budgets                           # 예산 설정
PUT /admin/budgets/:id                        # 예산 수정
DELETE /admin/budgets/:id                     # 예산 삭제
POST /admin/budgets/:id/reset                 # 수동 리셋
GET /admin/budgets/:entity_type/:entity_id/usage  # 사용량 조회
```

---

## 기술 설계 포인트

- **두 단계 체크**: 요청 시작 시 체크, 완료 후 집계 (스트리밍에서 출력 토큰 미확정 문제 해결)
- **근사적 Hard Limit**: 동시 요청이 많으면 정확한 Hard Limit 보장 어려움 → 10% 초과 허용 후 차단
- **기간 경계 처리**: 기간 리셋 시점에 진행 중인 요청의 비용 귀속 정책 (이전 기간 귀속)

---

## 의존성

- `phase1-mvp/05-virtual-key-auth.md` 완료
- `phase2-stability/05-cost-tracking.md` 완료 (비용 계산)
- Redis 연결

---

## 완료 기준

- [ ] Hard Budget 설정 시 초과 요청에서 429 반환 확인
- [ ] Soft Budget 도달 시 로그 경고 기록 확인
- [ ] 일간 예산 리셋이 자정에 자동 실행 확인
- [ ] 계층적 예산 체크 (키 → 팀 → 조직) 동작 확인
- [ ] Redis 장애 시 DB 폴백 동작 확인
- [ ] 예산 조회 API 정확한 사용량 반환 확인

---

## 예상 산출물

- `internal/budget/manager.go`
- `internal/budget/period.go`
- `internal/budget/scheduler.go` (자동 리셋)
- `internal/store/postgres/budget_store.go`
- `internal/store/redis/budget_cache.go`
- `migrations/005_create_budgets.sql`
- `internal/gateway/middleware/budget_check.go`
- `internal/gateway/handler/admin_budgets.go`
- `internal/budget/manager_test.go`
