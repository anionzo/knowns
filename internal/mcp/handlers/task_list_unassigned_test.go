package handlers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/howznguyen/knowns/internal/models"
	"github.com/howznguyen/knowns/internal/storage"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCP task list must select unassigned Tasks the way `knowns task list
// --unassigned` does, trimming assignees like the CLI and UI, and refuse the
// contradictory unassigned+assignee pair.
func TestHandleTaskListUnassigned(t *testing.T) {
	store := storage.NewStore(filepath.Join(t.TempDir(), ".knowns"))
	if err := store.Init("task-list-unassigned"); err != nil {
		t.Fatalf("init store: %v", err)
	}
	for _, task := range []*models.Task{
		{ID: "owned", Title: "owned", Status: "todo", Priority: "medium", Assignee: "alice"},
		{ID: "padded", Title: "padded", Status: "todo", Priority: "medium", Assignee: " alice "},
		{ID: "orphan", Title: "orphan", Status: "todo", Priority: "medium"},
		{ID: "blank", Title: "blank", Status: "todo", Priority: "medium", Assignee: "   "},
	} {
		if err := store.Tasks.Create(task); err != nil {
			t.Fatalf("create %s: %v", task.ID, err)
		}
	}

	getStore := func() *storage.Store { return store }
	call := func(args map[string]any) *mcp.CallToolResult {
		t.Helper()
		result, err := handleTaskList(getStore, mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}})
		if err != nil {
			t.Fatalf("handleTaskList: %v", err)
		}
		return result
	}

	text := call(map[string]any{"unassigned": true}).Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"orphan"`) || !strings.Contains(text, `"blank"`) ||
		strings.Contains(text, `"owned"`) || strings.Contains(text, `"padded"`) {
		t.Errorf("unassigned list = %s, want only orphan and blank", text)
	}

	text = call(map[string]any{"assignee": "alice"}).Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"owned"`) || !strings.Contains(text, `"padded"`) || strings.Contains(text, `"orphan"`) {
		t.Errorf("assignee alice list = %s, want owned and padded", text)
	}

	if result := call(map[string]any{"unassigned": true, "assignee": "alice"}); !result.IsError {
		t.Errorf("unassigned with assignee did not fail: %+v", result)
	}
}
