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

	"github.com/Tyravex/agent/internal/commands"
	"github.com/Tyravex/agent/internal/config"
	"github.com/Tyravex/agent/internal/version"
)

const (
	protocolVersion  = 1
	handshakeTimeout = 10 * time.Second

	// Maximum size of an ordinary protocol message.
	maxMessageBytes = 16 * 1024

	// Ownership snapshots may be larger than ordinary
	// messages. The extra space is reserved for the
	// WebSocket protocol envelope.
	maxOwnershipEnvelopeBytes = (1 << 20) + 4096
)

var errSessionClosed = errors.New("websocket session closed")

// Envelope is the common WebSocket protocol envelope.
//
// rawSize records the exact number of bytes received
// from the WebSocket connection. It is not serialized.
type Envelope struct {
	Protocol  int             `json:"protocol"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`

	rawSize int
}

// Welcome is the validated node.welcome payload.
type Welcome struct {
	NodeID                   string `json:"nodeId"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
	SessionID                string `json:"sessionId,omitempty"`
}

// Client manages WebSocket sessions and shared command
// ID history across reconnects.
type Client struct {
	config          config.Config
	commandRegistry commandIDStore
	lifecycle       clientLifecycle

	// Optional HTTP client used for WebSocket dialing.
	//
	// nil preserves the standard TLS verification.
	// Tests may use httptest.Server.Client() to trust
	// the test server's certificate.
	httpClient *http.Client
}

// Session represents one established WebSocket connection.
type Session struct {
	conn    *websocket.Conn
	welcome Welcome

	cancel context.CancelFunc
	done   chan struct{}

	// Serializes outgoing WebSocket writes.
	writeMu sync.Mutex

	// Protects session state and synchronizes ownership
	// activation with shutdown.
	mu      sync.Mutex
	pending chan error
	closed  bool
	readErr error

	commandState commandExecutionState

	// nil for standard WebSocket sessions.
	ownership *ownershipSession
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		config:          cfg,
		commandRegistry: newCommandIDRegistry(),
	}
}

// Connect creates a standard WebSocket session.
//
// No command executor or ownership synchronization is
// enabled by default.
func (c *Client) Connect(
	ctx context.Context,
) (*Session, error) {
	return c.ConnectWithExecutor(ctx, nil)
}

// ConnectWithExecutor creates a WebSocket session with
// the supplied command executor.
//
// Ownership synchronization remains disabled.
func (c *Client) ConnectWithExecutor(
	ctx context.Context,
	executor CommandExecutor,
) (*Session, error) {
	return c.connect(ctx, executor, nil)
}

// connect implements the shared WebSocket lifecycle.
//
// ownershipManager is supplied only by the explicit
// ConnectWithOwnership entry point.
//
// Opening an ownership connection does not authorize
// services. Authorization requires explicit activation
// and a valid ownership snapshot.
func (c *Client) connect(
	ctx context.Context,
	executor CommandExecutor,
	ownershipManager *commands.OwnershipConnectionManager,
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
	headers.Set(
		"Authorization",
		"Bearer "+c.config.NodeToken,
	)

	dialCtx, dialCancel := context.WithTimeout(
		ctx,
		handshakeTimeout,
	)
	defer dialCancel()

	conn, response, err := websocket.Dial(
		dialCtx,
		endpoint,
		&websocket.DialOptions{
			HTTPHeader: headers,
			HTTPClient: c.httpClient,
		},
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

	// Standard sessions never need the larger limit.
	// Ownership sessions may receive large snapshots,
	// but all other message types are still restricted
	// to maxMessageBytes.
	readLimit := int64(maxMessageBytes)

	if ownershipManager != nil {
		readLimit = int64(maxOwnershipEnvelopeBytes)
	}

	conn.SetReadLimit(readLimit)

	welcome, err := c.handshake(ctx, conn)
	if err != nil {
		conn.CloseNow()
		return nil, err
	}

	if ownershipManager != nil && !validOwnershipSessionID(welcome.SessionID) {
		conn.CloseNow()
		return nil, ErrOwnershipWelcomeSessionRequired
	}

	var ownership *ownershipSession

	if ownershipManager != nil {
		ownership, err = newOwnershipSession(
			ctx,
			ownershipManager,
		)
		if err != nil {
			conn.CloseNow()
			return nil, err
		}
	}

	// Bind the authenticated Panel session to the local
	// ownership generation before starting the read loop.
	if ownership != nil {
		if err := ownership.activate(ctx, welcome.SessionID); err != nil {
			ownership.close()
			conn.CloseNow()
			return nil, fmt.Errorf(
				"activate ownership session: %w",
				err,
			)
		}
	}

	sessionCtx, sessionCancel := context.WithCancel(ctx)

	session := &Session{
		conn:      conn,
		welcome:   welcome,
		cancel:    sessionCancel,
		done:      make(chan struct{}),
		ownership: ownership,
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

// handshake exchanges node.hello and node.welcome.
//
// The handshake validates the node identity and
// heartbeat interval. It does not activate ownership.
func (c *Client) handshake(
	ctx context.Context,
	conn *websocket.Conn,
) (Welcome, error) {
	ctx, cancel := context.WithTimeout(
		ctx,
		handshakeTimeout,
	)
	defer cancel()

	agentVersion := version.Current().Version
	if agentVersion == "" {
		agentVersion = "dev"
	}

	err := writeEnvelope(
		ctx,
		conn,
		"node.hello",
		struct {
			AgentVersion string   `json:"agentVersion"`
			Capabilities []string `json:"capabilities"`
		}{
			AgentVersion: agentVersion,
			Capabilities: []string{},
		},
	)
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

	// Ownership-enabled connections use a larger
	// WebSocket read limit. The handshake must still
	// obey the ordinary 16 KiB protocol limit.
	if envelope.rawSize > maxMessageBytes {
		return Welcome{}, errors.New(
			"node.welcome exceeds size limit",
		)
	}

	if envelope.Type != "node.welcome" {
		return Welcome{}, fmt.Errorf(
			"expected node.welcome, got %q",
			envelope.Type,
		)
	}

	var welcome Welcome

	if err := decodeStrict(
		envelope.Payload,
		&welcome,
	); err != nil {
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

// Welcome returns the validated handshake payload.
func (s *Session) Welcome() Welcome {
	return s.welcome
}

// Done is closed when the read loop exits.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Err returns the session's shutdown reason.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readErr
}

// Close terminates the WebSocket session and waits
// for the read loop to finish.
func (s *Session) Close() {
	s.shutdown(errSessionClosed)
	<-s.done
}

// shutdown closes a session exactly once.
//
// Ownership revocation occurs under the same mutex
// used by ActivateOwnership, preventing activation
// from surviving a concurrent shutdown.
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

	// Revoke ownership before releasing s.mu.
	//
	// OwnershipConnection.Close is generation-aware,
	// so an older connection cannot revoke a newer
	// connection's authorization.
	if s.ownership != nil {
		s.ownership.close()
	}

	s.mu.Unlock()

	if pending != nil {
		pending <- err
	}

	s.cancel()
	s.conn.CloseNow()
}

// readLoop handles incoming messages.
//
// Malformed, oversized or unsupported messages close
// the connection and revoke ownership.
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

		// Only ownership.snapshot may exceed the
		// ordinary 16 KiB limit.
		//
		// rawSize is measured from the original bytes,
		// not from a reserialized JSON representation.
		if envelope.Type !=
			commands.OwnershipSnapshotMessageType &&
			envelope.rawSize > maxMessageBytes {
			s.shutdown(errors.New(
				"ordinary protocol message exceeds size limit",
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

		case commands.OwnershipSnapshotMessageType:
			if s.ownership == nil {
				s.shutdown(
					ErrOwnershipTransportDisabled,
				)
				return
			}

			// Snapshot validation, revision checks,
			// session matching and generation fencing
			// are handled by ownershipSession and
			// OwnershipConnectionManager.
			if err := s.ownership.applySnapshot(
				ctx,
				envelope.Payload,
			); err != nil {
				s.shutdown(fmt.Errorf(
					"invalid ownership.snapshot: %w",
					err,
				))
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

// sendEnvelope serializes outgoing WebSocket writes
// and refuses writes after session shutdown.
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

// writeEnvelope sends one JSON protocol message.
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

// readEnvelope reads and validates a WebSocket
// protocol envelope.
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

	// Reject duplicate keys, including keys nested
	// inside the payload.
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return Envelope{}, fmt.Errorf(
			"invalid envelope JSON: %w",
			err,
		)
	}

	var envelope Envelope

	if err := decodeStrict(
		data,
		&envelope,
	); err != nil {
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

	// Record the original byte count. This is used
	// to enforce per-message-type size limits.
	envelope.rawSize = len(data)

	return envelope, nil
}

// validTimestamp requires a UTC RFC3339 timestamp.
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

// decodeStrict rejects unknown fields and trailing
// JSON values.
func decodeStrict(
	data []byte,
	target any,
) error {
	decoder := json.NewDecoder(
		bytes.NewReader(data),
	)
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
			return errors.New(
				"multiple JSON values",
			)
		}

		return err
	}

	return nil
}

// websocketURL builds the Agent WebSocket endpoint.
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

// newMessageID generates a random message identifier.
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
