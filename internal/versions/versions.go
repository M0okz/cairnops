// Package versions ordonne les versions logicielles observées et qualifie
// l'écart entre une version installée et la version cible d'un suivi.
//
// CairnOps ne décide jamais qu'une mise à jour existe sur la seule différence
// de deux chaînes : une préversion, une cible en retard sur l'installation ou
// un identifiant de build ne constituent pas une mise à jour à appliquer.
package versions

import (
	"regexp"
	"strconv"
	"strings"
)

// Situation décrit la relation entre la version installée et la version cible.
type Situation string

const (
	// Current : les deux versions désignent la même publication.
	Current Situation = "current"
	// Update : la cible est plus récente et de même stabilité ou plus stable.
	Update Situation = "update"
	// Prerelease : la cible est une préversion proposée à une installation stable.
	Prerelease Situation = "prerelease"
	// TargetOlder : la cible est antérieure à la version installée.
	TargetOlder Situation = "target_older"
	// Unordered : les versions diffèrent mais ne peuvent pas être ordonnées.
	Unordered Situation = "unordered"
)

// Level indique l'ampleur d'une mise à jour d'après la première composante modifiée.
type Level string

const (
	Major Level = "major"
	Minor Level = "minor"
	Patch Level = "patch"
)

// Assessment est la qualification d'une comparaison.
type Assessment struct {
	Situation Situation
	Level     Level
}

// Actionable indique si la comparaison établit une mise à jour à appliquer.
func (a Assessment) Actionable() bool { return a.Situation == Update }

var pattern = regexp.MustCompile(`^[vV]?([0-9]+(?:\.[0-9]+){0,3})(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var commitHash = regexp.MustCompile(`^g?[0-9a-f]{7,40}$`)

type parsed struct {
	numbers    []uint64
	prerelease string
}

func parse(raw string) (parsed, bool) {
	match := pattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return parsed{}, false
	}
	parts := strings.Split(match[1], ".")
	numbers := make([]uint64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return parsed{}, false
		}
		numbers = append(numbers, value)
	}
	return parsed{numbers: numbers, prerelease: normalizedPrerelease(match[2])}, true
}

// normalizedPrerelease ignore un identifiant de commit : « 2024.10.22-7ca5933 »
// désigne le build d'une publication, pas une préversion qui la précède.
func normalizedPrerelease(value string) string {
	if commitHash.MatchString(value) {
		return ""
	}
	return value
}

// Valid indique si une version suit un format ordonnable.
func Valid(raw string) bool {
	_, ok := parse(raw)
	return ok
}

// IsPrerelease indique si une version reconnue porte un suffixe de préversion.
func IsPrerelease(raw string) bool {
	version, ok := parse(raw)
	return ok && version.prerelease != ""
}

// Compare ordonne deux versions reconnues selon la précédence SemVer, étendue
// à une à quatre composantes numériques. Le second résultat est faux lorsque
// l'une des versions ne suit pas un format ordonnable.
func Compare(left, right string) (int, bool) {
	a, okA := parse(left)
	b, okB := parse(right)
	if !okA || !okB {
		return 0, false
	}
	if order := compareNumbers(a.numbers, b.numbers); order != 0 {
		return order, true
	}
	return comparePrerelease(a.prerelease, b.prerelease), true
}

func compareNumbers(left, right []uint64) int {
	for index := 0; index < len(left) || index < len(right); index++ {
		var l, r uint64
		if index < len(left) {
			l = left[index]
		}
		if index < len(right) {
			r = right[index]
		}
		if l != r {
			if l < r {
				return -1
			}
			return 1
		}
	}
	return 0
}

func comparePrerelease(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	a, b := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < len(a) && index < len(b); index++ {
		if a[index] == b[index] {
			continue
		}
		l, errL := strconv.ParseUint(a[index], 10, 64)
		r, errR := strconv.ParseUint(b[index], 10, 64)
		switch {
		case errL == nil && errR == nil:
			if l < r {
				return -1
			}
			return 1
		case errL == nil:
			return -1
		case errR == nil:
			return 1
		case a[index] < b[index]:
			return -1
		default:
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

// Assess qualifie l'écart entre la version installée et la version cible.
func Assess(installed, target string) Assessment {
	installed, target = strings.TrimSpace(installed), strings.TrimSpace(target)
	if installed == target {
		return Assessment{Situation: Current}
	}
	order, ok := Compare(installed, target)
	if !ok {
		return Assessment{Situation: Unordered}
	}
	switch {
	case order == 0:
		return Assessment{Situation: Current}
	case order > 0:
		return Assessment{Situation: TargetOlder}
	case IsPrerelease(target) && !IsPrerelease(installed):
		return Assessment{Situation: Prerelease, Level: level(installed, target)}
	default:
		return Assessment{Situation: Update, Level: level(installed, target)}
	}
}

func level(installed, target string) Level {
	a, _ := parse(installed)
	b, _ := parse(target)
	for index := 0; index < 2; index++ {
		var l, r uint64
		if index < len(a.numbers) {
			l = a.numbers[index]
		}
		if index < len(b.numbers) {
			r = b.numbers[index]
		}
		if l != r {
			if index == 0 {
				return Major
			}
			return Minor
		}
	}
	return Patch
}
