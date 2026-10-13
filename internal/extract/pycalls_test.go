package extract

import (
	"testing"

	"github.com/dobbo-ca/graphify-go/internal/idutil"
)

// pyCalls resolves receivers from one walk of the function: types and bound
// names come from the whole function, calls only from the body.
func TestPyCallsOneWalk(t *testing.T) {
	src := `import os
import json

class Client:
    pass

def run(c: Client, d=make(), e: Client = None):
    early.go()
    c.send()
    os.getcwd()
    json.dumps(1)
    plain()
    late.close()
    early = Client()
    late = Client()
    late = other()
    for json in []:
        json.loads()
    def inner(q: Client):
        q.ping()
`
	res := FileFromBytes("pkg/m.py", []byte(src))
	run := ""
	for _, d := range res.Defs {
		if d.Name == "run" {
			run = d.ID
		}
	}
	type call struct{ callee, recv, module, loc string }
	var got []call
	for i, n := range res.Calls {
		c := res.call(n)
		if c.CallerID == run || c.CallerID == idutil.MakeID(run, "inner") {
			mod := ""
			if i < len(res.CallMods) {
				mod = res.CallMods[i]
			}
			got = append(got, call{c.Callee, c.Recv, mod, c.Loc.String()})
		}
	}
	// make() in the parameter list is not a call of the body; late is bound
	// twice with different types so it stays untyped; json is rebound by the
	// for loop so it is no longer the module.
	want := []call{
		{"go", "Client", "", "L8"}, {"send", "Client", "", "L9"}, {"getcwd", "os", "os", "L10"},
		{"dumps", "json", "", "L11"}, {"plain", "", "", "L12"}, {"close", "late", "", "L13"},
		{"Client", "", "", "L14"}, {"Client", "", "", "L15"}, {"other", "", "", "L16"},
		{"loads", "json", "", "L18"}, {"ping", "Client", "", "L20"},
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}
