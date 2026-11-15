package protocol

import (
	"strings"
	"testing"
)

func TestServiceCatalogDescriptorContract(t *testing.T) {
	valid := `{"workingDirectory":"/app","install":{"mode":"nodejs","packageManager":"npm"},"startup":{"mode":"node-file","entrypoint":"bot.js"},"profileId":"dbm","runtimeVersion":"22"}`
	value, err := DecodeServiceInstallCatalog([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if value.ProfileID == nil || *value.ProfileID != "dbm" {
		t.Fatal("profile was not retained")
	}
	for _, entry := range []string{"../bot.js", "/bot.js", "bot.js;whoami", "$(whoami).js"} {
		if _, err := DecodeServiceInstallCatalog([]byte(strings.Replace(valid, "bot.js", entry, 1))); err == nil {
			t.Fatalf("accepted unsafe entrypoint %q", entry)
		}
	}
	if _, err := DecodeServiceInstallCatalog([]byte(strings.Replace(valid, `"profileId":"dbm"`, `"TOKEN":"secret"`, 1))); err == nil {
		t.Fatal("accepted secret value in descriptor")
	}
	if _, err := DecodeServiceInstallCatalog([]byte(valid + `{}`)); err == nil {
		t.Fatal("accepted trailing data")
	}
	if _, err := DecodeServiceInstallCatalog([]byte(strings.Replace(valid, `"workingDirectory":"/app"`, `"workingDirectory":"/app","workingDirectory":"/tmp"`, 1))); err == nil {
		t.Fatal("accepted duplicate keys")
	}
}
func TestCatalogCapabilitiesDoNotAdvertiseExecutionOrSecretDelivery(t *testing.T) {
	for _, capability := range ServiceInstallationCapabilities() {
		if capability == "service.install.v1" || capability == "service.install.v2" || capability == "service.environment.secure.v1" {
			t.Fatalf("unimplemented capability advertised: %s", capability)
		}
	}
}
