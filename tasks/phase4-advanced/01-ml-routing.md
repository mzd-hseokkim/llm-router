# 01. ML 기반 지능형 라우팅

## 목표
요청 특성(복잡도, 도메인, 예상 토큰 수)과 Provider 실시간 성능 데이터를 결합하여, 비용과 품질의 최적 균형점으로 자동 라우팅하는 ML 기반 라우팅 엔진을 구현한다.

---

## 요구사항 상세

### 라우팅 결정 요소
**요청 특성 분석**
- 프롬프트 복잡도 추정 (규칙 기반 휴리스틱)
  - 길이, 전문 용어 밀도, 코드 포함 여부
  - 멀티턴 대화 깊이
  - Tool Calling 포함 여부
- 예상 출력 토큰 수 예측 (회귀 모델)
- 요청 카테고리 분류 (코드/수학/창작/지식/일반)

**Provider 실시간 상태**
- 현재 에러율 (1분, 5분 EWMA)
- 평균 레이턴시 (P50, P95 EWMA)
- 현재 토큰 단가
- 쿼터 사용률 (API 제한까지 여유)

**비용-품질 스코어링**
```go
type RoutingScore struct {
    CostScore    float64  // 0.0 (고비용) ~ 1.0 (저비용)
    QualityScore float64  // 0.0 (저품질) ~ 1.0 (고품질)
    LatencyScore float64  // 0.0 (느림) ~ 1.0 (빠름)
    ReliabilityScore float64
}

func (s RoutingScore) Total(weights OptimizationWeights) float64 {
    return s.CostScore*weights.Cost +
           s.QualityScore*weights.Quality +
           s.LatencyScore*weights.Latency +
           s.ReliabilityScore*weights.Reliability
}
```

### 최적화 프로필 (사용자 설정)
```yaml
optimization:
  profile: balanced  # cost_optimized | quality_first | latency_first | balanced

  weights:
    cost: 0.3
    quality: 0.4
    latency: 0.2
    reliability: 0.1

  constraints:
    max_cost_per_request_usd: 0.10
    max_latency_p95_ms: 5000
    min_model_quality_tier: "medium"  # economy | medium | premium
```

### 모델 품질 티어 정의
```yaml
quality_tiers:
  economy:
    models: [gemini-2.0-flash, gpt-4o-mini, claude-haiku-3-5]
    use_cases: [simple_qa, summarization, classification]

  medium:
    models: [gpt-4o, claude-sonnet-4-20250514, gemini-1.5-pro]
    use_cases: [coding, analysis, writing]

  premium:
    models: [claude-opus-4-20250514, gpt-4o, gemini-2.0-pro]
    use_cases: [complex_reasoning, research, critical_decisions]
```

### 요청 복잡도 분류기 (경량 ML)
```python
# 오프라인 학습, Go에서 추론 실행 (ONNX Runtime)
class ComplexityClassifier:
    def features(self, prompt: str) -> np.ndarray:
        return [
            len(prompt) / 1000,           # 정규화된 길이
            count_code_blocks(prompt),    # 코드 블록 수
            count_technical_terms(prompt), # 기술 용어 밀도
            message_count,                # 멀티턴 깊이
            has_tools,                    # Tool Calling
        ]
```

### 피드백 루프 (강화학습)
- 요청 결과 기록 (성공/실패, 실제 레이턴시, 실제 비용)
- 주기적 모델 재학습 (주 1회)
- A/B 테스트로 ML 라우팅 vs 규칙 기반 비교

### Shadow Mode (안전한 배포)
```go
// 기존 라우팅으로 실제 요청 처리, ML 라우팅은 로그만 기록
func (r *MLRouter) RouteWithShadow(ctx context.Context, req *ChatRequest) (*Target, error) {
    actual := r.ruleRouter.Route(ctx, req)
    shadow := r.mlRouter.Route(ctx, req)

    // 비교 로그 기록 (실제로는 actual 사용)
    log.Info("shadow_routing",
        "actual", actual.Model,
        "shadow", shadow.Model,
        "agreement", actual.Model == shadow.Model,
    )

    return actual, nil
}
```

---

## 기술 설계 포인트

- **경량 추론**: 복잡한 ML 모델 대신 규칙 기반 + 간단한 통계 모델로 시작
- **ONNX Runtime**: Go에서 ML 모델 추론 (재컴파일 없이 모델 교체)
- **피처 캐싱**: 동일 프롬프트의 피처 추출 결과 캐싱 (5분 TTL)
- **점진적 배포**: Shadow Mode → 10% 트래픽 → 50% → 100%

---

## 의존성

- `phase3-enterprise/07-advanced-routing.md` 완료
- `phase2-stability/05-cost-tracking.md` 완료
- `phase3-enterprise/08-observability.md` 완료 (피드백 데이터)

---

## 완료 기준

- [ ] 복잡도 분류기가 테스트셋에서 85% 이상 정확도 달성
- [ ] Shadow Mode에서 ML vs 규칙 기반 라우팅 비교 로그 기록 확인
- [ ] cost_optimized 프로필에서 동일 품질 대비 30% 비용 절감 확인
- [ ] ML 라우팅 오버헤드 < 5ms 확인

---

## 예상 산출물

- `internal/router/ml/classifier.go`
- `internal/router/ml/scorer.go`
- `internal/router/ml/shadow.go`
- `internal/router/ml/feedback.go`
- `models/complexity_classifier.onnx`
- `internal/router/ml/classifier_test.go`
