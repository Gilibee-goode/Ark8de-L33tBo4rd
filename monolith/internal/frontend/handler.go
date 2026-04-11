// Package frontend — HTTP Handler layer for server-rendered HTML pages.
//
// This file is ONLY responsible for:
//   - Fetching data from the service layer
//   - Loading and executing HTML templates
//   - Writing HTML responses to the browser
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

	// --- Third-party ---
	"github.com/go-chi/chi/v5" // HTTP router — chi.URLParam extracts named URL segments like {id}

	// --- Internal packages ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/leaderboard" // read-only team ranking service
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware"   // PlayerIDFromContext — reads player ID from request context
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/player"       // player data: stats, skills, gear, kredits
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/team"         // team data: roster, gear pool, Arkade points
)

// FrontendHandler serves server-rendered HTML pages.
//
// It holds references to the three services it needs to fetch page data.
// Each handler method fetches data from the appropriate service and passes
// it to render(), which loads the template and writes the HTML response.
type FrontendHandler struct {
	leaderboardSvc *leaderboard.LeaderboardService
	teamSvc        *team.TeamService
	playerSvc      *player.PlayerService
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
	templatesDir string,
) *FrontendHandler {
	return &FrontendHandler{
		leaderboardSvc: leaderboardSvc,
		teamSvc:        teamSvc,
		playerSvc:      playerSvc,
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
			// Return 0 as a safe default — callers should check .Rank for nil
			// before calling deref, as we do in leaderboard.html.
			return 0
		}
		return *p // * is Go's dereference operator: reads the int at address p
	},
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
func (h *FrontendHandler) render(w http.ResponseWriter, page string, data any) {
	// filepath.Join builds a cross-platform file path from components.
	// On macOS/Linux: "templates/layout/base.html"
	// On Windows:     "templates\layout\base.html"
	basePath := filepath.Join(h.templatesDir, "layout", "base.html")
	pagePath := filepath.Join(h.templatesDir, page)

	// template.New("base.html") creates a template set whose root template is named "base.html".
	// .Funcs(templateFuncs) registers our custom functions — MUST come before ParseFiles,
	// because ParseFiles compiles the templates and they need function definitions available.
	// .ParseFiles(basePath, pagePath) loads both files. ParseFiles names each template by
	// its base filename (the last component of the path), so:
	//   "templates/layout/base.html" → template named "base.html"
	//   "templates/leaderboard.html" → template named "leaderboard.html"
	// All {{define "..."}} blocks in both files go into the same template set.
	t, err := template.New("base.html").Funcs(templateFuncs).ParseFiles(basePath, pagePath)
	if err != nil {
		slog.Error("frontend: failed to parse template", "page", page, "error", err)
		http.Error(w, "internal error: template parse failed", http.StatusInternalServerError)
		return
	}

	// Tell the browser we're sending HTML, not plain text or JSON.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Execute runs the "base.html" template (set as root in New above).
	// `data` becomes the dot (.) value inside templates — accessible as {{ .FieldName }}.
	if err := t.Execute(w, data); err != nil {
		// By the time Execute() errors, partial HTML may already be written to w.
		// We cannot change the HTTP status code at this point (headers already sent).
		// Just log for debugging.
		slog.Error("frontend: failed to execute template", "page", page, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Page data types
// ---------------------------------------------------------------------------

// leaderboardPageData is the data passed to leaderboard.html.
// Using a named struct (instead of map[string]any) gives compile-time safety:
// rename a field and the Go compiler catches every broken template reference.
type leaderboardPageData struct {
	Entries []leaderboard.LeaderboardEntry
}

// profilePageData bundles all data shown on the player profile page.
// The profile has multiple sections (stats, skills, gear, kredits), so we
// collect them into one struct to pass as the single template dot (.) value.
type profilePageData struct {
	// Profile holds public identity: username, class, profile photo URL.
	Profile *player.PublicPlayerResponse
	// Stats holds computed combat numbers: HP, armor, skill point budget.
	Stats *player.StatsResponse
	// Skills lists allocated skills and remaining skill point budget.
	Skills *player.SkillsResponse
	// Gear lists selected gear and team gear pool status.
	Gear *player.GearResponse
	// Kredits holds in-game currency balance and recent transactions.
	Kredits *player.KreditsResponse
}

// moderatorPageData bundles data for the moderator action panel.
type moderatorPageData struct {
	// Teams is all teams — shown in a dropdown so the moderator can pick one.
	Teams []*team.Team
}

// ---------------------------------------------------------------------------
// Page handlers
// ---------------------------------------------------------------------------

// Leaderboard handles GET / — renders the public leaderboard page.
func (h *FrontendHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.leaderboardSvc.GetLeaderboard(r.Context())
	if err != nil {
		slog.Error("frontend.Leaderboard", "error", err)
		http.Error(w, "failed to load leaderboard", http.StatusInternalServerError)
		return
	}
	h.render(w, "leaderboard.html", leaderboardPageData{Entries: entries})
}

// TeamDetail handles GET /teams/{id} — renders the public team detail page.
func (h *FrontendHandler) TeamDetail(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam reads a named URL segment defined in the route pattern.
	// For a route registered as "/teams/{id}", URLParam(r, "id") returns the actual UUID.
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

	// TeamDetailResponse already includes the roster and gear pool status.
	h.render(w, "team-detail.html", detail)
}

// Profile handles GET /profile — renders the authenticated player's own profile.
// The router applies Authenticate middleware before this handler runs,
// so we can safely read the player ID from context.
func (h *FrontendHandler) Profile(w http.ResponseWriter, r *http.Request) {
	// PlayerIDFromContext reads the player's UUID that Authenticate stored in the
	// request context after verifying the JWT. No JWT work needed here.
	playerID := middleware.PlayerIDFromContext(r.Context())

	// Fetch each section of the profile data sequentially.
	// A future optimisation would use goroutines + channels to fetch in parallel,
	// but sequential is far easier to read and debug.
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

	h.render(w, "profile.html", profilePageData{
		Profile: profile,
		Stats:   stats,
		Skills:  skills,
		Gear:    gear,
		Kredits: kredits,
	})
}

// ModeratorPanel handles GET /mod — renders the moderator action panel.
// The router applies Authenticate + RequireRole("moderator","admin") before this handler.
func (h *FrontendHandler) ModeratorPanel(w http.ResponseWriter, r *http.Request) {
	teams, err := h.teamSvc.ListTeams(r.Context())
	if err != nil {
		slog.Error("frontend.ModeratorPanel: ListTeams", "error", err)
		http.Error(w, "failed to load teams", http.StatusInternalServerError)
		return
	}
	h.render(w, "moderator.html", moderatorPageData{Teams: teams})
}
