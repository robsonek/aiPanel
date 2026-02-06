// Package iam implements Identity & Access Management.
package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/sqlite"
)

var (
	// ErrInvalidCredentials indicates email/password mismatch.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthorized indicates a missing/invalid session.
	ErrUnauthorized = errors.New("unauthorized")
)

// User is an authenticated user record.
type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Session is an authenticated session result.
type Session struct {
	Token     string
	User      User
	ExpiresAt time.Time
}

// Service provides IAM operations backed by panel.db.
type Service struct {
	store *sqlite.Store
	cfg   config.Config
	log   *slog.Logger
}

// NewService creates IAM service.
func NewService(store *sqlite.Store, cfg config.Config, log *slog.Logger) *Service {
	return &Service{store: store, cfg: cfg, log: log}
}

// CreateAdmin creates an admin user if email is valid.
func (s *Service) CreateAdmin(ctx context.Context, email, password string) error {
	if err := validateEmail(email); err != nil {
		return err
	}
	if len(password) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	sql := fmt.Sprintf(
		"INSERT INTO users(email, password_hash, role, created_at) VALUES('%s','%s','admin',%d);",
		sqlEscape(strings.ToLower(strings.TrimSpace(email))),
		sqlEscape(hash),
		now,
	)
	if err := s.store.ExecPanel(ctx, sql); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	return nil
}

// Login validates credentials and creates a session.
func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, hash, err := s.getUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if !verifyPassword(password, hash) {
		return nil, ErrInvalidCredentials
	}

	token, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	now := time.Now()
	expires := now.Add(s.cfg.SessionTTL)

	insert := fmt.Sprintf(
		"INSERT INTO sessions(token, user_id, expires_at, created_at) VALUES('%s',%d,%d,%d);",
		sqlEscape(token),
		user.ID,
		expires.Unix(),
		now.Unix(),
	)
	if err := s.store.ExecPanel(ctx, insert); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	_ = s.store.ExecAudit(ctx, fmt.Sprintf(
		"INSERT INTO audit_events(actor, action, details, created_at) VALUES('%s','auth.login','success',%d);",
		sqlEscape(user.Email),
		time.Now().Unix(),
	))

	return &Session{
		Token:     token,
		User:      user,
		ExpiresAt: expires,
	}, nil
}

// Logout invalidates an existing session token.
func (s *Service) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	sql := fmt.Sprintf("DELETE FROM sessions WHERE token='%s';", sqlEscape(token))
	if err := s.store.ExecPanel(ctx, sql); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// Authenticate validates a session token and returns associated user.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrUnauthorized
	}
	// Remove expired sessions opportunistically.
	_ = s.store.ExecPanel(ctx, fmt.Sprintf("DELETE FROM sessions WHERE expires_at <= %d;", time.Now().Unix()))

	query := fmt.Sprintf(`
SELECT u.id as id, u.email as email, u.role as role
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = '%s' AND s.expires_at > %d
LIMIT 1;`, sqlEscape(token), time.Now().Unix())
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil || len(rows) == 0 {
		return User{}, ErrUnauthorized
	}
	u, err := mapRowToUser(rows[0])
	if err != nil {
		return User{}, ErrUnauthorized
	}
	return u, nil
}

func (s *Service) getUserByEmail(ctx context.Context, email string) (User, string, error) {
	query := fmt.Sprintf(`
SELECT id, email, role, password_hash
FROM users
WHERE email = '%s'
LIMIT 1;`, sqlEscape(email))
	rows, err := s.store.QueryPanelJSON(ctx, query)
	if err != nil || len(rows) == 0 {
		return User{}, "", fmt.Errorf("user not found")
	}
	user, err := mapRowToUser(rows[0])
	if err != nil {
		return User{}, "", err
	}
	hash, _ := rows[0]["password_hash"].(string)
	if hash == "" {
		return User{}, "", fmt.Errorf("invalid password hash")
	}
	return user, hash, nil
}

func mapRowToUser(row map[string]any) (User, error) {
	id, err := toInt64(row["id"])
	if err != nil {
		return User{}, err
	}
	email, _ := row["email"].(string)
	role, _ := row["role"].(string)
	if email == "" || role == "" {
		return User{}, fmt.Errorf("invalid user row")
	}
	return User{ID: id, Email: email, Role: role}, nil
}

func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}
	_, err := mail.ParseAddress(email)
	if err != nil {
		return fmt.Errorf("invalid email")
	}
	return nil
}

// hashPassword stores iterative SHA-256 with random salt in a structured format.
// Format: sha256i$<iterations>$<salt-hex>$<hash-hex>
func hashPassword(password string) (string, error) {
	salt, err := randomHex(16)
	if err != nil {
		return "", err
	}
	iterations := 120000
	hash := iterativeSHA256(password, salt, iterations)
	return fmt.Sprintf("sha256i$%d$%s$%s", iterations, salt, hash), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "sha256i" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	got := iterativeSHA256(password, parts[2], iterations)
	return subtle.ConstantTimeCompare([]byte(got), []byte(parts[3])) == 1
}

func iterativeSHA256(password, salt string, iterations int) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	out := sum[:]
	for i := 1; i < iterations; i++ {
		next := sha256.Sum256(append(out, []byte(":"+password+":"+salt)...))
		out = next[:]
	}
	return hex.EncodeToString(out)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sqlEscape(in string) string {
	return strings.ReplaceAll(in, "'", "''")
}

func toInt64(v any) (int64, error) {
	switch t := v.(type) {
	case float64:
		return int64(t), nil
	case int64:
		return t, nil
	case string:
		i, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported int conversion type %T", v)
	}
}
