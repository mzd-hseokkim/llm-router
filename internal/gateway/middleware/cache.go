package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	exactcache "github.com/llm-router/gateway/internal/cache/exact"
	"github.com/llm-router/gateway/internal/gateway/types"
	"github.com/llm-router/gateway/internal/telemetry"
)

// SemanticLookup checks a semantic cache. Returns (response, similarity, nil) on hit;
// (nil, 0, nil) on miss.
type SemanticLookup func(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, float64, error)

// CacheMiddleware wraps handlers with exact-match and optional semantic caching.
type CacheMiddleware struct {
	cache               *exactcache.Cache
	temperatureZeroOnly bool
	group               singleflight.Group
	semantic            SemanticLookup
}

// NewCacheMiddleware creates a cache middleware.
func NewCacheMiddleware(cache *exactcache.Cache, temperatureZeroOnly bool) *CacheMiddleware {
	return &CacheMiddleware{
		cache:               cache,
		temperatureZeroOnly: temperatureZeroOnly,
	}
}

// WithSemantic attaches a semantic cache lookup to the middleware.
func (m *CacheMiddleware) WithSemantic(fn SemanticLookup) *CacheMiddleware {
	m.semantic = fn
	return m
}

// Handler returns the HTTP middleware function.
func (m *CacheMiddleware) Handler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			var req types.ChatCompletionRequest
			if err := json.Unmarshal(body, &req); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			headers := map[string]string{
				"Cache-Control":      r.Header.Get("Cache-Control"),
				"X-Gateway-No-Cache": r.Header.Get("X-Gateway-No-Cache"),
			}

			if !exactcache.IsCacheable(&req, m.temperatureZeroOnly, headers) {
				next.ServeHTTP(w, r)
				return
			}

			cacheKey := exactcache.BuildKey(&req)
			if cacheKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// --- Exact cache lookup ---
			if entry, cerr := m.cache.Get(r.Context(), cacheKey); cerr == nil && entry != nil {
				telemetry.SetCacheResult(r.Context(), "hit")
				telemetry.CacheRequestsTotal.WithLabelValues("exact", "hit").Inc()
				m.serveFromCache(w, entry, cacheKey, req.Stream)
				return
			}
			telemetry.CacheRequestsTotal.WithLabelValues("exact", "miss").Inc()

			// --- Semantic cache lookup ---
			if m.semantic != nil {
				if semResp, similarity, serr := m.semantic(r.Context(), &req); serr == nil && semResp != nil {
					telemetry.SetCacheResult(r.Context(), "semantic_hit")
					telemetry.CacheRequestsTotal.WithLabelValues("semantic", "hit").Inc()
					w.Header().Set("X-Cache", "SEMANTIC_HIT")
					w.Header().Set("X-Cache-Similarity", strconv.FormatFloat(similarity, 'f', 4, 64))
					w.Header().Set("X-Cache-Type", "semantic")
					if req.Stream {
						exactcache.ReplayAsStream(w, semResp)
					} else {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(semResp) //nolint:errcheck
					}
					return
				}
				telemetry.CacheRequestsTotal.WithLabelValues("semantic", "miss").Inc()
			}

			// --- Streaming: pass through without caching ---
			if req.Stream {
				next.ServeHTTP(w, r)
				return
			}

			// --- Non-streaming: intercept response via singleflight + recorder ---
			type cacheResult struct {
				body       []byte
				statusCode int
				headers    http.Header
			}

			raw, _, _ := m.group.Do(cacheKey, func() (interface{}, error) {
				rec := newCacheRecorder()
				next.ServeHTTP(rec, r)
				res := &cacheResult{body: rec.buf.Bytes(), statusCode: rec.code, headers: rec.hdr}

				// Async cache store on success
				if rec.code == http.StatusOK {
					var resp types.ChatCompletionResponse
					if jerr := json.Unmarshal(rec.buf.Bytes(), &resp); jerr == nil {
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							m.cache.Store(ctx, cacheKey, &resp, 0, 0) //nolint:errcheck
						}()
					}
				}
				return res, nil
			})

			if cr, ok := raw.(*cacheResult); ok {
				for k, vv := range cr.headers {
					for _, v := range vv {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(cr.statusCode)
				w.Write(cr.body) //nolint:errcheck
			}
		})
	}
}

func (m *CacheMiddleware) serveFromCache(w http.ResponseWriter, entry *exactcache.Entry, key string, stream bool) {
	age := exactcache.Age(entry)
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Key", key[:8])
	w.Header().Set("X-Cache-Age", strconv.FormatInt(age, 10))
	w.Header().Set("X-Cached-At", time.Unix(entry.CreatedAt, 0).UTC().Format(time.RFC3339))

	if stream {
		exactcache.ReplayAsStream(w, entry.Response)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(entry.Response) //nolint:errcheck
}

// cacheRecorder buffers a response for inspection and caching.
type cacheRecorder struct {
	hdr  http.Header
	buf  *bytes.Buffer
	code int
}

func newCacheRecorder() *cacheRecorder {
	return &cacheRecorder{hdr: make(http.Header), buf: &bytes.Buffer{}, code: http.StatusOK}
}

func (r *cacheRecorder) Header() http.Header         { return r.hdr }
func (r *cacheRecorder) WriteHeader(code int)        { r.code = code }
func (r *cacheRecorder) Write(b []byte) (int, error) { return r.buf.Write(b) }
