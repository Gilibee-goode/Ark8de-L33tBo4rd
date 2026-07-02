// api-gateway — single ingress for all microservices.
//
// Routing table (path prefix → upstream):
//   /auth/*            → auth-service
//   /api/players/*     → player-service
//   /api/skills        → player-service
//   /api/teams/*       → team-service
//   /api/leaderboard/* → leaderboard-service
//   everything else    → frontend-service (HTML pages, /static, login forms)
//
// The gateway rejects requests carrying an INVALID Bearer token up front
// (signature check), so garbage never reaches the services. Fine-grained
// role enforcement still happens inside each service (defense in depth).
package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/app"
)

// mustProxy builds a reverse proxy to the upstream base URL named by envName.
func mustProxy(envName string) *httputil.ReverseProxy {
	raw := app.MustEnv(envName)
	target, err := url.Parse(raw)
	if err != nil {
		slog.Error("invalid upstream URL", "env", envName, "url", raw, "error", err)
		os.Exit(1)
	}
	p := httputil.NewSingleHostReverseProxy(target)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("gateway: upstream unreachable", "upstream", target.Host, "path", r.URL.Path, "error", err)
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"upstream %s unavailable"}`, target.Host)
	}
	return p
}

// rejectInvalidJWT returns middleware that validates the signature of any
// Bearer token present on the request. Requests without a token pass through
// untouched — each service decides whether auth is required for a route.
func rejectInvalidJWT(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				tokenStr := strings.TrimPrefix(header, "Bearer ")
				_, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
					if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
					}
					return []byte(secret), nil
				})
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprint(w, `{"error":"invalid or expired token"}`)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	app.Bootstrap()
	jwtSecret := app.MustEnv("JWT_SECRET")

	authProxy := mustProxy("AUTH_SERVICE_URL")
	playerProxy := mustProxy("PLAYER_SERVICE_URL")
	teamProxy := mustProxy("TEAM_SERVICE_URL")
	leaderboardProxy := mustProxy("LEADERBOARD_SERVICE_URL")
	frontendProxy := mustProxy("FRONTEND_SERVICE_URL")

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(rejectInvalidJWT(jwtSecret))

	// The gateway's own liveness — it has no DB, so always ok.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	r.Mount("/auth", authProxy)
	r.Mount("/api/players", playerProxy)
	r.Mount("/api/skills", playerProxy)
	r.Mount("/api/teams", teamProxy)
	r.Mount("/api/leaderboard", leaderboardProxy)
	r.Mount("/", frontendProxy) // HTML pages, /static, /login, /profile, /mod

	app.RunServer(r, app.EnvOr("PORT", "8080"))
}
