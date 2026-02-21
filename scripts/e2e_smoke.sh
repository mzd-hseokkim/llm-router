#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MASTER_KEY="${MASTER_KEY:-admin123}"
MODEL="${MODEL:-}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
PASS=0; FAIL=0; SKIP=0
TEST_VK_ID=""; TEST_VK_KEY=""
SKIP_LLM=false

pass() { echo -e "${GREEN}PASS${NC} $1"; PASS=$((PASS + 1)); }
fail() { echo -e "${RED}FAIL${NC} $1 — $2"; FAIL=$((FAIL + 1)); }
skip() { echo -e "${YELLOW}SKIP${NC} $1 — $2"; SKIP=$((SKIP + 1)); }

assert_status() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$name"
  else
    fail "$name" "expected HTTP $expected, got $actual"
  fi
}

# JSON 필드 추출: json_get <json_string> <key>
# nested key 지원 (점 구분), 배열 인덱스 지원 (숫자 키)
json_get() {
  local json="$1" key="$2"
  echo "$json" | python3 -c "
import sys, json
try:
    d = json.loads(sys.stdin.read())
    keys = '$key'.lstrip('.').split('.')
    v = d
    for k in keys:
        if k == '': continue
        if isinstance(v, dict):
            v = v.get(k, '')
        elif isinstance(v, list) and k.isdigit():
            idx = int(k)
            v = v[idx] if idx < len(v) else ''
        else:
            v = ''; break
    if isinstance(v, (dict, list)):
        print(json.dumps(v, ensure_ascii=False))
    else:
        print('' if v is None else str(v))
except Exception:
    print('')
" 2>/dev/null || true
}

assert_json_field() {
  local name="$1" field="$2" body="$3"
  local val
  val=$(json_get "$body" "$field")
  if [[ -n "$val" && "$val" != "null" ]]; then
    pass "$name ($field=$val)"
  else
    fail "$name" "field '$field' is empty or missing in response"
  fi
}

# ─── 헬스체크 ────────────────────────────────────────────

test_health_live() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health/live")
  assert_status "test_health_live" "200" "$status"
}

test_health_ready() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/health/ready")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_health_ready" "200" "$status"
  local db_status
  db_status=$(json_get "$body" "checks.database")
  if [[ -n "$db_status" ]]; then
    pass "test_health_ready (DB: $db_status)"
  fi
}

test_health_providers() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health/providers")
  assert_status "test_health_providers" "200" "$status"
}

test_metrics() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/metrics")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_metrics (HTTP)" "200" "$status"
  if echo "$body" | grep -q "# HELP"; then
    pass "test_metrics (Prometheus format)"
  else
    fail "test_metrics" "# HELP not found in metrics output"
  fi
}

# ─── OpenAPI ─────────────────────────────────────────────

test_openapi_json() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/docs/openapi.json")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_openapi_json" "200" "$status"
  assert_json_field "test_openapi_json" ".openapi" "$body"
}

# ─── 인증 (401 케이스) ────────────────────────────────────

test_auth_no_key() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/v1/models")
  assert_status "test_auth_no_key" "401" "$status"
}

test_auth_bad_key() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer invalid-key-xyz" \
    "$BASE_URL/v1/models")
  assert_status "test_auth_bad_key" "401" "$status"
}

test_admin_no_key() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/admin/logs")
  assert_status "test_admin_no_key" "401" "$status"
}

test_admin_bad_key() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer wrong-master-key" \
    "$BASE_URL/admin/logs")
  assert_status "test_admin_bad_key" "401" "$status"
}

# ─── Virtual Key CRUD ─────────────────────────────────────

setup_virtual_key() {
  local resp
  resp=$(curl -sf -X POST "$BASE_URL/admin/keys" \
    -H "Authorization: Bearer $MASTER_KEY" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-smoke-test","rpm_limit":100}')
  TEST_VK_ID=$(json_get "$resp" "id")
  TEST_VK_KEY=$(json_get "$resp" "key")
  if [[ -z "$TEST_VK_ID" || "$TEST_VK_ID" == "null" ]]; then
    echo -e "${RED}ERROR${NC} Virtual Key 생성 실패: $resp"
    exit 1
  fi
  echo "  VK ID: $TEST_VK_ID"
}

cleanup() {
  if [[ -n "$TEST_VK_ID" && "$TEST_VK_ID" != "null" ]]; then
    curl -sf -X DELETE "$BASE_URL/admin/keys/$TEST_VK_ID" \
      -H "Authorization: Bearer $MASTER_KEY" > /dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

test_vk_create() {
  local resp status body tmp_id
  resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/admin/keys" \
    -H "Authorization: Bearer $MASTER_KEY" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-vk-create-test","rpm_limit":10}')
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_vk_create" "201" "$status"
  tmp_id=$(json_get "$body" "id")
  if [[ -n "$tmp_id" && "$tmp_id" != "null" ]]; then
    curl -sf -X DELETE "$BASE_URL/admin/keys/$tmp_id" \
      -H "Authorization: Bearer $MASTER_KEY" > /dev/null 2>&1 || true
  fi
}

test_vk_list() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/admin/keys" \
    -H "Authorization: Bearer $MASTER_KEY")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_vk_list" "200" "$status"
  assert_json_field "test_vk_list" ".data" "$body"
}

