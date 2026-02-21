# Phase 5 — Task 01: Smoke Test Script

## 목표

`scripts/e2e_smoke.sh` 를 작성한다. curl만 사용해 실행 중인 서버를 대상으로
핵심 엔드포인트 20개를 5분 이내에 검증한다.

---

## 전제 조건

- 서버가 `http://localhost:8080` 에서 실행 중
- `curl`, `jq` 설치됨
- 실행: `bash scripts/e2e_smoke.sh`

---

## 구현 상세

### 파일: `scripts/e2e_smoke.sh`

스크립트 구조:
1. **환경 변수** — `BASE_URL`, `MASTER_KEY`, `MODEL` 오버라이드 가능
2. **헬퍼 함수** — `assert_status`, `assert_json_field`, `pass`, `fail`
3. **테스트 함수** — 각 시나리오를 독립 함수로 구현
4. **임시 Virtual Key** — 테스트용 키를 생성하고 종료 시 삭제 (`trap`)
5. **결과 요약** — 통과/실패 카운트 출력

### 핵심 패턴

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MASTER_KEY="${MASTER_KEY:-admin123}"
MODEL="${MODEL:-}"   # 빈 경우 /v1/models 에서 자동 선택

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
PASS=0; FAIL=0; TEST_VK_ID=""; TEST_VK_KEY=""

pass() { echo -e "${GREEN}PASS${NC} $1"; ((PASS++)); }
fail() { echo -e "${RED}FAIL${NC} $1 — $2"; ((FAIL++)); }

assert_status() {
  local name="$1" expected="$2" actual="$3"
  [[ "$actual" == "$expected" ]] && pass "$name" || fail "$name" "expected $expected, got $actual"
}
```

### 테스트 함수 목록

```bash
# 1. 헬스체크 (인증 불필요)
test_health_live()      # GET /health/live → 200
test_health_ready()     # GET /health/ready → 200, status:ok
test_health_providers() # GET /health/providers → 200
test_metrics()          # GET /metrics → 200, # HELP 포함

# 2. OpenAPI
test_openapi_json()     # GET /docs/openapi.json → 200, .openapi 필드 존재

# 3. 인증 검증 (401 케이스)
test_auth_no_key()      # GET /v1/models (Authorization 헤더 없음) → 401
test_auth_bad_key()     # GET /v1/models (Bearer invalid-key) → 401
test_admin_no_key()     # GET /admin/logs → 401
test_admin_bad_key()    # GET /admin/logs (Bearer wrong) → 401

# 4. Virtual Key CRUD (Admin API)
test_vk_create()        # POST /admin/keys → 201, key/id 저장
test_vk_list()          # GET /admin/keys → 200, data 배열
test_vk_get()           # GET /admin/keys/{id} → 200
test_vk_update()        # PATCH /admin/keys/{id} → 200
test_models_with_vk()   # GET /v1/models (유효 VK) → 200, data 배열 + MODEL 자동 선택

# 5. 실제 LLM 호출 (Anthropic)
test_chat_basic()       # POST /v1/chat/completions → 200, choices[0].message.content 존재
test_chat_bad_payload() # POST /v1/chat/completions (messages 없음) → 400

# 6. Admin 조회 API
test_usage_summary()    # GET /admin/usage/summary → 200
test_circuit_breakers() # GET /admin/circuit-breakers → 200

# 7. 정리
test_vk_delete()        # DELETE /admin/keys/{id} → 204
```

### Virtual Key 생성 및 정리 패턴

```bash
setup_virtual_key() {
  local resp
  resp=$(curl -sf -X POST "$BASE_URL/admin/keys" \
    -H "Authorization: Bearer $MASTER_KEY" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-smoke-test","rpm_limit":100}')
  TEST_VK_ID=$(echo "$resp" | jq -r '.id')
  TEST_VK_KEY=$(echo "$resp" | jq -r '.key')
}

cleanup() {
  if [[ -n "$TEST_VK_ID" ]]; then
    curl -sf -X DELETE "$BASE_URL/admin/keys/$TEST_VK_ID" \
      -H "Authorization: Bearer $MASTER_KEY" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
```

### 모델 자동 선택 패턴

```bash
pick_model() {
  if [[ -n "$MODEL" ]]; then return; fi
  MODEL=$(curl -sf "$BASE_URL/v1/models" \
    -H "Authorization: Bearer $TEST_VK_KEY" \
    | jq -r '.data[0].id // empty')
  if [[ -z "$MODEL" ]]; then
    echo -e "${YELLOW}WARN${NC} 사용 가능한 모델 없음 — LLM 테스트 건너뜀"
    SKIP_LLM=true
  fi
}
SKIP_LLM=false
```

### LLM 호출 패턴

```bash
test_chat_basic() {
  [[ "$SKIP_LLM" == true ]] && { echo "SKIP test_chat_basic (no model)"; return; }
  local status resp
  resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $TEST_VK_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"say hi\"}],\"max_tokens\":5}")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  if [[ "$status" == "200" ]]; then
    local content
    content=$(echo "$body" | jq -r '.choices[0].message.content // empty')
    [[ -n "$content" ]] && pass "test_chat_basic" || fail "test_chat_basic" "empty content"
  else
    fail "test_chat_basic" "HTTP $status: $body"
  fi
}
```

### 결과 출력

```bash
main() {
  echo "=== LLM Router E2E Smoke Test ==="
  echo "BASE_URL: $BASE_URL"
  echo ""

  setup_virtual_key

  test_health_live
  test_health_ready
  test_health_providers
  test_metrics
  test_openapi_json
  test_auth_no_key
  test_auth_bad_key
  test_admin_no_key
  test_admin_bad_key
  test_vk_create
  test_vk_list
  test_vk_get
  test_vk_update
  pick_model
  test_models_with_vk
  test_chat_basic
  test_chat_bad_payload
  test_usage_summary
  test_circuit_breakers
  test_vk_delete

  echo ""
  echo "=============================="
  echo -e "결과: ${GREEN}${PASS} passed${NC}, ${RED}${FAIL} failed${NC}"
  [[ $FAIL -eq 0 ]] && exit 0 || exit 1
}

main "$@"
```

---

## 주의사항

- `test_vk_delete`는 항상 마지막에 실행 (cleanup 겸용)
- `trap cleanup EXIT` 로 스크립트 비정상 종료 시에도 키 삭제
- LLM 호출은 실제 Anthropic API 요청 발생 → 소량의 토큰 소비 (max_tokens:5)
- `SKIP_LLM=true` 시 chat 관련 테스트는 SKIP 메시지 출력 후 통과 처리

---

## 완료 기준

- [ ] `bash scripts/e2e_smoke.sh` 실행 시 전체 통과
- [ ] FAIL 0개
- [ ] 실행 시간 60초 이내
