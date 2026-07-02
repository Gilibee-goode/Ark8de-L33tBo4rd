// Package tests — Session & GUI login integration tests
//
// This file tests the browser-based authentication system:
//   - GET /login, GET /register — form pages render
//   - POST /login — form-based login with session cookie
//   - POST /register — form-based registration with session cookie
//   - POST /logout — session destruction and cookie clearing
//   - CSRF protection — POST without valid token returns 403
//   - Cookie-based auth — accessing protected pages with session cookie
//   - Navbar state — logged-in vs logged-out HTML content
//
// WHY SEPARATE FROM frontend_test.go?
//   frontend_test.go tests public pages (leaderboard, team detail) that don't
//   require authentication. This file tests the full login flow which needs a
//   cookie jar (to track session and CSRF cookies across requests) and form
//   submissions (application/x-www-form-urlencoded instead of JSON).
//
// HOW BROWSER-LIKE TESTING WORKS:
//   Real browsers automatically handle cookies — they store cookies from
//   Set-Cookie headers and resend them on subsequent requests. In Go tests,
//   we create an http.Client with a cookiejar.Jar to simulate this behaviour.
//   This lets us test the full cookie flow: get CSRF token → submit form →
//   receive session cookie → use session cookie on next request.
package tests

import (
	// --- Standard library ---
	"net/http"          // HTTP client and status codes
	"net/http/cookiejar" // provides a cookie jar that stores cookies between requests — simulates browser cookie behaviour
	"net/url"           // url.Values encodes form data as key=value pairs (like HTML form submissions)
	"strings"           // strings.Contains for checking HTML content
	"testing"           // Go's built-in test framework
)

// ---------------------------------------------------------------------------
// Browser client helper
// ---------------------------------------------------------------------------