test_vk_get() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/admin/keys/$TEST_VK_ID" \
    -H "Authorization: Bearer $MASTER_KEY")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_vk_get" "200" "$status"
  assert_json_field "test_vk_get" ".id" "$body"
}

test_vk_update() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" -X PATCH "$BASE_URL/admin/keys/$TEST_VK_ID" \
    -H "Authorization: Bearer $MASTER_KEY" \
    -H "Content-Type: application/json" \
    -d '{"name":"e2e-smoke-updated"}')
  assert_status "test_vk_update" "200" "$status"
}

test_vk_delete() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE_URL/admin/keys/$TEST_VK_ID" \
    -H "Authorization: Bearer $MASTER_KEY")
  assert_status "test_vk_delete" "204" "$status"
  TEST_VK_ID=""  # cleanup trap에서 재삭제 방지
}

# ─── 모델 선택 ────────────────────────────────────────────

pick_model() {
  if [[ -n "$MODEL" ]]; then return; fi
  local resp
  resp=$(curl -sf "$BASE_URL/v1/models" \
    -H "Authorization: Bearer $TEST_VK_KEY" 2>/dev/null || echo "{}")
  # Anthropic 모델 우선 선택 (API 키가 설정된 프로바이더)
  MODEL=$(echo "$resp" | python3 -c "
import sys, json
try:
    d = json.loads(sys.stdin.read())
    models = d.get('data', [])
    # anthropic 모델 우선
    for m in models:
        if 'anthropic' in m.get('id', '').lower() or 'anthropic' in m.get('owned_by', '').lower():
            print(m['id']); exit()
    # 없으면 첫 번째 모델
    if models: print(models[0]['id'])
except Exception:
    pass
" 2>/dev/null || true)
  if [[ -z "$MODEL" || "$MODEL" == "null" ]]; then
    echo -e "${YELLOW}WARN${NC} 사용 가능한 모델 없음 — LLM 테스트 건너뜀"
    SKIP_LLM=true
  else
    echo "  선택된 모델: $MODEL"
  fi
}

test_models_with_vk() {
  local resp status body
  resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/v1/models" \
    -H "Authorization: Bearer $TEST_VK_KEY")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  assert_status "test_models_with_vk" "200" "$status"
  assert_json_field "test_models_with_vk" ".data" "$body"
}

# ─── LLM 호출 ────────────────────────────────────────────

test_chat_basic() {
  if [[ "$SKIP_LLM" == true ]]; then
    skip "test_chat_basic" "no model available"
    return
  fi
  local resp status body content
  resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $TEST_VK_KEY" \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"say hi\"}],\"max_tokens\":5}")
  status=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | head -n -1)
  if [[ "$status" == "200" ]]; then
    content=$(json_get "$body" "choices.0.message.content")
    if [[ -n "$content" && "$content" != "null" ]]; then
      pass "test_chat_basic (content: $content)"
    else
      fail "test_chat_basic" "empty content in response"
    fi
  else
    fail "test_chat_basic" "HTTP $status: $body"
  fi
}

test_chat_bad_payload() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $TEST_VK_KEY" \
    -H "Content-Type: application/json" \
    -d '{"model":"some-model"}')
  assert_status "test_chat_bad_payload" "400" "$status"
}

# ─── Admin 조회 API ───────────────────────────────────────

test_usage_summary() {
  local status
  # entity_type, entity_id 쿼리 파라미터 필수
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/admin/usage/summary?entity_type=key&entity_id=$TEST_VK_ID" \
    -H "Authorization: Bearer $MASTER_KEY")
  assert_status "test_usage_summary" "200" "$status"
}

test_circuit_breakers() {
  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/admin/circuit-breakers" \
    -H "Authorization: Bearer $MASTER_KEY")
  assert_status "test_circuit_breakers" "200" "$status"
}

# ─── main ────────────────────────────────────────────────

main() {
  echo "=== LLM Router E2E Smoke Test ==="
  echo "BASE_URL:   $BASE_URL"
  echo "MASTER_KEY: ${MASTER_KEY:0:4}****"
  echo ""

  echo "[setup] Virtual Key 생성 중..."
  setup_virtual_key
  echo ""

  echo "--- Health ---"
  test_health_live
  test_health_ready
  test_health_providers
  test_metrics

  echo ""
  echo "--- OpenAPI ---"
  test_openapi_json

  echo ""
  echo "--- Auth (401 cases) ---"
  test_auth_no_key
  test_auth_bad_key
  test_admin_no_key
  test_admin_bad_key

  echo ""
  echo "--- Virtual Key CRUD ---"
  test_vk_create
  test_vk_list
  test_vk_get
  test_vk_update

  echo ""
  echo "--- Models ---"
  pick_model
  test_models_with_vk

  echo ""
  echo "--- Chat ---"
  test_chat_basic
  test_chat_bad_payload

  echo ""
  echo "--- Admin API ---"
  test_usage_summary
  test_circuit_breakers

  echo ""
  echo "--- Cleanup ---"
  test_vk_delete

  echo ""
  echo "=============================="
  echo -e "결과: ${GREEN}${PASS} passed${NC}, ${RED}${FAIL} failed${NC}, ${YELLOW}${SKIP} skipped${NC}"
  [[ $FAIL -eq 0 ]] && exit 0 || exit 1
}

main "$@"
