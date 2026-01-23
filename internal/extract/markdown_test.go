package extract

import (
	"strings"
	"testing"
)

func TestChunkMarkdown_Table(t *testing.T) {
	longPara := strings.Repeat("a", 1800)
	longPara2 := strings.Repeat("b", 1800)

	tests := []struct {
		name       string
		sourcePath string
		md         string
		wantMin    int
		wantMax    int
		wantPaths  [][]string
	}{
		{
			name:       "splits by headings and paragraphs with overlap",
			sourcePath: "notes/test.md",
			md: "# Top\n\n" +
				longPara + "\n\n" +
				longPara2 + "\n\n" +
				"## Sub\n\n" +
				"short content that is still >= 200 chars " + strings.Repeat("c", 220) + "\n",
			wantMin:   2,
			wantMax:   4,
			wantPaths: [][]string{{"Top"}, {"Top"}, {"Top", "Sub"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkMarkdown(tt.sourcePath, tt.md)
			if len(chunks) < tt.wantMin || len(chunks) > tt.wantMax {
				t.Fatalf("got %d chunks, want between %d and %d", len(chunks), tt.wantMin, tt.wantMax)
			}

			// Verify heading paths for first few chunks.
			for i := 0; i < len(tt.wantPaths) && i < len(chunks); i++ {
				got := strings.Join(chunks[i].HeadingPath, " > ")
				want := strings.Join(tt.wantPaths[i], " > ")
				if got != want {
					t.Fatalf("chunk[%d] heading path = %q, want %q", i, got, want)
				}
			}

			// Verify overlap: if we have at least 2 chunks in same section, chunk[1] should start with tail from chunk[0].
			if len(chunks) >= 2 && strings.Join(chunks[0].HeadingPath, "/") == strings.Join(chunks[1].HeadingPath, "/") {
				tail := chunks[0].Content
				if len(tail) > 200 {
					tail = tail[len(tail)-200:]
				}
				if !strings.HasPrefix(chunks[1].Content, tail) {
					t.Fatalf("expected chunk[1] to have 200-char overlap prefix from chunk[0]")
				}
			}
		})
	}
}

func TestChunkMarkdown_DuplicateHeadings(t *testing.T) {
	// Test that duplicate headings at the same level are handled correctly
	md := `# Introduction

This is the first introduction section with some content.

# Introduction

This is the second introduction section with different content.

## Details

Some details under the second introduction.

## Details

More details under a duplicate heading.
`

	chunks := ChunkMarkdown("test.md", md)

	if len(chunks) < 4 {
		t.Fatalf("expected at least 4 chunks, got %d", len(chunks))
	}

	// First section should have heading path ["Introduction"]
	if len(chunks[0].HeadingPath) != 1 || chunks[0].HeadingPath[0] != "Introduction" {
		t.Errorf("chunk[0] heading path = %v, want [\"Introduction\"]", chunks[0].HeadingPath)
	}
	if !strings.Contains(chunks[0].Content, "first introduction") {
		t.Errorf("chunk[0] should contain content from first section")
	}

	// Second section should also have heading path ["Introduction"]
	// Find the chunk from the second section
	var secondSectionChunk *Chunk
	for i := range chunks {
		if strings.Contains(chunks[i].Content, "second introduction") {
			secondSectionChunk = &chunks[i]
			break
		}
	}
	if secondSectionChunk == nil {
		t.Fatal("could not find chunk from second Introduction section")
	}
	if len(secondSectionChunk.HeadingPath) != 1 || secondSectionChunk.HeadingPath[0] != "Introduction" {
		t.Errorf("second section chunk heading path = %v, want [\"Introduction\"]", secondSectionChunk.HeadingPath)
	}

	// Verify that chunks from different sections with same heading path have different content
	if chunks[0].Content == secondSectionChunk.Content {
		t.Error("chunks from different sections with same heading should have different content")
	}

	// Verify duplicate H2 headings work
	var detailsChunks []Chunk
	for i := range chunks {
		if len(chunks[i].HeadingPath) == 2 && chunks[i].HeadingPath[1] == "Details" {
			detailsChunks = append(detailsChunks, chunks[i])
		}
	}
	if len(detailsChunks) < 2 {
		t.Errorf("expected at least 2 chunks with Details heading, got %d", len(detailsChunks))
	}
	// Both should have same heading path but different content
	if len(detailsChunks) >= 2 {
		path1 := strings.Join(detailsChunks[0].HeadingPath, ">")
		path2 := strings.Join(detailsChunks[1].HeadingPath, ">")
		if path1 != path2 {
			t.Errorf("duplicate Details headings should have same path, got %q and %q", path1, path2)
		}
		if detailsChunks[0].Content == detailsChunks[1].Content {
			t.Error("chunks from different Details sections should have different content")
		}
	}
}

func TestChunkMarkdown_HeadingPathEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		md         string
		wantPaths  [][]string
	}{
		{
			name:       "empty heading path (root level content)",
			sourcePath: "notes/root.md",
			md:         "Content at root level with no headings.\n\nMore content here.",
			wantPaths:  [][]string{{}},
		},
		{
			name:       "nested heading path with empty sections",
			sourcePath: "notes/nested.md",
			md: `# Level 1

Content under level 1.

## Level 2

Content under level 2.

### Level 3

Content under level 3.

## Another Level 2

Content under another level 2.`,
			wantPaths: [][]string{
				{"Level 1"},
				{"Level 1", "Level 2"},
				{"Level 1", "Level 2", "Level 3"},
				{"Level 1", "Another Level 2"},
			},
		},
		{
			name:       "heading path with special characters",
			sourcePath: "notes/special.md",
			md: `# Heading with / and \ characters

Content here.

## Sub-heading with "quotes" and 'apostrophes'

More content.`,
			wantPaths: [][]string{
				{"Heading with / and \\ characters"},
				{"Heading with / and \\ characters", "Sub-heading with \"quotes\" and 'apostrophes'"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := ChunkMarkdown(tt.sourcePath, tt.md)
			if len(chunks) < len(tt.wantPaths) {
				t.Fatalf("got %d chunks, want at least %d", len(chunks), len(tt.wantPaths))
			}

			// Verify heading paths match expected
			for i := 0; i < len(tt.wantPaths) && i < len(chunks); i++ {
				got := chunks[i].HeadingPath
				want := tt.wantPaths[i]
				if len(got) != len(want) {
					t.Errorf("chunk[%d] heading path length = %d, want %d", i, len(got), len(want))
					continue
				}
				for j := range want {
					if got[j] != want[j] {
						t.Errorf("chunk[%d].HeadingPath[%d] = %q, want %q", i, j, got[j], want[j])
					}
				}
			}
		})
	}
}
