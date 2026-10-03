package plugin

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
)

func (s *HTTPRoutesServer) handleSports(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !s.sportsFeatureEnabled() {
		return s.respondJSON(http.StatusOK, SportsPayload{})
	}
	if queryValue(request, "game_stats") != "" {
		return s.handleSportsGameStats(ctx, request)
	}
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	payload := s.preparedSportsPayload(queryValue(request, "refresh") == "1")
	return s.respondJSON(http.StatusOK, payload)
}

type SportsLeagueTeamsPayload struct {
	Teams []SportsTeam `json:"teams"`
}

func (s *HTTPRoutesServer) handleSportsLeagueTeams(ctx context.Context, request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	if !s.sportsFeatureEnabled() {
		return s.respondJSON(http.StatusOK, SportsLeagueTeamsPayload{Teams: []SportsTeam{}})
	}
	if request.GetMethod() != "" && request.GetMethod() != http.MethodGet {
		return textResponse(http.StatusMethodNotAllowed, "method not allowed"), nil
	}
	leagueID := strings.TrimSpace(queryValue(request, "league_id"))
	if leagueID == "" {
		return textResponse(http.StatusBadRequest, "league_id is required"), nil
	}
	payload := s.preparedSportsPayload(false)
	providerID := ""
	leagueName := leagueID
	sportName := ""
	for _, league := range payload.Leagues {
		if league.ID != leagueID {
			continue
		}
		providerID = league.ProviderID
		leagueName = firstNonEmpty(league.Name, leagueID)
		sportName = league.SportName
		break
	}
	teams := sportsLeagueEventTeams(payload.Events, leagueID)
	if provider, ok := s.sportsProvider.(sportsLeagueTeamProvider); ok && providerID != "" {
		rosterCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		providerTeams, err := provider.LeagueTeams(rosterCtx, providerID)
		cancel()
		if err == nil {
			teams = mergeSportsLeagueRosterTeams(leagueID, leagueName, sportName, teams, providerTeams)
		}
	}
	return s.respondJSON(http.StatusOK, SportsLeagueTeamsPayload{Teams: teams})
}

func sportsLeagueEventTeams(events []SportsEvent, leagueID string) []SportsTeam {
	teams := make([]SportsTeam, 0)
	for _, event := range events {
		if event.LeagueID != leagueID {
			continue
		}
		teams = append(teams, event.Away, event.Home)
	}
	return mergeSportsLeagueRosterTeams(leagueID, leagueID, "", teams)
}

func mergeSportsLeagueRosterTeams(leagueID, leagueName, sportName string, groups ...[]SportsTeam) []SportsTeam {
	identities := make([]SportsTeam, 0)
	for _, group := range groups {
		for _, team := range group {
			team = normalizeSportsTeam(team)
			if strings.TrimSpace(team.Name) == "" {
				continue
			}
			identities = append(identities, applySportsIdentityFallbacks(SportsEvent{
				LeagueID: leagueID, LeagueName: leagueName, SportName: sportName, Home: team,
			}).Home)
		}
	}
	nicknameAliases := uniqueSportsTeamNicknameAliases(identities)
	byKey := map[string]SportsTeam{}
	order := make([]string, 0)
	for _, identity := range identities {
		key := normalizeMatchText(identity.Name)
		if canonical := nicknameAliases[key]; canonical != "" {
			key = canonical
		}
		if key == "" {
			key = identity.ID
		}
		if existing, ok := byKey[key]; ok {
			identity = mergeSportsTeamIdentity(existing, identity)
		} else {
			order = append(order, key)
		}
		byKey[key] = identity
	}
	teams := make([]SportsTeam, 0, len(order))
	for _, key := range order {
		teams = append(teams, finalizeSportsTeamIdentity(byKey[key]))
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })
	return teams
}

func uniqueSportsTeamNicknameAliases(teams []SportsTeam) map[string]string {
	candidates := map[string]map[string]bool{}
	for _, team := range teams {
		fullName := normalizeMatchText(team.Name)
		parts := strings.Fields(fullName)
		for start := 1; start < len(parts); start++ {
			alias := strings.Join(parts[start:], " ")
			if candidates[alias] == nil {
				candidates[alias] = map[string]bool{}
			}
			candidates[alias][fullName] = true
		}
	}
	aliases := map[string]string{}
	for alias, fullNames := range candidates {
		if len(fullNames) != 1 {
			continue
		}
		for fullName := range fullNames {
			aliases[alias] = fullName
		}
	}
	return aliases
}

func mergeSportsTeamIdentity(primary, supplemental SportsTeam) SportsTeam {
	primary.aliasNames, primary.aliasIDs = mergeSportsTeamAliases(primary, supplemental)
	primary.LogoFallbackURL = firstNonEmpty(primary.LogoFallbackURL, supplemental.LogoFallbackURL)
	primary.ID = firstNonEmpty(primary.ID, supplemental.ID)
	if primary.Name == "" || len(normalizeMatchText(supplemental.Name)) > len(normalizeMatchText(primary.Name)) {
		primary.Name = supplemental.Name
	}
	primary.Abbreviation = firstNonEmpty(primary.Abbreviation, supplemental.Abbreviation)
	if primary.LogoURL == "" || (strings.HasPrefix(primary.LogoURL, gameThumbsPublicBaseURL+"/") && supplemental.LogoURL != "" && !strings.HasPrefix(supplemental.LogoURL, gameThumbsPublicBaseURL+"/")) {
		primary.LogoURL = supplemental.LogoURL
	}
	primary.PrimaryColor = firstNonEmpty(primary.PrimaryColor, supplemental.PrimaryColor)
	primary.SecondaryColor = firstNonEmpty(primary.SecondaryColor, supplemental.SecondaryColor)
	primary.Favorite = primary.Favorite || supplemental.Favorite
	return primary
}

func (s *HTTPRoutesServer) handleSportsFavorite(request *pluginv1.HandleHTTPRequest) (*pluginv1.HandleHTTPResponse, error) {
	return userStateUnavailableResponse(), nil
}
