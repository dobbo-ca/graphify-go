package extract

import "testing"

// One walk over the file must attribute each call to the body it sits in, in
// source order, and ignore calls outside every registered body.
func TestJSCallsOneWalk(t *testing.T) {
	src := `import x from "./x";
top();
function a() { one(); x.two(() => three()); const m = require("m"); }
const b = () => four(), c = function () { five.six(); };
export class K {
  m() { this.seven(); }
  n() {}
  o() { eight(); }
}
between();
obj.p = () => { nine(); };
export default function d() { ten(); }
after.all();
`
	res := FileFromBytes("src/f.ts", []byte(src))
	type call struct{ caller, callee, recv, loc string }
	var got []call
	for _, n := range res.Calls {
		c := res.call(n)
		got = append(got, call{c.CallerID, c.Callee, c.Recv, c.Loc.String()})
	}
	want := []call{
		{"src_f_ts_a", "one", "", "L3"}, {"src_f_ts_a", "two", "x", "L3"}, {"src_f_ts_a", "three", "", "L3"}, {"src_f_ts_a", "require", "", "L3"},
		{"src_f_ts_b", "four", "", "L4"}, {"src_f_ts_c", "six", "five", "L4"},
		{"src_f_ts_k_m", "seven", "this", "L6"}, {"src_f_ts_k_o", "eight", "", "L8"},
		{"src_f_ts_p", "nine", "", "L11"}, {"src_f_ts_d", "ten", "", "L12"},
	}
	if len(got) != len(want) {
		t.Fatalf("calls = %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
	if len(res.Imps) != 2 || res.Imps[1].Spec != "m" {
		t.Errorf("imports = %v, want ./x then m", res.Imps)
	}
}
