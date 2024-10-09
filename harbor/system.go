package harbor

import (
	"context"
	"time"
)

type SystemInfo struct {
	AuthMode           string    `json:"auth_mode"`
	BannerMessage      string    `json:"banner_message"`
	CurrentTime        time.Time `json:"current_time"`
	ExternalURL        string    `json:"external_url"`
	HarborVersion      string    `json:"harbor_version"`
	NotificationEnable bool      `json:"notification_enable"`
}

func (c *Client) SystemInfo(ctx context.Context) (*SystemInfo, error) {
	var info SystemInfo
	if err := c.cli.Get(ctx, "/systeminfo", nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.cli.Get(ctx, "/ping", nil, nil)
}
