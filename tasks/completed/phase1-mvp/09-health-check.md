# 09. 헬스체크 엔드포인트

## 목표
Gateway 자체 상태와 연결된 외부 서비스(Provider, DB, Redis)의 가용성을 모니터링하는 헬스체크 엔드포인트를 구현한다. 쿠버네티스 liveness/readiness probe와 로드 밸런서 헬스체크를 지원한다.

---

## 요구사항 상세

### 엔드포인트 목록
| 경로 | 용도 |
|------|------|
| `GET /health` | 전체 헬스 상태 (상세) |
| `GET /health/live` | Liveness probe (Gateway 프로세스 생존 여부) |
| `GET /health/ready` | Readiness probe (트래픽 수신 준비 완료 여부) |
| `GET /health/providers` | Provider별 상태 조회 |

### `/health/live` 응답
- Gateway 프로세스가 살아있으면 항상 200
- 데드락 또는 OOM 시에만 실패 (쿠버네티스가 재시작)
```json
{"status": "ok"}
```

### `/health/ready` 응답
- 필수 의존성(DB, Redis) 연결 가능 시 200
- 의존성 장애 시 503 (로드 밸런서가 트래픽 제외)
```json
{
  "status": "ready",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

### `/health` 전체 상태 응답
```json
{
  "status": "degraded",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "timestamp": "2026-01-01T00:00:00Z",
  "checks": {
    "database": {
      "status": "ok",
      "latency_ms": 2
    },
    "redis": {
      "status": "ok",
      "latency_ms": 1
    }
  },
  "providers": {
    "openai": {
      "status": "ok",
      "error_rate_1m": 0.02,
      "last_success_at": "2026-01-01T00:00:00Z"
    },
    "anthropic": {
      "status": "degraded",
      "error_rate_1m": 0.45,
      "last_error": "Service temporarily unavailable"
    },
    "gemini": {
      "status": "ok",
      "error_rate_1m": 0.0
    }
  }
}
```

**status 값**:
- `ok`: 정상
- `degraded`: 부분 장애 (일부 기능만 작동)
- `unhealthy`: 심각한 장애

### 체크 항목별 구현

**Database 체크**
```go
func checkDatabase(ctx context.Context, db *sql.DB) HealthCheck {
    ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    err := db.PingContext(ctx)
    // ...
}
```

**Redis 체크**
```go
func checkRedis(ctx context.Context, client *redis.Client) HealthCheck {
    ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()
    err := client.Ping(ctx).Err()
    // ...
}
```

**Provider 상태** (패시브 체크, 실제 요청 없음)
- 최근 1분간 에러율을 Redis에서 조회
- 에러율 > 50%: `unhealthy`
- 에러율 10~50%: `degraded`
- 에러율 < 10%: `ok`

### 액티브 헬스체크 (Provider)
- 주기: 60초마다 실행 (설정 가능)
- 방법: Provider API에 경량 요청 (예: 모델 목록 조회)
- 실패 임계값: 연속 3회 실패 시 `unhealthy`로 전환
- 자동 복구: 1회 성공 시 `ok`로 전환

### 메트릭 노출 (`/metrics`)
- Prometheus 포맷 (Phase 3에서 본격 구현, 기본 메트릭만)
- 기본 메트릭:
  - `gateway_requests_total{provider, model, status}`
  - `gateway_request_duration_seconds{provider, model}`
  - `gateway_provider_health{provider}` (0=unhealthy, 1=ok)

---

## 기술 설계 포인트

- **타임아웃 격리**: 각 체크는 독립적인 타임아웃 (하나의 실패가 전체 응답 지연 방지)
- **캐싱**: 헬스체크 결과를 5초 캐싱 (헬스체크 자체가 부하가 되지 않도록)
- **병렬 체크**: 모든 체크를 고루틴으로 병렬 실행
- **인증 불필요**: 헬스체크 엔드포인트는 인증 없이 접근 가능

---

## 의존성

- `01-project-setup.md` 완료 (DB, Redis 연결)
- `03-provider-adapters.md` 완료 (Provider 상태 조회)

---

## 완료 기준

- [ ] `/health/live` 200 응답 확인
- [ ] `/health/ready` DB/Redis 장애 시 503 반환 확인
- [ ] Provider 에러율 기반 상태 판정 동작 확인
- [ ] 쿠버네티스 probe 설정 예시 문서화
- [ ] 헬스체크 응답 시간 < 100ms
- [ ] `/metrics` 기본 Prometheus 메트릭 노출 확인

---

## 예상 산출물

- `internal/gateway/handler/health.go`
- `internal/health/checker.go`
- `internal/health/provider_health.go`
- `internal/telemetry/metrics.go` (기본 Prometheus 메트릭)
- `docs/kubernetes-probe-example.yaml`
