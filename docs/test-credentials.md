# Test Credentials Reference

All accounts are created by the seed script: `task seed` (from `monolith/`).

---

## Quick Reference

| Account          | Email                        | Password      | Role         | Class    | Lvl | Team        |
|------------------|------------------------------|---------------|--------------|----------|-----|-------------|
| admin            | admin@ark8de.dev             | Admin1234!    | admin        | --       | 1   | --          |
| moderator        | moderator@ark8de.dev         | Mod1234!      | moderator    | --       | 1   | --          |
| alpha_captain    | alpha.captain@ark8de.dev     | Alpha1234!    | team_owner   | merkava  | 3   | Team Alpha  |
| alpha_ninja      | alpha.ninja@ark8de.dev       | Alpha1234!    | player       | ninja    | 2   | Team Alpha  |
| alpha_psycho     | alpha.psycho@ark8de.dev      | Alpha1234!    | player       | psycho   | 1   | Team Alpha  |
| alpha_hacker     | alpha.hacker@ark8de.dev      | Alpha1234!    | player       | hacker   | 3   | Team Alpha  |
| beta_captain     | beta.captain@ark8de.dev      | Beta1234!     | team_owner   | kommando | 3   | Team Beta   |
| beta_smartass    | beta.smartass@ark8de.dev     | Beta1234!     | player       | smartass | 2   | Team Beta   |
| beta_merkava     | beta.merkava@ark8de.dev      | Beta1234!     | player       | merkava  | 1   | Team Beta   |
| beta_ninja       | beta.ninja@ark8de.dev        | Beta1234!     | player       | ninja    | 1   | Team Beta   |
| lone_wolf        | lone.wolf@ark8de.dev         | Wolf1234!     | player       | psycho   | 2   | (none)      |

---

## How to Authenticate

### Browser (cookie sessions)

Go to `/login` and enter email + password. A `session_id` cookie is set automatically.

### API (Bearer JWT)

```bash
# 1. Get a token
curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@ark8de.dev","password":"Admin1234!"}' | jq .token

# 2. Use the token
curl -s http://localhost:8080/auth/me \
  -H "Authorization: Bearer <token>"
```

---

## What Each Role Can Access

### player (alpha_ninja, alpha_psycho, alpha_hacker, beta_smartass, beta_merkava, beta_ninja, lone_wolf)

| Area                    | Routes                                               |
|-------------------------|------------------------------------------------------|
| View leaderboard        | `GET /` , `GET /api/leaderboard`                     |
| View team detail        | `GET /teams/{id}` , `GET /api/teams/{id}`            |
| View own profile        | `GET /profile` , `GET /auth/me`                      |
| Set class               | `PUT /api/players/me/class`                           |
| Skills (view / set)     | `GET /api/players/me/skills` , `PUT /api/players/me/skills` |
| Gear (view / set)       | `GET /api/players/me/gear` , `PUT /api/players/me/gear` |
| Stats                   | `GET /api/players/me/stats`                           |
| Kredits (view/transfer) | `GET /api/players/me/kredits` , `POST /api/players/me/kredits/transfer` |
| Send join request       | `POST /api/teams/{id}/join-requests`                  |

### team_owner (alpha_captain, beta_captain)

Everything a `player` can do, plus:

| Area                    | Routes                                               |
|-------------------------|------------------------------------------------------|
| Update own team         | `PUT /api/teams/{id}`                                 |
| Delete own team         | `DELETE /api/teams/{id}`                              |
| Lock/unlock roster      | `PUT /api/teams/{id}/lock`                            |
| Remove a member         | `DELETE /api/teams/{id}/members/{pid}`                |
| View join requests      | `GET /api/teams/{id}/join-requests`                   |
| Accept/reject requests  | `PUT /api/teams/{id}/join-requests/{rid}`             |
| Upload team logo        | `PUT /api/teams/{id}/logo`                            |

### moderator / admin (moderator, admin)

Everything above, plus:

| Area                    | Routes                                               |
|-------------------------|------------------------------------------------------|
| Moderator panel (HTML)  | `GET /mod`                                            |
| Award arkade points     | `POST /mod/award-points` , `PUT /api/teams/{id}/points` |
| View point history      | `GET /api/teams/{id}/points/history`                  |
| Grant kredits (HTML)    | `POST /mod/grant-kredits`                             |
| Grant kredits (API)     | `POST /api/players/{id}/kredits`                      |

---

## Scenario-Specific Test Accounts

### Testing the gear budget display

| Scenario                     | Log in as                         | What you'll see                               |
|------------------------------|-----------------------------------|-----------------------------------------------|
| Over budget (red warning)    | Any Team Alpha member             | Team gear pool = 17/16 (1 over; 12 base +4 from Grid is Good) |
| Under budget (healthy green) | Any Team Beta member              | Team gear pool = 8/12 (4 remaining, green)    |

### Testing team ownership actions

| Action                         | Log in as          | Target         |
|--------------------------------|--------------------|----------------|
| Edit team name/tag             | alpha_captain      | Team Alpha     |
| Accept/reject join requests    | beta_captain       | Team Beta      |
| Lock roster                    | alpha_captain      | Team Alpha     |
| Remove a member                | beta_captain       | Team Beta      |

### Testing moderator actions

| Action                         | Log in as          | Notes                                   |
|--------------------------------|--------------------|-----------------------------------------|
| Award arkade points            | moderator          | Use `/mod` panel or API                 |
| Grant kredits to a player      | admin              | Use `/mod` panel or API                 |
| View point history             | moderator          | API only: `GET /api/teams/{id}/points/history` |

### Testing as a player without a team

| Action                         | Log in as          | Notes                                   |
|--------------------------------|--------------------|-----------------------------------------|
| Browse leaderboard             | lone_wolf          | Public — no auth needed either          |
| Send a join request            | lone_wolf          | `POST /api/teams/{id}/join-requests`    |
| Set class / skills / gear      | lone_wolf          | psycho lvl 2, no team gear pool          |

### Testing permission boundaries (expect 403 Forbidden)

| Action tried                   | Log in as          | Expected result                          |
|--------------------------------|--------------------|------------------------------------------|
| Award points                   | alpha_captain      | 403 — only moderator/admin               |
| Grant kredits                  | lone_wolf          | 403 — only moderator/admin               |
| Edit another player's team     | beta_captain       | 403 — can only manage own team           |
| Access `/mod` panel            | alpha_ninja        | 403 — player role                        |
| Set a player's level           | beta_captain       | 403 — only moderator/admin               |

### Testing without authentication (expect 401 Unauthorized)

| Action tried                   | Expected result                                      |
|--------------------------------|------------------------------------------------------|
| `GET /auth/me` (no header)     | 401 — missing Authorization header                   |
| `PUT /api/players/me/class`    | 401 — authentication required                        |
| `POST /api/teams`              | 401 — authentication required                        |
| `GET /profile` (no cookie)     | 401 — not authenticated                              |

---

## Seeded Game Data Summary

| Data              | Team Alpha                          | Team Beta                           |
|-------------------|-------------------------------------|-------------------------------------|
| Members           | 4 (merkava/ninja/psycho/hacker)     | 4 (kommando/smartass/merkava/ninja) |
| Gear budget used  | 17 / 16 (over; +4 from Grid is Good) | 8 / 12 (under)                     |
| Arkade points     | 50 (25 + 10 + 15)                   | 25 (30 - 5)                         |
| Skills allocated  | Yes (varies per player)             | Yes (varies per player)             |
| Starting kredits  | 0 (grant via moderator)             | 0 (grant via moderator)             |
