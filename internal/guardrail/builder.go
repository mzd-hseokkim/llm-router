package guardrail

import (
	"encoding/json"

	"github.com/llm-router/gateway/internal/config"
)

// ConfigToRecords converts a GuardrailConfig into seed PolicyRecord objects for DB upsert.
// Call this when the guardrail_policies table is empty to initialise it from config.
func ConfigToRecords(gc config.GuardrailConfig) []*PolicyRecord {
	recs := make([]*PolicyRecord, 0, 5)

	// pii
	piiCategories := gc.PII.Categories
	if piiCategories == nil {
		piiCategories = []string{}
	}
	piiJSON, _ := json.Marshal(map[string]any{"categories": piiCategories})
	recs = append(recs, &PolicyRecord{
		GuardrailType: "pii",
		IsEnabled:     gc.PII.Enabled,
		Action:        defaultStr(gc.PII.Action, "log_only"),
		ConfigJSON:    piiJSON,
		SortOrder:     0,
	})

	// prompt_injection
	injJSON, _ := json.Marshal(map[string]any{})
	recs = append(recs, &PolicyRecord{
		GuardrailType: "prompt_injection",
		IsEnabled:     gc.PromptInjection.Enabled,
		Action:        defaultStr(gc.PromptInjection.Action, "block"),
		Engine:        defaultStr(gc.PromptInjection.Engine, "regex"),
		ConfigJSON:    injJSON,
		SortOrder:     1,
	})

	// content_filter
	cfCategories := gc.ContentFilter.Categories
	if cfCategories == nil {
		cfCategories = []string{}
	}
	cfJSON, _ := json.Marshal(map[string]any{"categories": cfCategories})
	recs = append(recs, &PolicyRecord{
		GuardrailType: "content_filter",
		IsEnabled:     gc.ContentFilter.Enabled,
		Action:        defaultStr(gc.ContentFilter.Action, "log_only"),
		Engine:        defaultStr(gc.ContentFilter.Engine, "regex"),
		ConfigJSON:    cfJSON,
		SortOrder:     2,
	})

	// custom_keywords
	blocked := gc.CustomKeywords.Blocked
	if blocked == nil {
		blocked = []string{}
	}
	kwJSON, _ := json.Marshal(map[string]any{"blocked": blocked})
	recs = append(recs, &PolicyRecord{
		GuardrailType: "custom_keywords",
		IsEnabled:     gc.CustomKeywords.Enabled,
		Action:        defaultStr(gc.CustomKeywords.Action, "block"),
		ConfigJSON:    kwJSON,
		SortOrder:     3,
	})

	// llm_judge — not a standalone guardrail; stores model preference
	ljJSON, _ := json.Marshal(map[string]any{"model": gc.LLMJudge.Model})
	recs = append(recs, &PolicyRecord{
		GuardrailType: "llm_judge",
		IsEnabled:     false,
		Action:        "log_only",
		ConfigJSON:    ljJSON,
		SortOrder:     4,
	})

	return recs
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
