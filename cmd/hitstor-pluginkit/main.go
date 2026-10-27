// hitstor-pluginkit builds, signs, and inspects HitStor plugin packages
// (.hpk). It is a developer and release tool; agents only consume packages.
//
//	hitstor-pluginkit keygen --out mykey
//	hitstor-pluginkit pack --source plugins/example/plugin.yaml \
//	    --binary bin/hitstor-plugin-example.exe --os windows --arch amd64 \
//	    --key-file mykey.key --out dist
//	hitstor-pluginkit verify dist/hitstor-plugin-example-0.1.0-windows-amd64.hpk
//
// The signing key is a base64 ed25519 seed, read from --key-file or from the
// HITSTOR_PLUGIN_SIGNING_KEY environment variable.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hooperits/hitstor/internal/pluginpkg"
)

// Build metadata injected via -ldflags at build time.
var Version = "dev"

const signingKeyEnv = "HITSTOR_PLUGIN_SIGNING_KEY"

func main() {
	root := &cobra.Command{
		Use:           "hitstor-pluginkit",
		Short:         "Build, sign, and inspect HitStor plugin packages",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newKeygenCmd(), newPackCmd(), newVerifyCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newKeygenCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate an ed25519 signing key pair (<out>.key and <out>.pub)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pub, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return err
			}
			keyPath, pubPath := out+".key", out+".pub"
			// O_EXCL: never overwrite an existing signing key.
			f, err := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			if err := os.WriteFile(pubPath, []byte(pluginpkg.EncodePublicKey(pub)+"\n"), 0o644); err != nil {
				return err
			}
			fmt.Printf("private key: %s (keep secret)\npublic key:  %s\n%s\n", keyPath, pubPath, pluginpkg.EncodePublicKey(pub))
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output path prefix")
	cmd.MarkFlagRequired("out")
	return cmd
}

func newPackCmd() *cobra.Command {
	var (
		source, binary, goos, goarch, keyFile, outDir string
		docs                                          []string
		unsigned                                      bool
	)
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Build a package for one platform from a source manifest and a binary",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := os.ReadFile(source)
			if err != nil {
				return err
			}
			m, err := pluginpkg.ParseSource(src)
			if err != nil {
				return err
			}
			manifest, err := m.ForPlatform(goos, goarch)
			if err != nil {
				return err
			}

			key, err := signingKey(keyFile, unsigned)
			if err != nil {
				return err
			}

			bin, err := os.ReadFile(binary)
			if err != nil {
				return err
			}
			files := map[string][]byte{pluginpkg.EntrypointFor(m.ID, goos): bin}
			for _, d := range docs {
				data, err := os.ReadFile(d)
				if err != nil {
					return err
				}
				files[filepath.Base(d)] = data
			}

			data, err := pluginpkg.Build(manifest, files, key)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			out := filepath.Join(outDir, pluginpkg.PackageName(m.ID, m.Version, goos, goarch))
			if err := os.WriteFile(out, data, 0o644); err != nil {
				return err
			}
			signer := "unsigned"
			if key != nil {
				signer = "signed by " + pluginpkg.EncodePublicKey(key.Public().(ed25519.PublicKey))
			}
			fmt.Printf("%s (%s)\n", out, signer)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&source, "source", "", "source plugin.yaml")
	f.StringVar(&binary, "binary", "", "plugin executable built for --os/--arch")
	f.StringVar(&goos, "os", "", "target operating system (e.g. windows)")
	f.StringVar(&goarch, "arch", "", "target architecture (e.g. amd64)")
	f.StringVar(&keyFile, "key-file", "", "signing key file (default: $"+signingKeyEnv+")")
	f.StringSliceVar(&docs, "doc", nil, "extra file to include at the package root (repeatable)")
	f.BoolVar(&unsigned, "unsigned", false, "build an unsigned package (development only)")
	f.StringVar(&outDir, "out", "dist", "output directory")
	for _, req := range []string{"source", "binary", "os", "arch"} {
		cmd.MarkFlagRequired(req)
	}
	return cmd
}

func signingKey(keyFile string, unsigned bool) (ed25519.PrivateKey, error) {
	if unsigned {
		if keyFile != "" {
			return nil, errors.New("--unsigned and --key-file are mutually exclusive")
		}
		return nil, nil
	}
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, err
		}
		return pluginpkg.ParsePrivateKey(string(data))
	}
	if env := os.Getenv(signingKeyEnv); env != "" {
		return pluginpkg.ParsePrivateKey(env)
	}
	return nil, fmt.Errorf("no signing key: pass --key-file, set $%s, or use --unsigned", signingKeyEnv)
}

func newVerifyCmd() *cobra.Command {
	var trusted []string
	cmd := &cobra.Command{
		Use:   "verify <package.hpk>",
		Short: "Show a package's manifest and check its signature",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkg, err := pluginpkg.ReadFile(args[0])
			if err != nil {
				return err
			}
			m := pkg.Manifest
			fmt.Printf("id:          %s\nname:        %s\nversion:     %s\nplatforms:   %s\nentrypoint:  %s\nprotocol:    %d\n",
				m.ID, m.Name, m.Version, strings.Join(m.Platforms, ", "), m.Entrypoint, m.ProtocolVersion)
			if m.MinAgentVersion != "" {
				fmt.Printf("min agent:   %s\n", m.MinAgentVersion)
			}
			d := pkg.Digest()
			fmt.Printf("digest:      %x\n", d)
			if !pkg.Signed() {
				fmt.Println("signature:   none")
				return pluginpkg.ErrUnsigned
			}
			policy, err := pluginpkg.NewTrustPolicy(trusted, false)
			if err != nil {
				return err
			}
			official := slices.Contains(pluginpkg.OfficialKeys, pkg.Signer())
			fmt.Printf("signer:      %s (official: %t)\n", pkg.Signer(), official)
			if err := pkg.Verify(policy); err != nil {
				return err
			}
			fmt.Println("signature:   valid")
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&trusted, "trusted-key", nil, "extra trusted base64 public key (repeatable)")
	return cmd
}
