package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/version"
)

const (
	protocolVersion  = 1
	handshakeTimeout = 10 * time.Second
	maxMessageBytes  = 16 * 1024
)

var errSessionClosed = errors.New("websocket session closed")

type Envelope struct {
	Protocol  int             `json:"protocol"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type Welcome struct {
	NodeID                   string `json:"nodeId"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
}

type Client struct {
	config          config.Config
	commandRegistry commandIDStore
	lifecycle       clientLifecycle
}

type Session struct {
	conn    *websocket.Conn
	welcome Welcome

	cancel context.CancelFunc
	done   chan struct{}

	// Serializes all outgoing messages after the handshake.
	writeMu sync.Mutex

	mu      sync.Mutex
	pending chan error
	closed  bool
	readErr error

	commandState commandExecutionState
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		config:          cfg,
		commandRegistry: newCommandIDRegistry(),
	}
}

// Connect creates a session without a command executor.
//
// Lifecycle commands remain disabled unless an executor
// is explicitly configured.
func (c *Client) Connect(ctx context.Context) (*Session, error) {
	return c.ConnectWithExecutor(ctx, nil)
}

// ConnectWithExecutor installs the executor before the
// WebSocket read loop begins.
//
// Sessions created by the same Client share command ID
// history across reconnects.
func (c *Client) ConnectWithExecutor(
	ctx context.Context,
	executor CommandExecutor,
) (*Session, error) {
	if err := c.beginConnect(); err != nil {
		return nil, err
	}
	defer c.finishConnect()
	endpoint, err := websocketURL(c.config.PanelURL)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("X-Node-Id", c.config.NodeID)
	headers.Set("Authorization", "Bearer "+c.config.NodeToken)

	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, response, err := websocket.Dial(
		dialCtx,
		endpoint,
		&websocket.DialOptions{HTTPHeader: headers},
	)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf(
				"websocket connection rejected (HTTP %d): %w",
				response.StatusCode,
				err,
			)
		}

		return nil, fmt.Errorf(
			"websocket connection failed: %w",
			err,
		)
	}

	conn.SetReadLimit(maxMessageBytes)

	welcome, err := c.handshake(ctx, conn)
	if err != nil {
		conn.CloseNow()
		return nil, err
	}

	sessionCtx, sessionCancel := context.WithCancel(ctx)

	session := &Session{
		conn:    conn,
		welcome: welcome,
		cancel:  sessionCancel,
		done:    make(chan struct{}),
		commandState: commandExecutionState{
			executor: executor,
			seen:     make(map[string]struct{}),
			registry: c.commandRegistry,
		},
	}
	c.trackSession(session)

	go session.readLoop(sessionCtx)

	return session, nil
}

func (c *Client) handshake(
	ctx context.Context,
	conn *websocket.Conn,
) (Welcome, error) {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	agentVersion := version.Current().Version
	if agentVersion == "" {
		agentVersion = "dev"
	}

	err := writeEnvelope(ctx, conn, "node.hello", struct {
		AgentVersion string   `json:"agentVersion"`
		Capabilities []string `json:"capabilities"`
	}{
		AgentVersion: agentVersion,
		Capabilities: []string{},
	})
	if err != nil {
		return Welcome{}, fmt.Errorf(
			"send node.hello: %w",
			err,
		)
	}

	envelope, err := readEnvelope(ctx, conn)
	if err != nil {
		return Welcome{}, fmt.Errorf(
			"receive node.welcome: %w",
			err,
		)
	}

	if envelope.Type != "node.welcome" {
		return Welcome{}, fmt.Errorf(
			"expected node.welcome, got %q",
			envelope.Type,
		)
	}

	var welcome Welcome
	if err := decodeStrict(envelope.Payload, &welcome); err != nil {
		return Welcome{}, fmt.Errorf(
			"invalid node.welcome payload: %w",
			err,
		)
	}

	if welcome.NodeID != c.config.NodeID ||
		welcome.HeartbeatIntervalSeconds < 5 ||
		welcome.HeartbeatIntervalSeconds > 300 {
		return Welcome{}, errors.New(
			"invalid node.welcome payload values",
		)
	}

	return welcome, nil
}

func (s *Session) Welcome() Welcome {
	return s.welcome
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readErr
}

func (s *Session) Close() {
	s.shutdown(errSessionClosed)
	<-s.done
}

