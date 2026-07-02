// Package frontend — HTTP Handler layer for server-rendered HTML pages.
//
// This file is ONLY responsible for:
//   - Fetching data from the service layer
//   - Loading and executing HTML templates
//   - Writing HTML responses to the browser
//   - Processing form submissions (login, register, logout, moderator actions)
//
// Unlike the JSON API handlers (which return JSON), this handler returns HTML
// that the browser renders directly — no JavaScript framework needed.
//
// This pattern is called "server-side rendering" (SSR). The server does all
// the data fetching and template filling; the browser just displays the result.
package frontend

import (
	// --- Standard library ---
	"errors"        // errors.Is() — matches sentinel errors returned by services
	"html/template" // Go's built-in HTML template engine — auto-escapes values to prevent XSS
	"log/slog"      // structured logging (Go 1.21+) — key=value log lines easy to search in prod
	"net/http"      // http.ResponseWriter, http.Request — the two types every HTTP handler takes
	"path/filepath" // filepath.Join — builds OS-safe file paths from path components
	"strconv"       // strconv.Atoi — converts form string values to integers

	// --- Third-party ---
	"github.com/go-chi/chi/v5" // HTTP router — chi.URLParam extracts named URL segments like {id}

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/auth"        // auth service for login/register
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/leaderboard" // read-only team ranking service
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware"   // PlayerIDFromContext, UsernameFromContext, CSRF, flash
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/player"      // player data: stats, skills, gear, kredits
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/session"     // session service for creating/destroying sessions
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/team"        // team data: roster, gear pool, Arkade points
)

// FrontendHandler serves server-rendered HTML pages.
//
// It holds references to the services it needs to fetch page data and process
// form submissions. Each handler method fetches data from the appropriate service
// and passes it to render(), which loads the template and writes the HTML response.
type FrontendHandler struct {
	leaderboardSvc *leaderboard.LeaderboardService
	teamSvc        *team.TeamService
	playerSvc      *player.PlayerService
	authSvc        *auth.AuthService
	sessionSvc     *session.SessionService
	// templatesDir is the path to the templates/ folder relative to the server's
	// working directory. We store it here so render() can build file paths at request time.
	templatesDir string
}

// NewFrontendHandler constructs a FrontendHandler.
// Pass "templates" for templatesDir when running from the monolith/ directory.
func NewFrontendHandler(
	leaderboardSvc *leaderboard.LeaderboardService,
	teamSvc *team.TeamService,
	playerSvc *player.PlayerService,
	authSvc *auth.AuthService,
	sessionSvc *session.SessionService,
	templatesDir string,
) *FrontendHandler {
	return &FrontendHandler{
		leaderboardSvc: leaderboardSvc,
		teamSvc:        teamSvc,
		playerSvc:      playerSvc,
		authSvc:        authSvc,
		sessionSvc:     sessionSvc,
		templatesDir:   templatesDir,
	}
}

// templateFuncs is a registry of custom functions available inside every Go template.
//
// Go's html/template package only allows functions registered in a FuncMap to be
// called from within templates. This is intentional: it prevents template authors
// from calling arbitrary, potentially dangerous Go code.
//
// template.FuncMap is a type alias for map[string]any.
// Keys become the function name used in templates: {{ deref .Rank }}
var templateFuncs = template.FuncMap{
	// "deref" takes a *int pointer and returns the int value it points to.
	//
	// WHY WE NEED THIS:
	// LeaderboardEntry.Rank is *int (a pointer to int), not a plain int.
	// The pointer can be nil, which means "this team has not been ranked yet".
	// Go templates cannot perform equality checks on pointers directly
	// (e.g. `eq .Rank 1` would fail with a type mismatch). Dereferencing first
	// gives us a plain int we can compare with `eq`.
	//
	// HOW DEREFERENCING WORKS:
	// In Go, a pointer (*int) stores a memory address where an int lives.
	// The `*` operator reads the value at that address — this is "dereferencing".
	"deref": func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	},
}

// ---------------------------------------------------------------------------
// PageContext — shared data available in every page template
// ---------------------------------------------------------------------------

// PageContext holds data that every page template needs — injected automatically
// by render() so the base layout can show/hide navbar elements based on login state.
//
// Every page-specific data struct embeds PageContext so the base template can
// always access .LoggedIn, .Username, .Role, .CSRFToken, and .Flash.
type PageContext struct {
	// LoggedIn is true when the request has a valid session or JWT.
	LoggedIn bool
	// Username is the authenticated player's username (empty if not logged in).
	Username string
	// Role is the authenticated player's role: "player", "team_owner", "moderator", "admin".
	Role string
	// CSRFToken is the double-submit cookie token — embedded in forms as a hidden field.
	CSRFToken string
	// Flash is a one-time notification from the previous request (e.g. "Account created!").
	// Nil if no flash message exists.
	Flash *middleware.FlashMessage
}

