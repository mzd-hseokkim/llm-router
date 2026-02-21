# 06. 시맨틱 캐싱 (Semantic Cache)

## 목표
정확히 동일하지 않지만 의미적으로 유사한 프롬프트에 대해 캐시된 응답을 반환한다. 임베딩 기반 코사인 유사도 검색으로 의미 유사성을 판단하며, 벡터 DB(pgvector)를 활용한다.

---

## 요구사항 상세

### 동작 흐름
```
1. 요청 수신
2. 정확 매칭 캐시 체크 (miss 시)
3. 요청 텍스트 임베딩 생성
4. 벡터 DB에서 유사 임베딩 검색 (cosine similarity ≥ threshold)
5. 유사 응답 발견 시 → 캐시 응답 반환
6. 미발견 시 → Provider 요청 실행
7. 응답 + 임베딩을 벡터 DB에 저장
```

### 임베딩 생성
```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dimensions() int
    ModelName() string
}

// 기본: OpenAI text-embedding-3-small (1536차원)
// 선택: Cohere embed, Sentence Transformers (로컬)
```

**캐시 키용 텍스트 생성**:
```go
func buildCacheText(req *ChatRequest) string {
    // 마지막 user 메시지만 사용 (또는 전체 대화)
    lastUserMsg := getLastUserMessage(req.Messages)
    return fmt.Sprintf("[model:%s] %s", req.Model, lastUserMsg)
}
```

### pgvector 스키마
```sql
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE semantic_cache (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model           VARCHAR(200) NOT NULL,
    embedding       vector(1536),           -- OpenAI text-embedding-3-small
    prompt_text     TEXT NOT NULL,
    response_json   JSONB NOT NULL,
    prompt_tokens   INTEGER,
    completion_tokens INTEGER,
    cost_usd        DECIMAL(12,8),
    hit_count       INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL
);

-- IVFFlat 인덱스 (ANN 검색)
CREATE INDEX idx_semantic_cache_embedding ON semantic_cache
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
```

### 유사도 검색 쿼리
```sql
SELECT
    id,
    response_json,
    1 - (embedding <=> $1) AS similarity,  -- cosine similarity
    cost_usd
FROM semantic_cache
WHERE
    model = $2
    AND expires_at > NOW()
ORDER BY embedding <=> $1  -- cosine distance ascending
LIMIT 1;
```

### 유사도 임계값 설정
```yaml
cache:
  semantic:
    enabled: true
    threshold: 0.95         # 기본 95% 유사도
    embedding_model: "openai/text-embedding-3-small"
    max_cache_entries: 100000
    ttl: 86400              # 24시간
```

- **임계값 높을수록**: 더 엄격한 매칭 (오매칭 감소, 히트율 감소)
- **임계값 낮을수록**: 더 넓은 매칭 (오매칭 증가, 히트율 증가)
- 모델별 임계값 별도 설정 가능

### 캐시 키 구성 요소
유사도 검색은 임베딩으로, 추가 필터는 메타데이터로:
- `model`: 동일 모델만 매칭 (필수)
- `temperature`: 0일 때만 캐시 (결정론적)
- `tools`: tool calling 활성화 여부 (호환성)

### 캐시 무효화 및 관리
- **TTL 기반**: 자동 만료
- **LRU 퇴출**: 최대 항목 수 초과 시 오래된 순서로 삭제
- **수동 삭제**: 특정 모델의 캐시 전체 삭제

### 임베딩 비용 최적화
- 임베딩 자체도 캐싱 (동일 텍스트 재요청 시)
- 배치 임베딩: 여러 텍스트를 한 번에 임베딩 처리
- 임베딩 생성 실패 시 시맨틱 캐시 건너뛰고 정상 처리 (비차단)

### 응답 헤더
```
X-Cache: SEMANTIC_HIT
X-Cache-Similarity: 0.97
X-Cache-Type: semantic
```

### 메트릭
- `semantic_cache_requests_total{result="hit|miss"}`
- `semantic_cache_similarity_histogram` (매칭된 유사도 분포)
- `semantic_cache_embedding_duration_ms`

---

## 기술 설계 포인트

- **임베딩 비용**: 임베딩 생성도 비용 발생 → 캐시 히트율이 임베딩 비용보다 높을 때 유효
- **인덱스 재구성**: IVFFlat 인덱스는 데이터 삽입 후 `VACUUM ANALYZE` 필요
- **실패 내성**: 임베딩 API 실패 시 시맨틱 캐시를 건너뛰고 정상 처리
- **병렬 처리**: 임베딩 생성과 정확 매칭 캐시 조회를 병렬 실행

---

## 의존성

- `phase3-enterprise/05-exact-caching.md` 완료
- pgvector PostgreSQL 확장 설치

---

## 완료 기준

- [ ] "오늘 날씨 어때?" → "오늘 날씨가 어떤가요?" 시맨틱 히트 확인
- [ ] 임계값 0.95에서 무관한 질문 매칭 안 됨 확인
- [ ] 임베딩 API 실패 시 정상 Provider 요청 진행 확인
- [ ] `X-Cache: SEMANTIC_HIT` + 유사도 헤더 반환 확인
- [ ] 100,000 캐시 항목에서 검색 < 10ms 성능 테스트

---

## 예상 산출물

- `internal/cache/semantic/cache.go`
- `internal/cache/semantic/embedder.go`
- `internal/store/postgres/vector_store.go`
- `migrations/011_create_semantic_cache.sql`
- `internal/cache/semantic/cache_test.go`
