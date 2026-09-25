package softwareupdates

import (
	"regexp"

	"github.com/M0okz/cairnops/internal/versions"
)

// versionPattern limite les catalogues aux étiquettes de publication
// explicites ; l'ordre lui-même est établi par le paquet versions.
var versionPattern = regexp.MustCompile(`^v?([0-9]+(?:\.[0-9]+){1,3})(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func compareVersions(a, b string) (int, bool) {
	return versions.Compare(a, b)
}
func inRange(v, installed, target string) bool {
	a, ok := compareVersions(v, installed)
	b, ok2 := compareVersions(v, target)
	if !ok || !ok2 || a <= 0 || b > 0 {
		return false
	}
	return !versions.IsPrerelease(v) || versions.IsPrerelease(target)
}
