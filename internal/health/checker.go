package health

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// CheckResult holds the outcome of a single dependency check.
type CheckResult struct {
	Status    string `json:"status"`              // "ok" | "unhealthy"
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// OverallStatus is the aggregate response for GET /health.
type OverallStatus struct {
	Status        string                 `json:"status"` // "ok" | "degraded" | "unhealthy"
	Version       string                 `json:"version"`
	UptimeSeconds int64                  `json:"uptime_seconds"`
	Timestamp     time.Time              `json:"timestamp"`
	Checks        map[string]CheckResult `json:"checks"`
}

// Checker runs parallel health checks with a short result cache.
type Checker struct {
	db        *pgxpool.Pool
	redis     *redis.Client
	startedAt time.Time
	version   string

	mu       sync.RWMutex
	cached   *OverallStatus
	cachedAt time.Time
	cacheTTL time.Duration
}

// NewChecker creates a Checker.
func NewChecker(db *pgxpool.Pool, rdb *redis.Client, version string) *Checker {
	return &Checker{
		db:        db,
		redis:     rdb,
		startedAt: time.Now(),
		version:   version,
		cacheTTL:  5 * time.Second,
	}
}

// Check returns a cached (≤5s) full health status.
func (c *Checker) Check(ctx context.Context) *OverallStatus {
	c.mu.RLock()
	if c.cached != nil && time.Since(c.cachedAt) < c.cacheTTL {
		result := *c.cached
		c.mu.RUnlock()
		return &result
	}
	c.mu.RUnlock()

	result := c.runChecks(ctx)

	c.mu.Lock()
	c.cached = result
	c.cachedAt = time.Now()
	c.mu.Unlock()

	return result
}

// ReadyChecks runs DB and Redis checks without caching (used by /health/ready).
func (c *Checker) ReadyChecks(ctx context.Context) map[string]CheckResult {
	return c.runDepsParallel(ctx)
}

func (c *Checker) runChecks(ctx context.Context) *OverallStatus {
	checks := c.runDepsParallel(ctx)

	overall := "ok"
	for _, cr := range checks {
		if cr.Status != "ok" {
			overall = "unhealthy"
			break
		}
	}

	return &OverallStatus{
		Status:        overall,
		Version:       c.version,
		UptimeSeconds: int64(time.Since(c.startedAt).Seconds()),
		Timestamp:     time.Now().UTC(),
		Checks:        checks,
	}
}

func (c *Checker) runDepsParallel(ctx context.Context) map[string]CheckResult {
	var mu sync.Mutex
	results := make(map[string]CheckResult, 2)

	var wg sync.WaitGroup
	add := func(name string, fn func() CheckResult) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := fn()
			mu.Lock()
			results[name] = r
			mu.Unlock()
		}()
	}

	add("database", func() CheckResult { return c.checkDB(ctx) })
	add("redis", func() CheckResult { return c.checkRedis(ctx) })
	wg.Wait()
	return results
}

func (c *Checker) checkDB(ctx context.Context) CheckResult {
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := c.db.Ping(tctx); err != nil {
		return CheckResult{Status: "unhealthy", LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()}
	}
	return CheckResult{Status: "ok", LatencyMs: time.Since(start).Milliseconds()}
}

func (c *Checker) checkRedis(ctx context.Context) CheckResult {
	tctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	start := time.Now()
	if err := c.redis.Ping(tctx).Err(); err != nil {
		return CheckResult{Status: "unhealthy", LatencyMs: time.Since(start).Milliseconds(), Error: err.Error()}
	}
	return CheckResult{Status: "ok", LatencyMs: time.Since(start).Milliseconds()}
}
