# A/B 테스트 가이드

LLM Router의 A/B 테스트 기능을 사용하면 두 개 이상의 LLM 모델을 동일한 트래픽으로 비교해 **비용, 응답 속도, 에러율** 측면에서 통계적으로 유의미한 차이를 측정할 수 있습니다.

---

## 목차

1. [개념 이해](#1-개념-이해)
2. [라이프사이클](#2-라이프사이클)
3. [빠른 시작](#3-빠른-시작)
4. [트래픽 분배 방식](#4-트래픽-분배-방식)
5. [결과 수집 및 통계 분석](#5-결과-수집-및-통계-분석)
6. [자동 중단 조건](#6-자동-중단-조건)
7. [Admin API 레퍼런스](#7-admin-api-레퍼런스)
8. [Admin UI 사용법](#8-admin-ui-사용법)
9. [주의사항 및 제한](#9-주의사항-및-제한)

---

## 1. 개념 이해

### 동작 원리

A/B 테스트는 **`POST /v1/chat/completions`** 요청을 가로채서, 요청자를 특정 variant(변형)에 **일관성 있게** 할당한 뒤, 해당 variant에 설정된 모델로 라우팅합니다.

```
클라이언트 요청 (model: "gpt-4o")
          ↓
  [A/B Test 미들웨어]
  - entity ID (Virtual Key) 해시
  - 50% → control variant → model: "gpt-4o"
  - 50% → treatment variant → model: "claude-sonnet-4-6"
          ↓
  실제 LLM 호출
          ↓
  결과 기록 (latency, cost, error, tokens)
```

### 핵심 특성

- **일관성**: 동일한 API 키(Virtual Key)는 항상 같은 variant에 할당됩니다.
- **투명성**: 클라이언트는 자신이 어떤 variant에 속하는지 알 필요가 없습니다. 요청 body의 `model` 필드가 자동으로 교체됩니다.
- **비침습적**: 실험 처리 중 에러가 발생해도 원본 요청은 그대로 진행됩니다 (fail-open).

---

## 2. 라이프사이클

```
draft ──start──→ running ──pause──→ paused
                    │                  │
                    │              start│
                    │                  ↓
                    │              running
                    │
                ┌───┴───────────────────┐
             promote               stop (수동/자동)
                │                       │
                ↓                       ↓
           completed                 stopped
```

| 상태 | 설명 | 트래픽 할당 |
|------|------|------------|
| `draft` | 생성 직후 대기 상태 | ❌ |
| `running` | 실험 진행 중 | ✅ |
| `paused` | 일시 중단 | ❌ |
| `completed` | promote로 winner 선정 후 종료 | ❌ |
| `stopped` | 수동 중단 또는 에러율 초과로 자동 중단 | ❌ |

---

## 3. 빠른 시작

### Step 1: 모델 등록 확인

A/B 테스트에서 사용할 모델이 먼저 **Providers** 페이지에 등록되어 있어야 합니다.

```bash
# 등록된 enabled 모델 전체 조회
GET /admin/models
Authorization: Bearer <admin_jwt>
```

### Step 2: 실험 생성

```bash
curl -X POST http://localhost:8080/admin/ab-tests \
  -H "Authorization: Bearer <admin_jwt>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "claude-vs-gpt4-cost-test",
    "traffic_split": [
      {"variant": "control",   "model": "gpt-4o",              "weight": 50},
      {"variant": "treatment", "model": "claude-sonnet-4-6",   "weight": 50}
    ],
    "target": {
      "sample_rate": 1.0
    },
    "min_samples": 500,
    "confidence_level": 0.95
  }'
```

응답으로 `id`가 반환됩니다. 이 `id`를 이후 단계에서 사용합니다.

### Step 3: 실험 시작

```bash
curl -X POST http://localhost:8080/admin/ab-tests/{id}/start \
  -H "Authorization: Bearer <admin_jwt>"
```

`status`가 `running`으로 변경되면 즉시 트래픽이 분배됩니다.

### Step 4: 트래픽 발생

클라이언트는 평소와 똑같이 요청합니다. `model` 필드에 어떤 값을 넣어도 미들웨어가 variant 모델로 교체합니다.

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <virtual_key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

응답 헤더에서 어떤 variant에 할당됐는지 확인할 수 있습니다:
```
X-AB-Test-ID: 550e8400-e29b-41d4-a716-446655440000
X-AB-Test-Variant: treatment
```

### Step 5: 결과 확인

```bash
curl http://localhost:8080/admin/ab-tests/{id}/results \
  -H "Authorization: Bearer <admin_jwt>"
```

응답 예시:
```json
{
  "test_id": "550e8400-...",
  "status": "running",
  "winner": "",
  "results": {
    "control": {
      "model": "gpt-4o",
      "samples": 450,
      "latency_p95_ms": 1250.5,
      "avg_cost_per_request": 0.012,
      "error_rate": 0.02
    },
    "treatment": {
      "model": "claude-sonnet-4-6",
      "samples": 445,
      "latency_p95_ms": 1100.3,
      "avg_cost_per_request": 0.008,
      "error_rate": 0.015
    }
  },
  "statistical_significance": {
    "latency": {
      "p_value": 0.032,
      "significant": true,
      "improvement_pct": -12.0
    },
    "cost": {
      "p_value": 0.048,
      "significant": true,
      "improvement_pct": -33.3
    },
    "error_rate": {
      "p_value": 0.18,
      "significant": false,
      "improvement_pct": -25.0
    }
  },
  "recommendation": "Switch to treatment: 33.3% cost reduction, 12.0% latency improvement"
}
```

### Step 6: winner 선정 및 종료

분석 결과를 바탕으로 winner를 결정합니다.

```bash
curl -X POST http://localhost:8080/admin/ab-tests/{id}/promote \
  -H "Authorization: Bearer <admin_jwt>" \
  -H "Content-Type: application/json" \
  -d '{"winner": "treatment"}'
```

실험이 `completed` 상태가 되고, 트래픽 분배가 중단됩니다. 이후 Virtual Key의 routing 설정에서 winner 모델로 실제 전환해야 합니다.

---

## 4. 트래픽 분배 방식

### Entity ID (할당 기준)

| 우선순위 | 출처 | 비고 |
|---------|------|------|
| 1 | `Authorization` 헤더 값 (Virtual Key) | 권장 — 사용자 단위 일관성 |
| 2 | 클라이언트 IP (`RemoteAddr`) | Virtual Key 없을 때 fallback |

같은 Virtual Key는 실험 기간 동안 **항상 같은 variant**에 배정됩니다.

### 할당 알고리즘

FNV-1a 32bit 해싱 기반 consistent hashing:

```
hash(experiment_id + "\x00" + entity_id) % 100 → bucket (0~99)
```

각 variant의 `weight`를 누적해서 bucket이 속하는 범위의 variant를 반환합니다.

**예시** (control 50, treatment 50):
- bucket 0~49 → `control`
- bucket 50~99 → `treatment`

> `weight` 합계는 반드시 **100**이어야 합니다. 합이 맞지 않으면 첫 번째 variant로 fallback합니다.

### sample_rate

`target.sample_rate`(0.0~1.0)로 전체 트래픽 중 실험에 참여할 비율을 제한할 수 있습니다.

```
sample_rate: 0.1  → 전체 요청의 10%만 실험에 참여
sample_rate: 1.0  → 전체 요청 참여 (기본값)
```

sample_rate 판정도 별도의 FNV-1a 해시로 결정되므로 일관성이 유지됩니다.

---

## 5. 결과 수집 및 통계 분석

### 수집 메트릭

각 요청 완료 후 **비동기로** 다음 데이터를 DB에 기록합니다:

| 메트릭 | 설명 |
|--------|------|
| `latency_ms` | 요청~응답 전체 소요 시간 (밀리초) |
| `prompt_tokens` | 입력 토큰 수 |
| `completion_tokens` | 출력 토큰 수 |
| `cost_usd` | 추정 비용 (달러) |
| `error` | 에러 발생 여부 |
| `finish_reason` | 완료 이유 (stop, length, error 등) |

### 통계 분석

`GET /admin/ab-tests/{id}/results` 호출 시 실시간으로 분석합니다.

**분석 메트릭 3가지:**

| 메트릭 | 검정 방법 | 개선 방향 |
|--------|---------|----------|
| Latency (P95) | Welch's t-test | 낮을수록 좋음 |
| Cost (평균) | Welch's t-test | 낮을수록 좋음 |
| Error Rate | Z-test (이항분포) | 낮을수록 좋음 |

> 각 variant에 **30개 이상** 샘플이 있어야 분석이 시작됩니다.

**Winner 자동 결정 조건:**
1. 유의한(p-value < alpha) 메트릭이 하나 이상 있어야 함
2. 유의한 **모든** 메트릭에서 treatment가 control보다 우수해야 함

**confidence_level과 alpha의 관계:**
```
alpha = 1 - confidence_level
confidence_level: 0.95  →  alpha: 0.05  →  p-value < 0.05면 유의
confidence_level: 0.99  →  alpha: 0.01  →  p-value < 0.01면 유의
```

---

## 6. 자동 중단 조건

한 variant의 **에러율이 20%를 초과**하면 (최소 100개 샘플 이후) 실험이 자동으로 `stopped` 상태가 됩니다.

이는 새 모델이 예상치 못한 장애를 일으킬 때 트래픽을 즉시 차단하기 위한 안전장치입니다.

---

## 7. Admin API 레퍼런스

모든 엔드포인트는 `Authorization: Bearer <admin_jwt>` 인증이 필요합니다.

### 실험 관리

| 메서드 | 경로 | 설명 |
|--------|------|------|
| `POST` | `/admin/ab-tests` | 실험 생성 (draft 상태) |
| `GET` | `/admin/ab-tests` | 실험 목록 조회 (최신순) |
| `GET` | `/admin/ab-tests/{id}` | 실험 상세 조회 |
| `DELETE` | `/admin/ab-tests/{id}` | 실험 삭제 (draft/stopped/completed만 가능) |

### 상태 전이

| 메서드 | 경로 | 설명 |
|--------|------|------|
| `POST` | `/admin/ab-tests/{id}/start` | draft/paused → running |
| `POST` | `/admin/ab-tests/{id}/pause` | running → paused |
| `POST` | `/admin/ab-tests/{id}/stop` | running → stopped |
| `POST` | `/admin/ab-tests/{id}/promote` | running → completed + winner 기록 |

### 결과 조회

| 메서드 | 경로 | 설명 |
|--------|------|------|
| `GET` | `/admin/ab-tests/{id}/results` | 통계 분석 결과 조회 |

### 모델 목록

| 메서드 | 경로 | 설명 |
|--------|------|------|
| `GET` | `/admin/models` | 등록된 enabled 모델 전체 목록 |

### POST /admin/ab-tests 요청 Body

```json
{
  "name": "string (필수)",
  "traffic_split": [
    {
      "variant": "control (필수)",
      "model": "등록된 model_id (필수)",
      "weight": 50
    },
    {
      "variant": "treatment (필수)",
      "model": "등록된 model_id (필수)",
      "weight": 50
    }
  ],
  "target": {
    "team_ids": null,
    "sample_rate": 1.0
  },
  "success_metrics": [],
  "min_samples": 1000,
  "confidence_level": 0.95,
  "start_at": "2026-01-01T00:00:00Z (선택)",
  "end_at": "2026-01-02T00:00:00Z (선택)"
}
```

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `traffic_split` | — | 최소 2개 필수. weight 합 = 100 |
| `target.sample_rate` | `1.0` | 실험 참여 비율 (0.0~1.0) |
| `min_samples` | `1000` | 목표 샘플 수 (메타데이터) |
| `confidence_level` | `0.95` | 통계 신뢰 수준 |
| `start_at` / `end_at` | 없음 | 시간 윈도우 제한 (없으면 무기한) |

---

## 8. Admin UI 사용법

Admin UI(`http://localhost:3001/ab-tests`)에서 모든 작업을 시각적으로 수행할 수 있습니다.

### 실험 생성

1. 우상단 **+ New A/B Test** 버튼 클릭
2. Experiment Name 입력
3. Traffic Split의 각 variant에서 **드롭다운으로 모델 선택** (등록된 enabled 모델만 표시)
4. Weight 합이 100인지 확인
5. Min Samples, Confidence, Sample Rate 조정
6. **Create Experiment** 클릭

### 실험 시작/중단

| 버튼 | 표시 조건 | 동작 |
|------|----------|------|
| `Start` | draft, paused | running으로 전환 |
| `Pause` | running | paused로 전환 |
| `Stop` | running, paused | stopped로 전환 |
| `Promote` | running, completed | winner 선정 후 completed |
| `Results` | running 이후 | 통계 결과 패널 토글 |
| `Delete` | draft, stopped, completed | 실험 삭제 (확인 다이얼로그) |

> **running 중인 실험은 삭제할 수 없습니다.** 먼저 Stop 후 삭제하세요.

---

## 9. 주의사항 및 제한

### 동시 실험

여러 실험이 `running` 상태이면 **첫 번째로 매칭된 실험만** 적용됩니다. 현재 중첩 실험(동시에 여러 실험 적용)은 지원하지 않습니다.

### 모델 등록 필수

`traffic_split`에 지정하는 `model`은 반드시 **Providers → Models**에 등록된 `model_id`여야 합니다. 미등록 모델로 실험을 실행하면 라우팅에 실패합니다.

### Promote 이후 실제 전환

`promote`는 실험을 종료하는 것일 뿐, **winner 모델로 실제 라우팅을 전환하지는 않습니다.** winner가 결정되면 Virtual Key의 routing 설정 또는 Fallback Chain을 직접 업데이트해야 합니다.

### 표준편차 가정

통계 분석에서 표준편차를 **평균의 20%로 가정**합니다. 실제 분포가 이 가정에서 크게 벗어나면 p-value 정확도가 낮아질 수 있습니다.

### team_ids 필터링

`target.team_ids`는 현재 **미구현**입니다. 설정해도 실제 필터링이 적용되지 않으며, 모든 entity가 `sample_rate`만 기준으로 할당됩니다.
