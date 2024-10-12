package harbor

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const (
	AdditionTypeBuildHistory = "build_history"
	AdditionTypeValuesYaml   = "values.yaml"
	AdditionTypeReadmeMd     = "readme.md"
	AdditionTypeDependencies = "dependencies"
	AdditionTypeSbom         = "sbom"
)

const MaxAdditionSize = 1 << 20 // 1MB

func (c *Client) GetAddition(ctx context.Context, project, repository, reference, addtion string) ([]byte, error) {
	resp, err := c.cli.Request(http.MethodGet, fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s/additions/%s", project, repository, reference, addtion)).
		Do(ctx)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(io.LimitReader(resp.Body, MaxAdditionSize))
	if err != nil {
		return buf, err
	}
	return buf, nil
}