// browserClient creates an HTTP client that behaves like a browser:
//   - Stores and resends cookies automatically (via a cookiejar.Jar)
//   - Does NOT follow redirects (returns the 303 response directly)
//
// WHY NOT FOLLOW REDIRECTS?
//   After POST /login, the server responds with 303 See Other + Location: /profile.
//   A normal browser (or Go's default http.Client) would follow the redirect
//   automatically. But in tests, we want to inspect the redirect response itself:
//   check the Location header, verify cookies were set, and read the status code.
//   Setting CheckRedirect to return http.ErrUseLastResponse stops the client
//   from following the redirect and returns the 303 response directly.
func browserClient(t *testing.T) *http.Client {
	t.Helper()

	// cookiejar.New creates a new cookie jar (an in-memory cookie store).
	// The nil parameter means we use default options (no public suffix list).
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}

	return &http.Client{
		Jar: jar,
		// CheckRedirect is called before the client follows a redirect.
		// Returning http.ErrUseLastResponse tells the client to stop and
		// return the redirect response (3xx) instead of following it.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// getCSRFToken makes a GET request to the given path and returns the CSRF
// token from the csrf_token cookie. The CSRF middleware sets this cookie on
// every GET request, so any page works.
//
// This simulates what a browser does: visit the page (GET), read the CSRF
// cookie, then include it as a hidden form field when submitting (POST).
func getCSRFToken(t *testing.T, client *http.Client, path string) string {
	t.Helper()

	resp, err := client.Get(testServer.URL + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	resp.Body.Close()

	// Find the csrf_token cookie in the jar.
	// url.Parse is needed because the cookie jar stores cookies by URL.
	serverURL, _ := url.Parse(testServer.URL)
	for _, cookie := range client.Jar.Cookies(serverURL) {
		if cookie.Name == "csrf_token" {
			return cookie.Value
		}
	}

	t.Fatal("csrf_token cookie not found after GET " + path)
	return ""
}

// postForm submits an HTML form using the same encoding a browser uses.
// It adds the CSRF token as a hidden form field (matching the csrf_token cookie
// already in the client's cookie jar).
//
// Parameters:
//   - client: the browser-like HTTP client (with cookie jar)
//   - path: the URL path to POST to (e.g. "/login")
//   - csrfToken: the CSRF token from getCSRFToken
//   - formData: key-value pairs for the form fields
func postForm(t *testing.T, client *http.Client, path, csrfToken string, formData url.Values) *http.Response {
	t.Helper()

	// Add the CSRF token to the form data — this is what the hidden <input> does in HTML.
	formData.Set("csrf_token", csrfToken)

	// url.Values.Encode() produces the same format as a browser form submission:
	// "email=test%40example.com&password=secret123&csrf_token=abc..."
	resp, err := client.PostForm(testServer.URL+path, formData)
	if err != nil {
		t.Fatalf("POST %s failed: %v", path, err)
	}

	return resp
}

// hasSessionCookie checks if the client's cookie jar contains a session_id cookie.
func hasSessionCookie(t *testing.T, client *http.Client) bool {
	t.Helper()
	serverURL, _ := url.Parse(testServer.URL)
	for _, cookie := range client.Jar.Cookies(serverURL) {
		if cookie.Name == "session_id" && cookie.Value != "" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests: Login/Register page rendering
// ---------------------------------------------------------------------------

func TestLoginPage_Renders(t *testing.T) {
	client := browserClient(t)
	resp, err := client.Get(testServer.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	assertContains(t, body, "Log In")
	assertContains(t, body, "csrf_token") // hidden CSRF field should be in the form
}

func TestRegisterPage_Renders(t *testing.T) {
	client := browserClient(t)
	resp, err := client.Get(testServer.URL + "/register")
	if err != nil {
		t.Fatalf("GET /register failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	assertContains(t, body, "Create Account")
	assertContains(t, body, "csrf_token")
}

// ---------------------------------------------------------------------------
// Tests: Registration flow
// ---------------------------------------------------------------------------

func TestRegisterSubmit_Success(t *testing.T) {
	// Clean slate for this test.
	truncateDataTables(t.Context())

	client := browserClient(t)

	// Step 1: Visit the register page to get a CSRF token (like a real browser).
	csrfToken := getCSRFToken(t, client, "/register")

	// Step 2: Submit the registration form.
	resp := postForm(t, client, "/register", csrfToken, url.Values{
		"username":         {"session_tester"},
		"email":            {"session_tester@test.dev"},
		"password":         {"ValidPass1!"},
		"confirm_password": {"ValidPass1!"},
	})
	resp.Body.Close()

	// Step 3: Verify the response is a redirect to /profile.
	// HTTP 303 (See Other) is the correct redirect after a POST — it tells the
	// browser to follow the redirect with a GET, preventing form resubmission.
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if location != "/profile" {
		t.Fatalf("expected redirect to /profile, got %q", location)
	}

	// Step 4: Verify a session cookie was set.
	if !hasSessionCookie(t, client) {
		t.Fatal("expected session_id cookie to be set after registration")
	}
}

func TestRegisterSubmit_PasswordMismatch(t *testing.T) {
	client := browserClient(t)
	csrfToken := getCSRFToken(t, client, "/register")

	resp := postForm(t, client, "/register", csrfToken, url.Values{
		"username":         {"mismatch_user"},
		"email":            {"mismatch@test.dev"},
		"password":         {"ValidPass1!"},
		"confirm_password": {"DifferentPass2!"},
	})
	resp.Body.Close()

	// Should redirect back to /register with a flash error.
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if location != "/register" {
		t.Fatalf("expected redirect to /register, got %q", location)
	}

	// No session cookie should be set.
	if hasSessionCookie(t, client) {
		t.Fatal("session cookie should NOT be set when passwords don't match")
	}
}

// ---------------------------------------------------------------------------
// Tests: Login flow
// ---------------------------------------------------------------------------

func TestLoginSubmit_Success(t *testing.T) {
	truncateDataTables(t.Context())

	// First, register a player via the JSON API so we have credentials to test login.
	registerPlayer(t, "login_tester", "login_tester@test.dev", "LoginPass1!")

	client := browserClient(t)
	csrfToken := getCSRFToken(t, client, "/login")

	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"login_tester@test.dev"},
		"password": {"LoginPass1!"},
	})
	resp.Body.Close()

	// Should redirect to /profile with a session cookie.
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/profile" {
		t.Fatalf("expected redirect to /profile, got %q", resp.Header.Get("Location"))
	}
	if !hasSessionCookie(t, client) {
		t.Fatal("expected session_id cookie after login")
	}
}

func TestLoginSubmit_BadPassword(t *testing.T) {
	truncateDataTables(t.Context())
	registerPlayer(t, "bad_pw_tester", "bad_pw@test.dev", "CorrectPass1!")

	client := browserClient(t)
	csrfToken := getCSRFToken(t, client, "/login")

	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"bad_pw@test.dev"},
		"password": {"WrongPassword1!"},
	})
	resp.Body.Close()

	// Should redirect back to /login (not /profile).
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %q", resp.Header.Get("Location"))
	}
	if hasSessionCookie(t, client) {
		t.Fatal("session cookie should NOT be set with wrong password")
	}
}

func TestLoginSubmit_NonexistentEmail(t *testing.T) {
	client := browserClient(t)
	csrfToken := getCSRFToken(t, client, "/login")

	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"nobody@nowhere.dev"},
		"password": {"DoesntMatter1!"},
	})
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %q", resp.Header.Get("Location"))
	}
}

// ---------------------------------------------------------------------------
// Tests: Logout flow
// ---------------------------------------------------------------------------

func TestLogout_ClearsSession(t *testing.T) {
	truncateDataTables(t.Context())
	registerPlayer(t, "logout_tester", "logout_tester@test.dev", "LogoutPass1!")

	client := browserClient(t)

	// Log in first.
	csrfToken := getCSRFToken(t, client, "/login")
	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"logout_tester@test.dev"},
		"password": {"LogoutPass1!"},
	})
	resp.Body.Close()

	if !hasSessionCookie(t, client) {
		t.Fatal("expected session cookie after login")
	}

	// Now log out.
	// Need to get a fresh CSRF token for the POST /logout request.
	csrfToken = getCSRFToken(t, client, "/")
	resp = postForm(t, client, "/logout", csrfToken, url.Values{})
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/" {
		t.Fatalf("expected redirect to /, got %q", resp.Header.Get("Location"))
	}

	// Session cookie should be cleared (MaxAge=-1 tells the jar to remove it).
	if hasSessionCookie(t, client) {
		t.Fatal("session cookie should be cleared after logout")
	}
}

