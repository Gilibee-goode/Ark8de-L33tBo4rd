// Package main — Database seed script (scaffold)
//
// This script populates the database with realistic test data for local
// development. It is NOT run automatically — execute it with:
//
//	make seed
//	# which runs: cd monolith && go run ../scripts/seed.go
//
// IMPORTANT: Run migrations first before seeding:
//
//	make migrate-up && make seed
//
// STATUS: Phase 0 scaffold — structure only.
// Full implementation will be completed in Phase 1 once the auth package
// (with bcrypt password hashing) is built.
//
// =================================================================
// TEST CREDENTIALS THAT WILL BE SEEDED
// =================================================================
//
// ADMIN ACCOUNT:
//   username: admin
//   email:    admin@ark8de.local
//   password: Admin1234!
//   role:     admin
//
// MODERATOR ACCOUNT:
//   username: moderator
//   email:    mod@ark8de.local
//   password: Mod1234!
//   role:     moderator
//
// TEAM ALPHA (Team Owner + 3 members):
//   Team Owner:
//     username: alpha_captain
//     email:    alpha_captain@ark8de.local
//     password: Player1234!
//     role:     team_owner
//     class:    tank
//   Members:
//     username: alpha_healer  / class: healer  / email: alpha_healer@ark8de.local
//     username: alpha_dps     / class: dps     / email: alpha_dps@ark8de.local
//     username: alpha_support / class: support / email: alpha_support@ark8de.local
//
// TEAM BETA (Team Owner + 3 members):
//   Team Owner:
//     username: beta_captain
//     email:    beta_captain@ark8de.local
//     password: Player1234!
//     role:     team_owner
//     class:    dps
//   Members:
//     username: beta_tank    / class: tank    / email: beta_tank@ark8de.local
//     username: beta_healer  / class: healer  / email: beta_healer@ark8de.local
//     username: beta_support / class: support / email: beta_support@ark8de.local
//
// UNAFFILIATED PLAYER:
//   username: lone_wolf
//   email:    lone_wolf@ark8de.local
//   password: Player1234!
//   role:     player
//
// GEAR SCENARIO (for testing the gear pool):
//   Team Alpha — gear_points_used will be UNDER budget (pool positive ✓)
//   Team Beta  — gear_points_used will be OVER budget (pool negative ✗, shown in red)
//
// SKILLS:
//   Each class role will have 4 seeded skills (16 total).
//   Players will have a subset of skills allocated.
// =================================================================
package main

import "fmt"

func main() {
	// Phase 0: This is a scaffold — not yet implemented.
	// Full implementation arrives in Phase 1 when the auth package
	// provides bcrypt hashing for password fields.
	fmt.Println("Seed script not yet implemented — will be completed in Phase 1")
	fmt.Println("")
	fmt.Println("When implemented, this script will create:")
	fmt.Println("  - 1 admin account")
	fmt.Println("  - 1 moderator account")
	fmt.Println("  - 2 full teams of 4 players each")
	fmt.Println("  - 1 unaffiliated player")
	fmt.Println("  - 16 skills (4 per class role)")
	fmt.Println("  - Gear selections for both teams")
	fmt.Println("  - Initial Arkade point assignments")
}

// seedPlayers will create all test player accounts with bcrypt-hashed passwords.
// It creates the admin, moderator, team owners, team members, and lone wolf.
// Each player is created with realistic stats (kredits, skill points).
func seedPlayers() {
	// TODO Phase 1: Use bcrypt.GenerateFromPassword() to hash passwords,
	// then INSERT rows into the players table.
	// Password: "Player1234!" for regular players, "Admin1234!" for admin, "Mod1234!" for mod.
}

// seedTeams will create the two test teams and assign their members.
// It creates Team Alpha and Team Beta, assigns owners, and adds members
// to the team_members join table.
func seedTeams() {
	// TODO Phase 1: INSERT into teams, then INSERT member rows.
	// Team Alpha: owner is alpha_captain, members are alpha_healer/dps/support.
	// Team Beta:  owner is beta_captain, members are beta_tank/healer/support.
}

// seedSkills will create 16 skills — 4 per class role (tank, dps, healer, support).
// Each skill has a name, description, effect, effect type, and skill point cost.
func seedSkills() {
	// TODO Phase 1: INSERT 16 rows into the skills table.
	// Then INSERT player_skill_allocations for each seeded player based on
	// their class_role and available skill_points_total.
}

// seedGearTypes is NOT needed here because gear types are seeded in migration
// 000007_create_gear_types.up.sql. They are part of the schema, not test data.
// This function is a placeholder to document that decision.
func seedGearTypes() {
	// Gear types (sword, bow_and_arrow, spear, shield) are inserted in
	// migration 000007. See db/migrations/000007_create_gear_types.up.sql.
	// No action needed here.
}
