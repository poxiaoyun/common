package harbor

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

type Tag struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	RepositoryID int       `json:"repository_id"`
	ArtifactID   int       `json:"artifact_id"`
	PullTime     time.Time `json:"pull_time"`
	PushTime     time.Time `json:"push_time"`
	Immutable    bool      `json:"immutable"`
}

type ListTagOptions struct {
	CommonOptions
	WithSignature       bool
	WithImmutableStatus bool
}

func (o ListTagOptions) ToQuery() url.Values {
	q := o.CommonOptions.ToQuery()
	if o.WithSignature {
		q.Set("with_signature", "true")
	}
	if o.WithImmutableStatus {
		q.Set("with_immutable_status", "true")
	}
	return q
}

func (c *Client) ListTags(ctx context.Context, project, repository string, reference string, o ListTagOptions) ([]Tag, error) {
	var tags []Tag
	if err := c.cli.
		Get(fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s/tags", project, repository, reference)).
		Queries(o.ToQuery()).
		Return(&tags).
		Send(ctx); err != nil {
		return nil, err
	}
	return tags, nil
}

func (c *Client) DeleteTag(ctx context.Context, project, repository string, reference string, tag string) error {
	return c.cli.Delete(fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s/tags/%s", project, repository, reference, tag)).Send(ctx)
}
