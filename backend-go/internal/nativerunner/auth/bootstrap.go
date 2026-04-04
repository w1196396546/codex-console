package auth

import (
	"context"
	"strings"
)

type BootstrapOptions struct {
	Path    string
	Headers Headers
}

type BootstrapResult struct {
	Response
}

func (c *Client) BootstrapWith(ctx context.Context, options BootstrapOptions) (BootstrapResult, error) {
	path := strings.TrimSpace(options.Path)
	if path == "" {
		path = "/"
	}

	response, err := c.Get(ctx, path, options.Headers)
	if err != nil {
		return BootstrapResult{}, err
	}

	return BootstrapResult{Response: response}, nil
}
