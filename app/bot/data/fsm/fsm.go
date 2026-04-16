package fsm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type State string

const (
	StateIdle     State = ""
	StateUsername State = "wait_username"
	StateAmount   State = "wait_amount"
)

type Session struct {
	State     State `json:"state"`
	MessageID int   `json:"message_id,omitempty"`
}

func (session Session) Empty() bool {
	return session.State == StateIdle && session.MessageID == 0
}

type Core struct {
	mu        sync.RWMutex
	sessions  map[int64]Session
	usernames map[int64]string
}

func New() *Core {
	return &Core{
		sessions:  make(map[int64]Session),
		usernames: make(map[int64]string),
	}
}

func (client *Core) Has(tgID int64) string {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	if !ok {
		return ""
	}

	return string(session.State)
}

func (client *Core) Get(tgID int64) (Session, bool) {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	return session, ok
}

func (client *Core) Start(tgID int64, state State) Session {
	session := Session{State: state}

	client.mu.Lock()
	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) Set(tgID int64, session Session) Session {
	client.mu.Lock()
	if session.Empty() {
		delete(client.sessions, tgID)
		delete(client.usernames, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) SetState(tgID int64, state State, msgID int) Session {
	client.mu.Lock()
	session := client.sessions[tgID]
	session.State = state
	session.MessageID = msgID

	if state == StateIdle {
		session.MessageID = 0
		delete(client.sessions, tgID)
		delete(client.usernames, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) SetUsername(tgID int64, username string) Session {
	client.mu.Lock()
	session := client.sessions[tgID]
	if username == "" {
		delete(client.usernames, tgID)
	} else {
		client.usernames[tgID] = username
	}
	client.mu.Unlock()

	return session
}

func (client *Core) GetUsername(tgID int64) (string, bool) {
	client.mu.RLock()
	username, ok := client.usernames[tgID]
	client.mu.RUnlock()

	return username, ok
}

func (client *Core) Clear(tgID int64) {
	client.mu.Lock()
	delete(client.sessions, tgID)
	delete(client.usernames, tgID)
	client.mu.Unlock()
}

func (client *Core) Snapshot() map[int64]Session {
	client.mu.RLock()
	snapshot := make(map[int64]Session, len(client.sessions))
	for tgID, session := range client.sessions {
		snapshot[tgID] = session
	}
	client.mu.RUnlock()

	return snapshot
}

func (client *Core) Save(path string) error {
	if path == "" {
		return errors.New("fsm: empty save path")
	}

	data, err := json.MarshalIndent(client.Snapshot(), "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

func (client *Core) Load(path string) error {
	if path == "" {
		return errors.New("fsm: empty load path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		client.mu.Lock()
		client.sessions = make(map[int64]Session)
		client.usernames = make(map[int64]string)
		client.mu.Unlock()
		return nil
	}

	sessions := make(map[int64]Session)
	if err := json.Unmarshal(data, &sessions); err != nil {
		return err
	}

	client.mu.Lock()
	client.sessions = sessions
	client.usernames = make(map[int64]string)
	client.mu.Unlock()

	return nil
}

var UserFSM = New()
