package plugin

import "testing"

func TestCheckCompatibility_devCore(t *testing.T) {
	meta := PluginMetadata{
		Version:         "1.0.0",
		MinCoreVersion:  "9.9.9",
		ProtocolVersion: int32(HandshakeConfig.ProtocolVersion),
	}
	if err := CheckCompatibility("dev", meta); err != nil {
		t.Fatalf("dev core should skip semver: %v", err)
	}
}

func TestCheckCompatibility_versionMismatch(t *testing.T) {
	meta := PluginMetadata{
		Version:         "1.0.0",
		MinCoreVersion:  "2.0.0",
		ProtocolVersion: int32(HandshakeConfig.ProtocolVersion),
	}
	if err := CheckCompatibility("1.0.0", meta); err == nil {
		t.Fatal("expected compatibility error")
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("1.2.0", "1.10.0") >= 0 {
		t.Fatal("1.2.0 < 1.10.0")
	}
	if compareVersions("v2.0.0", "1.9.9") <= 0 {
		t.Fatal("2.0.0 > 1.9.9")
	}
}
