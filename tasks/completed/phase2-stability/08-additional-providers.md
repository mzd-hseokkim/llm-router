# 08. 추가 Provider (Azure OpenAI, AWS Bedrock, Mistral)

## 목표
Phase 1의 3개 Provider(OpenAI, Anthropic, Gemini) 외에 Azure OpenAI, AWS Bedrock, Mistral을 추가 지원한다. 기존 Provider 어댑터 패턴을 활용하여 일관된 구조로 구현한다.

---

## 요구사항 상세

### Azure OpenAI 어댑터
- **API 베이스**: `https://{resource_name}.openai.azure.com/openai/deployments/{deployment_id}`
- **인증**: `api-key: {azure_api_key}` 헤더
- **API 버전 파라미터**: `?api-version=2024-02-01`
- **엔드포인트**: `POST /chat/completions` (기본 OpenAI와 동일 구조)
- **모델 식별자**: `azure/{deployment_id}` (예: `azure/gpt-4o-prod`)

**설정 구조**
```yaml
providers:
  - name: azure_openai
    type: azure
    resource_name: myresource
    deployments:
      - id: gpt-4o-prod
        model: gpt-4o
        api_key: ${AZURE_API_KEY}
        api_version: "2024-02-01"
```

**요청 변환**
- 거의 동일한 OpenAI 포맷
- `model` 파라미터 제거 (deployment ID가 URL에 포함)
- `azure-` 프리픽스 처리

**응답 변환**
- OpenAI와 동일한 응답 구조
- `model` 필드를 `azure/{deployment_id}` 로 정규화

### AWS Bedrock 어댑터
- **API**: AWS SDK v2 (`github.com/aws/aws-sdk-go-v2/service/bedrockruntime`)
- **인증**: AWS Signature V4 (IAM 역할 또는 Access Key + Secret)
- **API 엔드포인트**: Converse API (`POST /model/{model_id}/converse`)
- **모델 식별자**: `bedrock/{model_id}` (예: `bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0`)

**인증 설정**
```yaml
providers:
  - name: bedrock
    type: bedrock
    region: us-east-1
    auth:
      type: iam_role  # or "access_key"
      access_key_id: ${AWS_ACCESS_KEY_ID}
      secret_access_key: ${AWS_SECRET_ACCESS_KEY}
```

**Converse API 요청 변환** (OpenAI → Bedrock)
```json
{
  "modelId": "anthropic.claude-3-5-sonnet-20241022-v2:0",
  "messages": [
    {
      "role": "user",
      "content": [{"text": "Hello"}]
    }
  ],
  "system": [{"text": "You are a helpful assistant."}],
  "inferenceConfig": {
    "maxTokens": 1024,
    "temperature": 0.7
  }
}
```

**Converse API 응답 변환** (Bedrock → OpenAI)
```json
{
  "output": {
    "message": {
      "role": "assistant",
      "content": [{"text": "Hello!"}]
    }
  },
  "stopReason": "end_turn",
  "usage": {
    "inputTokens": 10,
    "outputTokens": 5
  }
}
```

**스트리밍**: Bedrock ConverseStream API 사용

### Mistral 어댑터
- **API 베이스**: `https://api.mistral.ai/v1`
- **인증**: `Authorization: Bearer {api_key}`
- **포맷**: OpenAI와 거의 동일 (최소 변환)
- **모델 식별자**: `mistral/{model_name}` (예: `mistral/mistral-large-latest`)
- **특수 처리**: `safe_prompt` 파라미터 지원

**지원 모델**
- `mistral-large-latest`
- `mistral-small-latest`
- `codestral-latest` (코드 생성)
- `mistral-embed` (임베딩)

### Cohere 어댑터
- **API 베이스**: `https://api.cohere.com/v2`
- **인증**: `Authorization: Bearer {api_key}`
- **모델 식별자**: `cohere/{model_name}`
- **포맷 변환**: chat_history vs messages 구조 차이 처리

**요청 변환 (OpenAI → Cohere v2)**
```json
{
  "model": "command-r-plus-08-2024",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "max_tokens": 1024
}
```

### Provider 어댑터 레지스트리 확장
```go
func RegisterProviders(registry *provider.Registry, config *Config) {
    registry.Register("openai", openai.New(config.OpenAI))
    registry.Register("anthropic", anthropic.New(config.Anthropic))
    registry.Register("gemini", gemini.New(config.Gemini))
    registry.Register("azure", azure.New(config.Azure))      // NEW
    registry.Register("bedrock", bedrock.New(config.Bedrock)) // NEW
    registry.Register("mistral", mistral.New(config.Mistral)) // NEW
    registry.Register("cohere", cohere.New(config.Cohere))    // NEW
}
```

### 모델 가격 업데이트 (models.yaml)
각 Provider의 모델 가격 정보 추가:
- Azure: deployment별 별도 가격 설정 가능
- Bedrock: 모델별 on-demand 가격
- Mistral: 각 모델 tier별 가격
- Cohere: Command 모델 가격

---

## 기술 설계 포인트

- **Provider별 연결 풀**: 각 Provider는 독립적인 HTTP 클라이언트 (연결 설정 최적화)
- **AWS SDK 통합**: `aws-sdk-go-v2`의 기존 인증 체인 활용 (환경변수, EC2 메타데이터, IAM Role)
- **Bedrock 리전**: 모델 가용성에 따른 멀티 리전 지원
- **동일 테스트 패턴**: 각 어댑터에 동일한 테스트 픽스처 적용

---

## 의존성

- `phase1-mvp/03-provider-adapters.md` 완료 (어댑터 패턴)
- `phase1-mvp/06-provider-key-management.md` 완료

---

## 완료 기준

- [ ] Azure OpenAI deployment로 채팅 완성 성공
- [ ] AWS Bedrock Claude 모델로 채팅 완성 성공
- [ ] Mistral Large로 채팅 완성 성공
- [ ] 각 Provider 스트리밍 동작 확인
- [ ] 각 Provider 오류 정규화 테스트 통과
- [ ] 모든 신규 Provider의 모델 가격 정보 등록 확인

---

## 예상 산출물

- `internal/provider/azure/adapter.go`
- `internal/provider/azure/transform.go`
- `internal/provider/bedrock/adapter.go`
- `internal/provider/bedrock/transform.go`
- `internal/provider/mistral/adapter.go`
- `internal/provider/cohere/adapter.go`
- `config/models.yaml` (신규 모델 추가)
- `internal/provider/azure/transform_test.go`
- `internal/provider/bedrock/transform_test.go`
