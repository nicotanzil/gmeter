package config

import (
	"strings"
	"testing"
)

func TestCSVStrictParsingAndMapping(t *testing.T) {
	source := "\ufeffFull Name,Note,user_id\r\n\"Alice, A\",\"first\r\nsecond\",external\r\nBob,\"a \"\"quote\"\"\",other\r\n"
	preview, err := PreviewCSV(source, "")
	if err != nil {
		t.Fatal(err)
	}
	if preview.RowCount != 2 || preview.Sample[0][0] != "Alice, A" || preview.Sample[0][1] != "first\nsecond" || preview.Sample[1][1] != `a "quote"` || preview.Columns["Full Name"] != "Full_Name" || preview.Columns["user_id"] != "data_user_id" {
		t.Fatalf("preview: %+v", preview)
	}
	p, err := CompileCSV(CSV{Content: source, Columns: map[string]string{"Full Name": "username", "Note": "note", "user_id": ""}})
	if err != nil || p.Rows[0]["username"] != "Alice, A" || len(p.Rows[0]) != 2 {
		t.Fatalf("compile: %+v %v", p, err)
	}
	if _, err := CompileCSV(CSV{Content: "name;item\nalice;tea", Delimiter: ";"}); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCSV(CSV{Content: "name\titem\nalice\ttea", Delimiter: "\t"}); err != nil {
		t.Fatal(err)
	}
	// Suggestions must remain valid and unique even after normalization/truncation.
	preview, err = PreviewCSV("a-b,a b,2x,"+strings.Repeat("x", 200)+"\na,b,c,d", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range preview.Columns {
		if !variableAllowed(name) {
			t.Fatalf("invalid suggestion %q", name)
		}
	}
	if preview.Columns["a-b"] == preview.Columns["a b"] {
		t.Fatal("duplicate suggestion")
	}
}

func TestCSVRejectsMalformedOrAmbiguousInputs(t *testing.T) {
	for name, c := range map[string]CSV{
		"empty": {}, "no data": {Content: "name\n"}, "empty header": {Content: ",b\na,b"}, "duplicate header": {Content: "a,a\n1,2"},
		"ragged": {Content: "a,b\n1"}, "quote": {Content: "a\n\"unfinished"}, "bare quote": {Content: "a\nb\"c"},
		"utf8": {Content: "a\n\xff"}, "delimiter": {Content: "a\nb", Delimiter: "xx"}, "newline delimiter": {Content: "a\nb", Delimiter: "\n"},
		"reserved": {Content: "user_id\nx"}, "bad name": {Content: "Full Name\nx"}, "unknown map": {Content: "a\nb", Columns: map[string]string{"other": "x"}},
		"duplicate targets": {Content: "a,b\nx,y", Columns: map[string]string{"a": "x", "b": "x"}}, "all ignored": {Content: "a\nb", Columns: map[string]string{"a": ""}},
		"sharing": {Content: "a\nb", Sharing: "fleet"}, "EOF": {Content: "a\nb", OnEOF: "ignore"}, "too many rows": {Content: "a\n" + strings.Repeat("x\n", MaxCSVRows+1)},
		"too many columns": {Content: strings.Repeat("a,", MaxCSVColumns) + "z\n"}, "too many cells": {Content: "a,b,c,d,e,f,g,h,i,j,k\n" + strings.Repeat("1,2,3,4,5,6,7,8,9,0,x\n", 9091)},
		"large": {Content: "a\n" + strings.Repeat("x", MaxConfigBytes)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := CompileCSV(c); err == nil {
				t.Fatal("invalid CSV accepted")
			}
		})
	}
}

func TestCSVConfigurationIsPortableAndRestrictedToLocalUsers(t *testing.T) {
	c := scenarioConfig()
	c.Mode = "users"
	c.Rate = 0
	c.Scenarios[0].CSV = &CSV{Content: "username,item\nalice,tea"}
	c.Scenarios[0].Steps[0].Body = "{{username}} {{item}}"
	p, err := c.Validate()
	if err != nil || !p.HasCSV() {
		t.Fatalf("CSV variables: %v", err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Mode = "rate"; c.Rate = 1 },
		func(c *Config) { c.Scenarios[0].Data = []map[string]string{{"a": "x"}} },
		func(c *Config) { c.Scenarios[0].CSV.Columns = map[string]string{"username": ""} },
		func(c *Config) {
			c.Scenarios[0].CSV.Content = "username,item\n" + strings.Repeat("\"\"", 270000) + ",x"
		},
	} {
		copy := c
		copy.Scenarios = append([]Scenario(nil), c.Scenarios...)
		csv := *c.Scenarios[0].CSV
		copy.Scenarios[0].CSV = &csv
		mutate(&copy)
		if _, err := copy.Validate(); err == nil {
			t.Fatal("invalid CSV configuration accepted")
		}
	}
}
