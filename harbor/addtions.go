package harbor

import (
	"context"
	"fmt"
	"io"

	"xiaoshiai.cn/common/httpclient"
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
	req := httpclient.Get(fmt.Sprintf("/projects/%s/repositories/%s/artifacts/%s/additions/%s", project, repository, reference, addtion))
	resp, err := c.cli.DoRaw(ctx, req)
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
