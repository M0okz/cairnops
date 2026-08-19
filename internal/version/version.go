package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}

func Info() BuildInfo {
	return BuildInfo{Version: Version, Commit: Commit, BuildDate: BuildDate}
}
