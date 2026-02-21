# 04. 가드레일 (Guardrails)

## 목표
LLM 요청/응답에서 프롬프트 인젝션, PII(개인식별정보), 유해 컨텐츠를 탐지하고 필터링한다. 입력(Request-side)과 출력(Response-side) 양방향 보호를 구현하며, 스트리밍 응답에서도 동작한다.

---

## 요구사항 상세

### 가드레일 유형
| 유형 | 방향 | 설명 |
|------|------|------|
| PII 탐지/마스킹 | 입력+출력 | 신용카드, 이메일, 전화번호, 주민번호 등 |
| 프롬프트 인젝션 탐지 | 입력 | 악의적 지시 삽입 시도 차단 |
| 컨텐츠 필터링 | 입력+출력 | 혐오/폭력/성인 컨텐츠 |
| 토픽 제한 | 입력 | 특정 주제 논의 차단 |
| 커스텀 키워드 필터 | 입력+출력 | 사용자 정의 금지 단어/패턴 |

### PII 탐지 패턴
```go
var PIIPatterns = map[string]*regexp.Regexp{
    "credit_card":    regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
    "ssn":            regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
    "email":          regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
    "phone_us":       regexp.MustCompile(`\b(?:\+1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
    "ip_address":     regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
    "korean_rrn":     regexp.MustCompile(`\b\d{6}-[1-4]\d{6}\b`),  // 주민등록번호
}

func MaskPII(text string, patterns map[string]*regexp.Regexp) string {
    for category, re := range patterns {
        text = re.ReplaceAllStringFunc(text, func(match string) string {
            return fmt.Sprintf("[%s_REDACTED]", strings.ToUpper(category))
        })
    }
    return text
}
```

### 프롬프트 인젝션 탐지 (규칙 기반)
```go
var InjectionPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)ignore (all |previous |above )?instructions?`),
    regexp.MustCompile(`(?i)you are now`),
    regexp.MustCompile(`(?i)act as (if you are|a|an)`),
    regexp.MustCompile(`(?i)forget (everything|all|your) (you|previous)`),
    regexp.MustCompile(`(?i)system prompt|initial instructions`),
    regexp.MustCompile(`(?i)jailbreak|dan mode|developer mode`),
}
```

### 가드레일 정책 설정
```yaml
# 팀/조직별 설정 가능
guardrails:
  pii:
    enabled: true
    action: mask     # block | mask | log_only
    categories: [credit_card, ssn, email, phone]

  prompt_injection:
    enabled: true
    action: block
    sensitivity: medium   # low | medium | high

  content_filter:
    enabled: true
    action: block
    categories: [hate, violence, sexual]

  topic_restrictions:
    enabled: true
    blocked_topics: ["competitor analysis", "legal advice"]
    action: block

  custom_keywords:
    enabled: true
    blocked: ["internal-secret", "confidential-project"]
    action: block
```

### 가드레일 실행 파이프라인
```
입력 요청
  ↓
입력 가드레일 체크
  ├── PII 마스킹/차단
  ├── 프롬프트 인젝션 탐지
  ├── 컨텐츠 필터
  └── 커스텀 패턴
  ↓
Provider 요청
  ↓
출력 가드레일 체크
  ├── PII 마스킹
  ├── 컨텐츠 필터
  └── 커스텀 패턴
  ↓
클라이언트 응답
```

### 스트리밍 가드레일
**전략 A: 버퍼 + 스캔**
- N개 토큰(기본 100)마다 누적 텍스트 스캔
- PII 발견 시 해당 토큰부터 마스킹
- 완료 시 전체 텍스트 최종 스캔

**전략 B: 지연 전송**
- 전체 응답 버퍼링 후 필터링 → 클라이언트 전송
- 레이턴시 증가 but 정확성 보장
- 설정으로 선택 가능

**기본값**: 전략 A (실시간성 우선)

### 차단 응답
```json
{
  "error": {
    "message": "Request blocked by content policy: PII detected in prompt.",
    "type": "content_policy_violation",
    "code": "pii_detected",
    "param": {
      "guardrail": "pii",
      "category": "credit_card"
    }
  }
}
```

### 가드레일 이벤트 로깅
```json
{
  "event": "guardrail_triggered",
  "guardrail": "pii",
  "action": "masked",
  "direction": "input",
  "request_id": "req_xxx",
  "categories": ["credit_card"],
  "timestamp": "2026-01-01T00:00:00Z"
}
```

### 성능 요구사항
- 가드레일 처리 오버헤드: < 5ms (P95)
- 비동기 처리: 로그 기록만 하는 `log_only` 액션은 비동기

---

## 기술 설계 포인트

- **플러그인 아키텍처**: 각 가드레일은 `Guardrail` 인터페이스 구현체로 플러그인 방식
- **캐싱**: 동일 텍스트에 대한 가드레일 결과 캐싱 (짧은 TTL)
- **설정 상속**: 팀 설정이 조직 설정을 오버라이드 (더 엄격한 쪽 우선)
- **성능 모니터링**: 가드레일별 처리 시간 메트릭 기록

---

## 의존성

- `phase1-mvp/02-openai-compatible-api.md` 완료
- `phase3-enterprise/01-multi-tenancy.md` 완료 (팀별 설정)

---

## 완료 기준

- [ ] 신용카드 번호가 포함된 요청 마스킹/차단 확인
- [ ] 프롬프트 인젝션 시도 차단 확인
- [ ] 스트리밍 응답에서도 PII 마스킹 동작 확인
- [ ] 가드레일 이벤트 로그 기록 확인
- [ ] 가드레일 오버헤드 < 5ms 성능 테스트 통과

---

## 예상 산출물

- `internal/guardrail/` (디렉토리)
  - `interface.go`, `pipeline.go`
  - `pii/detector.go`, `pii/masker.go`
  - `injection/detector.go`
  - `content/filter.go`
  - `keyword/filter.go`
- `internal/gateway/middleware/guardrail.go`
- `internal/guardrail/pii/detector_test.go`
- `internal/guardrail/injection/detector_test.go`
