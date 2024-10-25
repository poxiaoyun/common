package harbor

import (
	"context"
	"fmt"
	"net/url"

	"xiaoshiai.cn/common/errors"
)

type ListRobotOptions struct {
	CommonOptions
}

func (o ListRobotOptions) ToQuery() url.Values {
	return o.CommonOptions.ToQuery()
}

type Robot struct {
	ID           int               `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Secret       string            `json:"secret"`
	Level        string            `json:"level"`
	Duration     int               `json:"duration"`
	Editable     bool              `json:"editable"`
	Disable      bool              `json:"disable"`
	ExpiresAt    int               `json:"expires_at"`
	Permissions  []RobotPermission `json:"permissions"`
	CreationTime string            `json:"creation_time"`
	UpdateTime   string            `json:"update_time"`
}

type RobotPermission struct {
	Kind      string                  `json:"kind"`
	Namespace string                  `json:"namespace"`
	Access    []RobotPermissionAccess `json:"access"`
}

const (
	PermissionResourceArtifact           = "artifact"
	PermissionResourceArtifactLabel      = "artifact-label"
	PermissionResourceImmutableTag       = "immutable-tag"
	PermissionResourceLabel              = "label"
	PermissionResourceMetadata           = "metadata"
	PermissionResourceNotificationPolicy = "notification-policy"
	PermissionResourcePreheatPolicy      = "preheat-policy"
	PermissionResourceSBOM               = "sbom"
	PermissionResourceScan               = "scan"
	PermissionResourceScanner            = "scanner"
	PermissionResourceTag                = "tag"
	PermissionResourceTagRetention       = "tag-retention"
	PermissionResourceAccessory          = "accessory"
	PermissionResourceLog                = "log"
	PermissionResourceProject            = "project"
	PermissionResourceRepository         = "repository"
	PermissionResourceArtifactAddition   = "artifact-addition"
	PermissionResourceQuota              = "quota"
)

const (
	PermissionLevelProject = "project"
)

const (
	PermissionKindProject = "project"
)

const (
	PermissionActionCreate = "create"
	PermissionActionDelete = "delete"
	PermissionActionList   = "list"
	PermissionActionPull   = "pull"
	PermissionActionPush   = "push"
	PermissionActionRead   = "read"
	PermissionActionStop   = "stop"
	PermissionActionUpdate = "update"
)

const (
	PermissionEffectAllow = "allow"
	PermissionEffectDeny  = "deny"
)

type RobotPermissionAccess struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Effect   string `json:"effect"`
}

type CreateRobot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// ExpiresAt is unix timestamp in seconds.
	ExpiresAt int `json:"expires_at"`
	// Duration is the days of the robot account's expiration.
	Duration int  `json:"duration,omitempty"`
	Disable  bool `json:"disable"`
	// Level is the robot account's permission level.
	// The value can be "project" or "system".
	Level string `json:"level"`

	Permissions []RobotPermission `json:"permissions"`

	Access []RobotPermissionAccess `json:"access"`
}

func (c *Client) CreateRobotAccount(ctx context.Context, robot CreateRobot) (*Robot, error) {
	var r Robot
	if err := c.cli.Post("robots").JSON(robot).Return(&r).Send(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) UpdateRobotAccount(ctx context.Context, robot *Robot) error {
	return c.cli.Put(fmt.Sprintf("/robots/%d", robot.ID)).JSON(robot).Send(ctx)
}

func (c *Client) ListRobotAccounts(ctx context.Context, o ListRobotOptions) ([]Robot, error) {
	var robots []Robot
	if err := c.cli.Get("/robots").Queries(o.ToQuery()).Return(&robots).Send(ctx); err != nil {
		return nil, err
	}
	return robots, nil
}

func (c *Client) ListProjectRobotAccounts(ctx context.Context, project string, o ListRobotOptions) ([]Robot, error) {
	var robots []Robot
	if err := c.cli.Get("/projects/" + project + "/robots").Queries(o.ToQuery()).Return(&robots).Send(ctx); err != nil {
		return nil, err
	}
	return robots, nil
}

// https://github.com/goharbor/harbor/issues/10672
func (c *Client) CreateProjectRobotAccount(ctx context.Context, project string, robot CreateRobot) (*Robot, error) {
	var r Robot
	if err := c.cli.Post("/projects/" + project + "/robots").JSON(robot).Return(&r).Send(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetProjectRobotAccount(ctx context.Context, project string, id int) (*Robot, error) {
	var r Robot
	if err := c.cli.Get(fmt.Sprintf("/projects/%s/robots/%d", project, id)).Return(&r).Send(ctx); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) GetProjectRobotAccountByName(ctx context.Context, project string, name string) (*Robot, error) {
	encodedname := fmt.Sprintf("%s+%s", project, name)
	opt := ListRobotOptions{
		CommonOptions: CommonOptions{Q: fmt.Sprintf("name=%s", encodedname)},
	}
	robots, err := c.ListProjectRobotAccounts(ctx, project, opt)
	if err != nil {
		return nil, err
	}

	for _, robot := range robots {
		if robot.Name == "robot$"+encodedname {
			return &robot, nil
		}
	}
	return nil, errors.NewNotFound("robot account", name)
}

func (c *Client) DeleteProjectRobotAccount(ctx context.Context, project string, id int) error {
	return c.cli.Delete(fmt.Sprintf("/projects/%s/robots/%d", project, id)).Send(ctx)
}
