package plugin

import (
	"context"
	"sync"
	"time"

	"github.com/theramindex/silo-plugin-xtream-library/internal/cache"
)

type sportsGuideData struct {
	Unavailable   bool
	Events        []SportsEvent
	RefreshScores bool
	Channels      sportsChannelIndex
}

// A score refresh does not change the guide. Keep its parsed events and channel
// index until the snapshot changes; re-evaluate time-sensitive guide state at
// the next event boundary, with a five-minute ceiling for the rolling window.
// Only background builders use this cache, never an HTTP request.
type sportsGuideCache struct {
	mu               sync.Mutex
	ready            bool
	version          uint64
	builtAt, expires time.Time
	data             sportsGuideData
}

func (c *sportsGuideCache) get(snapshot cache.Snapshot, now time.Time) sportsGuideData {
	c.mu.Lock()
	defer c.mu.Unlock()
	if snapshot.Guide != nil && (!c.ready || c.version != snapshot.Version || !now.Before(c.expires) || now.Before(c.builtAt)) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var loaded cache.Snapshot
		var err error
		if !c.ready || c.version != snapshot.Version {
			loaded, err = snapshot.WithPrograms(ctx)
		} else {
			loaded = snapshot
			loaded.Catalog.Programs, err = snapshot.ProgramQuery(ctx, nil, now.Add(-24*time.Hour).Unix()-1, now.Add(72*time.Hour).Unix()+1)
			loaded.Guide = nil
		}
		cancel()
		if err != nil {
			return sportsGuideData{Unavailable: true}
		}
		snapshot = loaded
	}
	if !c.ready || c.version != snapshot.Version {
		c.data.Channels = newSportsChannelIndex(snapshot)
		c.ready, c.version = true, snapshot.Version
		c.expires = time.Time{}
	}
	if !now.Before(c.expires) || now.Before(c.builtAt) {
		c.data.Events, c.data.RefreshScores = sportsEventsFromGuideWithScoreHints(snapshot, now)
		c.builtAt, c.expires = now, now.Add(5*time.Minute)
		for _, event := range c.data.Events {
			for _, boundary := range []int64{event.StartUnix, event.EndUnix} {
				at := time.Unix(boundary, 0)
				if at.After(now) && at.Before(c.expires) {
					c.expires = at
				}
			}
		}
	}
	data := c.data
	data.Events = cloneSportsEvents(data.Events)
	return data
}
