// Package auth is the identity service: user accounts, password sign-in,
// access-token issuing (EdDSA JWT) and publication of the verification keys
// (JWKS) used by every other service.
//
// Accounts map a person to exactly one role and tenant:
//
//	driver         → fleet org + driver id     (mobile app)
//	fleet_manager  → fleet org                 (mobile app + web admin)
//	attendant      → station + staff id        (mobile scanner)
//	station_admin  → station + staff id        (web admin)
//
// Demo mode (DEMO_MODE=true) seeds one account per prototype persona and
// enables password-less "sign in as" for those accounts only, so the demo
// keeps the prototype's one-click role switching. Never enable it in production.
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/authn"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/httpx"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/i18n"
	"github.com/bonbonati/fuel-management-hub/backend/internal/platform/kv"
)

// User is an account.
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         authn.Role `json:"role"`
	OrgID        string     `json:"orgId,omitempty"`
	StationID    string     `json:"stationId,omitempty"`
	ActorID      string     `json:"actorId,omitempty"`
	Name         i18n.Text  `json:"name"`
	Active       bool       `json:"active"`
	Demo         bool       `json:"demo"`
}

// ErrNotFound is returned by Repo lookups for unknown users.
var ErrNotFound = errors.New("auth: user not found")

// Repo is the persistence port of the auth service.
type Repo interface {
	ByUsername(ctx context.Context, username string) (User, error) // case-insensitive
	ByID(ctx context.Context, id string) (User, error)
	Insert(ctx context.Context, u User) (string, error)
	ListDemo(ctx context.Context) ([]User, error)
}

// Service implements sign-in.
type Service struct {
	repo       Repo
	issuer     *authn.Issuer
	limiter    kv.Store
	demo       bool
	bcryptCost int
	dummyHash  []byte // compared against when the user does not exist (timing equalization)
}

// Options configures a Service.
type Options struct {
	DemoMode   bool
	BcryptCost int // defaults to 12
}

// NewService returns a Service.
func NewService(repo Repo, issuer *authn.Issuer, limiter kv.Store, opt Options) *Service {
	if opt.BcryptCost == 0 {
		opt.BcryptCost = 12
	}
	dummy, _ := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), opt.BcryptCost)
	return &Service{repo: repo, issuer: issuer, limiter: limiter, demo: opt.DemoMode, bcryptCost: opt.BcryptCost, dummyHash: dummy}
}

// Login throttling: fixed windows per username and per client IP.
const (
	throttleWindow = 5 * time.Minute
	maxPerUsername = 10
	maxPerIP       = 50
	maxUsernameLen = 64
	maxPasswordLen = 72 // bcrypt's input limit
)

var (
	errBadCredentials = httpx.NewError(http.StatusUnauthorized, "invalid_credentials",
		"اسم المستخدم أو كلمة المرور غير صحيحة.", "The username or password is incorrect.")
	errThrottled = httpx.NewError(http.StatusTooManyRequests, "too_many_attempts",
		"محاولات كثيرة. حاول مرة أخرى بعد بضع دقائق.", "Too many attempts. Try again in a few minutes.")
	errDemoDisabled = httpx.NotFound("demo_disabled", "وضع العرض غير مفعّل.", "Demo mode is not enabled.")
)

// TokenResponse is returned after a successful sign-in.
type TokenResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        User      `json:"user"`
}

