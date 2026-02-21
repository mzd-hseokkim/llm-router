# 08. 관측성 (OpenTelemetry / Prometheus)

## 목표
Gateway의 모든 운영 데이터를 산업 표준 관측성 스택과 통합한다. OpenTelemetry 기반 분산 트레이싱, Prometheus 메트릭, 구조화 로그를 구현하여 외부 모니터링 플랫폼(Datadog, Grafana, Jaeger 등)과 연동한다.

---

## 요구사항 상세

### OpenTelemetry 트레이싱
```go
// 요청 처리 트레이스
func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request) {
    ctx, span := tracer.Start(r.Context(), "gateway.chat_completion",
        trace.WithAttributes(
            attribute.String("model", req.Model),
            attribute.String("provider", target.Provider),
            attribute.Bool("stream", req.Stream),
        ),
    )
    defer span.End()

    // 자식 스팬: 인증
    _, authSpan := tracer.Start(ctx, "gateway.auth")
    // ... 인증 처리 ...
    authSpan.End()

    // 자식 스팬: 캐시 조회
    _, cacheSpan := tracer.Start(ctx, "gateway.cache_lookup")
    // ...
    cacheSpan.End()

    // 자식 스팬: Provider 호출
    _, providerSpan := tracer.Start(ctx, "provider.request",
        trace.WithAttributes(
            attribute.String("provider.name", target.Provider),
            attribute.String("provider.model", target.Model),
        ),
    )
    // ...
    providerSpan.End()
}
```

**주요 스팬**:
- `gateway.request` — 전체 요청 처리
- `gateway.auth` — 인증 및 권한 확인
- `gateway.cache_lookup` — 캐시 조회 (exact + semantic)
- `gateway.rate_limit` — 요청률 제한 체크
- `gateway.guardrail` — 가드레일 처리
- `provider.request` — Provider API 호출

**W3C TraceContext 전파**:
- 인바운드: `traceparent`, `tracestate` 헤더 수신
- 아웃바운드: Provider 요청에 전파

### Prometheus 메트릭 (전체 목록)
```
# 요청 관련
gateway_requests_total{provider, model, status, cache_result}
gateway_request_duration_seconds{provider, model, le} (히스토그램)
gateway_streaming_ttft_seconds{provider, model, le}   (첫 토큰 시간)

# 토큰/비용
gateway_tokens_total{provider, model, type="input|output"}
gateway_cost_usd_total{provider, model, team_id}

# Provider 관련
gateway_provider_requests_total{provider, status}
gateway_provider_latency_seconds{provider, le}
gateway_provider_health{provider}  # 0=down, 1=up

# 장애 복구
gateway_fallback_total{from_provider, to_provider, reason}
gateway_circuit_breaker_state{provider}  # 0=closed, 1=open, 2=half_open

# 캐시
gateway_cache_requests_total{type="exact|semantic", result="hit|miss"}
gateway_cache_hit_ratio{type="exact|semantic"}  # 게이지

# 요청률 제한
gateway_rate_limit_exceeded_total{entity_type, limit_type="rpm|tpm"}

# 시스템
gateway_active_connections     # 현재 활성 연결 수
gateway_uptime_seconds
gateway_goroutines             # 고루틴 수
```

### Prometheus 엔드포인트
```
GET /metrics
Content-Type: text/plain; version=0.0.4; charset=utf-8

# HELP gateway_requests_total Total number of requests
# TYPE gateway_requests_total counter
gateway_requests_total{provider="openai",model="gpt-4o",status="200",cache_result="miss"} 12345
...
```

### 외부 통합
**Grafana**:
- Prometheus 데이터소스 연결
- 사전 구성 대시보드 JSON 제공 (provisioning)

**Jaeger / Zipkin**:
- OTLP gRPC 또는 HTTP 익스포터
- 환경변수로 엔드포인트 설정: `OTEL_EXPORTER_OTLP_ENDPOINT`

**Datadog**:
- Datadog Agent OTLP 수신 설정 연동
- `DD_AGENT_HOST` 환경변수

**Langfuse** (LLM 특화 관측성):
- HTTP API로 요청/응답 전송
- 프롬프트, 토큰, 비용, 레이턴시 추적

### 구조화 로그 포맷 (표준화)
```json
{
  "level": "INFO",
  "timestamp": "2026-01-01T00:00:00.000Z",
  "caller": "handler/chat.go:123",
  "message": "request_completed",
  "trace_id": "abc123",
  "span_id": "def456",
  "request_id": "req_xxx",
  "model": "openai/gpt-4o",
  "provider": "openai",
  "latency_ms": 1234,
  "prompt_tokens": 100,
  "completion_tokens": 50,
  "cost_usd": 0.00075,
  "cache_hit": false,
  "stream": true,
  "status": 200
}
```

### 알림 규칙 (Prometheus AlertManager)
```yaml
groups:
  - name: gateway
    rules:
      - alert: HighErrorRate
        expr: rate(gateway_requests_total{status=~"5.."}[5m]) / rate(gateway_requests_total[5m]) > 0.05
        for: 2m
        labels:
          severity: warning

      - alert: ProviderDown
        expr: gateway_provider_health == 0
        for: 1m
        labels:
          severity: critical

      - alert: HighLatency
        expr: histogram_quantile(0.95, gateway_request_duration_seconds) > 10
        for: 5m
```

---

## 기술 설계 포인트

- **샘플링**: 고트래픽 환경에서는 트레이스 샘플링 적용 (기본 10%, 에러는 100%)
- **메트릭 카디널리티**: 라벨 값 제한 (모델/Provider는 화이트리스트로 제한)
- **성능 영향**: 메트릭 기록은 < 1μs, 트레이싱은 < 100μs 목표
- **컨텍스트 전파**: 모든 하위 호출에 트레이스 컨텍스트 전달

---

## 의존성

- `phase1-mvp/07-request-logging.md` 완료
- `phase1-mvp/09-health-check.md` 완료

---

## 완료 기준

- [ ] Prometheus `/metrics` 엔드포인트 표준 포맷 응답 확인
- [ ] Jaeger UI에서 요청별 분산 트레이스 조회 확인
- [ ] Grafana 대시보드에서 실시간 메트릭 시각화 확인
- [ ] 에러율 5% 초과 시 AlertManager 알림 발생 확인
- [ ] 트레이싱 오버헤드 < 100μs 성능 테스트

---

## 예상 산출물

- `internal/telemetry/otel.go` (OTel 초기화)
- `internal/telemetry/tracer.go`
- `internal/telemetry/metrics.go` (전체 메트릭 정의)
- `internal/telemetry/middleware.go`
- `docs/grafana-dashboard.json`
- `docs/alertmanager-rules.yaml`
- `docs/observability-setup.md`
