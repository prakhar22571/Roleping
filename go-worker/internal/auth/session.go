package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	SessionCookieName = "roleping_session"
	sessionDuration   = 30 * 24 * time.Hour
)

var (
	ErrBadSession     = errors.New("invalid session")
	ErrSessionExpired = errors.New("session expired")
)

// session is what a cookie encodes: which user, which password generation
// (epoch), and when it stops being valid.
type session struct {
	UserID  int64
	Epoch   int64
	Expires int64
}

// signSession renders a session as "<payload>.<hmac>", where payload is
// "userID:epoch:expiry". The HMAC covers the payload, so none of the three
// fields can be edited by the client.
func signSession(secret []byte, s session) string {
	payload := strconv.FormatInt(s.UserID, 10) + ":" +
		strconv.FormatInt(s.Epoch, 10) + ":" +
		strconv.FormatInt(s.Expires, 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func parseSession(secret []byte, value string) (session, error) {
	encPayload, sig, found := strings.Cut(value, ".")
	if !found {
		return session{}, ErrBadSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(encPayload)
	if err != nil {
		return session{}, ErrBadSession
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return session{}, ErrBadSession
	}

	parts := strings.Split(string(payload), ":")
	if len(parts) != 3 {
		return session{}, ErrBadSession
	}
	userID, err1 := strconv.ParseInt(parts[0], 10, 64)
	epoch, err2 := strconv.ParseInt(parts[1], 10, 64)
	expires, err3 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return session{}, ErrBadSession
	}
	if time.Now().Unix() > expires {
		return session{}, ErrSessionExpired
	}
	return session{UserID: userID, Epoch: epoch, Expires: expires}, nil
}

// SetSessionCookie issues a login cookie for the user.
//
// Secure is set unconditionally: the Worker is only ever reached over HTTPS
// in production, and `wrangler dev` on plain http://127.0.0.1 is exempted by
// browsers, which treat localhost as a secure context.
func SetSessionCookie(w http.ResponseWriter, secret []byte, userID, epoch int64) {
	expires := time.Now().Add(sessionDuration)
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    signSession(secret, session{UserID: userID, Epoch: epoch, Expires: expires.Unix()}),
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(sessionDuration / time.Second),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
