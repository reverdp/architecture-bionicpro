package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

type Session struct {
	ID                     string    `json:"session_id"`
	State                  string    `json:"-"`
	CodeVerifier           string    `json:"-"`
	AccessToken            string    `json:"-"`
	RefreshTokenCiphertext []byte    `json:"-"`
	RefreshTokenNonce      []byte    `json:"-"`
	TokenType              string    `json:"token_type,omitempty"`
	AccessExpiresAt        time.Time `json:"access_expires_at,omitempty"`
	RefreshExpiresAt       time.Time `json:"refresh_expires_at,omitempty"`
	Subject                string    `json:"subject,omitempty"`
	Authenticated          bool      `json:"authenticated"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type TokenSet struct {
	AccessToken      string
	RefreshToken     string
	TokenType        string
	ExpiresIn        int
	RefreshExpiresIn int
	Subject          string
}

type Store struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	aead       cipher.AEAD
	sessionTTL time.Duration
}

func NewStore(encryptionKey []byte, sessionTTL time.Duration) (*Store, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &Store{
		sessions:   make(map[string]*Session),
		aead:       aead,
		sessionTTL: sessionTTL,
	}, nil
}

func (s *Store) Create() (*Session, error) {
	id, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = session

	return cloneSession(session), nil
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok || s.isExpired(session) {
		return nil, false
	}
	return cloneSession(session), true
}

func (s *Store) SetPendingAuth(id, state, verifier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session not found")
	}

	session.State = state
	session.CodeVerifier = verifier
	session.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Store) SaveTokens(id string, tokens TokenSet) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := s.aead.Seal(nil, nonce, []byte(tokens.RefreshToken), []byte(id))
	now := time.Now().UTC()

	session.AccessToken = tokens.AccessToken
	session.RefreshTokenCiphertext = ciphertext
	session.RefreshTokenNonce = nonce
	session.TokenType = tokens.TokenType
	session.AccessExpiresAt = now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	session.RefreshExpiresAt = now.Add(time.Duration(tokens.RefreshExpiresIn) * time.Second)
	session.Subject = tokens.Subject
	session.Authenticated = true
	session.State = ""
	session.CodeVerifier = ""
	session.UpdatedAt = now

	return cloneSession(session), nil
}

func (s *Store) RefreshToken(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return "", fmt.Errorf("session not found")
	}
	if len(session.RefreshTokenCiphertext) == 0 || len(session.RefreshTokenNonce) == 0 {
		return "", fmt.Errorf("refresh token not found")
	}

	refreshToken, err := s.aead.Open(nil, session.RefreshTokenNonce, session.RefreshTokenCiphertext, []byte(id))
	if err != nil {
		return "", fmt.Errorf("decrypt refresh token: %w", err)
	}
	return string(refreshToken), nil
}

func (s *Store) Rotate(id string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.sessions[id]
	if !ok || s.isExpired(current) {
		return nil, fmt.Errorf("session not found")
	}

	newID, err := randomToken(32)
	if err != nil {
		return nil, err
	}

	rotated := *current
	rotated.ID = newID
	rotated.UpdatedAt = time.Now().UTC()

	if len(current.RefreshTokenCiphertext) > 0 && len(current.RefreshTokenNonce) > 0 {
		refreshToken, err := s.aead.Open(nil, current.RefreshTokenNonce, current.RefreshTokenCiphertext, []byte(id))
		if err != nil {
			return nil, fmt.Errorf("decrypt refresh token: %w", err)
		}

		nonce := make([]byte, s.aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return nil, fmt.Errorf("generate nonce: %w", err)
		}

		rotated.RefreshTokenNonce = nonce
		rotated.RefreshTokenCiphertext = s.aead.Seal(nil, nonce, refreshToken, []byte(newID))
	}

	s.sessions[newID] = &rotated
	delete(s.sessions, id)

	return cloneSession(&rotated), nil
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *Store) StateMatches(id, state string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok || s.isExpired(session) {
		return false
	}
	return session.State != "" && session.State == state
}

func (s *Store) CodeVerifier(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok || s.isExpired(session) {
		return "", false
	}
	return session.CodeVerifier, session.CodeVerifier != ""
}

func (s *Store) CleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, session := range s.sessions {
		if s.isExpired(session) {
			delete(s.sessions, id)
		}
	}
}

func (s *Store) isExpired(session *Session) bool {
	return time.Since(session.UpdatedAt) > s.sessionTTL
}

func cloneSession(in *Session) *Session {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func randomToken(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
