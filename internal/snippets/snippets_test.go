package snippets

import (
	"reflect"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	out, err := Render("interface Vlan{{ vlan_id }}\n hostname {{ host|upper }}",
		map[string]string{"vlan_id": "222", "host": "sw1"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "interface Vlan222\n hostname SW1" {
		t.Fatalf("render = %q", out)
	}
}

func TestRenderConditionalAndMissing(t *testing.T) {
	// truthy non-empty string enables the block; missing var renders empty
	out, err := Render("{% if enabled %}up {{ extra }}{% endif %}done",
		map[string]string{"enabled": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "up done" {
		t.Fatalf("render = %q", out)
	}
}

func TestRenderSyntaxError(t *testing.T) {
	if _, err := Render("{{ unclosed", nil); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRequiredVars(t *testing.T) {
	body := "hostname {{ hostname }}\n" +
		"{% if enabled %}vlan {{ vlan_id|default:\"1\" }}{% endif %}\n" +
		"{% for iface in interfaces %}{{ iface.name }}{% endfor %}"
	got := RequiredVars(body)
	want := []string{"enabled", "hostname", "interfaces", "vlan_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RequiredVars = %v, want %v", got, want)
	}
}

func TestRequiredVarsStringLiteral(t *testing.T) {
	got := RequiredVars(`{{ x|default:"hello world" }}`)
	if !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("RequiredVars = %v, want [x]", got)
	}
}

func TestUnresolved(t *testing.T) {
	body := "{{ hostname }} {{ vlan_id }} {% if enabled %}x{% endif %}"
	got := Unresolved(body, map[string]string{"hostname": "sw1"})
	want := []string{"enabled", "vlan_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Unresolved = %v, want %v", got, want)
	}
	if u := Unresolved(body, map[string]string{"hostname": "h", "vlan_id": "1", "enabled": "1"}); len(u) != 0 {
		t.Fatalf("Unresolved (all present) = %v, want none", u)
	}
}

func TestBracketedWrap(t *testing.T) {
	w := BracketedWrap("conf t\nend")
	if !strings.HasPrefix(w, "\x1b[200~") || !strings.HasSuffix(w, "\x1b[201~") {
		t.Fatalf("bracketed wrap missing markers: %q", w)
	}
	if !strings.Contains(w, "conf t\nend") {
		t.Fatal("bracketed wrap dropped body")
	}
}
