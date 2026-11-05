package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type Server struct {
	Host string `yaml:"Host" default:"localhost"`
	Port int    `yaml:"Port" default:"8080"`
}

type Config struct {
	Server Server `yaml:"Server"`
}

type floatingConfig struct {
	Sampler float64 `default:"1.0"`
}

func TestConfigLoad(t *testing.T) {
	var c Config
	MustLoad("./config_test.yaml", &c)
	t.Logf("config: %+v", c)
}

func TestDefaultsSupportFloat64(t *testing.T) {
	var c floatingConfig
	if err := setDefaults(&c); err != nil {
		t.Fatal(err)
	}
	if c.Sampler != 1.0 {
		t.Fatalf("Sampler = %v, want 1.0", c.Sampler)
	}
}

type level int

type scalars struct {
	Small    int8          `default:"-8"`
	Wide     int32         `default:"32"`
	Count    uint          `default:"7"`
	Bits     uint16        `default:"65535"`
	Ratio    float32       `default:"0.5"`
	Wait     time.Duration `default:"30s"`
	Nanos    time.Duration `default:"1500"`
	Level    level         `default:"3"`
	On       bool          `default:"true"`
	Kept     int           `default:"1"`
	unexport int           `default:"9"` //nolint:unused // left alone by the defaults
	Nested   struct {
		Name string `default:"inner"`
	}
}

// Every scalar kind takes a default, named types and time.Duration
// included; unhandled kinds used to end the process with a panic.
func TestDefaultsSupportEveryScalarKind(t *testing.T) {
	c := scalars{Kept: 42}
	if err := setDefaults(&c); err != nil {
		t.Fatal(err)
	}
	if c.Small != -8 || c.Wide != 32 || c.Count != 7 || c.Bits != 65535 || c.Ratio != 0.5 || c.On != true || c.Level != 3 {
		t.Fatalf("defaults = %+v", c)
	}
	if c.Wait != 30*time.Second || c.Nanos != 1500 {
		t.Fatalf("durations = %s, %s; want 30s and 1500ns", c.Wait, c.Nanos)
	}
	if c.Kept != 42 {
		t.Fatalf("Kept = %d, a set value was overwritten", c.Kept)
	}
	if c.Nested.Name != "inner" {
		t.Fatalf("Nested.Name = %q, want inner", c.Nested.Name)
	}
}

// A default that does not fit its field is reported, naming the field,
// instead of a panic or a silent zero.
func TestLoadReportsDefaultsThatDoNotFit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte("Name: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var sliced struct {
		Names []string `default:"a,b"`
	}
	if err := Load(path, &sliced); err == nil || !strings.Contains(err.Error(), "Names") {
		t.Fatalf("Load() with a slice default = %v, want the field reported", err)
	}
	var malformed struct {
		Port int `default:"eighty"`
	}
	if err := Load(path, &malformed); err == nil || !strings.Contains(err.Error(), "Port") {
		t.Fatalf("Load() with a malformed integer = %v, want the field reported", err)
	}
	var overflow struct {
		Tiny int8 `default:"300"`
	}
	if err := Load(path, &overflow); err == nil || !strings.Contains(err.Error(), "Tiny") {
		t.Fatalf("Load() with an overflowing default = %v, want the field reported", err)
	}
	if err := setDefaults(Config{}); err == nil {
		t.Fatal("setDefaults accepted a struct value instead of a pointer")
	}
	var good struct {
		Name string `yaml:"Name" default:"y"`
	}
	if err := Load(path, &good); err != nil || good.Name != "x" {
		t.Fatalf("Load() = %v, Name %q; want the file's value over the default", err, good.Name)
	}
}
