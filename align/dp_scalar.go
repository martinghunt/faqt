//go:build !goexperiment.simd || (!arm64 && !amd64)

package align

import "context"

func alignDPPlatform(ctx context.Context, q, t []byte, band int, extend bool, o Options) (dpResult, error) {
	return alignDPScalar(ctx, q, t, band, extend, o)
}

func dpArchitectureBackend() string { return "scalar" }

func alignDPPlatformBoundary(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	return alignDPScalarBoundary(ctx, q, t, band, extend, o, boundary)
}
