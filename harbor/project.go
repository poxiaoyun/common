package harbor

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"xiaoshiai.cn/common/httpclient"
)

type Project struct {
	Name      string `json:"name"`
	ProjectID int    `json:"project_id"`
	OwnerID   int    `json:"owner_id"`
	OwnerName string `json:"owner_name"`
	Deleted   bool   `json:"deleted"`
	RepoCount int    `json:"repo_count"`
	Public    bool   `json:"public"`
}

type ListProjectOptions struct {
	CommonOptions
	Name       string
	Public     *bool
	Owner      string
	WithDetail bool
}

func (o ListProjectOptions) ToQuery() url.Values {
	q := o.CommonOptions.ToQuery()
	if o.Name != "" {
		q.Set("name", o.Name)
	}
	if o.Owner != "" {
		q.Set("owner", o.Owner)
	}
	if o.Public != nil {
		q.Set("public", strconv.FormatBool(*o.Public))
	}
	if o.WithDetail {
		q.Set("with_detail", "true")
	}
	return q
}

func (c *Client) ListProjects(ctx context.Context, options ListProjectOptions) ([]Project, error) {
	var projects []Project
	if err := c.cli.Get(ctx, "/projects", options.ToQuery(), &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) GetProject(ctx context.Context, nameOrID string) (*Project, error) {
	var project Project
	if err := c.cli.Get(ctx, "/projects/"+string(nameOrID), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) HeadProject(ctx context.Context, name string) (bool, error) {
	resp, err := c.cli.DoRaw(
		ctx, httpclient.NewRequest(http.MethodHead, "/projects").Query("project_name", name),
	)
	if err != nil {
		return false, err
	}
	return resp.StatusCode == http.StatusOK, nil
}

type ApplyProject struct {
	Name         string `json:"project_name"`
	Public       bool   `json:"public"`
	StorageLimit int64  `json:"storage_limit"`
}

func (c *Client) CreateProject(ctx context.Context, project ApplyProject) error {
	return c.cli.Post(ctx, "/projects", project)
}

func (c *Client) UpdateProject(ctx context.Context, project ApplyProject) error {
	return c.cli.Put(ctx, "/projects/"+string(project.Name), nil, project)
}

func (c *Client) DeleteProject(ctx context.Context, name string) error {
	return c.cli.Delete(ctx, "/projects/"+string(name))
}
