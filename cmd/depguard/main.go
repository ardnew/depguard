package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	depguard "github.com/OpenPeeDeeP/depguard/v2"
	"golang.org/x/tools/go/analysis/singlechecker"
	"gopkg.in/yaml.v3"
)

var configFileRE = regexp.MustCompile(`^\.?depguard\.(yaml|yml|json|toml)$`)

var (
	fileTypes = map[string]configurator{
		"toml": &tomlConfigurator{},
		"yaml": &yamlConfigurator{},
		"yml":  &yamlConfigurator{},
		"json": &jsonConfigurator{},
	}
)

func main() {
	settings, err := getSettings()
	if err != nil {
		fmt.Printf("Could not find or read configuration file: %s\nUsing default configuration\n", err)
		settings = &depguard.LinterSettings{}
	}
	analyzer, err := depguard.NewAnalyzer(settings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	singlechecker.Main(analyzer)
}

type configurator interface {
	parse(io.Reader) (*depguard.LinterSettings, error)
}

type jsonConfigurator struct{}

func (*jsonConfigurator) parse(r io.Reader) (*depguard.LinterSettings, error) {
	set := &depguard.LinterSettings{}
	err := json.NewDecoder(r).Decode(set)
	if err != nil {
		return nil, fmt.Errorf("could not parse json file: %w", err)
	}
	return set, nil
}

type tomlConfigurator struct{}

func (*tomlConfigurator) parse(r io.Reader) (*depguard.LinterSettings, error) {
	set := &depguard.LinterSettings{}
	_, err := toml.NewDecoder(r).Decode(set)
	if err != nil {
		return nil, fmt.Errorf("could not parse toml file: %w", err)
	}
	return set, nil
}

type yamlConfigurator struct{}

func (*yamlConfigurator) parse(r io.Reader) (*depguard.LinterSettings, error) {
	set := &depguard.LinterSettings{}
	err := yaml.NewDecoder(r).Decode(set)
	if err != nil {
		return nil, fmt.Errorf("could not parse yaml file: %w", err)
	}
	return set, nil
}

func getSettings() (*depguard.LinterSettings, error) {
	fy, f, ft, err := findFile(".")
	if errors.Is(err, fs.ErrNotExist) {
		arg := []string{"list", "-f", "{{.Root}}"}
		out, err := exec.Command("go", arg...).Output()
		if err != nil {
			return nil, err
		}
		fy, f, ft, err = findFile(string(out))
	}
	if err != nil {
		return nil, err
	}
	file, err := fy.Open(f)
	if err != nil {
		return nil, fmt.Errorf("could not open %s to read: %w", f, err)
	}
	defer file.Close()
	return ft.parse(file)
}

func caller() (name, f string, n int) {
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			name = fn.Name()
			f, n = fn.FileLine(pc)
		}
	}
	return
}

func findFile(path string) (fs.FS, string, configurator, error) {
	fsys := os.DirFS(path)
	cwd, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, "", nil, fmt.Errorf("fs.ReadDir(<%q>): %w", path, err)
	}
	for _, entry := range cwd {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		matches := configFileRE.FindStringSubmatch(name)
		if len(matches) != 2 {
			continue
		}
		return fsys, matches[0], fileTypes[matches[1]], nil
	}
	fn, fp, ln := caller()
	return nil, "", nil, &fs.PathError{
		Op: fmt.Sprintf("%s@%s:%d", fn, fp, ln),
		Path: path,
		Err: fs.ErrNotExist,
	}
}
