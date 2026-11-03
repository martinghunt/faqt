package align

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func FuzzDPPlatform(f *testing.F) {
	f.Add([]byte("ACGTACGT"), []byte("ACTGTACGT"), byte(4), byte(20), true)
	f.Add([]byte("NNNNACGT"), []byte("NNNNACGT"), byte(1), byte(0), false)
	f.Add([]byte("AAA"), []byte("TTTTTTTTTTTT"), byte(2), byte(1), true)
	f.Fuzz(func(t *testing.T, q, target []byte, band, drop byte, extend bool) {
		if len(q) > 96 || len(target) > 96 {
			t.Skip()
		}
		norm := func(s []byte) []byte {
			out := make([]byte, len(s))
			for i, c := range s {
				out[i] = "ACGTN"[int(c)%5]
			}
			return out
		}
		q, target = norm(q), norm(target)
		o := DefaultOptions()
		o.ZDrop = int(drop)
		a, ea := alignDPScalar(context.Background(), q, target, int(band)+1, extend, o)
		b, eb := alignDPPlatform(context.Background(), q, target, int(band)+1, extend, o)
		if fmt.Sprint(ea) != fmt.Sprint(eb) || !reflect.DeepEqual(a, b) {
			t.Fatalf("scalar=%+v %v platform=%+v %v", a, ea, b, eb)
		}
	})
}
