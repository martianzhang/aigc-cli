package ideas

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// writeIdeasFixture writes an ideas.json fixture and returns its path.
func writeIdeasFixture(t *testing.T, entries []ideas.IdeaEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ideas.json")
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("cannot marshal ideas fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("cannot write ideas fixture: %v", err)
	}
	return path
}

func localDeps(dataPath string) Deps {
	return Deps{Cfg: &types.Config{Ideas: &types.IdeasConfig{DataPath: dataPath}}, OutputDir: filepath.Join(os.TempDir(), "aigc-ideas-test")}
}

func fixtureEntries() []ideas.IdeaEntry {
	return []ideas.IdeaEntry{
		{Title: "cat photo", Prompt: "a cute cat sitting on a mat", Lang: "en", ImageURLs: []string{"https://example.com/img/kitty.jpg"}},
		{Title: "dog photo", Prompt: "a happy dog running in the park", Lang: "en"},
	}
}

func TestRunLocalKeywordSearch(t *testing.T) {
	d := localDeps(writeIdeasFixture(t, fixtureEntries()))
	f := &cmdFlags{limit: 5, sources: []string{ideas.SourceLocal}}

	var runErr error
	output := captureStdout(func() {
		runErr = run(d, []string{"cat"}, f)
	})
	if runErr != nil {
		t.Fatalf("run() unexpected error: %v", runErr)
	}
	if !strings.Contains(output, "## cat photo") {
		t.Errorf("output missing the matching entry:\n%s", output)
	}
	if !strings.Contains(output, "Found 1 result(s)") {
		t.Errorf("output missing the result count:\n%s", output)
	}
}

func TestRunLocalRandomWithoutKeywords(t *testing.T) {
	d := localDeps(writeIdeasFixture(t, fixtureEntries()))
	f := &cmdFlags{limit: 5, sources: []string{ideas.SourceLocal}}

	var runErr error
	output := captureStdout(func() {
		runErr = run(d, nil, f)
	})
	if runErr != nil {
		t.Fatalf("run() unexpected error: %v", runErr)
	}
	if !strings.Contains(output, "随机灵感") || !strings.Contains(output, "(showing 1/2)") {
		t.Errorf("random output = %q, want one random local entry", output)
	}
}

func TestRunLocalFindImage(t *testing.T) {
	d := localDeps(writeIdeasFixture(t, fixtureEntries()))
	f := &cmdFlags{limit: 5, findImage: "kitty.jpg", sources: []string{ideas.SourceLocal}}

	var runErr error
	output := captureStdout(func() {
		runErr = run(d, nil, f)
	})
	if runErr != nil {
		t.Fatalf("run() unexpected error: %v", runErr)
	}
	if !strings.Contains(output, "## cat photo") || !strings.Contains(output, "图片: kitty.jpg") {
		t.Errorf("find-image output = %q, want the cat entry", output)
	}
}

func TestRunEmptyLocalDataset(t *testing.T) {
	d := localDeps(writeIdeasFixture(t, nil))
	f := &cmdFlags{limit: 5, sources: []string{ideas.SourceLocal}}

	var runErr error
	output := captureStdout(func() {
		runErr = run(d, []string{"cat"}, f)
	})
	if runErr != nil {
		t.Fatalf("run() unexpected error: %v", runErr)
	}
	if !strings.Contains(output, "ideas.json is empty") {
		t.Errorf("empty dataset output = %q, want the init hint", output)
	}
}

func TestRunUnknownSource(t *testing.T) {
	d := localDeps(writeIdeasFixture(t, fixtureEntries()))
	f := &cmdFlags{limit: 5, sources: []string{"duckduckgo"}}

	err := run(d, []string{"cat"}, f)
	if err == nil || !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("run() error = %v, want unknown source error", err)
	}
}

func TestRunLocalMissingFile(t *testing.T) {
	d := localDeps(filepath.Join(t.TempDir(), "missing", "ideas.json"))
	f := &cmdFlags{limit: 5, sources: []string{ideas.SourceLocal}}

	err := run(d, []string{"cat"}, f)
	if err == nil || !strings.Contains(err.Error(), "ideas init") {
		t.Fatalf("run() error = %v, want the ideas init hint", err)
	}
}

func TestSearchOnlineSourcesSkipsLocalOnly(t *testing.T) {
	sources, err := ideas.ResolveSources([]string{ideas.SourceLocal}, true)
	if err != nil {
		t.Fatalf("ResolveSources(local) unexpected error: %v", err)
	}
	if got := searchOnlineSources(sources, "cat", 5, false); got != nil {
		t.Fatalf("searchOnlineSources(local only) = %v, want nil", got)
	}
}

type failingSource struct{ name string }

func (s failingSource) Name() string { return s.name }

func (s failingSource) Search(context.Context, string, int) ([]ideas.IdeaEntry, error) {
	return nil, errors.New("boom")
}

func TestSearchOnlineSourcesWarnsOnlyWhenVerbose(t *testing.T) {
	sources := []ideas.Source{failingSource{name: ideas.SourceOpenArt}}

	quiet := captureStderr(func() {
		if got := searchOnlineSources(sources, "cat", 5, false); got != nil {
			t.Errorf("searchOnlineSources() = %v, want nil", got)
		}
	})
	if strings.Contains(quiet, "Warning") {
		t.Errorf("stderr = %q, want no warning without verbose", quiet)
	}

	loud := captureStderr(func() {
		searchOnlineSources(sources, "cat", 5, true)
	})
	if !strings.Contains(loud, "Warning") || !strings.Contains(loud, ideas.SourceOpenArt) {
		t.Errorf("stderr = %q, want a warning naming the source with verbose", loud)
	}
}
