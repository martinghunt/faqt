//go:build goexperiment.simd && amd64

package align

import (
	"context"
	"simd/archsimd"
)

// Keep all vector operations behind this runtime check. In particular the
// binary can be built with GOAMD64=v1 and safely use scalar DP on older CPUs.
func differenceAvailable() bool { return archsimd.X86.AVX2() }
func dpArchitectureBackend() string {
	if differenceAvailable() {
		return "avx2-16"
	}
	return "scalar"
}
func differenceFallback(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	return alignDPScalarBoundary(ctx, q, t, band, extend, o, boundary)
}
func differenceWiden0(v archsimd.Int8x16) archsimd.Int32x4 {
	return v.ExtendLo4ToInt32()
}
func differenceWiden4(v archsimd.Int8x16) archsimd.Int32x4 {
	var zero archsimd.Uint8x16
	return zero.ConcatShiftBytesRight(v.ToBits(), 4).BitsToInt8().ExtendLo4ToInt32()
}
func differenceWiden8(v archsimd.Int8x16) archsimd.Int32x4 {
	var zero archsimd.Uint8x16
	return zero.ConcatShiftBytesRight(v.ToBits(), 8).BitsToInt8().ExtendLo4ToInt32()
}
func differenceWiden12(v archsimd.Int8x16) archsimd.Int32x4 {
	var zero archsimd.Uint8x16
	return zero.ConcatShiftBytesRight(v.ToBits(), 12).BitsToInt8().ExtendLo4ToInt32()
}
