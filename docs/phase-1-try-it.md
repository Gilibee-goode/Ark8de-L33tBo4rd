# Phase 1 — Try It Yourself

This guide walks you through everything that was built in Phase 1 and shows you
exactly how to interact with it — in the browser and via the API.

---

## Before You Start

Make sure the stack is running and the database is seeded:

```bash
task dev          # starts postgres + app in Docker (first time: builds the image)
task migrate-up   # applies all SQL migrations
task seed         # loads test players, teams, skills, gear, Arkade points
```

Then open `http://localhost:8080` in your browser.

---

## What You Can See in the Browser

### 1. The Leaderboard — `http://localhost:8080/`

The homepage. No login required.

- Two teams are ranked by Arkade points: **Team Alpha** and **Team Beta**
- Rank medals: 🥇 🥈 for 1st and 2nd place
- Click **View** on either team to go to the team detail page

### 2. Team Detail Page — `http://localhost:8080/teams/<team-id>`

Click **View** from the leaderboard, or copy a team ID from the API and navigate directly.

You'll see:
- The roster with each player's class badge (colour-coded: blue=tank, red=dps, green=healer, amber=support)
- The gear pool box:
  - **Team Alpha** → red box (gear points used exceeds the 12-point budget)
  - **Team Beta** → green box (within budget)

### 3. The Profile Page — `http://localhost:8080/profile`

This page requires you to be logged in. Since there's no login form yet, you need to:

1. Login via the API (see curl section below) to get a JWT token
2. Store it in the browser so the profile page can read it

**Quick way to store the token:**

After you get your token from `POST /auth/login`, open the browser Console
(right-click → Inspect → Console tab) and run:

```javascript
localStorage.setItem('ark8de_token', 'PASTE_YOUR_TOKEN_HERE')
```

Then navigate to `http://localhost:8080/profile` — you'll see your stats, skills, gear, and Kredit balance.

> Note: The profile page reads auth from the cookie/header set by the browser during the request. Since we haven't built a cookie-based session yet, the `/profile` page in Phase 1 reads the JWT from the `Authorization` header. For now, use the API directly to view your data.

### 4. The Moderator Panel — `http://localhost:8080/mod`

Accessible only to moderators and admins. Same login requirement as the profile page.

If you log in as `moderator` or `admin` (see credentials below), the panel lets you:
- Award or deduct Arkade points from any team using a dropdown
- Grant Kredits to any player by entering their UUID

---

## Test Credentials

These were created by `task seed`. All passwords follow the same pattern.

| Role | Username | Password |
|---|---|---|
| **admin** | `admin` | `Admin1234!` |
| **moderator** | `moderator` | `Mod1234!` |
| Team Alpha owner (tank) | `alpha_captain` | `Alpha1234!` |
| Team Alpha healer | `alpha_healer` | `Alpha1234!` |
| Team Alpha dps | `alpha_dps` | `Alpha1234!` |
| Team Alpha support | `alpha_support` | `Alpha1234!` |
| Team Beta owner (dps) | `beta_captain` | `Beta1234!` |
| Team Beta tank | `beta_tank` | `Beta1234!` |
| Team Beta healer | `beta_healer` | `Beta1234!` |
| Team Beta support | `beta_support` | `Beta1234!` |
| No team | `lone_wolf` | `Wolf1234!` |

---

## Using the API with curl

### Register a new player

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username": "your_name", "email": "you@example.com", "password": "YourPass1!"}' | jq
```

Password requirements: at least 8 characters, one uppercase, one lowercase, one digit.

### Login and save your token

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "alpha.captain@ark8de.dev", "password": "Alpha1234!"}' \
  | jq -r '.token')

echo "Token: $TOKEN"
```

All subsequent commands use `$TOKEN`. Swap the email/password for any account above.

### View your own profile

```bash
curl -s http://localhost:8080/auth/me \
  -H "Authorization: Bearer $TOKEN" | jq
```

Shows your ID, username, role. **Save your player ID** — you'll need it for some operations:

```bash
MY_ID=$(curl -s http://localhost:8080/auth/me \
  -H "Authorization: Bearer $TOKEN" | jq -r '.id')
```

---

## Player Actions

### Set your class role

```bash
curl -s -X PUT http://localhost:8080/api/players/me/class \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"class_role": "tank"}' | jq
```

Valid classes: `tank`, `dps`, `healer`, `support`. Setting a class clears any existing skills.

### View your combat stats

```bash
curl -s http://localhost:8080/api/players/me/stats \
  -H "Authorization: Bearer $TOKEN" | jq
```

Shows HP, armor, skill points total/remaining. These are computed from your class + allocated skills.

### View available skills for your class

```bash
curl -s "http://localhost:8080/api/skills?class_role=tank" | jq
```

Each skill shows its `id`, `cost_skill_points`, `hp_bonus`, `armor_bonus`. Copy the IDs you want.

### Allocate skills

