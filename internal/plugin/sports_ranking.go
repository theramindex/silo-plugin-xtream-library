package plugin

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sportsRankingKnee = 8.0

var sportsRankingOrdinalPattern = regexp.MustCompile(`(?i)(?:#|\bno\.?\s+|\branked\s+)([1-9]|1\d|2[0-5])\b`)

// rankSportsEvents adds an explainable, deterministic editorial score without
// changing event identity, playback, or the user's Dispatcharr lineup.
func rankSportsEvents(events []SportsEvent, now time.Time) []SportsEvent {
	ranked := cloneSportsEvents(events)
	for index := range ranked {
		ranked[index].Ranking = rankSportsEvent(ranked[index], now)
	}
	return ranked
}

func rankSportsEvent(event SportsEvent, now time.Time) SportsEventRanking {
	signals := make([]SportsRankingSignal, 0, 8)
	add := func(key, label, detail string, points float64) {
		if points <= 0 {
			return
		}
		signals = append(signals, SportsRankingSignal{Key: key, Label: label, Detail: detail, Points: points})
	}

	if event.Live && !event.Completed {
		add("live", "Live now", "Happening now", 4.0)
	} else if event.StartUnix > 0 && !event.Completed {
		until := time.Unix(event.StartUnix, 0).Sub(now)
		switch {
		case until >= 0 && until <= 3*time.Hour:
			add("soon", "Starting soon", "Starts within three hours", 2.0)
		case until > 3*time.Hour && until <= 24*time.Hour:
			add("today", "On today", "Starts within 24 hours", 1.0)
		}
	}

	stageLabel, stagePoints := sportsStageSignal(event)
	if stagePoints > 0 {
		add("stage", stageLabel, firstNonEmpty(event.Round, event.EventType), stagePoints)
	}

	if sportsTextSignalsRivalry(strings.Join([]string{event.Name, event.ShortName, event.Description}, " ")) {
		add("rivalry", "Rivalry", "Rivalry or derby matchup", 2.0)
	}

	if rankPoints, detail := sportsRankSignal(event); rankPoints > 0 {
		add("rank", "Ranked matchup", detail, rankPoints)
	}

	if closeness, detail := sportsClosenessSignal(event); closeness > 0 {
		add("close", "Close matchup", detail, closeness)
	}

	if len(event.Channels) > 0 {
		add("coverage", "Coverage available", strconv.Itoa(len(event.Channels))+" matched feed(s)", 0.5)
	}

	raw := 0.0
	for _, signal := range signals {
		raw += signal.Points
	}
	sort.SliceStable(signals, func(i, j int) bool {
		if signals[i].Points != signals[j].Points {
			return signals[i].Points > signals[j].Points
		}
		return signals[i].Label < signals[j].Label
	})
	return SportsEventRanking{
		Score:   math.Round((10*math.Tanh(raw/sportsRankingKnee))*10) / 10,
		Raw:     math.Round(raw*10) / 10,
		Knee:    sportsRankingKnee,
		Signals: signals,
	}
}

func sportsStageSignal(event SportsEvent) (string, float64) {
	roundText := strings.ToLower(strings.Join([]string{event.Round, event.EventType}, " "))
	nameText := strings.ToLower(strings.Join([]string{event.Name, event.ShortName}, " "))
	for _, candidate := range []struct {
		terms  []string
		label  string
		points float64
	}{
		{[]string{"championship", "grand final", "world series", "super bowl", "stanley cup final", "finals game"}, "Championship", 3.5},
		{[]string{"semifinal", "semi-final", "conference final", "playoff", "play-off", "knockout"}, "Playoff stakes", 2.5},
		{[]string{"quarterfinal", "quarter-final", "round of 16", "elimination", "wild card"}, "Tournament stakes", 1.75},
		{[]string{"final"}, "Final", 2.75},
	} {
		for _, term := range candidate.terms {
			if strings.Contains(roundText, term) {
				return candidate.label, candidate.points
			}
		}
	}
	for _, candidate := range []struct {
		terms  []string
		label  string
		points float64
	}{
		{[]string{"championship game", "championship final", "grand final", "world series game", "super bowl", "stanley cup final", "finals game"}, "Championship", 3.5},
		{[]string{"semifinal", "semi-final", "conference final", "playoff", "play-off", "knockout"}, "Playoff stakes", 2.5},
		{[]string{"quarterfinal", "quarter-final", "round of 16", "elimination", "wild card"}, "Tournament stakes", 1.75},
	} {
		for _, term := range candidate.terms {
			if strings.Contains(nameText, term) {
				return candidate.label, candidate.points
			}
		}
	}
	return "", 0
}

func sportsTextSignalsRivalry(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, " rivalry") || strings.Contains(value, "rivalry ") || strings.Contains(value, " derby") || strings.Contains(value, "classic rivalry")
}

func sportsRankSignal(event SportsEvent) (float64, string) {
	typedRanks := make([]int, 0, 2)
	for _, rank := range []int{event.HomeRank, event.AwayRank} {
		if rank > 0 && rank <= 25 {
			typedRanks = append(typedRanks, rank)
		}
	}
	if len(typedRanks) >= 2 {
		best := typedRanks[0]
		if typedRanks[1] < best {
			best = typedRanks[1]
		}
		return 1.5 + math.Max(0, float64(15-best))/15, "Both sides are provider-ranked"
	}
	if len(typedRanks) == 1 {
		return 1.0, "Includes a provider-ranked side"
	}
	// Sportarr descriptions commonly retain provider ranking text even when the
	// provider does not expose typed rank fields. Keep this conservative.
	text := strings.Join([]string{event.Name, event.ShortName, event.Description}, " ")
	ranks := sportsExtractOrdinalRanks(text)
	if len(ranks) >= 2 {
		best := ranks[0]
		if ranks[1] < best {
			best = ranks[1]
		}
		return 1.5 + math.Max(0, float64(15-best))/15, "Both sides are ranked"
	}
	if len(ranks) == 1 {
		return 1.0, "Includes a ranked side"
	}
	return 0, ""
}

func sportsExtractOrdinalRanks(value string) []int {
	ranks := make([]int, 0, 2)
	for _, match := range sportsRankingOrdinalPattern.FindAllStringSubmatch(value, -1) {
		if len(match) != 2 {
			continue
		}
		rank, err := strconv.Atoi(match[1])
		if err == nil {
			ranks = append(ranks, rank)
		}
	}
	return ranks
}

func sportsClosenessSignal(event SportsEvent) (float64, string) {
	if event.Completed || event.Live {
		return 0, ""
	}
	if event.Spread != nil {
		spread := math.Abs(*event.Spread)
		switch {
		case spread <= 1:
			return 2.0, "Provider projects a near-even matchup"
		case spread <= 3:
			return 1.5, "Provider spread is within three points"
		case spread <= 7:
			return 0.75, "Provider spread is within seven points"
		}
	}
	text := strings.ToLower(strings.Join([]string{event.Description, event.StatusText}, " "))
	for _, hint := range []string{"pick'em", "pick em", "even odds", "toss-up", "toss up"} {
		if strings.Contains(text, hint) {
			return 2.0, "Projected as an even matchup"
		}
	}
	return 0, ""
}
