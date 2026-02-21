package guardrail

import "context"

// Direction indicates whether a guardrail check is on input (request) or output (response).
type Direction string

const (
	DirectionInput  Direction = "input"
	DirectionOutput Direction = "output"
)

// Action defines what to do when a guardrail triggers.
type Action string

const (
	ActionBlock   Action = "block"
	ActionMask    Action = "mask"
	ActionLogOnly Action = "log_only"
)

// Result is returned by a Guardrail check.
type Result struct {
	Triggered bool
	Action    Action
	Modified  string // modified text when action is mask
	Category  string // which sub-category triggered
	Guardrail string // guardrail name
}

// Guardrail is the interface each guardrail must implement.
type Guardrail interface {
	Name() string
	Check(ctx context.Context, text string, dir Direction) (*Result, error)
}

// BlockError is returned when a guardrail blocks a request.
type BlockError struct {
	Guardrail string
	Category  string
	Message   string
}

func (e *BlockError) Error() string { return e.Message }
