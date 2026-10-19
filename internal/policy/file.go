package policy

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The shape of policy.toml. default.toml, next to this file, is the policy a
// new install starts with and documents every field.
type policyFile struct {
	Version        int                    `toml:"version"`
	ShellTools     []string               `toml:"shell_tools"`
	DefaultProfile string                 `toml:"default_profile"`
	UseForEveryone string                 `toml:"use_for_everyone"`
	Profiles       map[string]profileFile `toml:"profiles"`
}

type profileFile struct {
	Why        string     `toml:"why"`
	Otherwise  Decision   `toml:"otherwise"`
	Unreadable Decision   `toml:"unreadable"`
	Scratch    []string   `toml:"scratch"`
	Rules      []ruleFile `toml:"rules"`
}

type ruleFile struct {
	Name            string   `toml:"name"`
	Decision        Decision `toml:"decision"`
	Why             string   `toml:"why"`
	Tools           []string `toml:"tools"`
	RunsAny         []string `toml:"runs_any"`
	RunsOnly        []string `toml:"runs_only"`
	Paths           []string `toml:"paths"`
	Where           string   `toml:"where"`
	RedirectsToFile bool     `toml:"redirects_to_file"`
}

//go:embed default.toml
var shippedPolicy []byte

// Default is the policy a new install starts with.
func Default() []byte { return slices.Clone(shippedPolicy) }

// EnsureFile writes the default policy to path if there is no file there yet.
func EnsureFile(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, shippedPolicy, 0o600)
}

// Load reads and checks a policy file. protected are Agentry's own files,
// which no agent may touch whatever the file says.
func Load(path, home string, protected []string) (*Policy, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	policy, err := Parse(text, home, protected)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return policy, nil
}

// Parse checks a policy and compiles its globs. Anything it doesn't
// understand is an error, never a guess: a typo must not open the gate.
func Parse(text []byte, home string, protected []string) (*Policy, error) {
	var f policyFile
	decoder := toml.NewDecoder(bytes.NewReader(text)).DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return nil, errors.New(strict.String())
		}
		var syntax *toml.DecodeError
		if errors.As(err, &syntax) {
			row, column := syntax.Position()
			return nil, fmt.Errorf("line %d, column %d: %s", row, column, syntax.Error())
		}
		return nil, err
	}
	if err := f.check(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(text)
	policy := &Policy{hash: hex.EncodeToString(sum[:8]), text: string(text), home: home, protected: protected,
		shellTools: f.ShellTools, defaultProfile: f.DefaultProfile, useForEveryone: f.UseForEveryone,
		profiles: map[string]*profile{}}
	for name, p := range f.Profiles {
		compiled, err := compileProfile(name, p, home)
		if err != nil {
			return nil, err
		}
		policy.profiles[name] = compiled
	}
	return policy, nil
}

func (f policyFile) check() error {
	if f.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", f.Version)
	}
	if len(f.Profiles) == 0 {
		return errors.New("there are no profiles")
	}
	for _, name := range []string{f.DefaultProfile, f.UseForEveryone} {
		if _, ok := f.Profiles[name]; name != "" && !ok {
			return fmt.Errorf("profile %q is named but not defined", name)
		}
	}
	if f.DefaultProfile == "" {
		return errors.New("default_profile is missing: it is the profile of agents that don't name one")
	}
	for name, p := range f.Profiles {
		if err := p.check(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
	}
	return nil
}

func (p profileFile) check() error {
	if !p.Otherwise.valid() {
		return fmt.Errorf(`otherwise must be "allow", "ask" or "deny", got %q`, p.Otherwise)
	}
	if p.Unreadable != "" && !p.Unreadable.valid() {
		return fmt.Errorf(`unreadable must be "allow", "ask" or "deny", got %q`, p.Unreadable)
	}
	for i, r := range p.Rules {
		if err := r.check(); err != nil {
			return fmt.Errorf("rule %d (%q): %w", i+1, r.Name, err)
		}
	}
	return nil
}

func (r ruleFile) check() error {
	switch {
	case strings.TrimSpace(r.Name) == "":
		return errors.New("a rule needs a name")
	case !r.Decision.valid():
		return fmt.Errorf(`decision must be "allow", "ask" or "deny", got %q`, r.Decision)
	case r.Where != "" && r.Where != "workspace" && r.Where != "outside":
		return fmt.Errorf(`where must be "workspace" or "outside", got %q`, r.Where)
	}
	return nil
}

func (d Decision) valid() bool { return d == Allow || d == Ask || d == Deny }

func compileProfile(name string, p profileFile, home string) (*profile, error) {
	compiled := &profile{name: name, why: p.Why, otherwise: p.Otherwise, unreadable: p.Unreadable}
	if compiled.unreadable == "" {
		compiled.unreadable = Ask
	}
	var err error
	if compiled.scratch, err = compileAll(p.Scratch, home, pathGlob); err != nil {
		return nil, fmt.Errorf("profile %q scratch: %w", name, err)
	}
	for _, r := range p.Rules {
		rule := rule{name: r.Name, why: r.Why, decision: r.Decision, tools: r.Tools, where: r.Where,
			redirectsToFile: r.RedirectsToFile}
		if rule.runsAny, err = compileAll(r.RunsAny, home, ignoreHome(commandGlob)); err == nil {
			if rule.runsOnly, err = compileAll(r.RunsOnly, home, ignoreHome(commandGlob)); err == nil {
				rule.paths, err = compileAll(r.Paths, home, pathGlob)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("profile %q rule %q: %w", name, r.Name, err)
		}
		compiled.rules = append(compiled.rules, rule)
	}
	return compiled, nil
}

func compileAll(texts []string, home string, compile func(text, home string) (glob, error)) ([]glob, error) {
	var globs []glob
	for _, text := range texts {
		g, err := compile(text, home)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", text, err)
		}
		globs = append(globs, g)
	}
	return globs, nil
}

func ignoreHome(compile func(string) (glob, error)) func(string, string) (glob, error) {
	return func(text, _ string) (glob, error) { return compile(text) }
}
