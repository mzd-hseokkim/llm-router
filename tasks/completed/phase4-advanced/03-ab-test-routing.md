# 03. A/B 테스트 라우팅

## 목표
트래픽의 일정 비율을 다른 모델 또는 프롬프트 버전으로 분배하여 체계적인 A/B 테스트를 수행한다. 테스트 결과를 통계적으로 분석하여 최적 모델/프롬프트를 선택하는 의사결정을 지원한다.

---

## 요구사항 상세

### A/B 테스트 설정
```json
{
  "id": "uuid",
  "name": "gpt4o-vs-claude-sonnet",
  "status": "running",
  "traffic_split": [
    {"variant": "control", "model": "openai/gpt-4o", "weight": 50},
    {"variant": "treatment", "model": "anthropic/claude-sonnet-4-20250514", "weight": 50}
  ],
  "target": {
    "team_ids": ["uuid"],  // 대상 팀 (null = 전체)
    "sample_rate": 0.1     // 10% 트래픽만 테스트 참여
  },
  "success_metrics": ["latency_p95", "cost_per_token", "error_rate"],
  "start_at": "2026-01-01T00:00:00Z",
  "end_at": "2026-01-14T00:00:00Z",
  "min_samples": 1000,
  "confidence_level": 0.95
}
```

### 사용자 할당 (Consistent Hashing)
```go
// 동일 사용자/키는 동일 Variant에 할당 (세션 일관성)
func assignVariant(test *ABTest, entityID string) string {
    hash := fnv.New32a()
    hash.Write([]byte(test.ID + entityID))
    bucket := hash.Sum32() % 100

    cumulative := uint32(0)
    for _, split := range test.TrafficSplit {
        cumulative += uint32(split.Weight)
        if bucket < cumulative {
            return split.Variant
        }
    }
    return test.TrafficSplit[0].Variant  // 기본값
}
```

### 메트릭 수집
```sql
CREATE TABLE ab_test_results (
    test_id         UUID,
    variant         VARCHAR(50),
    request_id      UUID,
    timestamp       TIMESTAMPTZ,
    model           VARCHAR(200),
    latency_ms      INTEGER,
    prompt_tokens   INTEGER,
    completion_tokens INTEGER,
    cost_usd        DECIMAL(12,8),
    error           BOOLEAN DEFAULT false,
    finish_reason   VARCHAR(20),
    PRIMARY KEY (test_id, request_id)
);
```

### 통계 분석 (결과 조회)
```json
{
  "test_id": "uuid",
  "status": "completed",
  "winner": "treatment",
  "results": {
    "control": {
      "model": "openai/gpt-4o",
      "samples": 5420,
      "latency_p95_ms": 3200,
      "avg_cost_per_request": 0.0180,
      "error_rate": 0.012
    },
    "treatment": {
      "model": "anthropic/claude-sonnet-4-20250514",
      "samples": 5380,
      "latency_p95_ms": 2800,
      "avg_cost_per_request": 0.0120,
      "error_rate": 0.008
    }
  },
  "statistical_significance": {
    "latency": {"p_value": 0.001, "significant": true, "improvement_pct": -12.5},
    "cost": {"p_value": 0.0001, "significant": true, "improvement_pct": -33.3},
    "error_rate": {"p_value": 0.02, "significant": true, "improvement_pct": -33.3}
  },
  "recommendation": "Switch to treatment (claude-sonnet): 33% cost reduction, 12.5% latency improvement, statistically significant."
}
```

### A/B 테스트 관리 API
```
POST   /admin/ab-tests               # 테스트 생성
GET    /admin/ab-tests               # 테스트 목록
GET    /admin/ab-tests/:id           # 테스트 상세 + 진행 상황
GET    /admin/ab-tests/:id/results   # 통계 분석 결과
POST   /admin/ab-tests/:id/pause     # 일시 중단
POST   /admin/ab-tests/:id/stop      # 조기 종료
POST   /admin/ab-tests/:id/promote   # 승자 variant로 라우팅 전환
```

### 자동 종료 조건
- 최소 샘플 수 충족 + 통계적 유의미성 확인 시 자동 종료
- 한쪽 Variant의 에러율이 20% 초과 시 안전 종료 (실험 중단)
- 지정된 end_at 도달

### 응답 헤더 (테스트 참여 표시)
```
X-AB-Test-ID: uuid
X-AB-Test-Variant: treatment
```

---

## 기술 설계 포인트

- **일관된 할당**: 동일 사용자는 항상 같은 Variant (쿠키/키 기반 해시)
- **실험 독립성**: 복수 테스트 동시 진행 시 서로 간섭 없음
- **통계 엔진**: Mann-Whitney U 검정 또는 t-검정 (Go 또는 Python 마이크로서비스)
- **점진적 롤아웃**: promote 시 0% → 10% → 50% → 100% 단계적 전환

---

## 의존성

- `phase3-enterprise/07-advanced-routing.md` 완료
- `phase4-advanced/02-prompt-management.md` 완료 (프롬프트 A/B 테스트)

---

## 완료 기준

- [ ] 50/50 트래픽 분배 정확도 테스트 (편차 < 5%)
- [ ] 동일 사용자 키의 Variant 일관성 테스트
- [ ] 통계 분석 결과 정확도 검증
- [ ] 승자 Variant 자동 전환 동작 확인
- [ ] 에러율 20% 초과 시 안전 종료 확인

---

## 예상 산출물

- `internal/abtest/` (디렉토리)
  - `experiment.go`, `assignment.go`, `collector.go`, `analyzer.go`
- `internal/store/postgres/abtest_store.go`
- `migrations/015_create_ab_tests.sql`
- `internal/gateway/middleware/abtest.go`
- `internal/gateway/handler/admin/abtests.go`
- `admin-ui/app/dashboard/experiments/page.tsx`
- `internal/abtest/analyzer_test.go`
