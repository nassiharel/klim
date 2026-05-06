//go:build !windows

package detector

func detectPE(_ string) string {
	return ""
}

func resolveChocoShimPlatform(_ string) string {
	return ""
}
