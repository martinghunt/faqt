//go:build goexperiment.simd && arm64

package align

import (
	"context"
	"simd/archsimd"
)

func differenceAvailable() bool     { return true }
func dpArchitectureBackend() string { return "neon16" }
func differenceFallback(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	return alignDPWideBoundary(ctx, q, t, band, extend, o, boundary)
}
func differenceWiden0(v archsimd.Int8x16) archsimd.Int32x4 {
	return v.ExtendLo8ToInt16().ExtendLo4ToInt32()
}
func differenceWiden4(v archsimd.Int8x16) archsimd.Int32x4 {
	return v.ExtendLo8ToInt16().HiToLo().ExtendLo4ToInt32()
}
func differenceWiden8(v archsimd.Int8x16) archsimd.Int32x4 {
	return v.HiToLo().ExtendLo8ToInt16().ExtendLo4ToInt32()
}
func differenceWiden12(v archsimd.Int8x16) archsimd.Int32x4 {
	return v.HiToLo().ExtendLo8ToInt16().HiToLo().ExtendLo4ToInt32()
}