```bash
curl -s -X PUT http://localhost:8080/api/players/me/skills \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"skill_ids": ["SKILL_UUID_1", "SKILL_UUID_2"]}' | jq
```

This replaces your entire skill allocation. You start with 10 skill points total.
The API returns an error if you exceed your budget or pick skills from the wrong class.

### View all available gear

```bash
curl -s http://localhost:8080/api/skills | jq   # skills
# gear types are not exposed as a public list in Phase 1 — use your player gear endpoint:
curl -s http://localhost:8080/api/players/me/gear \
  -H "Authorization: Bearer $TOKEN" | jq
```

### Select gear

```bash
curl -s -X PUT http://localhost:8080/api/players/me/gear \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"gear_type_ids": ["GEAR_UUID_1", "GEAR_UUID_2"]}' | jq
```

This replaces your entire gear selection. The response includes your team's gear pool status.

### Check your Kredit balance

```bash
curl -s http://localhost:8080/api/players/me/kredits \
  -H "Authorization: Bearer $TOKEN" | jq
```

### Transfer Kredits to another player

```bash
curl -s -X POST http://localhost:8080/api/players/me/kredits/transfer \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to_player_id": "TARGET_PLAYER_UUID", "amount": 50, "note": "good game"}' | jq
```

---

## Team Actions

### See all teams (public)

```bash
curl -s http://localhost:8080/api/teams | jq
```

Copy a team's `id` for the next commands.

### Get a team's full detail (public)

```bash
TEAM_ID="PASTE_TEAM_UUID_HERE"
curl -s http://localhost:8080/api/teams/$TEAM_ID | jq
```

### Create your own team (any logged-in player)

```bash
curl -s -X POST http://localhost:8080/api/teams \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Team Omega", "tag": "OMGA"}' | jq
```

Tag rules: 2–10 characters, letters and digits only. You must not already be a team owner.

### Send a join request

```bash
curl -s -X POST http://localhost:8080/api/teams/$TEAM_ID/join-requests \
  -H "Authorization: Bearer $TOKEN" | jq
```

### View pending join requests (as team owner)

Login as a team owner, then:

```bash
OWNER_TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "alpha.captain@ark8de.dev", "password": "Alpha1234!"}' \
  | jq -r '.token')

curl -s http://localhost:8080/api/teams/$TEAM_ID/join-requests \
  -H "Authorization: Bearer $OWNER_TOKEN" | jq
```

### Accept or reject a join request

```bash
REQUEST_ID="PASTE_REQUEST_UUID_HERE"

# Accept:
curl -s -X PUT http://localhost:8080/api/teams/$TEAM_ID/join-requests/$REQUEST_ID \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action": "accept"}' | jq

# Reject:
curl -s -X PUT http://localhost:8080/api/teams/$TEAM_ID/join-requests/$REQUEST_ID \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action": "reject"}' | jq
```

### Lock in your team

```bash
curl -s -X PUT http://localhost:8080/api/teams/$TEAM_ID/lock \
  -H "Authorization: Bearer $OWNER_TOKEN" | jq
```

Returns `{"is_locked_in": true}`. Call again to toggle it back off.

---

## Moderator Actions

Login as moderator first:

```bash
MOD_TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "moderator@ark8de.dev", "password": "Mod1234!"}' \
  | jq -r '.token')
```

### Award Arkade points to a team

```bash
curl -s -X PUT http://localhost:8080/api/teams/$TEAM_ID/points \
  -H "Authorization: Bearer $MOD_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"delta": 25, "reason": "Won the arena round"}' | jq
```

`delta` can be negative to deduct points. After this call, refresh the leaderboard
(`http://localhost:8080`) — the rankings update instantly.

### View the point history for a team

```bash
curl -s http://localhost:8080/api/teams/$TEAM_ID/points/history \
  -H "Authorization: Bearer $MOD_TOKEN" | jq
```

### Grant Kredits to a player

```bash
curl -s -X POST http://localhost:8080/api/players/$MY_ID/kredits \
  -H "Authorization: Bearer $MOD_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"amount": 200, "note": "Event participation reward"}' | jq
```

---

## The Leaderboard API (raw JSON)

```bash
# All teams ranked:
curl -s http://localhost:8080/api/leaderboard | jq

# One team's leaderboard card:
curl -s http://localhost:8080/api/leaderboard/teams/$TEAM_ID | jq
```

---

## Health Check

```bash
curl -s http://localhost:8080/healthz
# → "ok"
```

If the database is unreachable this returns `503 database unreachable`.

---

## What You Cannot Do Yet (Phase 2+)

| Feature | When |
|---|---|
| Profile page login form (no password form in the UI) | Phase 2 |
| Profile photo upload | Phase 2 (needs object storage) |
| Team logo upload | Phase 2 (needs object storage) |
| Real-time leaderboard updates (WebSocket / SSE) | Phase 3 |
| Unit and integration tests | Phase 2 |
