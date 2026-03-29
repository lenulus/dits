package domain

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
