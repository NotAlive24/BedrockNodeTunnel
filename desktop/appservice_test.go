package main

import (
	"os"
	"path/filepath"
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

