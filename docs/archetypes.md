# Ark8de Archetypes — Game Mechanics Reference

English translation of the source design sheet (`ארקייד- מכאניקת ארכיטיפים.xlsx`),
as implemented in migration `000014_archetype_redesign`. This is the canonical
mapping between the physical game and the app's data model.

## Rules implemented in the app

- **6 archetypes** replace the placeholder tank/dps/healer/support classes.
- **Base HP is 3** for everyone; archetype passives and skills add to it.
- **Levels 1–3** (moderator-managed, default 1). A skill of tier N requires level ≥ N.
- **One skill per tier** — at each tier the player picks the **blue** or **red**
  branch skill, never both. Leveling down prunes out-of-reach skills.
- **Weapon costs** draw from the team's shared 12-point gear pool.
  Class-restricted weapons are enforced on selection. Armor is free/untracked —
  armor restrictions are descriptive (players wear what their passive allows).
- The Hacker's *Grid is Good* adds **+4 gear points to the team pool** (data-driven
  via `skills.team_gear_bonus`).

## Archetypes & passives

| Archetype | Hebrew | RPG role | Passive |
|---|---|---|---|
| Smartass | התחמן/העוקץ | Rogue | No heavy armor, no shield |
| Ninja | הנינג'ה | Scout/light fighter | No heavy armor, no shield |
| Psycho | הפסיכי (סייקר) | Mage | No heavy/medium armor; no pole or long weapons |
| Hacker | ההאקר | Buffer/disruptor | Same as Psycho |
| Merkava | המרכבה | Heavy armor | No light armor; **+1 HP** |
| Kommando | הקומנדו | Fighter/support | No equipment restrictions |

## Skill trees (tier 1–3 = sheet rows 4–6)

| Archetype | Tier | Blue branch | Red branch |
|---|---|---|---|
| Smartass | 1 | Cheats — Freeze ×2 | The Coward — off-play, return to base |
| Smartass | 2 | Red Bull Shot — heal 1 HP ×4 | Playing Dirty — pre-battle weaken (−3 HP) |
| Smartass | 3 | Take a Shpitzur! — gift Freeze/Mass Knockback ×2 | Bullshitsu — fake a dying state |
| Ninja | 1 | The Elusive — ignore a hit ×2 | The Pusher — Knockback after melee hit ×2 |
| Ninja | 2 | Close-Range Intimidator — Fear after melee hit ×2 | The Coward |
| Ninja | 3 | Ninjitsu — palm-to-elbow hits don't count | It's Not Personal — 30s Freeze after melee hit ×2 |
| Psycho | 1 | Basic Psychosis — Knockback ×2 | Rechargeable Batteries — +4 power balls, 30s rest recharge |
| Psycho | 2 | Psychic Armor — +1 HP to self + 1 ally | The Intimidator — Fear ×2 |
| Psycho | 3 | Advanced Psychosis — Mass Knockback ×2 | Flipping Out — berserk instead of dying (7 HP, then explodes) |
| Hacker | 1 | Cheats — Freeze ×2 | Rechargeable Batteries |
| Hacker | 2 | It's a Feature — cleanse a debuff ×2, reflect it | It's a Bug — disable enemy equipment until 10s base fix |
| Hacker | 3 | **Grid is Good — team +4 gear points** | God Mode — ranged immunity, HP locked at 1, ×2 |
| Merkava | 1 | **Fridge — +1 HP (permanent)** | Nailed to the Floor — knockback immunity |
| Merkava | 2 | Windbreaker — ranged deals 1 instead of 2 | Close-Range Intimidator |
| Merkava | 3 | Too Dumb — ignore one effect ×2 | Brain Glitch — 1st dying → back to 2 HP; 2nd → instant death |
| Kommando | 1 | Psychic Armor | The Pusher |
| Kommando | 2 | Adrenaline Shot — dying ally back to 2 HP ×1 | Dramatic Speech — heal 1 HP ×4 |
| Kommando | 3 | Even More Psychic Armor — team-wide if stacked | Friend of the Quartermaster — double team ammo |

Tiers 4–5 exist in the sheet but are deliberately **not implemented** (level cap is 3).

## Weapon catalog

| Weapon | Cost | Nickname | Restricted to |
|---|---|---|---|
| short_weapon | 1 | | — |
| dagger | 1 | "docker" | — |
| long_weapon | 2 | | not Psycho/Hacker |
| spear | 3 | "long docker" | not Psycho/Hacker |
| bow | 4 | | — |
| pistol | 4 | | — |
| shield | 4 | "firewall" | not Smartass/Ninja |
| power_balls_x4 | 1 (pack of 4) | | Psycho & Hacker only |

## Game vocabulary (for future features)

נק"פ = hit points · הקפאה = Freeze · הדף / הדף המוני = Knockback / Mass Knockback ·
פחד = Fear · נטרול = Disable · גסיסה = dying state (distinct from death) ·
בסיס = base · נקסוס = Nexus · אופליי = off-play (arms crossed on chest) ·
כדורי כוח = power balls · מאסטר = game master
