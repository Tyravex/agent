package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Tyravex/agent/internal/config"
)

type Client struct {
	httpClient *http.Client
	config     config.Config
}

type heartbeatResponse struct {
	OK bool `json:"ok"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(
				_ *http.Request,
				_ []*http.Request,
			) error {
				return http.ErrUseLastResponse
			},
		},
		config: cfg,
	}
}

func (c *Client) Send(ctx context.Context) error {
	endpoint, err := url.Parse(c.config.PanelURL)
	if err != nil {
		return fmt.Errorf("invalid panel URL: %w", err)
	}

	endpoint.Path = strings.TrimRight(endpoint.Path, "/") +
		"/nodes/" + url.PathEscape(c.config.NodeID) + "/heartbeat"

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewBufferString("{}"),
	)
	if err != nil {
		return fmt.Errorf("create heartbeat request: %w", err)
	}

	request.Header.Set("x-node-token", c.config.NodeToken)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("heartbeat request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated &&
		response.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"heartbeat rejected (HTTP %d)",
			response.StatusCode,
		)
	}

	var result heartbeatResponse

	if err := json.NewDecoder(
		http.MaxBytesReader(nil, response.Body, 4096),
	).Decode(&result); err != nil {
		return fmt.Errorf("invalid heartbeat response: %w", err)
	}

	if !result.OK {
		return errors.New("heartbeat response did not confirm success")
	}

	return nil
}
