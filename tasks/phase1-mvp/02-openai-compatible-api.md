# 02. OpenAI-Compatible API 엔드포인트 구현

## 목표
클라이언트가 기존 OpenAI SDK의 `base_url`만 변경하여 즉시 사용 가능한 OpenAI-compatible REST API를 구현한다. 단일 진입점에서 모든 LLM Provider로의 라우팅을 처리한다.

---

## 요구사항 상세

### 지원 엔드포인트 (Phase 1)
| 메서드 | 경로 | 설명 |
|--------|------|------|
| POST | `/v1/chat/completions` | 채팅 완성 (스트리밍/비스트리밍) |
| POST | `/v1/completions` | 텍스트 완성 (레거시) |
| POST | `/v1/embeddings` | 텍스트 임베딩 |
| GET | `/v1/models` | 사용 가능한 모델 목록 |
| GET | `/v1/models/:model` | 특정 모델 정보 |

### 요청/응답 스키마
**Chat Completions 요청 (OpenAI 호환)**
```json
{
  "model": "anthropic/claude-sonnet-4-20250514",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "temperature": 0.7,
  "max_tokens": 1024,
  "stream": false,
  "stream_options": {"include_usage": true}
}
```

**모델 식별자 포맷**
- `provider/model_name` (예: `anthropic/claude-sonnet-4-20250514`)
- `openai/gpt-4o`, `google/gemini-2.0-flash` 등
- Provider 없이 모델명만 제공 시 기본 Provider로 라우팅

**응답 포맷 (OpenAI 호환)**
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "anthropic/claude-sonnet-4-20250514",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hello!"},
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 5,
    "total_tokens": 15
  }
}
```

### 미들웨어 체인
요청 처리 파이프라인 (순서대로):
1. Request ID 주입 (tracing)
2. 인증 미들웨어 (Virtual Key 검증)
3. 요청 파싱 및 유효성 검사
4. 모델 식별자 파싱 (provider + model 추출)
5. Provider 라우팅
6. Provider 어댑터 변환
7. Upstream 요청 실행
8. 응답 변환 (OpenAI 포맷으로 정규화)
9. 사용량 기록 (비동기)
10. 응답 반환

### 오류 응답 포맷 (OpenAI 호환)
```json
{
  "error": {
    "message": "Invalid model: unknown/model",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

### 요청 유효성 검사
- `model` 필드 필수
- `messages` 배열 필수 (chat completions)
- `temperature` 범위: 0.0 ~ 2.0
- `max_tokens` 양수
- 지원하지 않는 파라미터는 무시 (pass-through는 Provider 어댑터에서 처리)

### 헤더 처리
- **수신**: `Authorization: Bearer {virtual_key}`
- **전달 제외**: Authorization 헤더 (Provider 키로 교체)
- **추가**: `X-Request-ID`, `X-Gateway-Version`

---

## 기술 설계 포인트

- **핸들러 함수 시그니처**: `func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request)`
- **라우터**: `chi` 또는 표준 `net/http ServeMux` (Go 1.22 패턴 매칭)
- **컨텍스트 전파**: 요청 컨텍스트에 인증 정보, 요청 ID, 라우팅 메타데이터 주입
- **스트리밍 분기**: `stream: true` 시 SSE 핸들러로 분기, 별도 응답 경로
- **응답 ID 생성**: `chatcmpl-{ulid}` 포맷으로 고유성 보장
- **타임아웃 계층**: 클라이언트 연결 타임아웃 > 요청 처리 타임아웃 > Provider 호출 타임아웃

---

## 의존성

- `01-project-setup.md` 완료 후 진행
- Provider Adapter 인터페이스 정의 필요 (03과 병렬 개발 가능)

---

## 완료 기준

- [ ] `curl -X POST /v1/chat/completions` 요청 처리 성공
- [ ] `model: "anthropic/claude-opus-4-20250514"` 식별자 파싱 정확도 100%
- [ ] OpenAI Python SDK로 `base_url` 변경 후 정상 동작 확인
- [ ] 잘못된 요청에 대해 OpenAI-compatible 오류 응답 반환
- [ ] `/v1/models` 엔드포인트 등록된 모델 목록 반환
- [ ] 스트리밍/비스트리밍 모두 동작 (실제 Provider 연결 전 mock 응답으로 확인)
- [ ] 단위 테스트 커버리지 80%+

---

## 예상 산출물

- `internal/gateway/handler/chat.go`
- `internal/gateway/handler/completions.go`
- `internal/gateway/handler/embeddings.go`
- `internal/gateway/handler/models.go`
- `internal/gateway/middleware/requestid.go`
- `internal/gateway/middleware/auth.go`
- `internal/gateway/router/router.go`
- `internal/gateway/types/openai.go` (요청/응답 타입 정의)
- `internal/provider/interface.go` (Provider 인터페이스)
- `internal/gateway/handler/chat_test.go`
