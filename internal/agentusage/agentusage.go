package agentusage

import (
	"context"
	"sort"
	"sync"
	"time"
)

// fetcher fetches usage for a single provider. Implementations must never
// return an error out-of-band: failures belong in ProviderUsage.Error so a
// broken provider never hides the working ones.
type fetcher func(ctx context.Context) ProviderUsage

var fetchers = map[string]fetcher{
	"claude": fetchClaude,
	"codex":  fetchCodex,
}

// Providers returns the supported provider names, sorted.
func Providers() []string {
	names := make([]string, 0, len(fetchers))
	for name := range fetchers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Fetch collects usage for the given providers concurrently. With no names it
// fetches all supported providers. Unknown names yield a ProviderUsage with an
// Error set. Results are ordered to match Providers() for stable output.
func Fetch(ctx context.Context, names ...string) Snapshot {
	if len(names) == 0 {
		names = Providers()
	}

	results := make([]ProviderUsage, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			fn, ok := fetchers[name]
			if !ok {
				results[i] = ProviderUsage{Provider: name, Error: "unknown provider"}
				return
			}
			results[i] = fn(ctx)
		}(i, name)
	}
	wg.Wait()

	return Snapshot{GeneratedAt: time.Now(), Providers: results}
}