// Login verifies a username/password pair and issues an access token.
func (s *Service) Login(ctx context.Context, username, password, clientIP string) (TokenResponse, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > maxUsernameLen || len(password) > maxPasswordLen {
		return TokenResponse{}, errBadCredentials
	}
	if err := s.throttle(ctx, "login:u:"+strings.ToLower(username), maxPerUsername); err != nil {
		return TokenResponse{}, err
	}
	if clientIP != "" {
		if err := s.throttle(ctx, "login:ip:"+clientIP, maxPerIP); err != nil {
			return TokenResponse{}, err
		}
	}
	u, err := s.repo.ByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password)) // same cost as a real check
		return TokenResponse{}, errBadCredentials
	} else if err != nil {
		return TokenResponse{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || !u.Active {
		return TokenResponse{}, errBadCredentials
	}
	_ = s.limiter.Delete(ctx, "login:u:"+strings.ToLower(username))
	return s.issue(u)
}

// throttle increments the attempt counter for key and fails when over max.
func (s *Service) throttle(ctx context.Context, key string, max int64) error {
	n, err := s.limiter.Incr(ctx, key, throttleWindow)
	if err != nil {
		return err
	}
	if n > max {
		return errThrottled
	}
	return nil
}

// DemoLogin issues a token for a demo account without a password. Only
// available in demo mode, and only for accounts flagged as demo.
func (s *Service) DemoLogin(ctx context.Context, username string) (TokenResponse, error) {
	if !s.demo {
		return TokenResponse{}, errDemoDisabled
	}
	u, err := s.repo.ByUsername(ctx, strings.TrimSpace(username))
	if err != nil || !u.Demo || !u.Active {
		return TokenResponse{}, errBadCredentials
	}
	return s.issue(u)
}

// DemoAccounts lists the demo personas (empty unless demo mode is on).
func (s *Service) DemoAccounts(ctx context.Context) ([]User, error) {
	if !s.demo {
		return []User{}, nil
	}
	return s.repo.ListDemo(ctx)
}

// Me returns the user behind a verified token.
func (s *Service) Me(ctx context.Context, userID string) (User, error) {
	u, err := s.repo.ByID(ctx, userID)
	if err != nil {
		return User{}, httpx.ErrUnauthorized
	}
	return u, nil
}

func (s *Service) issue(u User) (TokenResponse, error) {
	tok, exp, err := s.issuer.Issue(u.ID, authn.Claims{Role: u.Role, Org: u.OrgID, Station: u.StationID, Actor: u.ActorID, Name: u.Name})
	if err != nil {
		return TokenResponse{}, err
	}
	return TokenResponse{AccessToken: tok, TokenType: "Bearer", ExpiresAt: exp, User: u}, nil
}

// HashPassword hashes a password with the service's bcrypt cost.
func (s *Service) HashPassword(pw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), s.bcryptCost)
	return string(h), err
}

// DemoPersonas are the prototype's personas, one account each. Usernames are
// the people's staff/driver codes so they match what the apps display.
var DemoPersonas = []User{
	{Username: "fleet.manager", Role: authn.RoleFleetManager, OrgID: "o1", ActorID: "m1", Name: i18n.T("مدير أسطول النماء", "Al Namaa fleet manager")},
	{Username: "D-3071", Role: authn.RoleDriver, OrgID: "o1", ActorID: "d3", Name: i18n.T("عبدالرحمن حسن", "Abdurahman Hassan")},
	{Username: "D-3012", Role: authn.RoleDriver, OrgID: "o1", ActorID: "d1", Name: i18n.T("فيصل عثمان", "Faisal Osman")},
	{Username: "D-3025", Role: authn.RoleDriver, OrgID: "o1", ActorID: "d2", Name: i18n.T("عمر الطيب", "Omar Altayeb")},
	{Username: "D-3088", Role: authn.RoleDriver, OrgID: "o1", ActorID: "d4", Name: i18n.T("سعد إبراهيم", "Saad Ibrahim")},
	{Username: "D-3117", Role: authn.RoleDriver, OrgID: "o1", ActorID: "d6", Name: i18n.T("حمد الأمين", "Hamad Alamin")},
	{Username: "ATT-09", Role: authn.RoleAttendant, StationID: "s1", ActorID: "a2", Name: i18n.T("إبراهيم بابكر", "Ibrahim Babiker")},
	{Username: "ATT-01", Role: authn.RoleStationAdmin, StationID: "s1", ActorID: "a1", Name: i18n.T("مجدي الشيخ", "Magdi Alsheikh")},
}

// SeedDemo creates the demo personas that do not exist yet, all with
// password. Existing accounts are never modified.
func (s *Service) SeedDemo(ctx context.Context, password string) (created int, err error) {
	for _, p := range DemoPersonas {
		if _, err := s.repo.ByUsername(ctx, p.Username); err == nil {
			continue
		} else if !errors.Is(err, ErrNotFound) {
			return created, err
		}
		h, err := s.HashPassword(password)
		if err != nil {
			return created, err
		}
		p.PasswordHash, p.Active, p.Demo = h, true, true
		if _, err := s.repo.Insert(ctx, p); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}
