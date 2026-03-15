package fs_test

import (
	"context"
	"errors"
	stdfs "io/fs"
	"slices"
	"testing"

	gbfs "github.com/ewhauser/gbash/fs"
)

func TestReusableSeededTrieKeepsSessionsIsolated(t *testing.T) {
	t.Parallel()

	factory := gbfs.Reusable(gbfs.SeededTrie(gbfs.InitialFiles{
		"/seed.txt": {Content: []byte("seed\n")},
	}))

	first, err := factory.New(context.Background())
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	second, err := factory.New(context.Background())
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}

	writeFile(t, first, "/seed.txt", "first-session\n")
	writeFile(t, first, "/only-first.txt", "first\n")

	if got, want := readFile(t, first, "/seed.txt"), "first-session\n"; got != want {
		t.Fatalf("first /seed.txt = %q, want %q", got, want)
	}
	if got, want := readFile(t, second, "/seed.txt"), "seed\n"; got != want {
		t.Fatalf("second /seed.txt = %q, want %q", got, want)
	}
	if _, err := second.Stat(context.Background(), "/only-first.txt"); !errors.Is(err, stdfs.ErrNotExist) {
		t.Fatalf("second Stat(/only-first.txt) error = %v, want not exist", err)
	}
}

func TestMountableFactorySupportsTrieDatasetAndScratch(t *testing.T) {
	t.Parallel()

	fsys, err := gbfs.Mountable(gbfs.MountableOptions{
		Base: gbfs.Memory(),
		Mounts: []gbfs.MountConfig{
			{
				MountPoint: "/dataset",
				Factory: gbfs.Reusable(gbfs.SeededTrie(gbfs.InitialFiles{
					"/docs/guide.txt": {Content: []byte("guide\n")},
				})),
			},
			{
				MountPoint: "/scratch",
				Factory:    gbfs.Memory(),
			},
		},
	}).New(context.Background())
	if err != nil {
		t.Fatalf("Mountable.New() error = %v", err)
	}

	if got, want := readFile(t, fsys, "/dataset/docs/guide.txt"), "guide\n"; got != want {
		t.Fatalf("dataset read = %q, want %q", got, want)
	}
	writeFile(t, fsys, "/scratch/log.txt", "note\n")
	if got, want := readFile(t, fsys, "/scratch/log.txt"), "note\n"; got != want {
		t.Fatalf("scratch read = %q, want %q", got, want)
	}
}

func TestMountableSearchableTrieProvider(t *testing.T) {
	t.Parallel()

	fsys, err := gbfs.Mountable(gbfs.MountableOptions{
		Base: gbfs.Memory(),
		Mounts: []gbfs.MountConfig{
			{
				MountPoint: "/dataset",
				Factory: gbfs.NewSearchableFactory(
					gbfs.Reusable(gbfs.SeededTrie(gbfs.InitialFiles{
						"/docs/guide.txt": {Content: []byte("needle\n")},
					})),
					nil,
				),
			},
			{
				MountPoint: "/scratch",
				Factory:    gbfs.Memory(),
			},
		},
	}).New(context.Background())
	if err != nil {
		t.Fatalf("Mountable.New() error = %v", err)
	}

	capable, ok := fsys.(gbfs.SearchCapable)
	if !ok {
		t.Fatalf("filesystem %T does not implement SearchCapable", fsys)
	}
	rootProvider, ok := capable.SearchProviderForPath("/")
	if ok || rootProvider != nil {
		t.Fatalf("SearchProviderForPath(/) = %v, %v, want nil,false", rootProvider, ok)
	}

	provider, ok := capable.SearchProviderForPath("/dataset")
	if !ok {
		t.Fatal("SearchProviderForPath(/dataset) = false, want true")
	}
	result, err := provider.Search(context.Background(), &gbfs.SearchQuery{
		Root:    "/dataset",
		Literal: "needle",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	var paths []string
	for _, hit := range result.Hits {
		paths = append(paths, hit.Path)
	}
	if got, want := paths, []string{"/dataset/docs/guide.txt"}; !slices.Equal(got, want) {
		t.Fatalf("search hits = %v, want %v", got, want)
	}
}
