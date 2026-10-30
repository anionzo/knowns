package cli

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/howznguyen/knowns/internal/models"
	"github.com/spf13/cobra"
)

// --- task assignees ---

var taskAssigneesCmd = &cobra.Command{
	Use: "assignees",
	Example: `  # Who holds how much work, by status
  knowns task assignees

  # Only what is in flight
  knowns task assignees --status in-progress

  # Parseable output for an agent
  knowns task assignees --json`,
	Short: "Count tasks per assignee",
	Long: `Count tasks per assignee, broken down by status. Tasks with no assignee
are counted in a final unassigned bucket. Takes the same filters as "task list".

Assignees match exactly and case-sensitively after trimming spaces (@Alice is
not @alice), "@me" is not resolved and is matched literally, and a task holds
one assignee string, so "@a, @b" is a single bucket.`,
	Args: cobra.NoArgs,
	RunE: runTaskAssignees,
}

// assigneeBucket is one assignee's share of the filtered tasks. The unassigned
// bucket has an empty Assignee and Unassigned set, so a consumer never has to
// guess whether "" is a name.
type assigneeBucket struct {
	Assignee   string         `json:"assignee"`
	Unassigned bool           `json:"unassigned,omitempty"`
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"byStatus"`
	tasks      []*models.Task
}

func runTaskAssignees(cmd *cobra.Command, args []string) error {
	tasks, err := listFilteredTasks(cmd, getStore())
	if err != nil {
		return err
	}
	buckets := bucketTasksByAssignee(tasks)

	if isJSON(cmd) {
		printJSON(buckets)
		return nil
	}
	if len(buckets) == 0 {
		fmt.Println(StyleDim.Render("No tasks found."))
		return nil
	}
	if isPlain(cmd) {
		fmt.Print(sprintAssigneesPlain(buckets, len(tasks)))
	} else {
		fmt.Print(renderAssigneesTable(buckets, len(tasks)))
	}
	return nil
}

// bucketTasksByAssignee groups tasks by assignee, largest bucket first, ties by
// name, and the unassigned bucket last whatever its size.
func bucketTasksByAssignee(tasks []*models.Task) []*assigneeBucket {
	byName := map[string]*assigneeBucket{}
	for _, t := range tasks {
		// Trimmed as in listFilteredTasks: whitespace-only joins the unassigned bucket.
		name := strings.TrimSpace(t.Assignee)
		b, ok := byName[name]
		if !ok {
			b = &assigneeBucket{Assignee: name, Unassigned: name == "", ByStatus: map[string]int{}}
			byName[name] = b
		}
		b.Total++
		b.ByStatus[t.Status]++
		b.tasks = append(b.tasks, t)
	}

	buckets := make([]*assigneeBucket, 0, len(byName))
	for _, b := range byName {
		buckets = append(buckets, b)
	}
	sort.Slice(buckets, func(i, j int) bool {
		left, right := buckets[i], buckets[j]
		if left.Unassigned != right.Unassigned {
			return right.Unassigned
		}
		if left.Total != right.Total {
			return left.Total > right.Total
		}
		return left.Assignee < right.Assignee
	})
	return buckets
}

// bucketLabel prints the unassigned bucket as "-", as the list does.
func bucketLabel(b *assigneeBucket) string {
	if b.Unassigned {
		return "-"
	}
	return b.Assignee
}

func sprintAssigneesPlain(buckets []*assigneeBucket, total int) string {
	var b strings.Builder
	nameW, totalW := 0, 0
	for _, bk := range buckets {
		nameW = max(nameW, lipgloss.Width(bucketLabel(bk)))
		totalW = max(totalW, len(fmt.Sprint(bk.Total)))
	}
	for _, bk := range buckets {
		fmt.Fprintf(&b, "%s  %*d  (%s)\n", padRight(bucketLabel(bk), nameW), totalW, bk.Total, plainStatusSummary(bk.tasks))
	}
	fmt.Fprintf(&b, "\nTotal: %d %s\n", total, pluralTasks(total))
	return b.String()
}

func renderAssigneesTable(buckets []*assigneeBucket, total int) string {
	const (
		indent = 2
		gap    = 2
	)
	nameW, totalW, statusW := len("ASSIGNEE"), len("TASKS"), len("STATUS")
	for _, bk := range buckets {
		nameW = max(nameW, lipgloss.Width(bucketLabel(bk)))
		statusW = max(statusW, lipgloss.Width(plainStatusSummary(bk.tasks)))
	}

	var b strings.Builder
	sep := strings.Repeat(" ", gap)
	pad := strings.Repeat(" ", indent)
	fmt.Fprintln(&b, pad+strings.Join([]string{
		StyleBold.Render(padRight("ASSIGNEE", nameW)),
		StyleBold.Render(padRight("TASKS", totalW)),
		StyleBold.Render("STATUS"),
	}, sep))
	fmt.Fprintln(&b, pad+RenderSeparator(nameW+totalW+statusW+gap*2))

	for _, bk := range buckets {
		name := padRight(bucketLabel(bk), nameW)
		if bk.Unassigned {
			name = StyleDim.Render(name)
		}
		fmt.Fprintln(&b, pad+strings.Join([]string{
			name,
			padRight(fmt.Sprint(bk.Total), totalW),
			styledStatusSummary(bk.tasks),
		}, sep))
	}
	fmt.Fprintf(&b, "\n%s%s\n", pad, StyleDim.Render(fmt.Sprintf("Total: %d %s", total, pluralTasks(total))))
	return b.String()
}

// styledStatusSummary colours each "N status" part of plainStatusSummary, so the
// styled and plain views keep one status order.
func styledStatusSummary(tasks []*models.Task) string {
	parts := strings.Split(plainStatusSummary(tasks), ", ")
	for i, part := range parts {
		if _, status, ok := strings.Cut(part, " "); ok {
			parts[i] = StatusStyle(status).Render(part)
		}
	}
	return strings.Join(parts, ", ")
}

func init() {
	addTaskListFilterFlags(taskAssigneesCmd)
}
