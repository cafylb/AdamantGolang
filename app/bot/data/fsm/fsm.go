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
	StateIdle State = ""

	StateUsernameStars State = "wait_username_stars"
	StateUsernamePremium State = "wait_username_premium"
	StateUsernameGifts State = "wait_username_gifts"
	StateAmount State = "wait_amount"
	StateMemo   State = "wait_memo"
)

type Session struct {
	State     State  `json:"state"`
	MessageID int    `json:"message_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Duration  int    `json:"duration,omitempty"`
	GiftID    int    `json:"gift_id,omitempty"`
	Memo      string `json:"memo,omitempty"`
	Anonymous bool   `json:"anonymous,omitempty"`
}

func (session Session) Empty() bool {
	return session.State == StateIdle &&
		session.MessageID == 0 &&
		session.Username == "" &&
		session.Duration == 0 &&
		session.GiftID == 0 &&
		session.Memo == "" &&
		!session.Anonymous
}

type Core struct {
	mu       sync.RWMutex
	sessions map[int64]Session
}

func New() *Core {
	return &Core{
		sessions: make(map[int64]Session),
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
	if msgID != 0 {
		session.MessageID = msgID
	}

	if state == StateIdle {
		session.State = StateIdle
	}

	if session.Empty() {
		delete(client.sessions, tgID)
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
	session.Username = username
	if session.Empty() {
		delete(client.sessions, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) GetUsername(tgID int64) (string, bool) {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	return session.Username, ok && session.Username != ""
}

func (client *Core) SetDuration(tgID int64, duration int) Session {
	client.mu.Lock()
	session := client.sessions[tgID]
	session.Duration = duration
	if session.Empty() {
		delete(client.sessions, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) GetDuration(tgID int64) (int, bool) {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	return session.Duration, ok && session.Duration != 0
}

func (client *Core) SetGift(tgID int64, giftID int, anonymous bool) Session {
	client.mu.Lock()
	session := client.sessions[tgID]
	session.GiftID = giftID
	session.Anonymous = anonymous
	if session.Empty() {
		delete(client.sessions, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) GetGift(tgID int64) (int, bool) {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	return session.GiftID, ok
}

func (client *Core) SetMemo(tgID int64, memo string) Session {
	client.mu.Lock()
	session := client.sessions[tgID]
	session.Memo = memo
	if session.Empty() {
		delete(client.sessions, tgID)
		client.mu.Unlock()
		return Session{}
	}

	client.sessions[tgID] = session
	client.mu.Unlock()

	return session
}

func (client *Core) GetMemo(tgID int64) (string, bool) {
	client.mu.RLock()
	session, ok := client.sessions[tgID]
	client.mu.RUnlock()

	return session.Memo, ok && session.Memo != ""
}

func (client *Core) Clear(tgID int64) {
	client.mu.Lock()
	delete(client.sessions, tgID)
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
		client.mu.Unlock()
		return nil
	}

	sessions := make(map[int64]Session)
	if err := json.Unmarshal(data, &sessions); err != nil {
		return err
	}

	client.mu.Lock()
	client.sessions = sessions
	client.mu.Unlock()

	return nil
}

var UserFSM = New()
