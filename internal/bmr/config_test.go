package bmr

import (
	"strings"
	"testing"
)

func testConfigOptions() ConfigOptions {
	return ConfigOptions{
		OutputDir:   "/var/lib/hitstor/bmr",
		AgentBinary: "/usr/bin/hitstor-agent",
		HookScript:  "/usr/share/hitstor/bmr/recover.sh",
		ServerAddr:  "backup.example.com:8420",
	}
}

func TestRenderConfigWiresReaRToHitStor(t *testing.T) {
	got, err := RenderConfig(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}

	// ReaR must not attempt its own backup: HitStor already holds the data,
	// and ReaR only has to recreate the machine's shape and call us back.
	want := []string{
		`BACKUP=EXTERNAL`,
		`EXTERNAL_BACKUP=()`,
		`EXTERNAL_RESTORE=('/usr/share/hitstor/bmr/recover.sh')`,
		`OUTPUT=ISO`,
		`OUTPUT_URL=file:///var/lib/hitstor/bmr`,
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("config missing %q\n--- config ---\n%s", w, got)
		}
	}
}

func TestRenderConfigCopiesRecoveryToolingIntoMedia(t *testing.T) {
	got, err := RenderConfig(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}

	// Without the agent binary and the hook inside the rescue image, the
	// booted media has nothing to run.
	for _, w := range []string{"/usr/bin/hitstor-agent", "/usr/share/hitstor/bmr/recover.sh"} {
		if !strings.Contains(got, w) {
			t.Errorf("COPY_AS_IS missing %q\n--- config ---\n%s", w, got)
		}
	}
}

func TestRenderConfigEnablesRescueNetworking(t *testing.T) {
	got, err := RenderConfig(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}

	// The recovery agent calls home over the network, so the rescue system
	// must configure an address without an operator typing one in.
	if !strings.Contains(got, "USE_DHCLIENT=yes") {
		t.Errorf("config does not bring up rescue networking\n--- config ---\n%s", got)
	}
}

func TestRenderConfigCarriesNoSecrets(t *testing.T) {
	opts := testConfigOptions()
	got, err := RenderConfig(opts)
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}

	// Removable media must never become a repository-compromise vector.
	for _, forbidden := range []string{"api_key", "API_KEY", "token", "TOKEN", "password", "PASSWORD"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("config leaks credential material via %q\n--- config ---\n%s", forbidden, got)
		}
	}
}

func TestRenderConfigMigrationMode(t *testing.T) {
	opts := testConfigOptions()
	opts.MigrationMode = true

	got, err := RenderConfig(opts)
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if !strings.Contains(got, "MIGRATION_MODE=") {
		t.Errorf("MIGRATION_MODE not set for a P2V build\n--- config ---\n%s", got)
	}

	plain, err := RenderConfig(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if strings.Contains(plain, "MIGRATION_MODE=") {
		t.Errorf("MIGRATION_MODE set on a same-hardware build\n--- config ---\n%s", plain)
	}
}

func TestRenderConfigDefaultsUnsetPaths(t *testing.T) {
	// An operator running `hitstor bmr generate-config` with no flags should
	// get the packaged layout, not an error.
	got, err := RenderConfig(ConfigOptions{})
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	for _, w := range []string{DefaultOutputDir, DefaultAgentBinary, DefaultHookScript} {
		if !strings.Contains(got, w) {
			t.Errorf("config did not fall back to default %q\n--- config ---\n%s", w, got)
		}
	}
}

func TestRenderConfigRejectsRelativePaths(t *testing.T) {
	// ReaR runs from an unpredictable working directory, and the hook runs
	// inside the rescue system; a relative path would resolve somewhere else
	// or not at all, and only fail once a machine is already dead.
	tests := map[string]func(*ConfigOptions){
		"relative output dir":  func(o *ConfigOptions) { o.OutputDir = "bmr" },
		"relative agent":       func(o *ConfigOptions) { o.AgentBinary = "hitstor-agent" },
		"relative hook script": func(o *ConfigOptions) { o.HookScript = "recover.sh" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			opts := testConfigOptions()
			mutate(&opts)
			if _, err := RenderConfig(opts); err == nil {
				t.Fatal("expected an error, got none")
			}
			if _, err := RenderHookScript(opts); err == nil {
				t.Fatal("RenderHookScript: expected an error, got none")
			}
		})
	}
}

func TestRenderHookScriptPromptsRatherThanEmbeddingCredentials(t *testing.T) {
	got, err := RenderHookScript(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderHookScript: %v", err)
	}

	if !strings.HasPrefix(got, "#!/bin/bash\n") {
		t.Errorf("hook script is missing its interpreter line:\n%s", got)
	}
	// The recovery token is minted after the media is built and must be typed
	// in at recovery time, never baked into the image.
	if !strings.Contains(got, "read ") {
		t.Errorf("hook script never prompts for the recovery token\n--- script ---\n%s", got)
	}
	// The server address is a convenience default, not a secret.
	if !strings.Contains(got, "backup.example.com:8420") {
		t.Errorf("hook script does not prefill the known server address\n--- script ---\n%s", got)
	}
}

func TestRenderHookScriptRestoresIntoReaRMountpoint(t *testing.T) {
	got, err := RenderHookScript(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderHookScript: %v", err)
	}

	// ReaR has recreated the layout and mounted the new root at /mnt/local by
	// the time EXTERNAL_RESTORE runs; restoring anywhere else rebuilds the
	// rescue ramdisk instead of the machine.
	if !strings.Contains(got, RecoveryRoot) {
		t.Errorf("hook script does not target %s\n--- script ---\n%s", RecoveryRoot, got)
	}
	if !strings.Contains(got, "/usr/bin/hitstor-agent") {
		t.Errorf("hook script does not launch the recovery agent\n--- script ---\n%s", got)
	}
}

func TestRenderHookScriptFailsTheReaRRunOnRestoreFailure(t *testing.T) {
	got, err := RenderHookScript(testConfigOptions())
	if err != nil {
		t.Fatalf("RenderHookScript: %v", err)
	}

	// ReaR decides whether to install a bootloader from this script's exit
	// code. Swallowing a failure would produce a confidently broken machine.
	if !strings.Contains(got, "set -e") && !strings.Contains(got, "exit ") {
		t.Errorf("hook script does not propagate failure\n--- script ---\n%s", got)
	}
}
