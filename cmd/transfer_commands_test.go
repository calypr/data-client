package cmd

import "testing"

func TestTransferCommandsExposeCollapsedTopLevelSurface(t *testing.T) {
	uploadCmd := findSubcommand(t, RootCmd, "upload")
	if uploadCmd.Flags().Lookup("upload-path") == nil {
		t.Fatal("upload missing --upload-path flag")
	}
	for _, flagName := range []string{"file", "file-path", "guid", "manifest", "failed-log-path", "multipart"} {
		if uploadCmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("upload missing --%s flag", flagName)
		}
	}

	downloadCmd := findSubcommand(t, RootCmd, "download")
	for _, flagName := range []string{"guid", "manifest", "download-path", "numparallel"} {
		if downloadCmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("download missing --%s flag", flagName)
		}
	}
}

func TestLegacyTransferCommandsAreHidden(t *testing.T) {
	for _, use := range []string{
		"upload-single",
		"upload-multiple",
		"upload-multipart",
		"retry-upload",
		"download-multiple",
	} {
		cmd := findSubcommand(t, RootCmd, use)
		if !cmd.Hidden {
			t.Fatalf("legacy command %q should be hidden", use)
		}
	}
}
