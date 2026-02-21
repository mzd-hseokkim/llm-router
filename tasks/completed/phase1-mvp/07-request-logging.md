# 07. 기본 요청 로깅

## 목표
모든 LLM API 요청/응답의 메타데이터를 구조화된 형식으로 기록한다. 비용 추적, 디버깅, 사용량 분석의 기반 데이터를 제공한다. 로깅은 요청 처리 성능에 최소한의 영향을 미쳐야 한다.

---

## 요구사항 상세

### 로깅 항목 (요청 완료 후 기록)
| 필드 | 타입 | 설명 |
|------|------|------|
| `id` | UUID | 고유 로그 ID |
| `request_id` | STRING | 요청 추적 ID (X-Request-ID) |
| `timestamp` | TIMESTAMPTZ | 요청 시작 시각 |
| `model` | STRING | 사용된 모델 (예: anthropic/claude-sonnet-4-20250514) |
| `provider` | STRING | 실제 사용된 Provider |
| `virtual_key_id` | UUID | 인증에 사용된 Virtual Key ID |
| `user_id` | UUID | 사용자 ID (nullable) |
| `team_id` | UUID | 팀 ID (nullable) |
| `org_id` | UUID | 조직 ID (nullable) |
| `prompt_tokens` | INTEGER | 입력 토큰 수 |
| `completion_tokens` | INTEGER | 출력 토큰 수 |
| `total_tokens` | INTEGER | 총 토큰 수 |
| `cost_usd` | DECIMAL | 계산된 비용 (USD) |
| `latency_ms` | INTEGER | 전체 응답 지연시간 |
| `ttft_ms` | INTEGER | 첫 토큰까지 시간 (스트리밍) |
| `status_code` | INTEGER | HTTP 상태 코드 |
| `finish_reason` | STRING | 완료 이유 (stop/length/error) |
| `cache_hit` | BOOLEAN | 캐시 응답 여부 |
| `is_streaming` | BOOLEAN | 스트리밍 요청 여부 |
| `error_code` | STRING | 오류 코드 (nullable) |
| `error_message` | STRING | 오류 메시지 (nullable) |
| `metadata` | JSONB | 추가 메타데이터 (태그 등) |

### 선택적 로깅 항목 (설정으로 제어)
- `request_body`: 요청 프롬프트/메시지 (개인정보 고려, 기본 OFF)
- `response_body`: 응답 텍스트 (기본 OFF)

### DB 스키마
```sql
CREATE TABLE request_logs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id          VARCHAR(36) NOT NULL,
    timestamp           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model               VARCHAR(200) NOT NULL,
    provider            VARCHAR(50) NOT NULL,
    virtual_key_id      UUID REFERENCES virtual_keys(id),
    user_id             UUID,
    team_id             UUID,
    org_id              UUID,
    prompt_tokens       INTEGER,
    completion_tokens   INTEGER,
    total_tokens        INTEGER,
    cost_usd            DECIMAL(12,8),
    latency_ms          INTEGER,
    ttft_ms             INTEGER,
    status_code         SMALLINT,
    finish_reason       VARCHAR(20),
    cache_hit           BOOLEAN DEFAULT false,
    is_streaming        BOOLEAN DEFAULT false,
    error_code          VARCHAR(50),
    error_message       TEXT,
    metadata            JSONB DEFAULT '{}'
) PARTITION BY RANGE (timestamp);

-- 월별 파티션 생성 (자동화 필요)
CREATE TABLE request_logs_2026_01 PARTITION OF request_logs
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE INDEX idx_logs_timestamp ON request_logs(timestamp DESC);
CREATE INDEX idx_logs_virtual_key ON request_logs(virtual_key_id, timestamp DESC);
CREATE INDEX idx_logs_model ON request_logs(model, timestamp DESC);
```

### 비동기 로깅 파이프라인
```
요청 처리 완료
     ↓
LogEntry 생성 (메모리)
     ↓
Buffered Channel (크기: 10,000)
     ↓
Background Worker (배치 처리)
     ↓
PostgreSQL COPY 또는 BULK INSERT (500건 배치 또는 1초 타임아웃)
```

**중요**: 로깅 실패가 요청 처리에 영향 주지 않음 (채널 풀 시 드롭 + 메트릭 기록)

### 구조화 로그 (표준 출력)
```json
{
  "level": "info",
  "ts": "2026-01-01T00:00:00Z",
  "msg": "request_completed",
  "request_id": "req_xxx",
  "model": "anthropic/claude-sonnet-4-20250514",
  "provider": "anthropic",
  "latency_ms": 1234,
  "tokens": 150,
  "cost_usd": 0.00234,
  "status": 200
}
```

### 로그 보존 및 정리
- 보존 기간 설정 (기본 90일)
- 자동 파티션 삭제: `pg_cron` 또는 애플리케이션 스케줄러
- 집계 테이블로 장기 보존: 일별 사용량 요약은 1년 보존

### 집계 쿼리 (사용량 조회)
```sql
-- 키별 일간 사용량
SELECT
    virtual_key_id,
    DATE_TRUNC('day', timestamp) as day,
    SUM(total_tokens) as tokens,
    SUM(cost_usd) as cost,
    COUNT(*) as requests
FROM request_logs
WHERE timestamp > NOW() - INTERVAL '30 days'
GROUP BY virtual_key_id, day;
```

---

## 기술 설계 포인트

- **배치 INSERT**: 개별 INSERT 대신 `COPY` 또는 `INSERT ... VALUES (...), (...)` 배치로 처리량 극대화
- **파티셔닝**: 월별 파티션으로 오래된 데이터 빠른 삭제 (`DROP PARTITION`)
- **채널 버퍼**: 순간적인 트래픽 급증 시 수용 가능한 버퍼 크기
- **스트리밍 로그 완성**: 스트리밍 응답의 경우 스트림 완료 후 로그 기록 (중간 상태 불필요)
- **에러 시 로깅**: 요청 실패 시에도 로그 기록 (실패 원인 분석)

---

## 의존성

- `01-project-setup.md` 완료 (DB, 로거 설정)
- `02-openai-compatible-api.md` 완료 (요청 처리 파이프라인)
- `05-virtual-key-auth.md` 완료 (키 ID 컨텍스트 주입)

---

## 완료 기준

- [ ] 모든 `/v1/chat/completions` 요청이 DB에 기록됨
- [ ] 스트리밍 요청도 완료 후 기록됨 (TTFT 포함)
- [ ] 실패한 요청도 error_code와 함께 기록됨
- [ ] 비동기 로깅으로 요청 지연시간에 영향 없음 (< 1ms 추가)
- [ ] 10,000 RPS 부하 테스트 시 로그 드롭 없음
- [ ] 로그 조회 API (기본 필터링) 정상 동작
- [ ] 30일 이상 된 로그 자동 삭제 동작 확인

---

## 예상 산출물

- `internal/telemetry/request_log.go`
- `internal/telemetry/log_writer.go` (배치 writer)
- `internal/store/postgres/log_store.go`
- `migrations/004_create_request_logs.sql`
- `internal/gateway/middleware/logging.go`
- `internal/gateway/handler/admin_logs.go`
- `internal/telemetry/log_writer_test.go`
