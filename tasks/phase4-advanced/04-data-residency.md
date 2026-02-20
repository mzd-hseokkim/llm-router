# 04. 데이터 레지던시 (Data Residency)

## 목표
GDPR, CCPA 등 지역별 데이터 보호 규정을 준수하기 위해 사용자 데이터(프롬프트/응답)를 특정 지역 내에서만 처리되도록 강제하는 데이터 레지던시 제어를 구현한다.

---

## 요구사항 상세

### 데이터 레지던시 요구사항 예시
| 지역 | 규정 | 요구사항 |
|------|------|---------|
| EU | GDPR | EU 내 데이터 처리 강제 |
| 한국 | 개인정보보호법 | 국내 데이터 처리 원칙 |
| 미국 정부 | FedRAMP | 특정 승인 클라우드만 사용 |
| 의료 (US) | HIPAA | BAA 체결된 Provider만 사용 |

### 레지던시 설정
```yaml
# 조직/팀 수준 설정
data_residency:
  enabled: true
  region_policy: "eu-only"  # 커스텀 정책 이름

policies:
  eu-only:
    allowed_regions: ["eu-west-1", "eu-central-1"]
    allowed_providers:
      - name: openai
        region: europe  # OpenAI EU 엔드포인트
      - name: anthropic
        region: eu
      - name: azure
        region: westeurope
    blocked_providers: ["gemini"]  # 구글은 EU 전용 엔드포인트 미지원 시

  us-only:
    allowed_regions: ["us-east-1", "us-west-2"]
    allowed_providers: [openai, anthropic, azure, bedrock]

  hipaa-compliant:
    allowed_providers:
      - name: azure
        note: "HIPAA BAA 체결 완료"
      - name: aws_bedrock
        note: "HIPAA BAA 체결 완료"
    blocked_providers: [openai, anthropic, gemini]
```

### 라우팅 강제 적용
```go
func (r *DataResidencyRouter) Route(ctx context.Context, req *ChatRequest) (*Target, error) {
    policy := r.getPolicy(ctx)  // 요청자의 데이터 레지던시 정책

    allowed := r.filterAllowedTargets(req.Targets, policy)
    if len(allowed) == 0 {
        return nil, ErrNoCompliantProvider{
            RequestedModel: req.Model,
            Policy:         policy.Name,
            BlockedReason:  "데이터 레지던시 정책 위반",
        }
    }

    return r.baseRouter.Route(ctx, req, allowed)
}
```

### 지역별 Provider 엔드포인트 설정
```yaml
providers:
  - name: openai_eu
    type: openai
    base_url: https://api.openai.com/v1  # OpenAI는 EU 전용 엔드포인트 없음
    region: global
    compliant_regions: []  # EU 데이터 레지던시 미충족

  - name: azure_eu
    type: azure
    base_url: https://myresource.openai.azure.com
    region: westeurope
    compliant_regions: [eu]
    data_governance:
      gdpr_compliant: true
      hipaa_compliant: false

  - name: bedrock_us
    type: bedrock
    region: us-east-1
    compliant_regions: [us, hipaa]
    data_governance:
      hipaa_compliant: true
      baa_signed: true
```

### 요청 지역 감지
```go
// 클라이언트 IP 기반 지역 추정 (선택적)
func detectRegion(r *http.Request) string {
    ip := realIP(r)
    country, _ := geoip.Lookup(ip)
    return countryToRegion(country)
}

// 명시적 헤더 (B2B 신뢰된 클라이언트)
func regionFromHeader(r *http.Request) string {
    return r.Header.Get("X-Data-Residency-Region")
}
```

### 정책 위반 응답
```json
{
  "error": {
    "message": "No compliant provider available for data residency policy 'eu-only'. Model 'gemini/gemini-2.0-flash' is not available in EU-compliant endpoints.",
    "type": "data_residency_violation",
    "code": "no_compliant_provider",
    "param": {
      "policy": "eu-only",
      "requested_model": "gemini/gemini-2.0-flash",
      "available_models": ["azure/gpt-4o-eu", "anthropic-eu/claude-sonnet-4-20250514"]
    }
  }
}
```

### 감사 및 컴플라이언스 보고
- 모든 요청의 처리 지역 기록
- 지역별 데이터 흐름 리포트
- 정책 위반 시도 감사 로그

### 관리 API
```
GET  /admin/data-residency/policies         # 정책 목록
POST /admin/data-residency/policies         # 정책 생성
PUT  /admin/data-residency/policies/:name   # 정책 수정
POST /admin/data-residency/validate         # 요청이 정책 준수 여부 확인
GET  /admin/data-residency/report           # 지역별 데이터 처리 보고서
```

---

## 기술 설계 포인트

- **정책 우선순위**: 키 > 사용자 > 팀 > 조직 순으로 가장 제한적인 정책 적용
- **캐싱**: 정책 조회는 Redis 캐싱 (1분 TTL)
- **테스트 가능성**: 정책 시뮬레이터 API로 배포 전 검증

---

## 의존성

- `phase3-enterprise/07-advanced-routing.md` 완료
- `phase3-enterprise/09-audit-log.md` 완료

---

## 완료 기준

- [ ] EU-only 정책 설정 후 비EU Provider 요청 시 오류 반환 확인
- [ ] HIPAA 정책에서 미승인 Provider 차단 확인
- [ ] 정책 검증 API로 사전 확인 동작 확인
- [ ] 데이터 처리 지역 감사 로그 기록 확인

---

## 예상 산출물

- `internal/residency/` (디렉토리)
  - `policy.go`, `router.go`, `report.go`
- `config/residency-policies.yaml`
- `internal/gateway/middleware/residency.go`
- `internal/gateway/handler/admin/residency.go`
- `internal/residency/router_test.go`
