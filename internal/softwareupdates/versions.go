package softwareupdates

import (
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^v?([0-9]+(?:\.[0-9]+){1,3})(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func compareVersions(a, b string) (int, bool) {
	x, y := versionPattern.FindStringSubmatch(a), versionPattern.FindStringSubmatch(b)
	if x == nil || y == nil {
		return 0, false
	}
	xn, yn := strings.Split(x[1], "."), strings.Split(y[1], ".")
	for i := 0; i < 4; i++ {
		var l, r uint64
		var err error
		if i < len(xn) {
			l, err = strconv.ParseUint(xn[i], 10, 64)
			if err != nil {
				return 0, false
			}
		}
		if i < len(yn) {
			r, err = strconv.ParseUint(yn[i], 10, 64)
			if err != nil {
				return 0, false
			}
		}
		if l < r {
			return -1, true
		}
		if l > r {
			return 1, true
		}
	}
	if x[2] == y[2] {
		return 0, true
	}
	if x[2] == "" {
		return 1, true
	}
	if y[2] == "" {
		return -1, true
	}
	xp, yp := strings.Split(x[2], "."), strings.Split(y[2], ".")
	for i := 0; i < len(xp) && i < len(yp); i++ {
		if xp[i] == yp[i] {
			continue
		}
		l, le := strconv.ParseUint(xp[i], 10, 64)
		r, re := strconv.ParseUint(yp[i], 10, 64)
		if le == nil && re == nil {
			if l < r {
				return -1, true
			}
			return 1, true
		}
		if le == nil {
			return -1, true
		}
		if re == nil {
			return 1, true
		}
		if xp[i] < yp[i] {
			return -1, true
		}
		return 1, true
	}
	if len(xp) < len(yp) {
		return -1, true
	}
	return 1, true
}
func inRange(v, installed, target string) bool {
	a, ok := compareVersions(v, installed)
	b, ok2 := compareVersions(v, target)
	if !ok || !ok2 || a <= 0 || b > 0 {
		return false
	}
	return versionPattern.FindStringSubmatch(v)[2] == "" || versionPattern.FindStringSubmatch(target)[2] != ""
}
