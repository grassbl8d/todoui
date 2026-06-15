package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Task is one Todoist task as parsed from the CLI's CSV output.
type Task struct {
	ID       string
	Priority string // p1 (highest) .. p4 (default)
	DueDate  string
	Project  string
	Labels   string
	Content  string
}

// Project / Label are lightweight name+id pairs used for the pickers.
type Project struct {
	ID   string
	Name string
}

type Label struct {
	ID   string
	Name string
}

const todoistBin = "todoist"

// run executes the todoist CLI and returns stdout, or an error that includes stderr.
func run(args ...string) (string, error) {
	cmd := exec.Command(todoistBin, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// parseCSV reads CSV with a header row and returns the records (excluding header).
func parseCSV(s string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.FieldsPerRecord = -1 // tolerate ragged rows
	var rows [][]string
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if first { // skip header
			first = false
			continue
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

// ListTasks returns tasks, optionally narrowed by a Todoist filter expression.
func ListTasks(filter string) ([]Task, error) {
	args := []string{"--csv", "--header", "list"}
	if strings.TrimSpace(filter) != "" {
		args = append(args, "--filter", filter)
	}
	out, err := run(args...)
	if err != nil {
		return nil, err
	}
	rows, err := parseCSV(out)
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(rows))
	for _, r := range rows {
		t := Task{}
		// Columns: ID,Priority,DueDate,Project,Labels,Content
		if len(r) > 0 {
			t.ID = r[0]
		}
		if len(r) > 1 {
			t.Priority = r[1]
		}
		if len(r) > 2 {
			t.DueDate = r[2]
		}
		if len(r) > 3 {
			t.Project = r[3]
		}
		if len(r) > 4 {
			t.Labels = r[4]
		}
		if len(r) > 5 {
			t.Content = r[5]
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// QuickAdd creates a task from a natural-language string
// (e.g. "Buy milk #Shopping @errand tomorrow p1").
func QuickAdd(text string) error {
	_, err := run("quick", text)
	return err
}

// ModifyProject moves a task to a different project by ID.
func ModifyProject(id, projectID string) error {
	_, err := run("modify", "--project-id", projectID, id)
	return err
}

// CloseTask completes a task by ID.
func CloseTask(id string) error {
	_, err := run("close", id)
	return err
}

// DeleteTask deletes a task by ID.
func DeleteTask(id string) error {
	_, err := run("delete", id)
	return err
}

// Sync refreshes the local cache from the server.
func Sync() error {
	_, err := run("sync")
	return err
}

// ListProjects returns all projects.
func ListProjects() ([]Project, error) {
	out, err := run("--csv", "--header", "projects")
	if err != nil {
		return nil, err
	}
	rows, err := parseCSV(out)
	if err != nil {
		return nil, err
	}
	ps := make([]Project, 0, len(rows))
	for _, r := range rows {
		if len(r) >= 2 {
			ps = append(ps, Project{ID: r[0], Name: r[1]})
		}
	}
	return ps, nil
}

// ListLabels returns all labels.
func ListLabels() ([]Label, error) {
	out, err := run("--csv", "--header", "labels")
	if err != nil {
		return nil, err
	}
	rows, err := parseCSV(out)
	if err != nil {
		return nil, err
	}
	ls := make([]Label, 0, len(rows))
	for _, r := range rows {
		if len(r) >= 2 {
			ls = append(ls, Label{ID: r[0], Name: r[1]})
		}
	}
	return ls, nil
}
