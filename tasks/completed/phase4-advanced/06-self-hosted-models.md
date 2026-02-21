# 06. 자체 호스팅 모델 연동 (Ollama, vLLM, TGI)

## 목표
Ollama, vLLM, Hugging Face Text Generation Inference(TGI) 등 자체 호스팅 LLM 추론 서버를 Gateway에 통합한다. 기업 내부 모델을 클라우드 LLM과 동일한 인터페이스로 사용할 수 있게 한다.

---

## 요구사항 상세

### 지원 자체 호스팅 엔진

**Ollama**
- **API 베이스**: `http://localhost:11434`
- **포맷**: OpenAI-compatible (`/api/chat`, `/api/generate`)
- **모델 식별자**: `ollama/llama3.2:3b`, `ollama/mistral:7b`
- **특수 기능**: 로컬 모델 풀링, 다중 동시 로드

**vLLM**
- **API 베이스**: `http://vllm-server:8000/v1`
- **포맷**: OpenAI-compatible (표준 `/v1/chat/completions`)
- **모델 식별자**: `vllm/meta-llama/Llama-3.1-8B-Instruct`
- **특수 기능**: continuous batching, GPU 최적화, guided decoding

**Hugging Face TGI**
- **API 베이스**: `http://tgi-server:8080`
- **포맷**: OpenAI-compatible (`/v1/chat/completions`) 및 자체 포맷
- **모델 식별자**: `tgi/mistralai/Mistral-7B-Instruct-v0.3`
- **특수 기능**: 양자화, LoRA 어댑터 지원

**LMStudio**
- OpenAI-compatible API (포트 1234)
- 개발 환경용

### 어댑터 구현 (OpenAI-Compatible 기반)
자체 호스팅 엔진은 대부분 OpenAI-compatible API를 제공하므로, 최소한의 커스터마이징으로 OpenAI 어댑터 재활용:

```go
type SelfHostedAdapter struct {
    *openai.Adapter                    // OpenAI 어댑터 임베딩
    baseURL  string                    // 자체 호스팅 서버 주소
    engine   SelfHostedEngine          // ollama | vllm | tgi
}

func (a *SelfHostedAdapter) ChatCompletion(ctx context.Context, req *types.ChatRequest) (*types.ChatResponse, error) {
    // 엔진별 특수 처리
    req = a.adaptRequest(req)
    return a.Adapter.ChatCompletion(ctx, req)
}

func (a *SelfHostedAdapter) adaptRequest(req *types.ChatRequest) *types.ChatRequest {
    switch a.engine {
    case EngineOllama:
        // Ollama 특수 파라미터 처리
    case EngineVLLM:
        // vLLM guided_json 등 추가 파라미터
    }
    return req
}
```

### 설정 구조
```yaml
providers:
  - name: ollama_local
    type: self_hosted
    engine: ollama
    base_url: http://localhost:11434
    models:
      - id: "ollama/llama3.2:3b"
        model_name: "llama3.2:3b"
        context_window: 131072
        pricing:
          input_per_million_tokens: 0    # 무료 (자체 인프라 비용은 별도)
          output_per_million_tokens: 0

  - name: vllm_cluster
    type: self_hosted
    engine: vllm
    base_url: http://vllm-service:8000
    models:
      - id: "vllm/llama3.1-8b"
        model_name: "meta-llama/Llama-3.1-8B-Instruct"
        context_window: 131072
```

### 헬스체크 (자체 호스팅 전용)
```go
func (a *SelfHostedAdapter) HealthCheck(ctx context.Context) error {
    switch a.engine {
    case EngineOllama:
        // GET /api/tags 로 모델 목록 조회
        return a.checkOllamaTags(ctx)
    case EngineVLLM:
        // GET /v1/models 로 로드된 모델 확인
        return a.checkVLLMModels(ctx)
    }
}
```

### 자동 재연결
- 자체 호스팅 서버 재시작 시 자동 재연결 (서킷 브레이커 활용)
- 모델 로딩 시간 고려한 긴 타임아웃 설정 (초기 로드: 30-300초)
- 롤링 업데이트 중 일시적 연결 실패 허용

### 혼합 라우팅 (클라우드 + 자체 호스팅)
```yaml
routes:
  - name: "internal-first"
    match:
      model_prefix: "internal/"
    strategy: failover
    targets:
      - provider: vllm_cluster        # 내부 GPU 클러스터 우선
        model: vllm/llama3.1-70b
        weight: 100
      - provider: openai              # GPU 부족 시 클라우드 폴백
        model: gpt-4o
        weight: 0
```

### 내부 컴퓨팅 비용 추적
```yaml
# 자체 호스팅 모델의 컴퓨팅 비용 설정 (실제 GPU 비용 반영)
pricing:
  vllm_cluster:
    gpu_cost_per_hour: 2.50    # GPU 인스턴스 시간당 비용
    tokens_per_hour: 1000000  # 시간당 처리 토큰 추정
    # → 입력/출력 토큰당 $0.0000025
```

---

## 기술 설계 포인트

- **OpenAI-Compatible 최대 활용**: 자체 구현 최소화, 어댑터 상속으로 코드 재사용
- **모델 사전 로딩**: Ollama/vLLM에 사용 모델 사전 로드 확인
- **스트리밍 검증**: 자체 호스팅 엔진의 SSE 포맷이 표준과 차이 여부 확인 필요

---

## 의존성

- `phase1-mvp/03-provider-adapters.md` 완료
- `phase1-mvp/09-health-check.md` 완료

---

## 완료 기준

- [ ] Ollama (로컬)에서 llama3.2 모델 채팅 성공
- [ ] vLLM 서버와 연동하여 채팅 성공
- [ ] TGI 서버 연동 성공
- [ ] 자체 호스팅 → 클라우드 폴백 동작 확인
- [ ] 자체 호스팅 모델 재시작 후 자동 재연결 확인

---

## 예상 산출물

- `internal/provider/selfhosted/adapter.go`
- `internal/provider/selfhosted/ollama.go`
- `internal/provider/selfhosted/vllm.go`
- `internal/provider/selfhosted/tgi.go`
- `config/models.yaml` (자체 호스팅 모델 추가)
- `docs/self-hosted-setup.md`
