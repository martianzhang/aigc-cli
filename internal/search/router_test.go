package search

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Compile-time guarantees that the test doubles satisfy the production interfaces.
var (
	_ Provider   = (*mockProvider)(nil)
	_ QuotaStore = (*mockQuotaStore)(nil)
)

// errSearchDown is a sentinel error used to assert error wrapping in Router.Search.
var errSearchDown = errors.New("provider is down")

// mockProvider is an in-memory Provider implementation. It performs no network
// I/O and records every Search invocation.
type mockProvider struct {
	name    string
	results []Result
	err     error

	mu        sync.Mutex
	calls     int
	lastQuery string
	lastLimit int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Search(query string, limit int) ([]Result, error) {
	m.mu.Lock()
	m.calls++
	m.lastQuery = query
	m.lastLimit = limit
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func (m *mockProvider) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *mockProvider) recordedQuery() (string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastQuery, m.lastLimit
}

// mockQuotaStore is an in-memory QuotaStore. A provider without an explicit
// usage entry is unlimited, mirroring SQLiteQuotaStore's "no record = unlimited".
type mockQuotaStore struct {
	mu      sync.Mutex
	blocked map[string]bool
	used    map[string]int
	total   map[string]int

	canUseCalls    map[string]int
	recordUseCalls map[string]int
}

func newMockQuotaStore() *mockQuotaStore {
	return &mockQuotaStore{
		blocked:        make(map[string]bool),
		used:           make(map[string]int),
		total:          make(map[string]int),
		canUseCalls:    make(map[string]int),
		recordUseCalls: make(map[string]int),
	}
}

func (m *mockQuotaStore) CanUse(provider string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.canUseCalls[provider]++
	return !m.blocked[provider]
}

func (m *mockQuotaStore) RecordUse(provider string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordUseCalls[provider]++
	m.used[provider]++
}

// ResetIfNeeded is a no-op; tests model period rollover through setUsage.
func (m *mockQuotaStore) ResetIfNeeded(string) {}

func (m *mockQuotaStore) Usage(provider string) (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.used[provider], m.total[provider]
}

func (m *mockQuotaStore) setUsage(provider string, used, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.used[provider] = used
	m.total[provider] = total
}

func (m *mockQuotaStore) block(providers ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, provider := range providers {
		m.blocked[provider] = true
	}
}

func (m *mockQuotaStore) recordUseCount(provider string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recordUseCalls[provider]
}

func (m *mockQuotaStore) canUseCount(provider string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.canUseCalls[provider]
}

// testProvider describes one provider registration for buildRouter.
type testProvider struct {
	name    string
	results []Result
	err     error
	info    *ProviderInfo // optional; nil means no config is registered
}

// buildRouter registers every spec into a fresh router backed by store.
func buildRouter(store *mockQuotaStore, specs ...testProvider) (*Router, map[string]*mockProvider) {
	r := NewRouter(store)
	mocks := make(map[string]*mockProvider, len(specs))
	for _, spec := range specs {
		mock := &mockProvider{name: spec.name, results: spec.results, err: spec.err}
		r.Register(spec.name, mock, spec.info)
		mocks[spec.name] = mock
	}
	return r, mocks
}

// sameElements reports whether got and want hold the same values, ignoring
// order (several Router helpers iterate over maps).
func sameElements(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(got))
	for _, v := range got {
		counts[v]++
	}
	for _, v := range want {
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func TestNewRouter(t *testing.T) {
	t.Parallel()

	t.Run("starts empty", func(t *testing.T) {
		t.Parallel()
		store := newMockQuotaStore()
		r := NewRouter(store)
		if r == nil {
			t.Fatal("NewRouter returned nil")
		}
		if got := len(r.providers); got != 0 {
			t.Errorf("providers = %d, want 0", got)
		}
		if got := len(r.configs); got != 0 {
			t.Errorf("configs = %d, want 0", got)
		}
		if r.store == nil {
			t.Error("store not retained")
		}
		if got := r.allProviders(); len(got) != 0 {
			t.Errorf("allProviders() = %v, want empty", got)
		}
		if p, ok := r.GetProvider("ghost"); ok || p != nil {
			t.Errorf("GetProvider(ghost) = (%v, %v), want (nil, false)", p, ok)
		}
		r.Close()
	})

	t.Run("nil store is accepted", func(t *testing.T) {
		t.Parallel()
		r := NewRouter(nil)
		// Search returns before touching the store when nothing is registered.
		if _, err := r.Search("q", 1, "auto", nil); err == nil {
			t.Fatal("Search() error = nil, want no-providers error")
		}
	})
}

func TestRouterRegister(t *testing.T) {
	t.Parallel()

	info := &ProviderInfo{
		Type:   "duckduckgo",
		APIKey: "sk-test",
		Tags:   []string{"free", "quality"},
		Quota:  100,
		Period: "daily",
		Weight: 3,
	}

	tests := []struct {
		name       string
		info       *ProviderInfo
		wantConfig bool
	}{
		{name: "with config", info: info, wantConfig: true},
		{name: "without config", info: nil, wantConfig: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			r := NewRouter(store)

			r.Register("alpha", &mockProvider{name: "alpha"}, tc.info)

			got, ok := r.GetProvider("alpha")
			if !ok {
				t.Fatal("GetProvider(alpha) not found after Register")
			}
			if got.Name() != "alpha" {
				t.Errorf("provider Name() = %q, want %q", got.Name(), "alpha")
			}
			cfg, ok := r.configs["alpha"]
			if ok != tc.wantConfig {
				t.Fatalf("config registered = %v, want %v", ok, tc.wantConfig)
			}
			if tc.wantConfig && cfg != tc.info {
				t.Errorf("config = %+v, want %+v", cfg, tc.info)
			}
		})
	}

	t.Run("replaces an existing provider", func(t *testing.T) {
		t.Parallel()
		store := newMockQuotaStore()
		r := NewRouter(store)
		r.Register("alpha", &mockProvider{name: "first"}, nil)
		r.Register("alpha", &mockProvider{name: "second"}, nil)

		got, ok := r.GetProvider("alpha")
		if !ok {
			t.Fatal("GetProvider(alpha) not found after re-register")
		}
		if got.Name() != "second" {
			t.Errorf("provider Name() = %q, want %q", got.Name(), "second")
		}
		if n := len(r.providers); n != 1 {
			t.Errorf("providers = %d, want 1 after re-register", n)
		}
	})
}

func TestRouterGetProvider(t *testing.T) {
	t.Parallel()

	store := newMockQuotaStore()
	r := NewRouter(store)
	r.Register("alpha", &mockProvider{name: "alpha"}, nil)
	r.Register("beta", &mockProvider{name: "beta"}, nil)

	tests := []struct {
		name      string
		lookup    string
		wantFound bool
		wantName  string
	}{
		{name: "registered", lookup: "alpha", wantFound: true, wantName: "alpha"},
		{name: "second registered", lookup: "beta", wantFound: true, wantName: "beta"},
		{name: "unknown", lookup: "gamma", wantFound: false},
		{name: "empty name", lookup: "", wantFound: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := r.GetProvider(tc.lookup)
			if ok != tc.wantFound {
				t.Fatalf("ok = %v, want %v", ok, tc.wantFound)
			}
			if tc.wantFound && got.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", got.Name(), tc.wantName)
			}
			if !tc.wantFound && got != nil {
				t.Errorf("provider = %v, want nil", got)
			}
		})
	}
}

