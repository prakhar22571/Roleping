package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"roleping-worker/internal/auth"
	"roleping-worker/internal/config"
	"roleping-worker/internal/db"
	"roleping-worker/internal/httprouter"
)

// generatedPasswordLength gives ~103 bits of entropy, which is what lets us
// keep the PBKDF2 work factor low enough to fit in a Worker's CPU budget.
const generatedPasswordLength = 18

const minPasswordLength = 10

func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.FromContext(r.Context()); ok {
		http.Redirect(w, r, "/jobs", http.StatusFound)
		return
	}
	renderLogin(w, r, "", "")
}

func renderLogin(w http.ResponseWriter, r *http.Request, email, errorMessage string) {
	if errorMessage != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	templ.Handler(LoginPage(email, errorMessage)).ServeHTTP(w, r)
}

func loginSubmitHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderLogin(w, r, "", "Could not read that form. Try again.")
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	secret := auth.SessionSecret()
	if len(secret) == 0 {
		// Without a signing key any cookie we issued would be forgeable.
		http.Error(w, "server is missing SESSION_SECRET", http.StatusInternalServerError)
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := db.GetUserByEmail(r.Context(), env.DB, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Same message whether the account is unknown, has no password, or the
	// password is wrong - otherwise this doubles as an account enumerator.
	const failed = "That email and password combination didn't work."
	if user == nil || !user.HasPassword() {
		renderLogin(w, r, email, failed)
		return
	}
	stored := auth.PasswordHash{
		Hash:       user.PasswordHash,
		Salt:       user.PasswordSalt,
		Iterations: user.PasswordIterations,
	}
	if !auth.VerifyPassword(stored, password) {
		renderLogin(w, r, email, failed)
		return
	}

	auth.SetSessionCookie(w, secret, user.ID, user.SessionEpoch)
	http.Redirect(w, r, "/jobs", http.StatusFound)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// requireOwner gates the user-administration routes.
func requireOwner(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	ident, ok := identity(w, r)
	if !ok {
		return ident, false
	}
	if !ident.IsOwner {
		http.Error(w, "only the owner can manage users", http.StatusForbidden)
		return ident, false
	}
	return ident, true
}

func renderUsers(w http.ResponseWriter, r *http.Request, env *config.Env, ident auth.Identity, notice *CredentialNotice) {
	users, err := db.ListUsers(r.Context(), env.DB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templ.Handler(UsersPage(users, strings.ToLower(env.OwnerEmail), ident.Email, ident.IsOwner, notice)).ServeHTTP(w, r)
}

func usersPageHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := requireOwner(w, r)
	if !ok {
		return
	}
	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderUsers(w, r, env, ident, nil)
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := requireOwner(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" || !strings.Contains(email, "@") {
		http.Error(w, "a valid email is required", http.StatusBadRequest)
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := db.GetOrCreateUser(r.Context(), env.DB, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	notice, err := assignGeneratedPassword(r, env, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderUsers(w, r, env, ident, notice)
}

func resetUserPasswordHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := db.GetUserByID(r.Context(), env.DB, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.NotFound(w, r)
		return
	}

	notice, err := assignGeneratedPassword(r, env, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Resetting bumps session_epoch, so this user's own cookie is now stale.
	// Re-issue it rather than silently logging the owner out of their own
	// session mid-click.
	if user.ID == ident.UserID {
		if refreshed, err := db.GetUserByID(r.Context(), env.DB, user.ID); err == nil && refreshed != nil {
			auth.SetSessionCookie(w, auth.SessionSecret(), refreshed.ID, refreshed.SessionEpoch)
		}
	}

	renderUsers(w, r, env, ident, notice)
}

func assignGeneratedPassword(r *http.Request, env *config.Env, user *db.User) (*CredentialNotice, error) {
	password, err := auth.GeneratePassword(generatedPasswordLength)
	if err != nil {
		return nil, err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	if err := db.SetUserPassword(r.Context(), env.DB, user.ID, hashed.Hash, hashed.Salt, hashed.Iterations); err != nil {
		return nil, err
	}
	return &CredentialNotice{Email: user.Email, Password: password}, nil
}

func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(httprouter.PathValue(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := db.GetUserByID(r.Context(), env.DB, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Removing the owner would leave nobody able to administer users.
	if user != nil && strings.EqualFold(user.Email, env.OwnerEmail) {
		http.Error(w, "the owner account cannot be removed", http.StatusBadRequest)
		return
	}

	if err := db.DeleteUser(r.Context(), env.DB, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderUsers(w, r, env, ident, nil)
}

func accountPageHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}
	templ.Handler(AccountPage(ident.Email, ident.IsOwner, "", "")).ServeHTTP(w, r)
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	ident, ok := identity(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}
	current := r.FormValue("current_password")
	next := r.FormValue("new_password")

	render := func(message, errorMessage string) {
		templ.Handler(AccountPage(ident.Email, ident.IsOwner, message, errorMessage)).ServeHTTP(w, r)
	}

	if len(next) < minPasswordLength {
		render("", "New password must be at least 10 characters.")
		return
	}

	env, err := config.Load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := db.GetUserByID(r.Context(), env.DB, ident.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "account not found", http.StatusInternalServerError)
		return
	}

	stored := auth.PasswordHash{
		Hash:       user.PasswordHash,
		Salt:       user.PasswordSalt,
		Iterations: user.PasswordIterations,
	}
	if !user.HasPassword() || !auth.VerifyPassword(stored, current) {
		render("", "Current password is incorrect.")
		return
	}

	hashed, err := auth.HashPassword(next)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := db.SetUserPassword(r.Context(), env.DB, user.ID, hashed.Hash, hashed.Salt, hashed.Iterations); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The epoch bump just invalidated this request's own cookie; issue a
	// fresh one so changing your password doesn't sign you out.
	refreshed, err := db.GetUserByID(r.Context(), env.DB, user.ID)
	if err == nil && refreshed != nil {
		auth.SetSessionCookie(w, auth.SessionSecret(), refreshed.ID, refreshed.SessionEpoch)
	}

	render("Password changed. Other signed-in sessions have been signed out.", "")
}
