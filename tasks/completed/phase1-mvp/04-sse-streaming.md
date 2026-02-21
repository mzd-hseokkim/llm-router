# 04. SSE 스트리밍 프록시

## 목표
Provider의 Server-Sent Events 스트림을 클라이언트에게 실시간으로 중계하는 스트리밍 프록시를 구현한다. TCP 청크 경계와 SSE 이벤트 경계의 불일치를 올바르게 처리하고, 백프레셔 및 연결 관리를 안정적으로 수행한다.

---

## 요구사항 상세

### SSE 스트리밍 흐름
```
Client → Gateway → Provider
         ↑
    SSE 중계 (OpenAI delta 포맷 통일)
```

1. 클라이언트 요청 `stream: true`
2. Gateway가 Provider에 스트리밍 요청
3. Provider SSE 청크를 수신하며 파싱
4. OpenAI delta 포맷으로 변환
5. 클라이언트에게 즉시 전달 (flush)

### SSE 클라이언트 응답 포맷
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","created":1234567890,"model":"anthropic/claude-sonnet-4-20250514","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
```

### SSE 파서 요구사항
- **TCP 청크 경계 처리**: 하나의 SSE 이벤트가 여러 TCP 패킷에 걸쳐 올 수 있음
  - 연결별 라인 버퍼 유지 (`bufio.Scanner` 또는 상태 머신)
  - `\n\n` (빈 줄)을 이벤트 경계로 인식
- **불완전 이벤트 처리**: 이벤트 중간에 청크가 끊기면 버퍼에 누적 후 완성 시 처리
- **멀티 라인 `data:` 필드**: `data:` 라인이 여러 개인 경우 개행으로 연결
- **이벤트 타입 파싱**: `event:`, `data:`, `id:`, `retry:` 필드 처리
- **[DONE] 감지**: `data: [DONE]` 수신 시 스트리밍 종료

### Provider별 SSE 파싱
**OpenAI SSE 포맷** (기준)
```
data: {"choices":[{"delta":{"content":"text"}}]}
data: [DONE]
```

**Anthropic SSE 포맷**
```
event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"text"}}

event: message_stop
data: {"type":"message_stop"}
```

**Gemini SSE 포맷** (누적형 응답)
```
data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"}}]}
data: {"candidates":[{"content":{"parts":[{"text":"Hello world"}],"role":"model"},"finishReason":"STOP"}]}
```
- Gemini는 누적형(cumulative)이므로 이전 텍스트를 제외한 delta 계산 필요

### 백프레셔 관리
- **Write Buffer**: 연결별 write buffer (기본 32KB)
- **High-Water Mark**: 버퍼가 임계값 초과 시 upstream 읽기 일시 중단
- **Slow Client 처리**: 클라이언트가 너무 느리면 연결 강제 종료 (write timeout: 30s)
- **ResponseController**: Go 1.20+의 `http.ResponseController` 사용하여 flush 제어

### 연결 관리
- **클라이언트 연결 종료 감지**: `r.Context().Done()` 채널 모니터링
  - 클라이언트 연결 끊김 시 Provider upstream 요청 즉시 취소
- **Keepalive**: 30초 이상 토큰이 없으면 `: keepalive` 코멘트 전송
- **연결 타임아웃**: 스트리밍 최대 시간 설정 (기본 300s)

### 토큰 사용량 추적 (스트리밍)
- `stream_options: {"include_usage": true}` 자동 주입
- 스트림 마지막 청크의 `usage` 필드 파싱
- Provider가 usage를 미제공 시 실시간 토큰 카운팅으로 추정

### 응답 헤더 설정
```
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-cache, no-transform
Connection: keep-alive
X-Accel-Buffering: no          # Nginx 버퍼링 비활성화
Transfer-Encoding: chunked     # HTTP/1.1
```

---

## 기술 설계 포인트

- **채널 기반 파이프라인**: Provider SSE → `chan StreamChunk` → 클라이언트 전송
  ```go
  type StreamChunk struct {
      Delta      string
      FinishReason *string
      Usage      *Usage
      Error      error
  }
  ```
- **고루틴 구조**:
  1. Provider 수신 고루틴: SSE 읽기 및 파싱 → 채널 발행
  2. 클라이언트 전송 고루틴: 채널 소비 → flush
  3. 두 고루틴 모두 컨텍스트 취소 시 종료
- **에러 스트리밍**: 스트림 중간 에러 발생 시 SSE error 이벤트 전송 후 종료
- **메모리 효율**: 응답 본문을 메모리에 버퍼링하지 않고 즉시 전달

---

## 의존성

- `02-openai-compatible-api.md` 완료
- `03-provider-adapters.md` 완료 (Provider별 SSE 파싱 로직)

---

## 완료 기준

- [ ] 세 Provider(OpenAI, Anthropic, Gemini) 스트리밍 동작 확인
- [ ] TCP 청크 분할 상황 시뮬레이션 테스트 통과
- [ ] 클라이언트 연결 종료 시 upstream 요청 취소 확인
- [ ] `curl -N` 명령으로 실시간 토큰 수신 확인
- [ ] OpenAI Python SDK의 스트리밍 모드로 정상 동작 확인
- [ ] 스트리밍 완료 후 토큰 사용량 정확히 기록
- [ ] 10분 이상 장기 스트리밍 연결 안정성 테스트

---

## 예상 산출물

- `internal/gateway/proxy/stream.go`
- `internal/gateway/proxy/sse_parser.go`
- `internal/provider/openai/stream.go`
- `internal/provider/anthropic/stream.go`
- `internal/provider/gemini/stream.go`
- `internal/gateway/handler/chat_stream.go`
- `internal/gateway/proxy/stream_test.go`
- `internal/gateway/proxy/sse_parser_test.go`
