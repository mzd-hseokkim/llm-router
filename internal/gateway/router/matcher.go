package router

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"github.com/llm-router/gateway/internal/auth"
	"github.com/llm-router/gateway/internal/gateway/types"
)

// compiledRule wraps a RouteRule with a pre-compiled regex for fast matching.
type compiledRule struct {
	types.RouteRule
	compiledRegex *regexp.Regexp // non-nil only when Match.ModelRegex is set
}

// regexCache caches compiled regular expressions to avoid re-compilation on
// every request.
type regexCache struct {
	mu    sync.RWMutex
	cache map[string]*regexp.Regexp
}

var globalRegexCache = &regexCache{cache: make(map[string]*regexp.Regexp)}

func (rc *regexCache) compile(pattern string) (*regexp.Regexp, error) {
	rc.mu.RLock()
	if r, ok := rc.cache[pattern]; ok {
		rc.mu.RUnlock()
		return r, nil
	}
	rc.mu.RUnlock()

	r, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	rc.mu.Lock()
	rc.cache[pattern] = r
	rc.mu.Unlock()
	return r, nil
}

// compileRule pre-compiles the regex pattern (if any) and returns a compiledRule.
func compileRule(rule types.RouteRule) (compiledRule, error) {
	cr := compiledRule{RouteRule: rule}
	if rule.Match.ModelRegex != "" {
		r, err := globalRegexCache.compile(rule.Match.ModelRegex)
		if err != nil {
			return compiledRule{}, err
		}
		cr.compiledRegex = r
	}
	return cr, nil
}

// Matches reports whether the given request satisfies the rule's conditions.
// ctx must contain the authenticated VirtualKey (via auth.SetVirtualKey).
func (cr *compiledRule) Matches(ctx context.Context, req *types.ChatCompletionRequest, estimatedTokens int) bool {
	m := cr.Match

	// --- Model matching ---
	if m.Model != "" && req.Model != m.Model {
		return false
	}
	if m.ModelPrefix != "" && !strings.HasPrefix(req.Model, m.ModelPrefix) {
		return false
	}
	if cr.compiledRegex != nil && !cr.compiledRegex.MatchString(req.Model) {
		return false
	}

	// --- Auth context matching ---
	vk := auth.GetVirtualKey(ctx)
	if vk != nil {
		if m.KeyID != nil && vk.ID != *m.KeyID {
			return false
		}
		if m.UserID != nil && (vk.UserID == nil || *vk.UserID != *m.UserID) {
			return false
		}
		if m.TeamID != nil && (vk.TeamID == nil || *vk.TeamID != *m.TeamID) {
			return false
		}
		if m.OrgID != nil && (vk.OrgID == nil || *vk.OrgID != *m.OrgID) {
			return false
		}
	} else {
		// No virtual key — fail if any auth field is required.
		if m.KeyID != nil || m.UserID != nil || m.TeamID != nil || m.OrgID != nil {
			return false
		}
	}

	// --- Metadata matching (all key-value pairs must match) ---
	if len(m.Metadata) > 0 {
		if len(req.Metadata) == 0 {
			return false
		}
		for k, v := range m.Metadata {
			if req.Metadata[k] != v {
				return false
			}
		}
	}

	// --- Context token count ---
	if m.MinContextTokens > 0 && estimatedTokens < m.MinContextTokens {
		return false
	}
	if m.MaxContextTokens > 0 && estimatedTokens > m.MaxContextTokens {
		return false
	}

	// --- Tool use ---
	if m.HasTools && len(req.Tools) == 0 {
		return false
	}

	return true
}

// estimateTokens gives a rough token count for a message list (~4 chars/token).
func estimateTokens(messages []types.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
	}
	return total
}
