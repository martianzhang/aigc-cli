package redact

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDictionaryEngine(t *testing.T) {
	cfg := &types.RedactConfig{Dictionary: []types.RedactDictionaryEntry{
		{Value: "北京云创智能科技有限公司", Type: "ORG"},
		{Value: "某某项目"},
	}}
	eng, err := NewDictionaryEngine(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Detect(context.Background(), NewDocument("t", "被告北京云创智能科技有限公司，项目某某项目，再次某某项目。"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 3 {
		t.Fatalf("spans = %+v", result.Spans)
	}
	if result.Spans[0].Type != "ORG" || result.Spans[1].Type != "SENSITIVE" {
		t.Fatalf("types = %+v", result.Spans)
	}
}

func TestDictionaryRejectsEmpty(t *testing.T) {
	if _, err := NewDictionaryEngine(&types.RedactConfig{Dictionary: []types.RedactDictionaryEntry{{Value: " "}}}, ""); err == nil {
		t.Fatal("expected empty value error")
	}
	if eng, err := NewDictionaryEngine(nil, ""); err != nil || eng != nil {
		t.Fatal("nil config should return nil engine")
	}
}

func TestDictionaryFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dict"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "dict", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("orgs.tsv", "# comment\r\nvalue\ttype\r\n北京云创智能科技有限公司\tORG\r\n某某项目\r\n")
	write("people.csv", "value,type\n\"北京云创,智能\",ORG\n李四,PERSON\n")
	write("extra.json", `{"entries":[{"value":"合同HT-2024-1","type":"CONTRACT_NO"}]}`)

	cfg := &types.RedactConfig{DictionaryFiles: []string{
		"dict/orgs.tsv", "dict/people.csv", "dict/extra.json",
	}}
	eng, err := NewDictionaryEngine(cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Detect(context.Background(), NewDocument("t",
		"北京云创智能科技有限公司，某某项目，北京云创,智能，李四，合同HT-2024-1"))
	if err != nil {
		t.Fatal(err)
	}
	typesSeen := map[string]bool{}
	for _, s := range result.Spans {
		typesSeen[s.Type] = true
	}
	for _, want := range []string{"ORG", "SENSITIVE", "PERSON", "CONTRACT_NO"} {
		if !typesSeen[want] {
			t.Fatalf("missing type %s in %+v", want, result.Spans)
		}
	}
}

func TestDictionaryFileErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
	}{
		{"bad.txt", "x"},
		{"bad.tsv", "\tORG\n"},
	}
	for _, c := range cases {
		path := filepath.Join(dir, c.name)
		if err := os.WriteFile(path, []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := NewDictionaryEngine(&types.RedactConfig{DictionaryFiles: []string{path}}, dir); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
	if _, err := NewDictionaryEngine(&types.RedactConfig{DictionaryFiles: []string{"missing.tsv"}}, dir); err == nil {
		t.Fatal("expected missing file error")
	}
}
