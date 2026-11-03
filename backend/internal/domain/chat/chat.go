package chat

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Text       string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}
