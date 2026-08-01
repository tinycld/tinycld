package jsvm

import "testing"

// Pins transformSource's esbuild output byte-for-byte, so an esbuild bump or
// an options change is a deliberate, reviewed event — hook files transpiled
// earlier must keep running exactly as freshly-transpiled ones do.
//
// This test once had a paired twin in the multi-org router (publish-time
// transpileForStore, internal/controlplane/transpile_golden_test.go); design
// §7 step 5 deleted the router-side member along with the package store —
// hosted packages are now transpiled inside the builder's workspace pipeline,
// not by the router — leaving this as the sole definition.

const goldenTSFixture = `interface Hook {
    name: string
}
const h: Hook = { name: 'golden' }
const picked = h?.name ?? 'fallback'
routerAdd('GET', '/golden', (e) => e.string(200, picked))
`

const goldenJSOutput = "const h = { name: \"golden\" };\nconst picked = h?.name ?? \"fallback\";\nrouterAdd(\"GET\", \"/golden\", (e) => e.string(200, picked));\n//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsiPHN0ZGluPiJdLAogICJzb3VyY2VzQ29udGVudCI6IFsiaW50ZXJmYWNlIEhvb2sge1xuICAgIG5hbWU6IHN0cmluZ1xufVxuY29uc3QgaDogSG9vayA9IHsgbmFtZTogJ2dvbGRlbicgfVxuY29uc3QgcGlja2VkID0gaD8ubmFtZSA/PyAnZmFsbGJhY2snXG5yb3V0ZXJBZGQoJ0dFVCcsICcvZ29sZGVuJywgKGUpID0+IGUuc3RyaW5nKDIwMCwgcGlja2VkKSlcbiJdLAogICJtYXBwaW5ncyI6ICJBQUdBLE1BQU0sSUFBVSxFQUFFLE1BQU0sU0FBUztBQUNqQyxNQUFNLFNBQVMsR0FBRyxRQUFRO0FBQzFCLFVBQVUsT0FBTyxXQUFXLENBQUMsTUFBTSxFQUFFLE9BQU8sS0FBSyxNQUFNLENBQUM7IiwKICAibmFtZXMiOiBbXQp9Cg==\n"

func TestTransformSource_MatchesGolden(t *testing.T) {
	got, err := transformSource("fixture.pb.ts", []byte(goldenTSFixture))
	if err != nil {
		t.Fatalf("transformSource: %v", err)
	}
	if string(got) != goldenJSOutput {
		t.Fatalf("transpile output diverged from the golden (did esbuild options or its version change?)\n got: %q\nwant: %q", got, goldenJSOutput)
	}
}
