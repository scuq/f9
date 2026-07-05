package app

import "testing"

func TestParseOptionsKeyFileAgent(t *testing.T) {
	o, err := parseOptions(map[string]string{"useAgent": "false", "keyFile": "/k"})
	if err != nil {
		t.Fatal(err)
	}
	if o.UseAgent == nil || *o.UseAgent {
		t.Fatalf("useAgent: %+v", o.UseAgent)
	}
	if o.KeyFile == nil || *o.KeyFile != "/k" {
		t.Fatalf("keyFile: %+v", o.KeyFile)
	}
	if _, err := parseOptions(map[string]string{"useAgent": "maybe"}); err == nil {
		t.Fatal("useAgent=maybe should error")
	}
}
