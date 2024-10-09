package harbor

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"xiaoshiai.cn/common/httpclient"
)

type ListRepositoryOptions struct {
	CommonOptions
}

func (o ListRepositoryOptions) ToQuery() url.Values {
	q := o.CommonOptions.ToQuery()
	return q
}

type Repository struct {
	ID             int       `json:"id"`
	ProjectID      int       `json:"project_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	ArtifacrtCount int       `json:"artifact_count"`
	PullCount      int       `json:"pull_count"`
	CreationTime   time.Time `json:"creation_time"`
	UpdateTime     time.Time `json:"update_time"`
}

func (c *Client) ListRepositories(ctx context.Context, project string,
	options ListRepositoryOptions,
) ([]Repository, int, error) {
	var repositories []Repository
	req := httpclient.Get(fmt.Sprintf("/projects/%s/repositories", project)).Queries(options.ToQuery()).Return(&repositories)
	resp, err := c.cli.Do(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	total, _ := strconv.Atoi(resp.Header.Get("X-Total-Count"))
	return repositories, total, nil
}

func (c *Client) GetRepository(ctx context.Context, project, repository string) (*Repository, error) {
	var repo Repository
	err := c.cli.Get(ctx, fmt.Sprintf("/projects/%s/repositories/%s", project, repository), nil, &repo)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}
