package main

import (
	"os"
	"path/filepath"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestValidateProfile(t *testing.T) {
 valid := Profile{ID:"test",Name:"Home",Region:"AUTO",LocalTCP:"127.0.0.1:19132",LocalUDPPort:47334}
 if err:=validateProfile(valid); err!=nil { t.Fatalf("valid profile rejected: %v",err) }
 cases:=[]Profile{
  {Name:"",LocalTCP:"127.0.0.1:19132",LocalUDPPort:47334},
  {Name:"bad tcp",LocalTCP:"localhost",LocalUDPPort:47334},
  {Name:"bad tcp port",LocalTCP:"127.0.0.1:70000",LocalUDPPort:47334},
  {Name:"bad udp",LocalTCP:"127.0.0.1:19132",LocalUDPPort:0},
  {Name:"bad ip",LocalTCP:"127.0.0.1:19132",LocalUDPIP:"not-an-ip",LocalUDPPort:47334},
 }
 for _,p:=range cases { if err:=validateProfile(p); err==nil { t.Errorf("expected invalid profile to fail: %+v",p) } }
}

func TestProfileArgs(t *testing.T) {
 p:=Profile{Region:"UK",LocalTCP:"127.0.0.1:19132",LocalUDPIP:"192.168.1.4",LocalUDPPort:47334}
 got:=profileArgs(p)
 want:=[]string{"-local-http","127.0.0.1:19132","-local-udp-port","47334","-node","UK","-local-udp-ip","192.168.1.4"}
 if len(got)!=len(want) { t.Fatalf("args length mismatch: %#v",got) }
 for i:=range want { if got[i]!=want[i] { t.Errorf("args[%d]=%q want %q",i,got[i],want[i]) } }
}

func TestChecksumForAsset(t *testing.T) {
 manifest:="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  nethernet-client-linux-amd64\n"
 if got:=checksumForAsset(manifest,"nethernet-client-linux-amd64");got=="" { t.Fatal("expected checksum entry") }
 if got:=checksumForAsset(manifest,"other");got!="" { t.Fatalf("unexpected checksum %q",got) }
}

func TestCleanLogLineRedactsToken(t *testing.T) {
 got:=cleanLogLine("[Tunnel Client] token=supersecret authenticated tunnel established")
 if got=="" { t.Fatal("expected relevant log line") }
 if got=="[Tunnel Client] token=supersecret authenticated tunnel established" { t.Fatal("token was not redacted") }
}

func TestResolveEngineFindsUpdaterInstalledAsset(t *testing.T) {
	root := t.TempDir()
	enginePath := filepath.Join(root, "bin", engineAssetName())
	if err := os.MkdirAll(filepath.Dir(enginePath), 0700); err != nil {
		t.Fatalf("create engine directory: %v", err)
	}
	if err := os.WriteFile(enginePath, []byte("test engine placeholder"), 0600); err != nil {
		t.Fatalf("create engine placeholder: %v", err)
	}

	service := &AppService{configDir: root}
	got := service.resolveEngine()
	if got != enginePath {
		t.Fatalf("resolveEngine() = %q, want updater-installed engine %q", got, enginePath)
	}
}


func TestCleanLogLineNormalizesMojibakeAndRedactsClaimCodes(t *testing.T) {
	input := "[Bedrock Node Tunnel] â€¢ Oceania (Sydney Hub): https://oceania.bedrocknode.net/?claim=3DD314"
	got := cleanLogLine(input)
	if strings.Contains(got, "â€¢") {
		t.Fatalf("mojibake bullet was not normalized: %q", got)
	}
	if !strings.Contains(got, "• Oceania") {
		t.Fatalf("expected normalized bullet and hub name, got %q", got)
	}
	if strings.Contains(got, "3DD314") {
		t.Fatalf("one-time claim code was not redacted: %q", got)
	}
	if !strings.Contains(got, "claim=[redacted]") {
		t.Fatalf("redacted claim marker missing: %q", got)
	}
}

func TestLoadConfigPreservesCorruptOriginalAndWritesPrivateDefaults(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "desktop.json")
	corrupt := []byte("{ not valid json")
	if err := os.WriteFile(configPath, corrupt, 0644); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	service := &AppService{configDir: root, configPath: configPath}
	service.loadConfig()

	backup, err := os.ReadFile(configPath + ".corrupt")
	if err != nil {
		t.Fatalf("read corrupt config backup: %v", err)
	}
	if string(backup) != string(corrupt) {
		t.Fatalf("corrupt backup = %q, want %q", backup, corrupt)
	}
	if len(service.GetProfiles()) != 1 {
		t.Fatalf("default profile count = %d, want 1", len(service.GetProfiles()))
	}
	if service.lastError == "" {
		t.Fatal("expected a warning about the preserved invalid config")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("stat recovered config: %v", err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("config permissions = %o, want 600", got)
		}
	}
}

func TestActiveProfileCannotBeEditedOrDeletedWhileTunnelRuns(t *testing.T) {
	root := t.TempDir()
	service := &AppService{
		configDir: root,
		configPath: filepath.Join(root, "desktop.json"),
		config: appConfig{
			SchemaVersion: 1,
			ActiveProfile: "default",
			Profiles: []Profile{
				defaultProfile(),
				{ID: "other", Name: "Other", Region: "AUTO", LocalTCP: "127.0.0.1:19133", LocalUDPPort: 47335},
			},
		},
		cmd: &exec.Cmd{},
	}
	updated := defaultProfile()
	updated.Name = "Changed while running"
	if _, err := service.SaveProfile(updated); err == nil {
		t.Fatal("editing active profile while running should fail")
	}
	if _, err := service.DeleteProfile("default"); err == nil {
		t.Fatal("deleting active profile while running should fail")
	}

	// New, inactive profiles may still be prepared while an existing tunnel runs.
	if _, err := service.SaveProfile(Profile{Name: "Next", Region: "AUTO", LocalTCP: "127.0.0.1:19134", LocalUDPPort: 47336}); err != nil {
		t.Fatalf("saving a new inactive profile while running: %v", err)
	}

	service.cmd = nil
	if _, err := service.DeleteProfile("default"); err != nil {
		t.Fatalf("deleting active profile while stopped: %v", err)
	}
	if got := service.config.ActiveProfile; got != "other" {
		t.Fatalf("active profile after delete = %q, want other", got)
	}
}
