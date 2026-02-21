package cost

// EstimateTokens returns a rough token count estimate using the rule of thumb
// that 1 token ≈ 4 characters. This is used as a fallback when the Provider
// does not return usage metadata (e.g. non-streaming requests from some providers).
func EstimateTokens(text string) int {
	n := len([]rune(text)) / 4
	if n < 1 {
		return 1
	}
	return n
}
