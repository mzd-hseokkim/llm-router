# 07. 차지백/쇼백 리포트 (Chargeback/Showback)

## 목표
부서/팀별 LLM 사용 비용을 정확히 할당하고 내부 청구하는 차지백(Chargeback) 시스템과, 정보 공유 목적의 쇼백(Showback) 리포트를 구현한다. 외부 빌링 시스템 연동을 위한 API도 제공한다.

---

## 요구사항 상세

### 차지백(Chargeback) vs 쇼백(Showback)
- **차지백**: 실제 내부 청구 — 팀 예산에서 LLM 비용 차감
- **쇼백**: 정보 제공 — "당신의 팀이 이만큼 사용했습니다" (청구 없음)

### 비용 할당 모델
**직접 할당 (Direct Attribution)**
```go
// Virtual Key → 팀 → 조직으로 비용 직접 귀속
cost := calculateCost(request)
allocate(request.TeamID, cost)
allocate(request.OrgID, cost)
```

**공유 리소스 할당 (Shared Cost Allocation)**
```go
// 공유 Gateway 인프라 비용 배분
type AllocationMethod string
const (
    MethodProportional AllocationMethod = "proportional"  // 사용량 비례
    MethodEqual        AllocationMethod = "equal"         // 균등 배분
    MethodCustom       AllocationMethod = "custom"        // 사용자 정의 %
)
```

### 마크업(Markup) 설정
LLM 서비스 내부 재판매 시 마크업 적용:
```go
type MarkupConfig struct {
    TeamID     UUID
    Percentage float64   // 20 = 20% 마크업
    FixedUSD   float64   // 토큰당 고정 추가 비용
    CapUSD     float64   // 최대 마크업 (NULL = 무제한)
}

func (m *MarkupConfig) Apply(baseCost float64) float64 {
    markup := baseCost * m.Percentage / 100
    if m.CapUSD > 0 {
        markup = min(markup, m.CapUSD)
    }
    return baseCost + markup + m.FixedUSD
}
```

### 월간 차지백 리포트 구조
```json
{
  "period": "2026-01",
  "generated_at": "2026-02-01T00:00:00Z",
  "currency": "USD",
  "summary": {
    "total_cost_usd": 15240.50,
    "total_tokens": 1234567890,
    "total_requests": 500000
  },
  "by_team": [
    {
      "team_id": "uuid",
      "team_name": "ML Team",
      "cost_usd": 8500.00,
      "markup_usd": 850.00,
      "total_charged_usd": 9350.00,
      "tokens": 700000000,
      "requests": 280000,
      "by_model": [
        {
          "model": "openai/gpt-4o",
          "cost_usd": 5000.00,
          "tokens": 400000000
        },
        {
          "model": "anthropic/claude-sonnet-4-20250514",
          "cost_usd": 3500.00,
          "tokens": 300000000
        }
      ],
      "by_project": [
        {"tag": "product-ai", "cost_usd": 6000.00},
        {"tag": "internal-tools", "cost_usd": 2500.00}
      ]
    }
  ]
}
```

### 태그 기반 비용 분류
```json
// 요청 시 태그 전달
{
  "model": "openai/gpt-4o",
  "messages": [...],
  "metadata": {
    "project": "product-ai",
    "feature": "recommendation",
    "environment": "production",
    "cost_center": "CC-12345"
  }
}
```

태그 기반으로 비용 분류 및 집계:
```sql
SELECT
    metadata->>'project' as project,
    metadata->>'cost_center' as cost_center,
    SUM(cost_usd) as total_cost
FROM request_logs
WHERE team_id = $1 AND timestamp BETWEEN $2 AND $3
GROUP BY project, cost_center
ORDER BY total_cost DESC;
```

### 리포트 생성 및 배포
```
GET /admin/reports/chargeback?period=2026-01&format=json|csv|pdf
GET /admin/reports/showback?period=2026-01&team_id=uuid
POST /admin/reports/generate?period=2026-01    # 리포트 사전 생성 (캐싱)
GET /admin/reports/schedule                    # 자동 월간 리포트 설정
```

**자동 월간 리포트**:
- 매월 1일 전월 리포트 자동 생성
- 이메일 또는 Slack으로 팀 리더에게 자동 전송
- CSV/PDF 첨부

### 외부 빌링 시스템 API
```
GET /api/billing/usage?from=2026-01-01&to=2026-01-31
→ {
    "items": [
      {
        "team_id": "uuid",
        "period_start": "2026-01-01",
        "period_end": "2026-01-31",
        "quantity_tokens": 700000000,
        "unit_price_usd": 0.000010,
        "amount_usd": 7000.00,
        "metadata": {"team_name": "ML Team"}
      }
    ]
  }
```

Stripe, Oracle ERP, SAP 등 외부 빌링 시스템 연동용.

---

## 기술 설계 포인트

- **집계 효율**: 월간 집계는 daily_usage 테이블 기반으로 빠른 조회
- **리포트 캐싱**: 생성된 리포트 파일을 스토리지에 캐싱 (재요청 시 즉시 반환)
- **PDF 생성**: `chromedp` 또는 `wkhtmltopdf`로 HTML → PDF 변환
- **비용 소수점**: DECIMAL(14,8)로 마이크로 달러 단위까지 정확한 계산

---

## 의존성

- `phase2-stability/05-cost-tracking.md` 완료
- `phase3-enterprise/01-multi-tenancy.md` 완료

---

## 완료 기준

- [ ] 월간 차지백 리포트 JSON/CSV 생성 확인
- [ ] 마크업 적용된 청구액 계산 정확도 확인
- [ ] 태그 기반 비용 분류 집계 정확도 확인
- [ ] 자동 월간 이메일 발송 확인
- [ ] 외부 빌링 API 응답 포맷 확인

---

## 예상 산출물

- `internal/billing/` (디렉토리)
  - `chargeback.go`, `showback.go`, `markup.go`, `report.go`
- `internal/store/postgres/billing_store.go`
- `internal/gateway/handler/admin/reports.go`
- `internal/gateway/handler/api/billing.go`
- `internal/billing/scheduler.go` (자동 월간 생성)
- `internal/billing/chargeback_test.go`
