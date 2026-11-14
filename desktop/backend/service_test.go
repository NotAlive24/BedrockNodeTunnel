package backend
import("strings";"testing")
func TestValidate(t *testing.T){v,e:=validate(Settings{});if e!=nil{t.Fatal(e)};if v.LocalTCP!="127.0.0.1:19132"||v.LocalUDPPort!=47334{t.Fatalf("%#v",v)};v.LocalTCP="127.0.0.1:70000";if _,e=validate(v);e==nil{t.Fatal("invalid port accepted")}}
func TestArgs(t *testing.T){v:=defaults();v.Node="ASIA";a:=strings.Join(clientArgs(v)," ");if !strings.Contains(a,"-node ASIA")||strings.Contains(a,"token"){t.Fatal(a)}}
func TestRedaction(t *testing.T){v:=sanitize("connected token=secret");if strings.Contains(v,"secret")||!strings.Contains(v,"[REDACTED]"){t.Fatal(v)}}
func TestUnit(t *testing.T){v:=renderUnit("/home/user path/nethernet-client",[]string{"-web","https://host/$X/%Y"});for _,s:=range []string{"/home/user path/nethernet-client","$$X","%%Y","Restart=on-failure"}{if !strings.Contains(v,s){t.Fatalf("missing %s",s)}}}

func TestStartupArguments(t *testing.T) {
	got := startupArguments("/opt/Nethernet Desktop/nethernet-desktop", true)
	if len(got) != 2 || got[0] != "/opt/Nethernet Desktop/nethernet-desktop" || got[1] != "--background" {
		t.Fatalf("unexpected startup arguments: %#v", got)
	}
	got = startupArguments("/opt/nethernet-desktop", false)
	if len(got) != 1 || got[0] != "/opt/nethernet-desktop" {
		t.Fatalf("unexpected normal startup arguments: %#v", got)
	}
}