func (s *Session) shutdown(err error) {
	if err == nil {
		err = errSessionClosed
	}

	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()
		return
	}

	s.closed = true
	s.readErr = err

	pending := s.pending
	s.pending = nil

	s.mu.Unlock()

	if pending != nil {
		pending <- err
	}

	s.cancel()
	s.conn.CloseNow()
}

func (s *Session) readLoop(ctx context.Context) {
	defer close(s.done)

	for {
		envelope, err := readEnvelope(ctx, s.conn)
		if err != nil {
			s.shutdown(fmt.Errorf(
				"websocket read failed: %w",
				err,
			))
			return
		}

		switch envelope.Type {
		case "command.execute":
			if err := s.handleCommand(
				ctx,
				envelope.Payload,
			); err != nil {
				s.shutdown(err)
				return
			}

		case "node.heartbeat.ack":
			if err := validateHeartbeatACK(
				envelope.Payload,
			); err != nil {
				s.shutdown(err)
				return
			}

			s.mu.Lock()

			if s.closed {
				s.mu.Unlock()
				return
			}

			pending := s.pending
			if pending == nil {
				s.mu.Unlock()
				s.shutdown(errors.New(
					"unexpected heartbeat ACK",
				))
				return
			}

			s.pending = nil
			pending <- nil

			s.mu.Unlock()

		default:
			s.shutdown(fmt.Errorf(
				"unsupported panel message: %s",
				envelope.Type,
			))
			return
		}
	}
}

func (s *Session) sendEnvelope(
	ctx context.Context,
	messageType string,
	payload any,
) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	closed := s.closed
	readErr := s.readErr
	s.mu.Unlock()

	if closed {
		if readErr != nil {
			return readErr
		}

		return errSessionClosed
	}

	return writeEnvelope(
		ctx,
		s.conn,
		messageType,
		payload,
	)
}

func writeEnvelope(
	ctx context.Context,
	conn *websocket.Conn,
	messageType string,
	payload any,
) error {
	id, err := newMessageID()
	if err != nil {
		return err
	}

	data, err := json.Marshal(struct {
		Protocol  int    `json:"protocol"`
		ID        string `json:"id"`
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Payload   any    `json:"payload"`
	}{
		Protocol: protocolVersion,
		ID:       id,
		Type:     messageType,
		Timestamp: time.Now().UTC().Format(
			time.RFC3339Nano,
		),
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf(
			"encode message: %w",
			err,
		)
	}

	return conn.Write(
		ctx,
		websocket.MessageText,
		data,
	)
}

func readEnvelope(
	ctx context.Context,
	conn *websocket.Conn,
) (Envelope, error) {
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		return Envelope{}, err
	}

	if messageType != websocket.MessageText {
		return Envelope{}, errors.New(
			"binary protocol message",
		)
	}

	var envelope Envelope
	if err := decodeStrict(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf(
			"invalid envelope JSON: %w",
			err,
		)
	}

	if envelope.Protocol != protocolVersion ||
		len(envelope.ID) < 1 ||
		len(envelope.ID) > 128 ||
		len(envelope.Type) == 0 {
		return Envelope{}, errors.New(
			"invalid protocol envelope",
		)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(
		envelope.Payload,
		&payload,
	); err != nil || payload == nil {
		return Envelope{}, errors.New(
			"protocol payload must be an object",
		)
	}

	if !validTimestamp(envelope.Timestamp) {
		return Envelope{}, errors.New(
			"invalid protocol timestamp",
		)
	}

	return envelope, nil
}

func validTimestamp(value string) bool {
	if !strings.HasSuffix(value, "Z") {
		return false
	}

	_, err := time.Parse(
		"2006-01-02T15:04:05.999999999Z",
		value,
	)

	return err == nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(
		err,
		io.EOF,
	) {
		if err == nil {
			return errors.New("multiple JSON values")
		}

		return err
	}

	return nil
}

func websocketURL(panelURL string) (string, error) {
	parsed, err := url.Parse(panelURL)
	if err != nil {
		return "", fmt.Errorf(
			"invalid panel URL: %w",
			err,
		)
	}

	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"

	case "https":
		parsed.Scheme = "wss"

	default:
		return "", errors.New(
			"panel URL must use http or https",
		)
	}

	if parsed.Host == "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		return "", errors.New(
			"invalid panel URL",
		)
	}

	parsed.Path = strings.TrimRight(
		parsed.Path,
		"/",
	) + "/agent/v1/ws"

	return parsed.String(), nil
}

func newMessageID() (string, error) {
	var data [16]byte

	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf(
			"generate message ID: %w",
			err,
		)
	}

	return hex.EncodeToString(data[:]), nil
}
