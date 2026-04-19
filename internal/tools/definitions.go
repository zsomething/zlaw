package tools

// Definition describes a built-in tool.
type Definition struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parameters  []Param `json:"parameters"`
}

// Param describes a single parameter for a tool.
type Param struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Tools returns the list of all built-in tool definitions.
func Tools() []Definition {
	return []Definition{
		// File operations (local)
		{Name: "read", Description: "Read the contents of a file.", Parameters: []Param{
			{Name: "path", Type: "string", Description: "Path to file to read", Required: true},
			{Name: "offset", Type: "number", Description: "Line offset to start reading", Required: false},
			{Name: "limit", Type: "number", Description: "Max lines to read", Required: false},
		}},
		{Name: "write", Description: "Write content to a file. Creates parent directories if needed.", Parameters: []Param{
			{Name: "path", Type: "string", Description: "Path to file to write", Required: true},
			{Name: "content", Type: "string", Description: "Content to write", Required: true},
		}},
		{Name: "edit", Description: "Replace exact string in a file. Fails if string not found or ambiguous.", Parameters: []Param{
			{Name: "path", Type: "string", Description: "Path to file", Required: true},
			{Name: "old_string", Type: "string", Description: "String to replace", Required: true},
			{Name: "new_string", Type: "string", Description: "Replacement string", Required: true},
		}},
		{Name: "glob", Description: "Find files matching a glob pattern.", Parameters: []Param{
			{Name: "pattern", Type: "string", Description: "Glob pattern (e.g., **/*.go)", Required: true},
		}},
		{Name: "grep", Description: "Search for text within files using regex.", Parameters: []Param{
			{Name: "pattern", Type: "string", Description: "Regex pattern", Required: true},
			{Name: "path", Type: "string", Description: "Directory to search", Required: false},
			{Name: "file_pattern", Type: "string", Description: "File glob pattern", Required: false},
		}},

		// System (local)
		{Name: "bash", Description: "Execute a shell command.", Parameters: []Param{
			{Name: "command", Type: "string", Description: "Shell command to execute", Required: true},
			{Name: "cwd", Type: "string", Description: "Working directory", Required: false},
			{Name: "timeout", Type: "number", Description: "Timeout in seconds", Required: false},
		}},

		// Web (local)
		{Name: "web_fetch", Description: "Fetch content from a URL.", Parameters: []Param{
			{Name: "url", Type: "string", Description: "URL to fetch", Required: true},
			{Name: "prompt", Type: "string", Description: "Extract specific info from page", Required: false},
		}},
		{Name: "web_search", Description: "Search the web.", Parameters: []Param{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "top_n", Type: "number", Description: "Number of results", Required: false},
		}},
		{Name: "http_request", Description: "Make an HTTP request.", Parameters: []Param{
			{Name: "method", Type: "string", Description: "HTTP method", Required: true},
			{Name: "url", Type: "string", Description: "Request URL", Required: true},
			{Name: "headers", Type: "object", Description: "Request headers", Required: false},
			{Name: "body", Type: "string", Description: "Request body", Required: false},
		}},

		// Memory (local)
		{Name: "memory_save", Description: "Store information in persistent memory.", Parameters: []Param{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
			{Name: "value", Type: "string", Description: "Value to store", Required: true},
		}},
		{Name: "memory_recall", Description: "Retrieve information from persistent memory.", Parameters: []Param{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
		}},
		{Name: "memory_delete", Description: "Delete information from persistent memory.", Parameters: []Param{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
		}},

		// Cron (local)
		{Name: "cronjob_list", Description: "List all scheduled cron jobs.", Parameters: []Param{}},
		{Name: "cronjob_create", Description: "Create a new scheduled cron job.", Parameters: []Param{
			{Name: "id", Type: "string", Description: "Job ID", Required: true},
			{Name: "schedule", Type: "string", Description: "Cron expression", Required: true},
			{Name: "task", Type: "string", Description: "Task prompt", Required: true},
			{Name: "target", Type: "string", Description: "Push target", Required: false},
		}},
		{Name: "cronjob_delete", Description: "Delete a cron job by ID.", Parameters: []Param{
			{Name: "id", Type: "string", Description: "Job ID to delete", Required: true},
		}},

		// Skills (local)
		{Name: "skill_load", Description: "Load a skill plugin for this session.", Parameters: []Param{
			{Name: "name", Type: "string", Description: "Skill name", Required: true},
		}},

		// Utilities (local)
		{Name: "time", Description: "Get current date and time in UTC.", Parameters: []Param{}},
		{Name: "configure", Description: "Update a runtime agent setting.", Parameters: []Param{
			{Name: "field", Type: "string", Description: "Setting name", Required: true},
			{Name: "value", Type: "string", Description: "New value", Required: true},
		}},

		// Agent operations (require hub connection)
		{Name: "agent_delegate", Description: "Delegate a task to another agent in the hub.", Parameters: []Param{
			{Name: "id", Type: "string", Description: "Target agent ID", Required: true},
			{Name: "task", Type: "string", Description: "Task description", Required: true},
			{Name: "context", Type: "object", Description: "Optional context", Required: false},
		}},
		{Name: "agent_list", Description: "List all agents registered in the hub.", Parameters: []Param{}},
		{Name: "agent_get", Description: "Get details for a specific agent.", Parameters: []Param{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_status", Description: "Get status of a named agent.", Parameters: []Param{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_stop", Description: "Stop a running agent. Cannot stop self.", Parameters: []Param{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_restart", Description: "Restart an agent. Cannot restart self.", Parameters: []Param{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "hub_status", Description: "Get hub information (name, JetStream status).", Parameters: []Param{}},
	}
}
