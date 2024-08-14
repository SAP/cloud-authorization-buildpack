package common

var versionNumber = "dev"

func VersionNumber() string {
	return versionNumber
}

func SetVersion(version string) {
	versionNumber = version
}