func TestRouterSearch(t *testing.T) {
	t.Parallel()

	alphaResults := []Result{{URL: "https://alpha.example/1", Title: "Alpha One"}}
	betaResults := []Result{
		{URL: "https://beta.example/1", Title: "Beta One"},
		{URL: "https://beta.example/2", Title: "Beta Two"},
	}

	tests := []struct {
		name          string
		providers     []testProvider
		blocked       []string
		strategy      string
		preferred     []string
		wantProvider  string
		wantResults   []Result
		wantErr       string // exact error message; empty means success
		wantErrIs     error
		wantRecordUse map[string]int
	}{
		{
			name:    "no providers registered",
			wantErr: "no search providers registered",
		},
		{
			name:          "succeeds with the mock provider",
			providers:     []testProvider{{name: "alpha", results: alphaResults}},
			strategy:      "auto",
			wantProvider:  "alpha",
			wantResults:   alphaResults,
			wantRecordUse: map[string]int{"alpha": 1},
		},
		{
			name: "falls back to a working provider",
			providers: []testProvider{
				{name: "broken", err: errSearchDown},
				{name: "beta", results: betaResults},
			},
			wantProvider:  "beta",
			wantResults:   betaResults,
			wantRecordUse: map[string]int{"broken": 0, "beta": 1},
		},
		{
			name: "skips providers without remaining quota",
			providers: []testProvider{
				{name: "alpha", results: alphaResults},
				{name: "beta", results: betaResults},
			},
			blocked:       []string{"alpha"},
			strategy:      "manual",
			preferred:     []string{"alpha", "beta"},
			wantProvider:  "beta",
			wantResults:   betaResults,
			wantRecordUse: map[string]int{"alpha": 0, "beta": 1},
		},
		{
			name:      "wraps the last error when every provider fails",
			providers: []testProvider{{name: "alpha", err: errSearchDown}},
			wantErr:   "all providers failed, last error: " + errSearchDown.Error(),
			wantErrIs: errSearchDown,
		},
		{
			name:      "errors when quota removes every candidate",
			providers: []testProvider{{name: "alpha", results: alphaResults}},
			blocked:   []string{"alpha"},
			wantErr:   "no providers available (all exhausted quota or none configured)",
		},
		{
			name:      "errors for manual strategy with unknown providers",
			providers: []testProvider{{name: "alpha", results: alphaResults}},
			strategy:  "manual",
			preferred: []string{"ghost"},
			wantErr:   "no providers available",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			store.block(tc.blocked...)
			r, _ := buildRouter(store, tc.providers...)

			got, err := r.Search("test query", 5, tc.strategy, tc.preferred)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Search() = %+v, want error %q", got, tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Errorf("Search() error = %q, want %q", err.Error(), tc.wantErr)
				}
				if tc.wantErrIs != nil && !errors.Is(err, tc.wantErrIs) {
					t.Errorf("Search() error does not wrap %v", tc.wantErrIs)
				}
				if got != nil {
					t.Errorf("Search() result = %+v, want nil on error", got)
				}
			} else {
				if err != nil {
					t.Fatalf("Search() error = %v, want nil", err)
				}
				if got.Provider != tc.wantProvider {
					t.Errorf("Search() provider = %q, want %q", got.Provider, tc.wantProvider)
				}
				if !reflect.DeepEqual(got.Results, tc.wantResults) {
					t.Errorf("Search() results = %+v, want %+v", got.Results, tc.wantResults)
				}
			}

			for _, spec := range tc.providers {
				want := tc.wantRecordUse[spec.name]
				if n := store.recordUseCount(spec.name); n != want {
					t.Errorf("RecordUse(%q) called %d times, want %d", spec.name, n, want)
				}
			}
		})
	}
}

