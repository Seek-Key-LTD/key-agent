// Package oracle implements session.Service backed by Oracle ADB.
package oracle

import (
	"context"
	"encoding/json"
	"fmt"
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
	sessID := req.SessionID
	if sessID == "" {
		sessID = fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), req.UserID)
	}

	stateJSON, _ := json.Marshal(req.State)

	// INSERT INTO PICO_SESSION (app_name, user_id, session_id, state_json)
	// VALUES (:app_name, :user_id, :session_id, :state_json)

	// TODO: actual DB execution
	// If session already exists, return it (idempotent create)

	return &session.CreateResponse{
		Session: &localSession{
			appName:  req.AppName,
			userID:   req.UserID,
			sessionID: sessID,
			state:     req.State,
		},
	}, nil
}

// Get retrieves a session from Oracle.
func (s *OracleSession) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	// SELECT state_json, events_json, updated_at
	// FROM PICO_SESSION
	// WHERE app_name = :app_name AND user_id = :user_id AND session_id = :session_id

	// TODO: actual DB execution + optional filters (NumRecentEvents, After)

	return nil, session.ErrSessionNotFound
}

// List lists sessions for a given app + user.
func (s *OracleSession) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	// SELECT app_name, user_id, session_id, updated_at
	// FROM PICO_SESSION
	// WHERE app_name = :app_name AND user_id = :user_id
	// ORDER BY updated_at DESC

	// TODO: actual DB execution

	return &session.ListResponse{Sessions: []session.Session{}}, nil
}

// Delete removes a session from Oracle.
func (s *OracleSession) Delete(ctx context.Context, req *session.DeleteRequest) error {
	// DELETE FROM PICO_SESSION
	// WHERE app_name = :app_name AND user_id = :user_id AND session_id = :session_id

	// TODO: actual DB execution

	return nil
}

// AppendEvent appends an event to a session and updates its state.
func (s *OracleSession) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if sess.ID() == "" {
		return fmt.Errorf("session ID is empty")
	}

	// Update session state from event state delta
	newState := mergeState(sess.State(), event.Actions.StateDelta)

	// UPDATE PICO_SESSION
	// SET state_json = :state_json, events_json = events_json || :event_json, updated_at = CURRENT_TIMESTAMP
	// WHERE app_name = :app_name AND user_id = :user_id AND session_id = :session_id

	// TODO: actual DB execution

	return nil
}

// localSession implements session.Session interface for Create response.
type localSession struct {
	appName   string
	userID    string
	sessionID string
	state     map[string]any
}

func (l *localSession) ID() string         { return l.sessionID }
func (l *localSession) AppName() string    { return l.appName }
func (l *localSession) UserID() string     { return l.userID }
func (l *localSession) State() session.State {
	return &memState{state: l.state}
}
func (l *localSession) Events() session.Events {
	return &memEvents{}
}
func (l *localSession) LastUpdateTime() time.Time {
	return time.Time{}
}

// memState implements session.State.
type memState struct {
	state map[string]any
}

func (m *memState) Get(key string) (any, error) {
	if v, ok := m.state[key]; ok {
		return v, nil
	}
	return nil, session.ErrStateKeyNotExist
}

func (m *memState) All() any {
	// TODO: return iter.Seq2[string, any] when fully implemented
	return m.state
}

func (m *memState) Set(key string, value any) error {
	m.state[key] = value
	return nil
}

// memEvents implements session.Events.
type memEvents struct{}

func (e *memEvents) Len() int              { return 0 }
func (e *memEvents) At(i int) *session.Event { return nil }
func (e *memEvents) All() any              { return nil } // TODO: iter.Seq[*session.Event]

// mergeState merges event state delta into existing session state.
func mergeState(existing session.State, delta map[string]any) map[string]any {
	result := make(map[string]any)
	// TODO: copy from existing state
	for k, v := range delta {
		result[k] = v
	}
	return result
}

// SQL
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
