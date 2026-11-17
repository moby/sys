package mountinfo

import "testing"

func TestInfoOptional(t *testing.T) {
	type idOK struct {
		id int
		ok bool
	}
	for _, tc := range []struct {
		optional      string
		peerGroup     idOK
		master        idOK
		propagateFrom idOK
		unbindable    bool
	}{
		{optional: ""},
		{optional: "shared:1", peerGroup: idOK{1, true}},
		// Make sure "shared:12" is not treated as "shared:1".
		{optional: "shared:12", peerGroup: idOK{12, true}},
		{optional: "master:3", master: idOK{3, true}},
		{optional: "master:3 propagate_from:5", master: idOK{3, true}, propagateFrom: idOK{5, true}},
		{optional: "shared:2 master:3", peerGroup: idOK{2, true}, master: idOK{3, true}},
		{optional: "unbindable", unbindable: true},
		// Malformed or unknown fields.
		{optional: "shared"},
		{optional: "shared:x"},
		{optional: "shared: master:1", master: idOK{1, true}},
		{optional: "sharedx:1 unbindablex foo:7"},
	} {
		i := Info{Optional: tc.optional}
		check := func(name string, got, want idOK) {
			if got != want {
				t.Errorf("%q: %s: got %+v, want %+v", tc.optional, name, got, want)
			}
		}
		id, ok := i.PeerGroup()
		check("PeerGroup", idOK{id, ok}, tc.peerGroup)
		id, ok = i.Master()
		check("Master", idOK{id, ok}, tc.master)
		id, ok = i.PropagateFrom()
		check("PropagateFrom", idOK{id, ok}, tc.propagateFrom)
		if got := i.Unbindable(); got != tc.unbindable {
			t.Errorf("%q: Unbindable: got %v, want %v", tc.optional, got, tc.unbindable)
		}
	}
}