// TestRouterSearchRecordsUseAndForwardsArgs covers the successful path in detail:
// the query and limit reach the provider and RecordUse runs exactly once.
func TestRouterSearchRecordsUseAndForwardsArgs(t *testing.T) {
	t.Parallel()

	store := newMockQuotaStore()
	wantResults := []Result{{URL: "https://alpha.example/1", Title: "Alpha One"}}
	r, mocks := buildRouter(store, testProvider{name: "alpha", results: wantResults})

	got, err := r.Search("golang table tests", 7, "auto", nil)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if got.Provider != "alpha" {
		t.Errorf("Search() provider = %q, want %q", got.Provider, "alpha")
	}
	if !reflect.DeepEqual(got.Results, wantResults) {
		t.Errorf("Search() results = %+v, want %+v", got.Results, wantResults)
	}
	if n := store.recordUseCount("alpha"); n != 1 {
		t.Errorf("RecordUse(alpha) called %d times, want 1", n)
	}
	if n := mocks["alpha"].callCount(); n != 1 {
		t.Errorf("provider Search called %d times, want 1", n)
	}
	query, limit := mocks["alpha"].recordedQuery()
	if query != "golang table tests" || limit != 7 {
		t.Errorf("provider received (%q, %d), want (%q, %d)", query, limit, "golang table tests", 7)
	}
}

