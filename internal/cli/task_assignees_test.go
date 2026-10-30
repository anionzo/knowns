package cli

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/howznguyen/knowns/internal/models"
	"github.com/howznguyen/knowns/internal/storage"
	"github.com/spf13/cobra"
)

func seedAssigneeTasks(t *testing.T) *storage.Store {
	t.Helper()
	projectRoot := t.TempDir()
	t.Chdir(projectRoot)
	store := storage.NewStore(filepath.Join(projectRoot, ".knowns"))
	if err := store.Init("cli"); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*models.Task{
		{ID: "a1", Title: "a1", Status: "todo", Priority: "medium", Assignee: "alice"},
		{ID: "a2", Title: "a2", Status: "done", Priority: "medium", Assignee: "alice"},
		{ID: "a3", Title: "a3", Status: "todo", Priority: "low", Assignee: " alice "},
		{ID: "b1", Title: "b1", Status: "in-progress", Priority: "high", Assignee: "bob"},
		{ID: "c1", Title: "c1", Status: "todo", Priority: "medium", Assignee: "carol"},
		{ID: "u1", Title: "u1", Status: "todo", Priority: "medium"},
		{ID: "u2", Title: "u2", Status: "todo", Priority: "high"},
		{ID: "u3", Title: "u3", Status: "in-progress", Priority: "medium"},
		{ID: "w1", Title: "w1", Status: "todo", Priority: "low", Assignee: "   "},
	} {
		if err := store.Tasks.Create(task); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func setFlags(t *testing.T, cmd *cobra.Command, values map[string]string) {
	t.Helper()
	for name, value := range values {
		flag := cmd.Flags().Lookup(name)
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		})
	}
}

func taskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	sort.Strings(ids)
	return ids
}

// Assignees compare trimmed, as the UI does: a whitespace-only assignee is
// unassigned, and --assignee alice also selects " alice ".
func TestTaskListUnassignedKeepsOnlyTasksWithoutAssignee(t *testing.T) {
	store := seedAssigneeTasks(t)

	setFlags(t, taskListCmd, map[string]string{"assignee": " alice"})
	tasks, err := listFilteredTasks(taskListCmd, store)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskIDs(tasks); !reflect.DeepEqual(got, []string{"a1", "a2", "a3"}) {
		t.Fatalf("--assignee alice = %v, want a1 a2 a3", got)
	}
	setFlags(t, taskListCmd, map[string]string{"assignee": ""})

	setFlags(t, taskListCmd, map[string]string{"unassigned": "true"})
	tasks, err = listFilteredTasks(taskListCmd, store)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskIDs(tasks); !reflect.DeepEqual(got, []string{"u1", "u2", "u3", "w1"}) {
		t.Fatalf("unassigned list = %v, want u1 u2 u3 w1", got)
	}

	setFlags(t, taskListCmd, map[string]string{"assignee": "alice"})
	if _, err := listFilteredTasks(taskListCmd, store); err == nil {
		t.Fatal("--unassigned with --assignee did not fail")
	}
}

func TestTaskAssigneesCountsPerAssigneeByStatusWithUnassignedLast(t *testing.T) {
	store := seedAssigneeTasks(t)

	tasks, err := listFilteredTasks(taskAssigneesCmd, store)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name       string
		unassigned bool
		total      int
		byStatus   map[string]int
	}
	var got []row
	for _, b := range bucketTasksByAssignee(tasks) {
		got = append(got, row{b.Assignee, b.Unassigned, b.Total, b.ByStatus})
	}
	// The unassigned bucket is the largest but still comes last; equal totals
	// order by name.
	want := []row{
		{"alice", false, 3, map[string]int{"todo": 2, "done": 1}},
		{"bob", false, 1, map[string]int{"in-progress": 1}},
		{"carol", false, 1, map[string]int{"todo": 1}},
		{"", true, 4, map[string]int{"todo": 3, "in-progress": 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buckets = %+v\nwant      %+v", got, want)
	}

	// The list filters narrow what is counted.
	setFlags(t, taskAssigneesCmd, map[string]string{"status": "in-progress"})
	tasks, err = listFilteredTasks(taskAssigneesCmd, store)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range bucketTasksByAssignee(tasks) {
		names = append(names, bucketLabel(b))
	}
	if !reflect.DeepEqual(names, []string{"bob", "-"}) {
		t.Fatalf("in-progress buckets = %v, want bob -", names)
	}
}

// Plain columns align by display width. Byte or rune counts put the count out
// of line for names whose runes are not one column each: "@tiến" typed with
// combining accents (as macOS often stores it) or a wide CJK name.
func TestTaskAssigneesPlainAlignsMultiByteNames(t *testing.T) {
	tasks := []*models.Task{
		{ID: "t1", Status: "todo", Assignee: "@tiến"},
		{ID: "t2", Status: "todo", Assignee: "@tiến"},
		{ID: "t3", Status: "done", Assignee: "@田中"},
		{ID: "t4", Status: "done", Assignee: "@alexandra"},
		{ID: "t5", Status: "todo"},
	}
	out := sprintAssigneesPlain(bucketTasksByAssignee(tasks), len(tasks))

	col := -1
	for _, line := range strings.Split(out, "\n") {
		open := strings.Index(line, "(")
		if open < 0 {
			continue
		}
		if w := lipgloss.Width(line[:open]); col < 0 {
			col = w
		} else if w != col {
			t.Fatalf("status column starts at %d, want %d:\n%s", w, col, out)
		}
	}
	if col < 0 {
		t.Fatalf("no bucket rows in output:\n%s", out)
	}
}
