package docker

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

type Container struct {
	ID      string
	Name    string
	Image   string
	State   string
	Status  string
	Created int64
}

type Info struct {
	ServerVersion string
	Containers    []Container
}

type Client struct {
	api *client.Client
}

func NewClient() (*Client, error) {
	return NewClientWithHost("")
}

func NewClientWithHost(host string) (*Client, error) {
	host = strings.TrimSpace(host)

	options := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}

	if host != "" {
		parsed, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("invalid Docker host: %w", err)
		}

		switch parsed.Scheme {
		case "unix", "npipe":
			if parsed.Path == "" {
				return nil, fmt.Errorf(
					"invalid Docker host: missing socket or pipe path",
				)
			}

		case "tcp":
			if parsed.Hostname() == "" || parsed.Port() == "" {
				return nil, fmt.Errorf(
					"invalid Docker host: TCP address requires host and port",
				)
			}

		default:
			return nil, fmt.Errorf(
				"unsupported Docker host scheme: %q",
				parsed.Scheme,
			)
		}

		options = append(options, client.WithHost(host))
	}

	api, err := client.New(options...)
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}

	return &Client{api: api}, nil
}

func (c *Client) Close() error {
	return c.api.Close()
}

func (c *Client) Inspect(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := c.api.Ping(ctx, client.PingOptions{})
	if err != nil {
		return Info{}, fmt.Errorf("Docker Engine unavailable: %w", err)
	}

	version, err := c.api.ServerVersion(
		ctx,
		client.ServerVersionOptions{},
	)
	if err != nil {
		return Info{}, fmt.Errorf("read Docker version: %w", err)
	}

	listResult, err := c.api.ContainerList(
		ctx,
		client.ContainerListOptions{
			All: true,
		},
	)
	if err != nil {
		return Info{}, fmt.Errorf("list Docker containers: %w", err)
	}

	result := Info{
		ServerVersion: version.Version,
		Containers:    make([]Container, 0, len(listResult.Items)),
	}

	for _, item := range listResult.Items {
		name := ""

		if len(item.Names) > 0 {
			name = item.Names[0]
		}

		result.Containers = append(result.Containers, Container{
			ID:      item.ID,
			Name:    name,
			Image:   item.Image,
			State:   string(item.State),
			Status:  item.Status,
			Created: item.Created,
		})
	}

	return result, nil
}
