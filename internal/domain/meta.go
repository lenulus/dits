package domain

import "fmt"

type MetaConfig struct {
	Version    MetaVersion `json:"version"`
	ProjectKey string      `json:"project_key"`
	Labels     []Label     `json:"labels"`
	Workflows  []Workflow  `json:"workflows"`
	IssueTypes []IssueType `json:"issue_types"`
	Priorities []string    `json:"priorities"`
}

type Label struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

type Workflow struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Statuses    []WorkflowStatus `json:"statuses"`
	Transitions []Transition     `json:"transitions"`
}

type WorkflowStatus struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Category string `json:"category"` // "open", "in_progress", "done"
}

type Transition struct {
	From string `json:"from"` // "*" means any
	To   string `json:"to"`
}

type IssueType struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	WorkflowSlug string `json:"workflow_slug"`
}

// --- Validation ---

func (m *MetaConfig) HasLabel(slug string) bool {
	for _, l := range m.Labels {
		if l.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasStatus(status string) bool {
	for _, w := range m.Workflows {
		for _, s := range w.Statuses {
			if s.Slug == status {
				return true
			}
		}
	}
	return false
}

func (m *MetaConfig) HasIssueType(slug string) bool {
	for _, t := range m.IssueTypes {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasPriority(priority string) bool {
	for _, p := range m.Priorities {
		if p == priority {
			return true
		}
	}
	return false
}

func (m *MetaConfig) GetWorkflowForType(typeSlug string) *Workflow {
	wfSlug := ""
	for _, t := range m.IssueTypes {
		if t.Slug == typeSlug {
			wfSlug = t.WorkflowSlug
			break
		}
	}
	if wfSlug == "" {
		return nil
	}
	for i := range m.Workflows {
		if m.Workflows[i].Slug == wfSlug {
			return &m.Workflows[i]
		}
	}
	return nil
}

// --- Mutation ---

func (m *MetaConfig) AddLabel(label Label) error {
	if m.HasLabel(label.Slug) {
		return fmt.Errorf("label %q already exists", label.Slug)
	}
	m.Labels = append(m.Labels, label)
	m.Version++
	return nil
}

func (m *MetaConfig) RemoveLabel(slug string) error {
	found := false
	result := make([]Label, 0, len(m.Labels))
	for _, l := range m.Labels {
		if l.Slug == slug {
			found = true
			continue
		}
		result = append(result, l)
	}
	if !found {
		return fmt.Errorf("label %q not found", slug)
	}
	m.Labels = result
	m.Version++
	return nil
}

func (m *MetaConfig) AddIssueType(it IssueType) error {
	if m.HasIssueType(it.Slug) {
		return fmt.Errorf("issue type %q already exists", it.Slug)
	}
	// Verify workflow exists.
	found := false
	for _, w := range m.Workflows {
		if w.Slug == it.WorkflowSlug {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("workflow %q not found", it.WorkflowSlug)
	}
	m.IssueTypes = append(m.IssueTypes, it)
	m.Version++
	return nil
}

func DefaultMetaConfig(projectKey string) MetaConfig {
	return MetaConfig{
		Version:    1,
		ProjectKey: projectKey,
		Labels:     []Label{},
		Workflows: []Workflow{
			{
				Slug: "default",
				Name: "Default",
				Statuses: []WorkflowStatus{
					{Slug: "open", Name: "Open", Category: "open"},
					{Slug: "in_progress", Name: "In Progress", Category: "in_progress"},
					{Slug: "closed", Name: "Closed", Category: "done"},
				},
				Transitions: []Transition{
					{From: "*", To: "open"},
					{From: "*", To: "in_progress"},
					{From: "*", To: "closed"},
				},
			},
		},
		IssueTypes: []IssueType{
			{Slug: "task", Name: "Task", WorkflowSlug: "default"},
			{Slug: "bug", Name: "Bug", WorkflowSlug: "default"},
		},
		Priorities: []string{"low", "medium", "high", "critical"},
	}
}