// ---------------------------------------------------------------------------
// Tests: CSRF protection
// ---------------------------------------------------------------------------

func TestCSRF_MissingToken_Returns403(t *testing.T) {
	// Submit a POST /login WITHOUT a CSRF token.
	// The CSRF middleware should reject it with 403.
	client := browserClient(t)

	// POST directly without getting a CSRF token first.
	resp, err := client.PostForm(testServer.URL+"/login", url.Values{
		"email":    {"anyone@test.dev"},
		"password": {"whatever"},
	})
	if err != nil {
		t.Fatalf("POST /login failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden without CSRF token, got %d", resp.StatusCode)
	}
}

func TestCSRF_WrongToken_Returns403(t *testing.T) {
	client := browserClient(t)

	// Get a valid CSRF cookie by visiting the page.
	_ = getCSRFToken(t, client, "/login")

	// Submit with a different (wrong) CSRF token in the form field.
	resp, err := client.PostForm(testServer.URL+"/login", url.Values{
		"email":      {"anyone@test.dev"},
		"password":   {"whatever"},
		"csrf_token": {"this-is-a-wrong-token"},
	})
	if err != nil {
		t.Fatalf("POST /login failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden with wrong CSRF token, got %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Tests: Cookie-based auth for protected pages
// ---------------------------------------------------------------------------

func TestProfile_RequiresAuth(t *testing.T) {
	// GET /profile without any authentication should return 401.
	client := browserClient(t)

	resp, err := client.Get(testServer.URL + "/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	resp.Body.Close()

	// AuthenticateWithSessions returns a JSON 401 when no session or token is found.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /profile, got %d", resp.StatusCode)
	}
}

func TestProfile_WorksWithSessionCookie(t *testing.T) {
	truncateDataTables(t.Context())
	registerPlayer(t, "profile_tester", "profile_tester@test.dev", "ProfilePass1!")

	// Create a browser client, log in via the form, then visit /profile.
	client := browserClient(t)

	// Log in.
	csrfToken := getCSRFToken(t, client, "/login")
	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"profile_tester@test.dev"},
		"password": {"ProfilePass1!"},
	})
	resp.Body.Close()

	// Now visit /profile — the session cookie should authenticate us.
	// Note: our client doesn't follow redirects, but /profile is not a redirect
	// (it renders HTML directly for authenticated users).
	resp, err := client.Get(testServer.URL + "/profile")
	if err != nil {
		t.Fatalf("GET /profile failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authenticated /profile, got %d\nBody: %s", resp.StatusCode, body)
	}
	// The profile page should contain the player's username.
	assertContains(t, body, "profile_tester")
}

// ---------------------------------------------------------------------------
// Tests: Navbar state (logged-in vs logged-out)
// ---------------------------------------------------------------------------

func TestNavbar_LoggedOut_ShowsLoginLink(t *testing.T) {
	// Visit the leaderboard (public page) without authentication.
	client := browserClient(t)
	resp, err := client.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// The navbar should show login and register links when not logged in.
	assertContains(t, body, "/login")
	assertContains(t, body, "/register")
}

func TestNavbar_LoggedIn_ShowsUsername(t *testing.T) {
	truncateDataTables(t.Context())
	registerPlayer(t, "navbar_tester", "navbar_tester@test.dev", "NavbarPass1!")

	client := browserClient(t)

	// Log in via the form.
	csrfToken := getCSRFToken(t, client, "/login")
	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"navbar_tester@test.dev"},
		"password": {"NavbarPass1!"},
	})
	resp.Body.Close()

	// Visit the leaderboard (public page) — the navbar should now show the username.
	resp, err := client.Get(testServer.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// The navbar should show the logged-in user's name and a logout option.
	assertContains(t, body, "navbar_tester")
	assertContains(t, body, "/logout")

	// Login/register links should NOT appear when logged in.
	if strings.Contains(body, `href="/login"`) {
		t.Error("navbar should not show login link when user is logged in")
	}
}

// ---------------------------------------------------------------------------
// Tests: Login redirects when already authenticated
// ---------------------------------------------------------------------------

func TestLoginPage_RedirectsWhenLoggedIn(t *testing.T) {
	truncateDataTables(t.Context())
	registerPlayer(t, "already_logged", "already_logged@test.dev", "AlreadyPass1!")

	client := browserClient(t)

	// Log in.
	csrfToken := getCSRFToken(t, client, "/login")
	resp := postForm(t, client, "/login", csrfToken, url.Values{
		"email":    {"already_logged@test.dev"},
		"password": {"AlreadyPass1!"},
	})
	resp.Body.Close()

	// Now visit /login again — should redirect to /profile since we're already logged in.
	resp, err := client.Get(testServer.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect when already logged in, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Location") != "/profile" {
		t.Fatalf("expected redirect to /profile, got %q", resp.Header.Get("Location"))
	}
}
