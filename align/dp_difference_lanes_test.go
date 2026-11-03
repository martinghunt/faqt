//go:build goexperiment.simd && (arm64 || amd64)

package align

import (
	"simd/archsimd"
	"testing"
)

func TestDifferenceVectorLanes(t *testing.T) {
	if !differenceAvailable() {
		t.Skip("SIMD unavailable on this CPU")
	}
	values := [16]int8{-128, -127, -93, -1, 0, 1, 31, 63, 64, 65, 90, 100, 120, 125, 126, 127}
	v := archsimd.LoadInt8x16Array(&values)
	for group, fn := range []func(archsimd.Int8x16) archsimd.Int32x4{differenceWiden0, differenceWiden4, differenceWiden8, differenceWiden12} {
		var got [4]int32
		fn(v).StoreArray(&got)
		for lane := range got {
			if got[lane] != int32(values[group*4+lane]) {
				t.Fatalf("group %d lane %d: got %d want %d", group, lane, got[lane], values[group*4+lane])
			}
		}
	}
}