// TestRouterSearchAllProvidersFail locks the multi-provider failure contract:
// the returned error wraps exactly one provider error (the last one tried).
func TestRouterSearchAllProvidersFail(t *testing.T) {
	t.Parallel()

	errA := errors.New("provider alpha down")
	errB := errors.New("provider beta down")
	store := newMockQuotaStore()
	r, _ := buildRouter(store,
		testProvider{name: "alpha", err: errA},
		testProvider{name: "beta", err: errB},
	)

	got, err := r.Search("q", 3, "auto", nil)
	if err == nil {
		t.Fatalf("Search() = %+v, want error", got)
	}
	if !strings.HasPrefix(err.Error(), "all providers failed, last error: ") {
		t.Errorf("Search() error = %q, want all-providers-failed prefix", err.Error())
	}
	if errors.Is(err, errA) == errors.Is(err, errB) {
		t.Errorf("Search() error should wrap exactly one provider error: %v", err)
	}
	if n := store.recordUseCount("alpha") + store.recordUseCount("beta"); n != 0 {
		t.Errorf("RecordUse called %d times for failed providers, want 0", n)
	}
}

func TestRouterSelectProviders(t *testing.T) {
	t.Parallel()

	qualityInfo := &ProviderInfo{Tags: []string{"quality"}}
	freeInfo := &ProviderInfo{Tags: []string{"free"}}

	tests := []struct {
		name      string
		providers []testProvider
		usage     map[string][2]int // provider -> {used, total}
		strategy  string
		preferred []string
		want      []string
		anyOrder  bool // compare as a set (map iteration order leaks through)
	}{
		{
			name:      "manual returns preferred order verbatim",
			providers: []testProvider{{name: "alpha"}, {name: "beta"}},
			strategy:  "manual",
			preferred: []string{"beta", "alpha"},
			want:      []string{"beta", "alpha"},
		},
		{
			name:      "manual is case-insensitive",
			providers: []testProvider{{name: "alpha"}},
			strategy:  "MANUAL",
			preferred: []string{"alpha"},
			want:      []string{"alpha"},
		},
		{
			name:      "manual with no preferred providers returns nothing",
			providers: []testProvider{{name: "alpha"}},
			strategy:  "manual",
			want:      nil,
		},
		{
			name: "quality keeps only quality-tagged providers",
			providers: []testProvider{
				{name: "top", info: qualityInfo},
				{name: "freebie", info: freeInfo},
				{name: "plain"},
			},
			strategy: "quality",
			want:     []string{"top"},
		},
		{
			name: "quality falls back to every provider when none is tagged",
			providers: []testProvider{
				{name: "alpha", info: freeInfo},
				{name: "beta"},
			},
			strategy: "quality",
			want:     []string{"alpha", "beta"},
			anyOrder: true,
		},
		{
			name: "cheap keeps only free-tagged providers",
			providers: []testProvider{
				{name: "top", info: qualityInfo},
				{name: "freebie", info: freeInfo},
				{name: "plain"},
			},
			strategy: "cheap",
			want:     []string{"freebie"},
		},
		{
			name: "cheap falls back to every provider when none is free",
			providers: []testProvider{
				{name: "alpha", info: qualityInfo},
				{name: "beta"},
			},
			strategy: "cheap",
			want:     []string{"alpha", "beta"},
			anyOrder: true,
		},
		{
			name: "auto orders free providers by remaining quota then appends non-free quality",
			providers: []testProvider{
				{name: "bigFree", info: freeInfo},
				{name: "smallFree", info: freeInfo},
				{name: "unlimitedFree", info: freeInfo},
				{name: "dual", info: &ProviderInfo{Tags: []string{"free", "quality"}}},
				{name: "qualityOnly", info: qualityInfo},
			},
			usage: map[string][2]int{
				"bigFree":       {0, 1000}, // remaining 1000
				"smallFree":     {5, 10},   // remaining 5
				"unlimitedFree": {12, 0},   // remaining 999 (unlimited)
				"dual":          {99, 100}, // remaining 1
			},
			strategy: "auto",
			want:     []string{"dual", "smallFree", "unlimitedFree", "bigFree", "qualityOnly"},
		},
		{
			name: "auto returns every provider when no tags are configured",
			providers: []testProvider{
				{name: "alpha"},
				{name: "beta"},
			},
			strategy: "auto",
			want:     []string{"alpha", "beta"},
			anyOrder: true,
		},
		{
			name: "auto with only quality providers returns them",
			providers: []testProvider{
				{name: "alpha", info: qualityInfo},
				{name: "beta", info: qualityInfo},
			},
			strategy: "auto",
			want:     []string{"alpha", "beta"},
			anyOrder: true,
		},
		{
			name: "empty strategy behaves like auto",
			providers: []testProvider{
				{name: "freebie", info: freeInfo},
				{name: "top", info: qualityInfo},
			},
			strategy: "",
			want:     []string{"freebie", "top"},
		},
		{
			name: "unknown strategy behaves like auto",
			providers: []testProvider{
				{name: "freebie", info: freeInfo},
				{name: "top", info: qualityInfo},
			},
			strategy: "bogus",
			want:     []string{"freebie", "top"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			for name, usage := range tc.usage {
				store.setUsage(name, usage[0], usage[1])
			}
			r, _ := buildRouter(store, tc.providers...)

			got := r.selectProviders(tc.strategy, tc.preferred)

			if tc.anyOrder {
				if !sameElements(got, tc.want) {
					t.Errorf("selectProviders(%q) = %v, want same elements as %v", tc.strategy, got, tc.want)
				}
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selectProviders(%q) = %v, want %v", tc.strategy, got, tc.want)
			}
		})
	}
}

func TestRouterFilterAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []string
		blocked    []string
		want       []string
	}{
		{
			name:       "keeps usable candidates in order",
			candidates: []string{"a", "b", "c"},
			want:       []string{"a", "b", "c"},
		},
		{
			name:       "drops providers without quota",
			candidates: []string{"a", "b", "c"},
			blocked:    []string{"b"},
			want:       []string{"a", "c"},
		},
		{
			name:       "returns nothing when all are exhausted",
			candidates: []string{"a", "b"},
			blocked:    []string{"a", "b"},
			want:       nil,
		},
		{
			name:       "empty input",
			candidates: nil,
			want:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			store.block(tc.blocked...)
			r := NewRouter(store)

			got := r.filterAvailable(tc.candidates)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("filterAvailable(%v) = %v, want %v", tc.candidates, got, tc.want)
			}
			for _, name := range tc.candidates {
				if n := store.canUseCount(name); n != 1 {
					t.Errorf("CanUse(%q) called %d times, want 1", name, n)
				}
			}
		})
	}
}

func TestRouterSortByWeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		candidates []string
		configs    map[string]*ProviderInfo
	}{
		{name: "empty candidates", candidates: nil},
		{name: "single candidate", candidates: []string{"only"}},
		{name: "no configs uses equal weight", candidates: []string{"alpha", "beta", "gamma"}},
		{
			name:       "configured weights including zero fallback",
			candidates: []string{"alpha", "beta", "gamma"},
			configs: map[string]*ProviderInfo{
				"alpha": {Weight: 7},
				"beta":  {Weight: 0}, // falls back to weight 1
				"gamma": {Weight: 3},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			r := NewRouter(store)
			for name, info := range tc.configs {
				r.Register(name, &mockProvider{name: name}, info)
			}
			for _, name := range tc.candidates {
				if _, ok := r.GetProvider(name); !ok {
					r.Register(name, &mockProvider{name: name}, nil)
				}
			}

			// Weighted selection is random; assert the invariants over many runs.
			for i := 0; i < 20; i++ {
				got := r.sortByWeight(tc.candidates)
				if !sameElements(got, tc.candidates) {
					t.Fatalf("sortByWeight(%v) run %d = %v, want a permutation", tc.candidates, i, got)
				}
			}
		})
	}
}

// TestRouterSortByWeightPrefersHigherWeight asserts the weighted draw is biased:
// with weights 1000 vs 1, the light provider coming first is practically impossible.
func TestRouterSortByWeightPrefersHigherWeight(t *testing.T) {
	t.Parallel()

	store := newMockQuotaStore()
	r := NewRouter(store)
	r.Register("heavy", &mockProvider{name: "heavy"}, &ProviderInfo{Weight: 1000})
	r.Register("light", &mockProvider{name: "light"}, &ProviderInfo{Weight: 1})

	const rounds = 200
	heavyFirst := 0
	for i := 0; i < rounds; i++ {
		got := r.sortByWeight([]string{"heavy", "light"})
		if !sameElements(got, []string{"heavy", "light"}) {
			t.Fatalf("sortByWeight returned %v, want both providers", got)
		}
		if got[0] == "heavy" {
			heavyFirst++
		}
	}

	// P(light first) = 1/1001 per round; a light majority over 200 rounds is negligible.
	if heavyFirst <= rounds/2 {
		t.Errorf("heavy provider first %d/%d times, want a clear majority", heavyFirst, rounds)
	}
}

