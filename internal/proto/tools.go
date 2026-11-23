package proto

// The MCP tools a node offers its AI sessions (contracts/mcp-tools.md). These
// are not wire frames between nodes: they travel between a CLI and the node's
// MCP server, whose names follow MCP, not contracts/conventions.md. So
// testdata/tools.json is read with encoding/json, unknown fields refused, and
// not with Decode.

// ToolLists is testdata/tools.json: the tools/list result of each kind of
// session (contracts/mcp-tools.md section 5). The names and argument schemas
// are the contract; a tool's description is not part of it and is not here.
type ToolLists struct {
	Secretary ToolList `json:"secretary"`
	// Group is the group session of a working AI.
	Group  ToolList `json:"group"`
	Review ToolList `json:"review"`
	// TaskLevel1 and TaskLevel23 are a task session at level 1, and at level
	// 2 or 3.
	TaskLevel1  ToolList `json:"task_level_1"`
	TaskLevel23 ToolList `json:"task_level_2_3"`
	External    ToolList `json:"external"`
}

// ToolList is the result of tools/list, in the order of the tables of
// contracts/mcp-tools.md.
type ToolList struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Name        string    `json:"name"`
	InputSchema ToolInput `json:"inputSchema"`
}

// ToolInput is the JSON Schema of a tool's arguments: always an object, which
// takes no argument it does not name.
type ToolInput struct {
	Type                 string             `json:"type"`
	Properties           map[string]ToolArg `json:"properties"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties bool               `json:"additionalProperties"`
}

// ToolArg is the JSON Schema of one argument, or of an element or member of
// one: the keywords the tools use and no others.
type ToolArg struct {
	Type                 string             `json:"type"`
	Enum                 []string           `json:"enum,omitempty"`
	Default              any                `json:"default,omitempty"`
	Minimum              *int               `json:"minimum,omitempty"`
	Maximum              *int               `json:"maximum,omitempty"`
	Items                *ToolArg           `json:"items,omitempty"`
	Properties           map[string]ToolArg `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	AdditionalProperties *bool              `json:"additionalProperties,omitempty"`
}
