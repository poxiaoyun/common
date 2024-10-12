package harbor

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

type ListArtifactOptions struct {
	CommonOptions
	GetArtifactOptions
	LatestInRepositiory bool
}

type GetArtifactOptions struct {
	WithTag             bool
	WithLabel           bool
	WithScanOverview    bool
	WithSignature       bool
	WithSBOMOverview    bool
	WithImmutableStatus bool
	WithAccessory       bool
}

func (o ListArtifactOptions) ToQuery() url.Values {
	q := o.CommonOptions.ToQuery()
	if o.WithTag {
		q.Set("with_tag", "true")
	}
	if o.WithLabel {
		q.Set("with_label", "true")
	}
	if o.WithScanOverview {
		q.Set("with_scan_overview", "true")
	}
	if o.WithSignature {
		q.Set("with_signature", "true")
	}
	if o.WithSBOMOverview {
		q.Set("with_sbom_overview", "true")
	}
	if o.WithImmutableStatus {
		q.Set("with_immutable_status", "true")
	}
	if o.WithAccessory {
		q.Set("with_accessory", "true")
	}
	if o.LatestInRepositiory {
		q.Set("latest_in_repository", "true")
	}
	return q
}

type Artifacrt struct {
	ID                int                    `json:"id"`
	Type              string                 `json:"type"`
	MediaType         string                 `json:"media_type"`
	ManifestMediaType string                 `json:"manifest_media_type"`
	ArtifactType      string                 `json:"artifact_type"`
	ProjectID         int                    `json:"project_id"`
	RepositoryID      int                    `json:"repository_id"`
	RepositoryName    string                 `json:"repository_name"`
	Digest            string                 `json:"digest"`
	Size              int64                  `json:"size"`
	Icon              string                 `json:"icon"`
	PushTime          time.Time              `json:"push_time"`
	PullTime          time.Time              `json:"pull_time"`
	ExtraAttrs        map[string]interface{} `json:"extra_attrs"`
	Annotations       map[string]string      `json:"annotations"`
	Tags              []Tag                  `json:"tags"`
	Label             map[string]string      `json:"label"`
	References        []Reference            `json:"references"`
}

type Reference struct {
	Annotations map[string]string `json:"annotations"`
	Platform    ReferencePlatform `json:"platform"`
	ChildDigest string            `json:"child_digest"`
	ChildID     int               `json:"child_id"`
	ParentID    int               `json:"parent_id"`
	URLs        []string          `json:"urls"`
}

type ReferencePlatform struct {
	OSFeatures   []string `json:"OsFeatures"`
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	Variant      string   `json:"variant"`
}

func (c *Client) ListArtifacts(ctx context.Context, project string, repository string, options ListArtifactOptions) ([]Artifacrt, int, error) {
	var artifacts []Artifacrt
	resp, err := c.cli.
		Get("/projects/" + project + "/repositories/" + repository + "/artifacts").
		Queries(options.ToQuery()).
		Return(&artifacts).
		Do(ctx)
	if err != nil {
		return nil, 0, err
	}
	total, _ := strconv.Atoi(resp.Header.Get("X-Total-Count"))
	return artifacts, total, nil
}

func (c *Client) GetArtifact(ctx context.Context, project string, repository string, reference string, options GetArtifactOptions) (*Artifacrt, error) {
	var artifact Artifacrt
	if err := c.cli.
		Get("/projects/" + project + "/repositories/" + repository + "/artifacts/" + reference).
		Return(&artifact).
		Send(ctx); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (c *Client) DeleteArtifact(ctx context.Context, project string, repository string, reference string) error {
	return c.cli.Delete("/projects/" + project + "/repositories/" + repository + "/artifacts/" + reference).Send(ctx)
}
