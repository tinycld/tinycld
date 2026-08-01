package pkgbuild

// Hooks for the external test package (pkgbuild_test), which must stay
// external so it can import pkgbuildtest without a test import cycle.
var (
	FetchMemberWith    = fetchMemberWith
	CopyScaffoldExtras = copyScaffoldExtras
)

type PackFn = packFn
