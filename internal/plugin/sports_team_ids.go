package plugin

import (
	"strings"
)

// legacyEmptySportsTeamID is the ID earlier builds hashed for a team with no
// name (one-sided listings such as tournaments and races). Every nameless side
// shared it, so a follow saved against it is a phantom "Saved team" the UI
// should purge. Nameless teams now carry no ID at all.
const legacyEmptySportsTeamID = "sports-team:cbe5cfdf7c2118a9"

const sportsTeamLegacyIDLimit = 8

func stableSportsTeamID(team SportsTeam) string {
	return "sports-team:" + sportsHash(strings.ToLower(strings.TrimSpace(team.Name+"|"+team.Abbreviation)))
}

func normalizeSportsTeam(team SportsTeam) SportsTeam {
	team.ID = strings.TrimSpace(team.ID)
	team.Name = strings.TrimSpace(team.Name)
	team.Abbreviation = strings.TrimSpace(team.Abbreviation)
	team.LogoURL = safeSportsImageURL(team.LogoURL)
	team.PrimaryColor = strings.TrimSpace(team.PrimaryColor)
	team.SecondaryColor = strings.TrimSpace(team.SecondaryColor)
	if team.Name == "" {
		// A side without a name is not a followable team.
		team.ID = ""
		return team
	}
	if team.ID == "" {
		team.ID = stableSportsTeamID(team)
	}
	return team
}

// finalizeSportsTeamIdentity sets the follow identities a team is emitted with.
func finalizeSportsTeamIdentity(team SportsTeam) SportsTeam {
	team.Favorite = false
	if strings.TrimSpace(team.Name) == "" {
		team.ID = ""
		team.FollowIDs = nil
		team.LegacyIDs = nil
		return team
	}
	team.FollowIDs = sportsTeamFollowIDs(team)
	team.LegacyIDs = sportsTeamLegacyIDs(team)
	return team
}

// sportsTeamFollowIDs lists the IDs the same name hashes to with the other
// abbreviations a listing may carry: guide copies use initials, provider
// copies their own abbreviation, and some none at all.
func sportsTeamFollowIDs(team SportsTeam) []string {
	name := strings.TrimSpace(team.Name)
	if name == "" {
		return nil
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, abbreviation := range []string{team.Abbreviation, sportsTeamInitials(name), ""} {
		id := stableSportsTeamID(SportsTeam{Name: name, Abbreviation: abbreviation})
		if !seen[id] && id != team.ID {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// sportsTeamLegacyIDs lists IDs earlier builds produced for other spellings of
// this team: copies merged into it (Sportarr and guide names) and the short
// names guide listings use ("Yankees", "Michigan"). IDs already in id or
// followIds are left out.
func sportsTeamLegacyIDs(team SportsTeam) []string {
	name := strings.TrimSpace(team.Name)
	if name == "" {
		return nil
	}
	skip := map[string]bool{team.ID: true, "": true, legacyEmptySportsTeamID: true}
	for _, id := range team.FollowIDs {
		skip[id] = true
	}
	ids := []string{}
	add := func(id string) {
		if skip[id] || len(ids) >= sportsTeamLegacyIDLimit {
			return
		}
		skip[id] = true
		ids = append(ids, id)
	}
	for _, id := range team.aliasIDs {
		add(id)
	}
	names := append([]string{name}, team.aliasNames...)
	for _, alias := range team.aliasNames {
		alias = strings.TrimSpace(alias)
		if alias == "" || strings.EqualFold(alias, name) {
			continue
		}
		add(stableSportsTeamID(SportsTeam{Name: alias, Abbreviation: sportsTeamInitials(alias)}))
		add(stableSportsTeamID(SportsTeam{Name: alias, Abbreviation: team.Abbreviation}))
		add(stableSportsTeamID(SportsTeam{Name: alias}))
	}
	for _, full := range names {
		for _, short := range sportsTeamShortNames(full) {
			add(stableSportsTeamID(SportsTeam{Name: short, Abbreviation: sportsTeamInitials(short)}))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// sportsTeamShortNames returns the nickname ("Yankees", "Red Sox") and
// location ("Michigan") spellings a guide may list for a full team name.
func sportsTeamShortNames(name string) []string {
	words := strings.Fields(strings.TrimSpace(name))
	if len(words) < 2 {
		return nil
	}
	shorts := []string{}
	add := func(value string) {
		normalized := normalizeMatchText(value)
		if len([]rune(normalized)) < 4 || sportsGenericTeamWord(normalized) {
			return
		}
		for _, existing := range shorts {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		shorts = append(shorts, value)
	}
	add(words[len(words)-1])
	if len(words) > 2 {
		add(strings.Join(words[len(words)-2:], " "))
	}
	add(strings.Join(words[:len(words)-1], " "))
	return shorts
}

func sportsGenericTeamWord(normalized string) bool {
	switch normalized {
	case "united", "city", "county", "state", "football", "basketball", "women", "womens", "men", "mens", "team", "club", "athletic", "sporting", "real", "the":
		return true
	}
	return false
}

// mergeSportsTeamAliases records the names and IDs two merged copies carried,
// so follows saved against either copy still resolve.
func mergeSportsTeamAliases(primary, supplemental SportsTeam) ([]string, []string) {
	names := make([]string, 0, len(primary.aliasNames)+len(supplemental.aliasNames)+2)
	seenNames := map[string]bool{}
	for _, group := range [][]string{primary.aliasNames, {primary.Name, supplemental.Name}, supplemental.aliasNames} {
		for _, value := range group {
			key := strings.ToLower(strings.TrimSpace(value))
			if key == "" || seenNames[key] {
				continue
			}
			seenNames[key] = true
			names = append(names, strings.TrimSpace(value))
		}
	}
	ids := make([]string, 0, len(primary.aliasIDs)+len(supplemental.aliasIDs)+2)
	seenIDs := map[string]bool{}
	for _, group := range [][]string{primary.aliasIDs, {primary.ID, supplemental.ID}, supplemental.aliasIDs} {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" || value == legacyEmptySportsTeamID || seenIDs[value] {
				continue
			}
			seenIDs[value] = true
			ids = append(ids, value)
		}
	}
	return names, ids
}

// sportsTeamIdentityKey keeps event identities stable for nameless sides, which
// earlier builds keyed with the shared empty-name hash.
func sportsTeamIdentityKey(team SportsTeam) string {
	if strings.TrimSpace(team.ID) == "" && strings.TrimSpace(team.Name) == "" {
		return legacyEmptySportsTeamID
	}
	return team.ID
}
