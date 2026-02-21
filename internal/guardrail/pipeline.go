package guardrail

import (
	"context"
	"log/slog"
)

// Pipeline runs a set of guardrails in sequence for input and output directions.
type Pipeline struct {
	input  []Guardrail
	output []Guardrail
	logger *slog.Logger
}

// NewPipeline creates a pipeline with the given input and output guardrails.
func NewPipeline(input, output []Guardrail, logger *slog.Logger) *Pipeline {
	return &Pipeline{input: input, output: output, logger: logger}
}

// CheckInput runs input guardrails on text.
// Returns (modified text, block error, processing error).
func (p *Pipeline) CheckInput(ctx context.Context, text string) (string, *BlockError, error) {
	return p.run(ctx, text, DirectionInput, p.input)
}

// CheckOutput runs output guardrails on text.
func (p *Pipeline) CheckOutput(ctx context.Context, text string) (string, *BlockError, error) {
	return p.run(ctx, text, DirectionOutput, p.output)
}

// HasOutput returns true if there are output guardrails configured.
func (p *Pipeline) HasOutput() bool {
	return len(p.output) > 0
}

func (p *Pipeline) run(ctx context.Context, text string, dir Direction, guards []Guardrail) (string, *BlockError, error) {
	current := text
	for _, g := range guards {
		result, err := g.Check(ctx, current, dir)
		if err != nil {
			p.logger.Warn("guardrail check error", "guardrail", g.Name(), "error", err)
			continue
		}
		if !result.Triggered {
			continue
		}

		p.logger.Info("guardrail triggered",
			"event", "guardrail_triggered",
			"guardrail", result.Guardrail,
			"action", result.Action,
			"direction", dir,
			"category", result.Category)

		switch result.Action {
		case ActionBlock:
			return current, &BlockError{
				Guardrail: result.Guardrail,
				Category:  result.Category,
				Message:   "Request blocked by content policy: " + result.Category + " detected.",
			}, nil
		case ActionMask:
			current = result.Modified
		case ActionLogOnly:
			// already logged above
		}
	}
	return current, nil, nil
}
