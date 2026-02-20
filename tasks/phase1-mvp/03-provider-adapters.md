# 03. Provider Adapter (OpenAI, Anthropic, Gemini)

## 목표
각 LLM Provider의 고유 API 포맷을 OpenAI-compatible 포맷과 상호 변환하는 어댑터 레이어를 구현한다. Phase 1에서는 OpenAI, Anthropic Claude, Google Gemini 세 Provider를 지원한다.

---

## 요구사항 상세

### Provider 어댑터 인터페이스
```go
type Provider interface {
    // OpenAI 포맷 요청을 Provider 고유 포맷으로 변환 후 실행
    ChatCompletion(ctx context.Context, req *types.ChatRequest) (*types.ChatResponse, error)
    // 스트리밍: Provider SSE를 OpenAI SSE 이벤트로 변환
    ChatCompletionStream(ctx context.Context, req *types.ChatRequest) (<-chan types.StreamChunk, error)
    // 임베딩 요청 처리
    Embeddings(ctx context.Context, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error)
    // Provider 식별자
    Name() string
    // 지원 모델 목록
    SupportedModels() []string
}
```

### OpenAI 어댑터
- **API 베이스**: `https://api.openai.com/v1`
- **인증**: `Authorization: Bearer {api_key}`
- **요청 변환**: 거의 변환 없음 (기준 포맷)
- **특수 처리**:
  - `stream_options.include_usage: true` 자동 주입 (토큰 사용량 추적)
  - `model` 필드에서 `openai/` 프리픽스 제거
- **지원 기능**: Chat, Completions, Embeddings, Images, Audio, Tool Calling, Vision

### Anthropic Claude 어댑터
- **API 베이스**: `https://api.anthropic.com/v1`
- **인증**: `x-api-key: {api_key}` 헤더 (Authorization 아님)
- **API 버전 헤더**: `anthropic-version: 2023-06-01`
- **요청 변환 (OpenAI → Anthropic)**:
  ```
  messages[].role: "assistant" → "assistant" (동일)
  messages[].role: "user" → "user" (동일)
  system 메시지 → system 파라미터로 분리
  max_tokens → max_tokens (필수, 미제공 시 기본값 4096 주입)
  tool_choice: "auto" → {"type": "auto"}
  ```
- **응답 변환 (Anthropic → OpenAI)**:
  ```
  content[].text → choices[].message.content
  stop_reason: "end_turn" → finish_reason: "stop"
  stop_reason: "max_tokens" → finish_reason: "length"
  usage.input_tokens → usage.prompt_tokens
  usage.output_tokens → usage.completion_tokens
  ```
- **스트리밍 이벤트 매핑**:
  ```
  message_start → (무시 또는 usage 추출)
  content_block_delta.delta.text → choices[].delta.content
  message_delta.stop_reason → choices[].finish_reason
  message_stop → [DONE]
  ```
- **지원 기능**: Chat, Tool Calling, Vision, Prompt Caching

### Google Gemini 어댑터
- **API 베이스**: `https://generativelanguage.googleapis.com/v1beta`
- **인증**: `?key={api_key}` 쿼리 파라미터 또는 `Authorization: Bearer {token}`
- **요청 변환 (OpenAI → Gemini)**:
  ```
  messages[] → contents[]
  message.role: "user" → content.role: "user"
  message.role: "assistant" → content.role: "model"
  message.content: string → content.parts[].text
  system message → systemInstruction.parts[].text
  max_tokens → generationConfig.maxOutputTokens
  temperature → generationConfig.temperature
  top_p → generationConfig.topP
  stop → generationConfig.stopSequences
  ```
- **응답 변환 (Gemini → OpenAI)**:
  ```
  candidates[0].content.parts[0].text → choices[0].message.content
  candidates[0].finishReason: "STOP" → finish_reason: "stop"
  usageMetadata.promptTokenCount → usage.prompt_tokens
  usageMetadata.candidatesTokenCount → usage.completion_tokens
  ```
- **스트리밍**: SSE 포맷 (text/event-stream), 각 chunk는 전체 response 포함 (누적형)
- **지원 기능**: Chat, Embeddings, Vision, Tool Calling

### HTTP 클라이언트 설정 (공통)
- 연결 풀링: Provider별 최대 연결 수 설정 (기본 100)
- 타임아웃: Dial 5s, TLS 5s, 응답 헤더 30s, 전체 요청 120s
- HTTP/2 지원
- 자동 압축 해제 (gzip)
- 재시도는 어댑터 외부 레이어에서 처리 (어댑터는 단일 시도만)

### 모델 식별자 레지스트리
```yaml
# config/models.yaml
models:
  - id: "anthropic/claude-opus-4-20250514"
    provider: "anthropic"
    model_name: "claude-opus-4-20250514"
    context_window: 200000
    pricing:
      input_per_million: 15.0
      output_per_million: 75.0
  - id: "openai/gpt-4o"
    provider: "openai"
    model_name: "gpt-4o"
    context_window: 128000
    pricing:
      input_per_million: 2.5
      output_per_million: 10.0
```

---

## 기술 설계 포인트

- **변환 순수 함수**: Request/Response 변환 로직은 부수효과 없는 순수 함수로 구현 (테스트 용이)
- **에러 타입 정규화**: Provider별 고유 오류를 공통 `GatewayError` 타입으로 변환
  - HTTP 429 → `ErrRateLimited`
  - HTTP 5xx → `ErrProviderUnavailable`
  - HTTP 400/401 → `ErrInvalidRequest`
- **스트리밍 어댑터**: Provider SSE 스트림을 파싱하여 OpenAI delta 포맷 채널로 변환
- **멀티모달 처리**: `message.content`가 배열인 경우 (텍스트 + 이미지) 처리
- **Tool Calling 정규화**: OpenAI tool_calls ↔ Anthropic tool_use ↔ Gemini functionCall 상호 변환

---

## 의존성

- `01-project-setup.md` 완료
- `02-openai-compatible-api.md`의 Provider 인터페이스 정의 공유

---

## 완료 기준

- [ ] OpenAI API로 `gpt-4o` 모델 채팅 성공
- [ ] Anthropic API로 `claude-sonnet-4-20250514` 모델 채팅 성공 (system 메시지 포함)
- [ ] Gemini API로 `gemini-2.0-flash` 모델 채팅 성공
- [ ] 세 Provider 모두 스트리밍 동작 확인
- [ ] Provider 고유 오류가 공통 에러 타입으로 정규화됨
- [ ] 각 어댑터의 Request/Response 변환 단위 테스트 통과
- [ ] HTTP 클라이언트 커넥션 풀 설정 확인

---

## 예상 산출물

- `internal/provider/interface.go`
- `internal/provider/openai/adapter.go`
- `internal/provider/openai/transform.go`
- `internal/provider/anthropic/adapter.go`
- `internal/provider/anthropic/transform.go`
- `internal/provider/gemini/adapter.go`
- `internal/provider/gemini/transform.go`
- `internal/provider/errors.go`
- `internal/provider/registry.go`
- `config/models.yaml`
- `internal/provider/openai/transform_test.go`
- `internal/provider/anthropic/transform_test.go`
- `internal/provider/gemini/transform_test.go`