func TestRouterAllProviders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers []testProvider
		want      []string
	}{
		{name: "empty router", want: nil},
		{
			name:      "single provider",
			providers: []testProvider{{name: "alpha"}},
			want:      []string{"alpha"},
		},
		{
			name: "returns configured and unconfigured providers",
			providers: []testProvider{
				{name: "alpha", info: &ProviderInfo{Tags: []string{"free"}}},
				{name: "beta"},
				{name: "gamma", info: &ProviderInfo{Weight: 2}},
			},
			want: []string{"alpha", "beta", "gamma"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			r, _ := buildRouter(store, tc.providers...)

			got := r.allProviders()
			if !sameElements(got, tc.want) {
				t.Errorf("allProviders() = %v, want same elements as %v", got, tc.want)
			}
		})
	}
}

func TestRouterFilterByTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		providers []testProvider
		tag       string
		want      []string
	}{
		{
			name:      "matches one provider",
			providers: []testProvider{{name: "alpha", info: &ProviderInfo{Tags: []string{"quality"}}}},
			tag:       "quality",
			want:      []string{"alpha"},
		},
		{
			name: "matches multiple providers across configs",
			providers: []testProvider{
				{name: "alpha", info: &ProviderInfo{Tags: []string{"free", "quality"}}},
				{name: "beta", info: &ProviderInfo{Tags: []string{"free"}}},
				{name: "gamma", info: &ProviderInfo{Tags: []string{"quality"}}},
				{name: "plain"},
			},
			tag:  "free",
			want: []string{"alpha", "beta"},
		},
		{
			name:      "no match returns nothing",
			providers: []testProvider{{name: "alpha", info: &ProviderInfo{Tags: []string{"quality"}}}},
			tag:       "free",
			want:      nil,
		},
		{
			name:      "tag match is case-sensitive",
			providers: []testProvider{{name: "alpha", info: &ProviderInfo{Tags: []string{"Quality"}}}},
			tag:       "quality",
			want:      nil,
		},
		{
			name:      "provider without config never matches",
			providers: []testProvider{{name: "plain"}},
			tag:       "quality",
			want:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			r, _ := buildRouter(store, tc.providers...)

			got := r.filterByTag(tc.tag)
			if !sameElements(got, tc.want) {
				t.Errorf("filterByTag(%q) = %v, want same elements as %v", tc.tag, got, tc.want)
			}
		})
	}
}

func TestRouterQuotaRemaining(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		used      int
		total     int
		hasRecord bool
		want      int
	}{
		{name: "unlimited when total is zero", hasRecord: true, want: 999},
		{name: "unlimited when total is negative", used: 5, total: -1, hasRecord: true, want: 999},
		{name: "ignores used when unlimited", used: 42, total: 0, hasRecord: true, want: 999},
		{name: "unknown provider is unlimited", want: 999},
		{name: "limited returns remaining quota", used: 3, total: 10, hasRecord: true, want: 7},
		{name: "exhausted returns zero", used: 10, total: 10, hasRecord: true, want: 0},
		{name: "over-used returns negative", used: 12, total: 10, hasRecord: true, want: -2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMockQuotaStore()
			if tc.hasRecord {
				store.setUsage("alpha", tc.used, tc.total)
			}
			r := NewRouter(store)

			if got := r.quotaRemaining("alpha"); got != tc.want {
				t.Errorf("quotaRemaining(alpha) = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRouterConcurrentAccess exercises Register/GetProvider/Search under the
// router mutex; run with -race to catch unsynchronized map access.
func TestRouterConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := newMockQuotaStore()
	results := []Result{{URL: "https://alpha.example/1", Title: "Alpha One"}}
	r, _ := buildRouter(store, testProvider{name: "alpha", results: results})

	const workers = 8
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("dynamic-%d", id)
			for j := 0; j < 25; j++ {
				r.Register(name, &mockProvider{name: name}, nil)
				if _, ok := r.GetProvider(name); !ok {
					t.Errorf("GetProvider(%q) = false after Register", name)
					return
				}
			}
		}(i)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				got, err := r.Search("concurrent", 1, "manual", []string{"alpha"})
				if err != nil {
					t.Errorf("Search() error = %v", err)
					return
				}
				if got.Provider != "alpha" {
					t.Errorf("Search() provider = %q, want %q", got.Provider, "alpha")
					return
				}
			}
		}()
	}
	wg.Wait()

	if n := store.recordUseCount("alpha"); n != workers*25 {
		t.Errorf("RecordUse(alpha) called %d times, want %d", n, workers*25)
	}
}
