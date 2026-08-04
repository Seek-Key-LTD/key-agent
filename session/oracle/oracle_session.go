// Package oracle implements session.Service backed by Oracle ADB.
//
// Pure Go — uses sijms/go-ora/v2, no ODPI-C / CGO required.
package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	_ "github.com/sijms/go-ora/v2"

	"google.golang.org/adk/v2/session"
)

// OracleSession implements session.Service using Oracle ADB.
type OracleSession struct {
	dsn string
	db  *sql.DB
}

// NewOracleSession creates a new Oracle-backed session service.
func NewOracleSession(dsn string) *OracleSession {
	return &OracleSession{dsn: dsn}
}

// Connect opens the Oracle connection pool.
func (s *OracleSession) Connect(ctx context.Context) error {
	db, err := sql.Open("oracle", s.dsn)
	if err != nil {
		return fmt.Errorf("oracle open: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("oracle ping: %w", err)
	}
	s.db = db
	return nil
}

// Close releases the connection pool.
func (s *OracleSession) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Create creates a new session in Oracle.
func (s *OracleSession) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	sessID := req.SessionID
	if sessID == "" {
		sessID = fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), req.UserID)
	}
	stateJSON, _ := json.Marshal(req.State)
	_, err := s.db.ExecContext(ctx, createSessionSQL,
		sql.Named("app_name", req.AppName),
		sql.Named("user_id", req.UserID),
		sql.Named("session_id", sessID),
		sql.Named("state_json", string(stateJSON)),
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	return &session.CreateResponse{
		Session: newLocalSession(req.AppName, req.UserID, sessID, req.State),
	}, nil
}

// Get retrieves a session from Oracle.
func (s *OracleSession) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	var stateJSON, eventsJSON string
	var updatedAt time.Time
	err := s.db.QueryRowContext(ctx, getSessionSQL,
		sql.Named("app_name", req.AppName),
		sql.Named("user_id", req.UserID),
		sql.Named("session_id", req.SessionID),
	).Scan(&stateJSON, &eventsJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	var state map[string]any
	_ = json.Unmarshal([]byte(stateJSON), &state)
	sess := newLocalSession(req.AppName, req.UserID, req.SessionID, state)
	return &session.GetResponse{Session: sess}, nil
}

// List lists sessions for a given app + user.
func (s *OracleSession) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	rows, err := s.db.QueryContext(ctx, listSessionsSQL,
		sql.Named("app_name", req.AppName),
		sql.Named("user_id", req.UserID),
	)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []session.Session
	for rows.Next() {
		var app, user, sid string
		var ts time.Time
		if err := rows.Scan(&app, &user, &sid, &ts); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		sessions = append(sessions, newLocalSession(app, user, sid, nil))
	}
	if sessions == nil {
		sessions = []session.Session{}
	}
	return &session.ListResponse{Sessions: sessions}, nil
}

// Delete removes a session from Oracle.
func (s *OracleSession) Delete(ctx context.Context, req *session.DeleteRequest) error {
	if s.db == nil {
		return fmt.Errorf("not connected")
	}
	_, err := s.db.ExecContext(ctx, deleteSessionSQL,
		sql.Named("app_name", req.AppName),
		sql.Named("user_id", req.UserID),
		sql.Named("session_id", req.SessionID),
	)
	return err
}

// AppendEvent appends an event to a session and updates its state.
func (s *OracleSession) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if s.db == nil {
		return fmt.Errorf("not connected")
	}
	if sess.ID() == "" {
		return fmt.Errorf("session ID is empty")
	}

	state := sess.State()
	stateMap := map[string]any{}
	for k, v := range state.All() {
		stateMap[k] = v
	}
	stateJSON, _ := json.Marshal(stateMap)

	eventJSON, _ := json.Marshal(event)

	_, err := s.db.ExecContext(ctx, appendEventSQL,
		sql.Named("app_name", sess.AppName()),
		sql.Named("user_id", sess.UserID()),
		sql.Named("session_id", sess.ID()),
		sql.Named("state_json", string(stateJSON)),
		sql.Named("event_json", string(eventJSON)),
	)
	return err
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
	if state == nil {
		state = map[string]any{}
	}
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

// ── SQL statements ──
const createSessionSQL = `
INSERT INTO PICO_SESSION (APP_NAME, USER_ID, SESSION_ID, STATE_JSON, CREATED_TS, UPDATED_TS)
VALUES (:app_name, :user_id, :session_id, :state_json, SYSTIMESTAMP, SYSTIMESTAMP)
`

const getSessionSQL = `
SELECT STATE_JSON, EVENTS_JSON, UPDATED_TS
FROM PICO_SESSION
WHERE APP_NAME = :app_name AND SESSION_ID = :session_id
`

const listSessionsSQL = `
SELECT APP_NAME, USER_ID, SESSION_ID, UPDATED_TS
FROM PICO_SESSION
WHERE APP_NAME = :app_name AND USER_ID = :user_id
ORDER BY UPDATED_TS DESC
`

const deleteSessionSQL = `
DELETE FROM PICO_SESSION
WHERE APP_NAME = :app_name AND SESSION_ID = :session_id
`

const appendEventSQL = `
UPDATE PICO_SESSION
SET STATE_JSON = :state_json,
    EVENTS_JSON = EVENTS_JSON || :event_json,
    UPDATED_TS = SYSTIMESTAMP
WHERE APP_NAME = :app_name AND SESSION_ID = :session_id
`
