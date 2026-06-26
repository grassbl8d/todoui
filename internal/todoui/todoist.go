package todoui

// Task is one Todoist task in the display model.
type Task struct {
	ID        string
	Priority  string // p1 (highest) .. p4 (default)
	DueDate   string // sortable "YYYY-MM-DD[ HH:MM]" (may carry ↻ for recurring)
	Deadline  string // "YYYY-MM-DD" or ""
	Project   string // "#Name"
	Labels    string // "@a @b"
	Content   string
	Recurring bool
	Done      bool // completed (shown only in the read-only completed view)
}

// Project is a lightweight id+name pair used for the pickers.
type Project struct {
	ID   string
	Name string // "#Name"
}
