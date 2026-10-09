package serviceuser

import (
	"errors"
	domainauth "service-sender/internal/domain/auth"
	domainuser "service-sender/internal/domain/user"
	"service-sender/internal/dto"
	interfaceauth "service-sender/internal/interfaces/auth"
	interfaceuser "service-sender/internal/interfaces/user"
	"service-sender/utils"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type resetUserRepo struct {
	interfaceuser.RepoUserInterface
	user     domainuser.Users
	updates  int
	emailErr error
}

func (r *resetUserRepo) GetByEmail(string) (domainuser.Users, error) { return r.user, r.emailErr }
func (r *resetUserRepo) GetByPhone(string) (domainuser.Users, error) {
	return domainuser.Users{}, gorm.ErrRecordNotFound
}
func (r *resetUserRepo) GetByID(string) (domainuser.Users, error) { return r.user, nil }
func (r *resetUserRepo) Update(user domainuser.Users) error       { r.user = user; r.updates++; return nil }

type resetBlacklistRepo struct {
	interfaceauth.RepoAuthInterface
	mu     sync.Mutex
	tokens map[string]domainauth.Blacklist
	err    error
}

func (r *resetBlacklistRepo) GetByToken(token string) (domainauth.Blacklist, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return domainauth.Blacklist{}, r.err
	}
	if entry, ok := r.tokens[token]; ok {
		return entry, nil
	}
	return domainauth.Blacklist{}, gorm.ErrRecordNotFound
}

func (r *resetBlacklistRepo) Store(entry domainauth.Blacklist) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	if _, ok := r.tokens[entry.Token]; ok {
		return gorm.ErrDuplicatedKey
	}
	r.tokens[entry.Token] = entry
	return nil
}

func TestPasswordResetTokenIsolationAndReplay(t *testing.T) {
	t.Setenv("JWT_KEY", "test-only-key-for-password-reset-regression")
	t.Setenv("RESET_TTL_SECONDS", "900")
	users := &resetUserRepo{user: domainuser.Users{Id: "user-1", Email: "user@example.com", Role: utils.RoleViewer}}
	blacklist := &resetBlacklistRepo{tokens: make(map[string]domainauth.Blacklist)}
	svc := NewUserService(users, blacklist, nil, nil)
	token, err := svc.ForgotPassword(dto.ForgotPasswordRequest{Email: users.user.Email})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := utils.JwtClaim(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims["token_type"] != "password_reset" {
		t.Errorf("reset token has wrong type: %v", claims["token_type"])
	}
	if ttl := time.Until(time.Unix(int64(claims["exp"].(float64)), 0)); ttl <= 0 || ttl > 15*time.Minute {
		t.Errorf("reset expiry exceeds 15 minutes: %v", ttl)
	}
	access, err := utils.GenerateJwt(&users.user, "login")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(dto.ResetPasswordRequest{Token: access, NewPassword: "NewPassword1!"}); err == nil {
		t.Error("access token accepted for password reset")
	}
	if users.updates != 0 {
		t.Error("password changed using access token")
	}
	blacklist.err = errors.New("blacklist unavailable")
	if err := svc.ResetPassword(dto.ResetPasswordRequest{Token: token, NewPassword: "NewPassword1!"}); err == nil {
		t.Error("reset succeeded with unavailable blacklist")
	}
	if users.updates != 0 {
		t.Error("password changed without consuming token")
	}
	blacklist.err = nil
	if err := svc.ResetPassword(dto.ResetPasswordRequest{Token: token, NewPassword: "NewPassword1!"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(dto.ResetPasswordRequest{Token: token, NewPassword: "OtherPassword1!"}); err == nil {
		t.Error("used reset token accepted")
	}
	if users.updates != 1 {
		t.Errorf("expected exactly one update, got %d", users.updates)
	}
	second, err := svc.ForgotPassword(dto.ForgotPasswordRequest{Email: users.user.Email})
	if err != nil {
		t.Fatal(err)
	}
	if second == token {
		t.Error("separate reset requests issued identical tokens")
	}
}

func TestResetPasswordRejectsMissingUserID(t *testing.T) {
	t.Setenv("JWT_KEY", "test-only-key-for-password-reset-regression")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"token_type": "password_reset", "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte("test-only-key-for-password-reset-regression"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUserService(&resetUserRepo{}, &resetBlacklistRepo{tokens: make(map[string]domainauth.Blacklist)}, nil, nil)
	if err := svc.ResetPassword(dto.ResetPasswordRequest{Token: token, NewPassword: "NewPassword1!"}); err == nil {
		t.Fatal("token without user_id accepted")
	}
}

func TestResetPasswordByEmailHashesValidatedPassword(t *testing.T) {
	users := &resetUserRepo{user: domainuser.Users{Id: "user-1", Email: "user@example.com"}}
	svc := NewUserService(users, nil, nil, nil)
	if err := svc.ResetPasswordByEmail("USER@example.com", "weakpass"); err == nil || users.updates != 0 {
		t.Fatal("weak password accepted")
	}
	if err := svc.ResetPasswordByEmail("USER@example.com", "NewPassword1!"); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(users.user.Password), []byte("NewPassword1!")); err != nil {
		t.Fatal("new password was not hashed correctly")
	}
}

func TestRegistrationRejectsEmailLookupFailure(t *testing.T) {
	lookupErr := errors.New("ambiguous email")
	svc := NewUserService(&resetUserRepo{emailErr: lookupErr}, nil, nil, nil)
	if _, err := svc.RegisterUser(dto.UserRegister{Email: "user@example.com", Password: "weakpass"}); !errors.Is(err, lookupErr) {
		t.Errorf("registration ignored email lookup failure: %v", err)
	}
	if _, err := svc.AdminCreateUser(dto.AdminCreateUser{Email: "user@example.com", Password: "weakpass"}, utils.RoleAdmin); !errors.Is(err, lookupErr) {
		t.Errorf("admin registration ignored email lookup failure: %v", err)
	}
}