// contextSetter is an interface for page data structs that can receive a PageContext.
// Every page data struct implements this via its embedded PageContext field.
//
// WHY AN INTERFACE?
//   The render() method needs to inject PageContext into any page data struct,
//   but it receives data as `any` (Go's empty interface). We can't set fields
//   on `any` directly. This interface lets render() call SetPageContext on
//   any struct that supports it, without knowing the concrete type.
type contextSetter interface {
	SetPageContext(PageContext)
}

// render parses the layout template and a page-specific template, then writes
// the combined HTML response.
//
// HOW GO TEMPLATES WORK IN THIS PROJECT:
//
//	templates/layout/base.html  — the HTML skeleton (head, nav, footer)
//	  uses {{block "title" .}} and {{block "content" .}} as placeholder slots
//
//	templates/leaderboard.html  — the page-specific content
//	  uses {{define "title"}} and {{define "content"}} to fill those slots
//
// ParseFiles loads both files into one template set. The {{define}} blocks
// in the page file automatically override the {{block}} placeholders in base.html.
// Execute("base.html") runs the layout, which calls the overridden blocks,
// producing the complete HTML page.
//
// We re-parse templates on every request (rather than caching at startup).
// This is slightly slower but lets you edit a template and see the change
// immediately on refresh — without restarting the server.
func (h *FrontendHandler) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	// Build the PageContext from the request context.
	// OptionalAuthenticateWithSessions middleware puts these values in context
	// when a valid session or JWT is found.
	pc := PageContext{
		LoggedIn:  middleware.PlayerIDFromContext(r.Context()) != "",
		Username:  middleware.UsernameFromContext(r.Context()),
		Role:      middleware.RoleFromContext(r.Context()),
		CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		Flash:     middleware.FlashFromRequest(r),
	}

	// If there was a flash message, clear the cookie so it only shows once
	// (the "read-once" pattern).
	if pc.Flash != nil {
		middleware.ClearFlash(w)
	}

	// Inject the PageContext into the page data struct if it supports it.
	// All our page data structs embed PageContext and implement contextSetter.
	if setter, ok := data.(contextSetter); ok {
		setter.SetPageContext(pc)
	}

	basePath := filepath.Join(h.templatesDir, "layout", "base.html")
	pagePath := filepath.Join(h.templatesDir, page)

	t, err := template.New("base.html").Funcs(templateFuncs).ParseFiles(basePath, pagePath)
	if err != nil {
		slog.Error("frontend: failed to parse template", "page", page, "error", err)
		http.Error(w, "internal error: template parse failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := t.Execute(w, data); err != nil {
		slog.Error("frontend: failed to execute template", "page", page, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Page data types
// ---------------------------------------------------------------------------

// leaderboardPageData is the data passed to leaderboard.html.
type leaderboardPageData struct {
	PageContext
	Entries []leaderboard.LeaderboardEntry
}

// SetPageContext implements contextSetter — called by render() to inject shared data.
func (d *leaderboardPageData) SetPageContext(pc PageContext) { d.PageContext = pc }

// teamDetailPageData is the data passed to team-detail.html.
type teamDetailPageData struct {
	PageContext
	Detail *team.TeamDetailResponse
}

func (d *teamDetailPageData) SetPageContext(pc PageContext) { d.PageContext = pc }

// profilePageData bundles all data shown on the player profile page.
type profilePageData struct {
	PageContext
	Profile *player.PublicPlayerResponse
	Stats   *player.StatsResponse
	Skills  *player.SkillsResponse
	Gear    *player.GearResponse
	Kredits *player.KreditsResponse
}

func (d *profilePageData) SetPageContext(pc PageContext) { d.PageContext = pc }

// moderatorPageData bundles data for the moderator action panel.
type moderatorPageData struct {
	PageContext
	Teams []*team.Team
}

func (d *moderatorPageData) SetPageContext(pc PageContext) { d.PageContext = pc }

// authPageData is the data passed to login.html and register.html.
// It only contains the shared PageContext (login/register forms have no extra data).
type authPageData struct {
	PageContext
}

func (d *authPageData) SetPageContext(pc PageContext) { d.PageContext = pc }

// ---------------------------------------------------------------------------
// Page handlers — public pages
// ---------------------------------------------------------------------------

// Leaderboard handles GET / — renders the public leaderboard page.
func (h *FrontendHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.leaderboardSvc.GetLeaderboard(r.Context())
	if err != nil {
		slog.Error("frontend.Leaderboard", "error", err)
		http.Error(w, "failed to load leaderboard", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "leaderboard.html", &leaderboardPageData{Entries: entries})
}

// TeamDetail handles GET /teams/{id} — renders the public team detail page.
func (h *FrontendHandler) TeamDetail(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")

	detail, err := h.teamSvc.GetTeamDetail(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, team.ErrTeamNotFound) {
			http.Error(w, "team not found", http.StatusNotFound)
			return
		}
		slog.Error("frontend.TeamDetail", "error", err)
		http.Error(w, "failed to load team", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "team-detail.html", &teamDetailPageData{Detail: detail})
}

// ---------------------------------------------------------------------------
// Page handlers — authenticated pages
// ---------------------------------------------------------------------------

// Profile handles GET /profile — renders the authenticated player's own profile.
func (h *FrontendHandler) Profile(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	profile, err := h.playerSvc.GetPublicProfile(r.Context(), playerID)
	if err != nil {
		slog.Error("frontend.Profile: GetPublicProfile", "error", err)
		http.Error(w, "failed to load profile", http.StatusInternalServerError)
		return
	}

	stats, err := h.playerSvc.GetStats(r.Context(), playerID)
	if err != nil {
		slog.Error("frontend.Profile: GetStats", "error", err)
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	skills, err := h.playerSvc.GetSkills(r.Context(), playerID)
	if err != nil {
		slog.Error("frontend.Profile: GetSkills", "error", err)
		http.Error(w, "failed to load skills", http.StatusInternalServerError)
		return
	}

	gear, err := h.playerSvc.GetGear(r.Context(), playerID)
	if err != nil {
		slog.Error("frontend.Profile: GetGear", "error", err)
		http.Error(w, "failed to load gear", http.StatusInternalServerError)
		return
	}

	kredits, err := h.playerSvc.GetKredits(r.Context(), playerID)
	if err != nil {
		slog.Error("frontend.Profile: GetKredits", "error", err)
		http.Error(w, "failed to load kredits", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "profile.html", &profilePageData{
		Profile: profile,
		Stats:   stats,
		Skills:  skills,
		Gear:    gear,
		Kredits: kredits,
	})
}

// ModeratorPanel handles GET /mod — renders the moderator action panel.
func (h *FrontendHandler) ModeratorPanel(w http.ResponseWriter, r *http.Request) {
	teams, err := h.teamSvc.ListTeams(r.Context())
	if err != nil {
		slog.Error("frontend.ModeratorPanel: ListTeams", "error", err)
		http.Error(w, "failed to load teams", http.StatusInternalServerError)
		return
	}
	h.render(w, r, "moderator.html", &moderatorPageData{Teams: teams})
}

// ---------------------------------------------------------------------------
// Auth handlers — login, register, logout
// ---------------------------------------------------------------------------

// LoginPage handles GET /login — renders the login form.
func (h *FrontendHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If the player is already logged in, redirect to the profile page.
	if middleware.PlayerIDFromContext(r.Context()) != "" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	h.render(w, r, "login.html", &authPageData{})
}

// LoginSubmit handles POST /login — validates credentials, creates a session, redirects.
//
// Flow:
//  1. Parse email and password from the form body
//  2. Call AuthService.Login to validate credentials
//  3. On success: create a session, set the cookie, redirect to /profile
//  4. On failure: set a flash error message, redirect back to GET /login
func (h *FrontendHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	playerResp, _, err := h.authSvc.Login(r.Context(), auth.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		// Don't reveal whether it was the email or password that was wrong.
		middleware.SetFlash(w, "error", "Invalid email or password")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Create a server-side session for this login.
	sess, err := h.sessionSvc.CreateSession(r.Context(), playerResp.ID, playerResp.Role, playerResp.Username)
	if err != nil {
		slog.Error("frontend.LoginSubmit: CreateSession", "error", err)
		middleware.SetFlash(w, "error", "Login failed — please try again")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Set the session cookie. The browser will send this automatically on every request.
	//
	// HttpOnly: true  — JavaScript cannot read this cookie (prevents XSS token theft)
	// SameSite: Lax   — cookie sent on same-site requests and top-level navigations
	//                    (not on cross-site POST, which helps prevent CSRF)
	// Path: "/"       — cookie valid for all routes, not just /login
	// MaxAge: 604800  — 7 days in seconds (matches session.SessionDuration)
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName,
		Value:    sess.ID,
		Path:     "/",
		MaxAge:   int(session.SessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	middleware.SetFlash(w, "success", "Welcome back, "+playerResp.Username+"!")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// RegisterPage handles GET /register — renders the registration form.
func (h *FrontendHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	if middleware.PlayerIDFromContext(r.Context()) != "" {
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
		return
	}
	h.render(w, r, "register.html", &authPageData{})
}

// RegisterSubmit handles POST /register — creates an account, session, and redirects.
//
// Flow:
//  1. Parse form fields (username, email, password, confirm_password)
//  2. Validate confirm_password matches password
//  3. Call AuthService.Register to create the account
//  4. On success: create a session, set the cookie, redirect to /profile
//  5. On failure: set a flash error message, redirect back to GET /register
func (h *FrontendHandler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate password confirmation on the server side.
	// Client-side validation is a UX nicety but is trivially bypassed —
	// the server must always validate.
	if password != confirmPassword {
		middleware.SetFlash(w, "error", "Passwords do not match")
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	playerResp, _, err := h.authSvc.Register(r.Context(), auth.RegisterRequest{
		Username: username,
		Email:    email,
		Password: password,
	})
	if err != nil {
		// Show the specific validation error — Register returns user-friendly messages.
		middleware.SetFlash(w, "error", err.Error())
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	sess, err := h.sessionSvc.CreateSession(r.Context(), playerResp.ID, playerResp.Role, playerResp.Username)
	if err != nil {
		slog.Error("frontend.RegisterSubmit: CreateSession", "error", err)
		middleware.SetFlash(w, "error", "Account created but login failed — please log in manually")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName,
		Value:    sess.ID,
		Path:     "/",
		MaxAge:   int(session.SessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	middleware.SetFlash(w, "success", "Account created! Welcome, "+playerResp.Username+"!")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// Logout handles POST /logout — destroys the session and redirects to home.
func (h *FrontendHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Read the session cookie to find which session to destroy.
	cookie, err := r.Cookie(session.CookieName)
	if err == nil && cookie.Value != "" {
		// Destroy the session in the database. Ignore errors — the cookie
		// will be cleared regardless, so the player is logged out either way.
		_ = h.sessionSvc.DestroySession(r.Context(), cookie.Value)
	}

	// Clear the session cookie by setting MaxAge=-1.
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Also clear the CSRF cookie so a fresh one is generated on next visit.
	http.SetCookie(w, &http.Cookie{
		Name:   "csrf_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	middleware.SetFlash(w, "success", "You have been logged out")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Moderator form handlers
// ---------------------------------------------------------------------------

// AwardPoints handles POST /mod/award-points — awards Arkade points to a team.
// This replaces the JavaScript-based form submission from the original moderator panel.
func (h *FrontendHandler) AwardPoints(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	teamID := r.FormValue("team_id")
	deltaStr := r.FormValue("delta")
	reason := r.FormValue("reason")

	delta, err := strconv.Atoi(deltaStr)
	if err != nil || teamID == "" {
		middleware.SetFlash(w, "error", "Please select a team and enter a numeric delta")
		http.Redirect(w, r, "/mod", http.StatusSeeOther)
		return
	}

	err = h.teamSvc.AddArkadePoints(r.Context(), playerID, teamID, team.AddArkadePointsRequest{
		Delta:  delta,
		Reason: reason,
	})
	if err != nil {
		middleware.SetFlash(w, "error", "Failed to award points: "+err.Error())
		http.Redirect(w, r, "/mod", http.StatusSeeOther)
		return
	}

	middleware.SetFlash(w, "success", "Arkade points updated!")
	http.Redirect(w, r, "/mod", http.StatusSeeOther)
}

// GrantKredits handles POST /mod/grant-kredits — grants Kredits to a player.
// This replaces the JavaScript-based form submission from the original moderator panel.
func (h *FrontendHandler) GrantKredits(w http.ResponseWriter, r *http.Request) {
	playerID := r.FormValue("player_id")
	amountStr := r.FormValue("amount")
	note := r.FormValue("note")

	callerID := middleware.PlayerIDFromContext(r.Context())

	amount, err := strconv.Atoi(amountStr)
	if err != nil || playerID == "" {
		middleware.SetFlash(w, "error", "Please enter a player ID and numeric amount")
		http.Redirect(w, r, "/mod", http.StatusSeeOther)
		return
	}

	err = h.playerSvc.GrantKredits(r.Context(), playerID, callerID, player.GrantKreditsRequest{
		Amount: amount,
		Note:   note,
	})
	if err != nil {
		middleware.SetFlash(w, "error", "Failed to grant Kredits: "+err.Error())
		http.Redirect(w, r, "/mod", http.StatusSeeOther)
		return
	}

	middleware.SetFlash(w, "success", "Kredits granted!")
	http.Redirect(w, r, "/mod", http.StatusSeeOther)
}
