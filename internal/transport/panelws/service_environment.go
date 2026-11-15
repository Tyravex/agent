package panelws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/Tyravex/agent/internal/transport/protocol"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FetchServiceEnvironment is separate from command persistence and only accepts HTTPS.
// The installation executor must pass bindings from its validated active session.
func (c *Client) FetchServiceEnvironment(ctx context.Context, binding protocol.EnvironmentBinding, allowed map[string]bool) (map[string]string, error) {
	fail := func() (map[string]string, error) { return nil, errors.New("service environment delivery unavailable") }
	endpoint, err := url.Parse(c.config.PanelURL)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || binding.NodeID != c.config.NodeID {
		return fail()
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/agent/nodes/" + url.PathEscape(c.config.NodeID) + "/environment"
	body, err := json.Marshal(map[string]string{"commandId": binding.CommandID, "sessionId": binding.SessionID})
	if err != nil {
		return fail()
	}
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fail()
	}
	req.Header.Set("Authorization", "Bearer "+c.config.NodeToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	source := c.httpClient
	if source == nil {
		source = http.DefaultClient
	}
	httpClient := *source
	// A redirect must never receive the Agent token.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := httpClient.Do(req)
	if err != nil {
		return fail()
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return fail()
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 131073))
	if err != nil || len(data) > 131072 {
		return fail()
	}
	return protocol.OpenServiceEnvironment(data, c.config.NodeToken, binding, allowed, time.Now())
}
