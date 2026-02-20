# 10. 알림 시스템 (Alerting)

## 목표
예산 초과, Provider 장애, 레이턴시 임계값 초과, 보안 이벤트 등 중요한 이벤트 발생 시 적절한 채널로 실시간 알림을 전달한다. Webhook, Slack, Email 채널을 지원한다.

---

## 요구사항 상세

### 알림 이벤트 유형
| 이벤트 | 심각도 | 설명 |
|--------|--------|------|
| `budget.soft_limit` | warning | 예산 Soft Limit 도달 |
| `budget.hard_limit` | critical | 예산 Hard Limit 초과 (요청 차단) |
| `provider.down` | critical | Provider 헬스체크 실패 |
| `provider.degraded` | warning | Provider 에러율 증가 |
| `latency.high` | warning | P95 레이턴시 임계값 초과 |
| `error_rate.high` | warning | 에러율 임계값 초과 |
| `security.injection` | warning | 프롬프트 인젝션 감지 |
| `security.pii` | info | PII 감지 및 마스킹 |
| `rate_limit.exceeded` | info | 요청률 제한 초과 빈번 |
| `key.expiring` | warning | Virtual Key 7일 내 만료 예정 |

### 알림 채널 설정
```yaml
alerting:
  channels:
    - name: "ops-slack"
      type: slack
      webhook_url: ${SLACK_WEBHOOK_URL}
      channel: "#llm-alerts"

    - name: "email-ops"
      type: email
      smtp_host: smtp.company.com
      smtp_port: 587
      from: "gateway-alerts@company.com"
      to: ["ops@company.com", "ml-team@company.com"]

    - name: "custom-webhook"
      type: webhook
      url: https://my-system.com/webhooks/gateway
      method: POST
      headers:
        Authorization: "Bearer ${WEBHOOK_TOKEN}"
      retry: 3

  routing:
    - events: [budget.hard_limit, provider.down]
      severity: critical
      channels: [ops-slack, email-ops]

    - events: [budget.soft_limit, provider.degraded]
      severity: warning
      channels: [ops-slack]

    - events: ["security.*"]
      channels: [custom-webhook]
```

### Slack 알림 메시지 포맷
```json
{
  "blocks": [
    {
      "type": "header",
      "text": {"type": "plain_text", "text": "🚨 Critical Alert: Provider Down"}
    },
    {
      "type": "section",
      "fields": [
        {"type": "mrkdwn", "text": "*Provider:*\nOpenAI"},
        {"type": "mrkdwn", "text": "*Error Rate:*\n87% (last 5m)"},
        {"type": "mrkdwn", "text": "*Duration:*\n3 minutes"},
        {"type": "mrkdwn", "text": "*Fallback:*\nAnthropic (active)"}
      ]
    },
    {
      "type": "actions",
      "elements": [
        {"type": "button", "text": {"text": "View Dashboard"}, "url": "https://gateway.company.com/dashboard"},
        {"type": "button", "text": {"text": "Reset Circuit Breaker"}, "url": "https://gateway.company.com/admin/circuit-breakers/openai/reset"}
      ]
    }
  ]
}
```

### Webhook 페이로드
```json
{
  "event": "budget.soft_limit",
  "severity": "warning",
  "timestamp": "2026-01-01T00:00:00Z",
  "details": {
    "entity_type": "team",
    "entity_id": "uuid",
    "entity_name": "ML Team",
    "current_spend_usd": 80.50,
    "soft_limit_usd": 80.00,
    "hard_limit_usd": 100.00,
    "period": "monthly",
    "period_end": "2026-01-31T00:00:00Z"
  },
  "gateway_version": "1.0.0"
}
```

### 알림 중복 방지 (Deduplication)
```go
type AlertDeduplicator struct {
    redis  *redis.Client
    window time.Duration  // 기본 15분
}

func (d *AlertDeduplicator) ShouldSend(eventType, entityID string) bool {
    key := fmt.Sprintf("alert:dedup:%s:%s", eventType, entityID)
    result, _ := d.redis.SetNX(ctx, key, 1, d.window).Result()
    return result  // true = 새 알림, false = 중복 (무시)
}
```

### 알림 관리 API
```
POST /admin/alerts/test?channel=slack    # 테스트 알림 전송
GET  /admin/alerts/config                # 현재 알림 설정 조회
PUT  /admin/alerts/config                # 알림 설정 수정
GET  /admin/alerts/history               # 알림 발송 이력
POST /admin/alerts/silence               # 특정 알림 일시 침묵 (유지보수 시)
```

### 알림 발송 이력 저장
```sql
CREATE TABLE alert_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type  VARCHAR(100) NOT NULL,
    severity    VARCHAR(20) NOT NULL,
    channel     VARCHAR(100) NOT NULL,
    status      VARCHAR(20) NOT NULL,   -- 'sent', 'failed', 'deduplicated'
    payload     JSONB NOT NULL,
    error       TEXT,
    sent_at     TIMESTAMPTZ DEFAULT NOW()
);
```

### 알림 발송 재시도
- HTTP 알림 실패 시 3회 재시도 (지수 백오프)
- 최종 실패 시 dead-letter 큐에 저장
- 이력에 실패 원인 기록

---

## 기술 설계 포인트

- **이벤트 버스**: 내부 이벤트 버스를 통해 알림 이벤트 발행 (느슨한 결합)
- **알림 채널 추상화**: `Notifier` 인터페이스로 채널 종류 교체 가능
- **비동기 발송**: 알림 발송은 요청 처리 경로 외부에서 비동기 실행
- **침묵 기간**: 유지보수 시간 동안 특정 알림 억제 기능

---

## 의존성

- `phase2-stability/04-budget-management.md` 완료
- `phase2-stability/01-failover-fallback.md` 완료
- `phase3-enterprise/08-observability.md` 완료

---

## 완료 기준

- [ ] Slack 알림 테스트 메시지 전송 성공
- [ ] 예산 Soft Limit 도달 시 알림 자동 발생
- [ ] Provider 장애 시 알림 1분 내 발송
- [ ] 15분 내 동일 이벤트 중복 알림 방지 확인
- [ ] 알림 발송 이력 조회 API 동작 확인

---

## 예상 산출물

- `internal/alerting/` (디렉토리)
  - `notifier.go` (인터페이스)
  - `slack.go`, `email.go`, `webhook.go`
  - `deduplicator.go`
  - `router.go` (이벤트 → 채널 라우팅)
- `internal/alerting/event_bus.go`
- `migrations/013_create_alert_history.sql`
- `internal/gateway/handler/admin/alerts.go`
- `internal/alerting/slack_test.go`
