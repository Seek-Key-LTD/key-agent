// Package oracle implements session.Service backed by Oracle ADB.
package oracle

import (
	"context"
	"fmt"
	"iter"
	"time"

	"google.golang.org/adk/v2/session"
)

// OracleSession implements session.Service using Oracle ADB.
type OracleSession struct {
	dsn string
}

// NewOracleSession creates a new Oracle-backed session service.
func NewOracleSession(dsn string) *OracleSession {
	return &OracleSession{dsn: dsn}
}

// Create creates a new session in Oracle.
func (s *OracleSession) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	_ = ctx
	sessID := req.SessionID
	if sessID == "" {
		sessID = fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), req.UserID)
	}

	return &session.CreateResponse{
		Session: newLocalSession(req.AppName, req.UserID, sessID, req.State),
	}, nil
}

// Get retrieves a session from Oracle.
func (s *OracleSession) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	_ = ctx
	// TODO: SELECT state_json, events_json FROM PICO_SESSION ...
	return nil, fmt.Errorf("session not found")
}

// List lists sessions for a given app + user.
func (s *OracleSession) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	_ = ctx
	// TODO: SELECT app_name, user_id, session_id FROM PICO_SESSION ...
	return &session.ListResponse{Sessions: []session.Session{}}, nil
}

// Delete removes a session from Oracle.
func (s *OracleSession) Delete(ctx context.Context, req *session.DeleteRequest) error {
	_ = ctx
	// TODO: DELETE FROM PICO_SESSION ...
	return nil
}

// AppendEvent appends an event to a session and updates its state.
func (s *OracleSession) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	_ = ctx
	if sess.ID() == "" {
		return fmt.Errorf("session ID is empty")
	}

	state := sess.State()
	for k, v := range event.Actions.StateDelta {
		if err := state.Set(k, v); err != nil {
			return fmt.Errorf("set state %q: %w", k, err)
		}
	}

	// TODO: UPDATE PICO_SESSION ...
	return nil
}

// ── local implementations ──

type localSession struct {
	appName   string
	userID    string
	sessionID string
	state     map[string]any
	events    []*session.Event
}

func newLocalSession(appName, userID, sessionID string, state map[string]any) *localSession {
	return &localSession{
		appName:   appName,
		userID:    userID,
		sessionID: sessionID,
		state:     state,
		events:    make([]*session.Event, 0),
	}
}

func (l *localSession) ID() string           { return l.sessionID }
func (l *localSession) AppName() string      { return l.appName }
func (l *localSession) UserID() string       { return l.userID }
func (l *localSession) State() session.State { return memState(l.state) }
func (l *localSession) Events() session.Events {
	return (*memEvents)(&l.events)
}
func (l *localSession) LastUpdateTime() time.Time { return time.Time{} }

// memState implements session.State.
type memState map[string]any

func (m memState) Get(key string) (any, error) {
	if v, ok := m[key]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("state key %q not found", key)
}

func (m memState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (m memState) Set(key string, value any) error {
	m[key] = value
	return nil
}

// memEvents implements session.Events.
type memEvents []*session.Event

func (e memEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}

func (e memEvents) Len() int          { return len(e) }
func (e memEvents) At(i int) *session.Event {
	if i >= 0 && i < len(e) {
		return e[i]
	}
	return nil
}

// ── SQL ──
const createSessionSQL = `
INSERT INTO PICO_SESSION (APP_NAME, USER_ID, SESSION_ID, STATE_JSON, CREATED_AT, UPDATED_AT)
VALUES (:app_name, :user_id, :session_id, :state_json, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`

const getSessionSQL = `
SELECT STATE_JSON, EVENTS_JSON, UPDATED_AT
FROM PICO_SESSION
WHERE APP_NAME = :app_name AND USER_ID = :user_id AND SESSION_ID = :session_id
`

const listSessionsSQL = `
SELECT APP_NAME, USER_ID, SESSION_ID, UPDATED_AT
FROM PICO_SESSION
WHERE APP_NAME = :app_name AND USER_ID = :user_id
ORDER BY UPDATED_AT DESC
`

const deleteSessionSQL = `
DELETE FROM PICO_SESSION
WHERE APP_NAME = :app_name AND USER_ID = :user_id AND SESSION_ID = :session_id
`

const appendEventSQL = `
UPDATE PICO_SESSION
SET STATE_JSON = :state_json, EVENTS_JSON = events_json || :event_json, UPDATED_AT = CURRENT_TIMESTAMP
WHERE APP_NAME = :app_name AND USER_ID = :user_id AND SESSION_ID = :session_id
`
