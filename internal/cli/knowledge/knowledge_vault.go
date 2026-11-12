package knowledge

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/fsutil"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

// maxVaultEntryBytes bounds one vault archive member (64 MB).
const maxVaultEntryBytes = 64 << 20

var kbVaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Vault management (encrypted storage)",
	Long: `Manage the encrypted vault: export and import keys and data.

  kb vault export backup.tar.gz    Export vault to a tar.gz file
  kb vault import backup.tar.gz    Import vault from a tar.gz file`,
	Example: `  aigc-cli kb vault export backup.tar.gz
  aigc-cli kb vault import backup.tar.gz
  aigc-cli kb list --vault`,
	SilenceUsage: true,
}

var kbVaultExportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export vault to a tar.gz file",
	Long: `Export the vault contents and identity key to a tarball.
The identity key is included in plaintext — keep the archive secure.`,
	Example: `  aigc-cli kb vault export backup.tar.gz
  aigc-cli kb vault export ~/backups/vault-2026.tar.gz`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		outputPath := args[0]

		// Load identity
		identity, err := vault.LoadIdentity()
		if err != nil {
			return fmt.Errorf("load identity: %w", err)
		}

		// Open vault
		if _, err := vault.Open(vaultBaseDir); err != nil {
			return fmt.Errorf("open vault: %w", err)
		}

		// Create tar.gz
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)

		// Write identity
		identityData := []byte(identity.String())
		if err := addFileToTar(tw, "identity.txt", identityData); err != nil {
			return err
		}

		// Write metadata — Vault has no exported fields, so the real metadata
		// always lives in metadata.json; fall back to an empty object if missing.
		metaPath := filepath.Join(vaultBaseDir, "metadata.json")
		metaData, err := os.ReadFile(metaPath)
		if err != nil {
			metaData = []byte("{}")
		}

		if err := addFileToTar(tw, "metadata.json", metaData); err != nil {
			return err
		}

		// Write encrypted docs
		docsDir := filepath.Join(vaultBaseDir, "docs")
		entries, err := os.ReadDir(docsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(docsDir, entry.Name()))
				if err != nil {
					ui.Eprintf("  Warning: skipping %s: %v\n", entry.Name(), err)
					continue
				}
				if err := addFileToTar(tw, "docs/"+entry.Name(), data); err != nil {
					return err
				}
			}
		}

		if err := tw.Close(); err != nil {
			return err
		}
		if err := gw.Close(); err != nil {
			return err
		}

		if err := fsutil.WritePrivate(outputPath, buf.Bytes()); err != nil {
			return fmt.Errorf("write archive: %w", err)
		}

		ui.Eprintf("Vault exported to %s\n", outputPath)
		return nil
	},
}

var kbVaultImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import vault from a tar.gz file",
	Example: `  aigc-cli kb vault import backup.tar.gz
  aigc-cli kb vault import ~/backups/vault-2026.tar.gz`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		archivePath := args[0]

		f, err := os.Open(archivePath)
		if err != nil {
			return fmt.Errorf("open archive: %w", err)
		}
		defer f.Close()

		gr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("read gzip: %w", err)
		}
		tr := tar.NewReader(gr)

		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("read tar: %w", err)
			}

			// Entries are small (identity + JSON mappings); cap each one so a
			// crafted archive cannot inflate into an unbounded allocation.
			data, err := io.ReadAll(io.LimitReader(tr, maxVaultEntryBytes+1))
			if err != nil {
				return fmt.Errorf("read %s: %w", header.Name, err)
			}
			if int64(len(data)) > maxVaultEntryBytes {
				return fmt.Errorf("archive entry %s exceeds %d MB", header.Name, maxVaultEntryBytes>>20)
			}

			switch header.Name {
			case "identity.txt":
				keyStr := strings.TrimSpace(string(data))
				if err := vault.ImportIdentity(keyStr); err != nil {
					return fmt.Errorf("import identity: %w", err)
				}
				ui.Eprintf("  Imported identity key\n")

			case "metadata.json":
				metaPath := filepath.Join(vaultBaseDir, "metadata.json")
				if err := os.MkdirAll(filepath.Dir(metaPath), 0o700); err != nil {
					return err
				}
				if err := fsutil.WritePrivate(metaPath, data); err != nil {
					return fmt.Errorf("write metadata: %w", err)
				}
				ui.Eprintf("  Imported metadata\n")

			default:
				if strings.HasPrefix(header.Name, "docs/") {
					docName := filepath.Base(header.Name)
					dest := filepath.Join(vaultBaseDir, "docs", docName)
					if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
						return err
					}
					if err := fsutil.WritePrivate(dest, data); err != nil {
						return fmt.Errorf("write %s: %w", docName, err)
					}
					ui.Eprintf("  Imported: %s\n", docName)
				}
			}
		}

		ui.Eprintf("Vault imported from %s\n", archivePath)
		return nil
	},
}

func init() {
	kbVaultCmd.AddCommand(kbVaultExportCmd)
	kbVaultCmd.AddCommand(kbVaultImportCmd)
}

func addFileToTar(tw *tar.Writer, name string, data []byte) error {
	if err := tw.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: int64(fsutil.PrivateFileMode),
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
